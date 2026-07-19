/*
   Panvara
   internal/infrastructure/postgres/revision_store.go    2026-07-15
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

var _ application.RevisionStore = (*RevisionStore)(nil)

// RevisionStore persists project-scoped immutable module revisions in PostgreSQL.
type RevisionStore struct {
	pool *pgxpool.Pool
}

// NewRevisionStore constructs the append-only PostgreSQL revision adapter.
func NewRevisionStore(pool *pgxpool.Pool) (*RevisionStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL revision store: nil pool")
	}
	return &RevisionStore{pool: pool}, nil
}

// Register inserts one immutable revision or returns the existing equivalent
// fact without replacing its first source or registration provenance.
func (store *RevisionStore) Register(
	ctx context.Context,
	value appmodule.Revision,
) (appmodule.Revision, bool, error) {
	validated, err := rebuildRevision(value)
	if err != nil {
		return appmodule.Revision{}, false, fmt.Errorf("register PostgreSQL module revision: %w", err)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return appmodule.Revision{}, false, fmt.Errorf("begin PostgreSQL module revision registration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	stored, created, err := registerModuleRevisionTx(ctx, tx, validated)
	if err != nil {
		return appmodule.Revision{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return appmodule.Revision{}, false, fmt.Errorf("commit PostgreSQL module revision registration: %w", err)
	}
	return stored, created, nil
}

// registerModuleRevisionTx inserts or verifies one revision inside a caller-
// owned transaction. Publish uses this helper so Revision and Release facts
// can never commit independently.
func registerModuleRevisionTx(
	ctx context.Context,
	tx pgx.Tx,
	validated appmodule.Revision,
) (appmodule.Revision, bool, error) {
	var inserted int
	err := tx.QueryRow(ctx, `
		INSERT INTO panvara_module_revision (
			project_id, module_name, revision_hash, module_version,
			spec_version, ir_format,
			source_format, source_hash, source_bytes, canonical_ir_bytes,
			openapi_bytes, manager_schema_bytes, origin, registered_by, registered_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		)
		ON CONFLICT (project_id, module_name, revision_hash) DO NOTHING
		RETURNING 1
	`, revisionArguments(validated)...).Scan(&inserted)
	created := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return appmodule.Revision{}, false, fmt.Errorf("insert PostgreSQL module revision: %w", err)
	}
	if !created {
		existing, getErr := getModuleRevision(ctx, tx, validated.ProjectID(), validated.ModuleName(), validated.RevisionHash())
		if getErr != nil {
			return appmodule.Revision{}, false, fmt.Errorf("read existing PostgreSQL module revision: %w", getErr)
		}
		if !existing.SameParentArtifacts(validated) {
			return appmodule.Revision{}, false, fmt.Errorf(
				"%w: existing revision parent artifacts differ for the same content identity",
				application.ErrRevisionCorrupt,
			)
		}
	}
	for _, identity := range validated.DataSchemaIdentities() {
		if err := registerDataSchemaIdentity(ctx, tx, validated, identity); err != nil {
			return appmodule.Revision{}, false, err
		}
	}
	stored, err := getModuleRevision(ctx, tx, validated.ProjectID(), validated.ModuleName(), validated.RevisionHash())
	if err != nil {
		return appmodule.Revision{}, false, fmt.Errorf("read registered PostgreSQL module revision: %w", err)
	}
	return stored, created, nil
}

// List returns a bounded project/module page ordered newest first with a stable hash tie-breaker.
func (store *RevisionStore) List(
	ctx context.Context,
	projectID project.ID,
	module string,
	limit int,
) ([]appmodule.RevisionSummary, error) {
	if !projectID.Valid() || !appmodule.ValidModuleName(module) || limit < 1 || limit > application.MaxRevisionListLimit {
		return nil, fmt.Errorf("%w: invalid PostgreSQL revision list input", application.ErrRevisionInvalid)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT revision.project_id::text, revision.module_name, revision.revision_hash,
		       revision.module_version, revision.spec_version, revision.ir_format,
		       revision.source_format, revision.source_hash, revision.origin,
		       revision.registered_by, revision.registered_at,
		       COALESCE(
		           jsonb_agg(
		               jsonb_build_object(
		                   'format', identity.data_schema_format,
		                   'fingerprint', identity.data_schema_fingerprint
		               ) ORDER BY identity.data_schema_format
		           ) FILTER (WHERE identity.data_schema_format IS NOT NULL),
		           '[]'::jsonb
		       )::text
		FROM panvara_module_revision AS revision
		LEFT JOIN panvara_module_revision_data_schema AS identity
		  ON identity.project_id = revision.project_id
		 AND identity.module_name = revision.module_name
		 AND identity.revision_hash = revision.revision_hash
		WHERE revision.project_id = $1 AND revision.module_name = $2
		GROUP BY revision.project_id, revision.module_name, revision.revision_hash
		ORDER BY revision.registered_at DESC, revision.revision_hash ASC
		LIMIT $3
	`, projectID.String(), module, limit)
	if err != nil {
		return nil, fmt.Errorf("list PostgreSQL module revisions: %w", err)
	}
	defer rows.Close()
	values := make([]appmodule.RevisionSummary, 0, limit)
	for rows.Next() {
		value, err := scanRevisionSummary(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL module revisions: %w", err)
	}
	return values, nil
}

// Get returns one exact project/module/revision fact.
func (store *RevisionStore) Get(
	ctx context.Context,
	projectID project.ID,
	module string,
	revisionHash string,
) (appmodule.Revision, error) {
	if !projectID.Valid() || !appmodule.ValidModuleName(module) || !appmodule.ValidContentHash(revisionHash) {
		return appmodule.Revision{}, fmt.Errorf("%w: invalid PostgreSQL revision identity", application.ErrRevisionInvalid)
	}
	value, err := getModuleRevision(ctx, store.pool, projectID, module, revisionHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return appmodule.Revision{}, application.ErrRevisionNotFound
	}
	if err != nil {
		return appmodule.Revision{}, err
	}
	return value, nil
}

const revisionParentSelect = `
	SELECT project_id::text, module_name, revision_hash, module_version,
	       spec_version, ir_format, source_format, source_hash, source_bytes, canonical_ir_bytes,
	       openapi_bytes, manager_schema_bytes, origin, registered_by, registered_at
	FROM panvara_module_revision
`

func revisionArguments(value appmodule.Revision) []any {
	return []any{
		value.ProjectID().String(), value.ModuleName(), value.RevisionHash(), value.ModuleVersion(),
		value.SpecVersion(), value.IRFormat(), string(value.SourceFormat()), value.SourceHash(),
		value.Source(), value.CanonicalIR(), value.OpenAPI(), value.ManagerSchema(),
		string(value.Origin()), value.RegisteredBy(), value.RegisteredAt(),
	}
}

type revisionScanner interface {
	Scan(...any) error
}

func scanRevisionParent(row revisionScanner) (appmodule.RevisionMaterial, error) {
	var (
		projectText, module, revisionHash, moduleVersion string
		specVersion, sourceFormat, sourceHash            string
		origin, registeredBy                             string
		irFormat                                         int
		source, canonicalIR, openAPI, managerSchema      []byte
		registeredAt                                     time.Time
	)
	if err := row.Scan(
		&projectText, &module, &revisionHash, &moduleVersion,
		&specVersion, &irFormat, &sourceFormat, &sourceHash, &source, &canonicalIR,
		&openAPI, &managerSchema, &origin, &registeredBy, &registeredAt,
	); err != nil {
		return appmodule.RevisionMaterial{}, err
	}
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return appmodule.RevisionMaterial{}, fmt.Errorf("%w: parse persisted revision project: %v", application.ErrRevisionCorrupt, err)
	}
	return appmodule.RevisionMaterial{
		ProjectID: projectID, ModuleName: module, ModuleVersion: moduleVersion,
		RevisionHash: revisionHash, SpecVersion: specVersion, IRFormat: irFormat,
		SourceFormat: appmodule.SourceFormat(sourceFormat), SourceHash: sourceHash, Source: source,
		CanonicalIR: canonicalIR, OpenAPI: openAPI, ManagerSchema: managerSchema,
		Origin: appmodule.RevisionOrigin(origin), RegisteredBy: registeredBy, RegisteredAt: registeredAt,
	}, nil
}

func rebuildRevision(value appmodule.Revision) (appmodule.Revision, error) {
	return appmodule.NewRevision(appmodule.RevisionMaterial{
		ProjectID: value.ProjectID(), ModuleName: value.ModuleName(), ModuleVersion: value.ModuleVersion(),
		RevisionHash: value.RevisionHash(), DataSchemaIdentities: value.DataSchemaIdentities(),
		SpecVersion: value.SpecVersion(), IRFormat: value.IRFormat(),
		SourceFormat: value.SourceFormat(), SourceHash: value.SourceHash(), Source: value.Source(),
		CanonicalIR: value.CanonicalIR(), OpenAPI: value.OpenAPI(), ManagerSchema: value.ManagerSchema(),
		Origin: value.Origin(), RegisteredBy: value.RegisteredBy(), RegisteredAt: value.RegisteredAt(),
	})
}

type revisionQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func registerDataSchemaIdentity(
	ctx context.Context,
	tx pgx.Tx,
	revision appmodule.Revision,
	identity appmodule.DataSchemaIdentity,
) error {
	var inserted int
	err := tx.QueryRow(ctx, `
		INSERT INTO panvara_module_revision_data_schema (
			project_id, module_name, revision_hash, data_schema_format, data_schema_fingerprint
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, module_name, revision_hash, data_schema_format) DO NOTHING
		RETURNING 1
	`, revision.ProjectID().String(), revision.ModuleName(), revision.RevisionHash(),
		identity.Format(), identity.Fingerprint()).Scan(&inserted)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("insert PostgreSQL data schema identity: %w", err)
	}
	var existing string
	if err := tx.QueryRow(ctx, `
		SELECT data_schema_fingerprint
		FROM panvara_module_revision_data_schema
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
		  AND data_schema_format = $4
	`, revision.ProjectID().String(), revision.ModuleName(), revision.RevisionHash(), identity.Format()).Scan(&existing); err != nil {
		return fmt.Errorf("read existing PostgreSQL data schema identity: %w", err)
	}
	if existing != identity.Fingerprint() {
		return fmt.Errorf(
			"%w: data schema format %d has conflicting fingerprints",
			application.ErrRevisionCorrupt,
			identity.Format(),
		)
	}
	return nil
}

func getModuleRevision(
	ctx context.Context,
	querier revisionQuerier,
	projectID project.ID,
	module string,
	revisionHash string,
) (appmodule.Revision, error) {
	material, err := scanRevisionParent(querier.QueryRow(ctx, revisionParentSelect+`
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, projectID.String(), module, revisionHash))
	if err != nil {
		return appmodule.Revision{}, err
	}
	identities, err := loadDataSchemaIdentities(ctx, querier, projectID, module, revisionHash)
	if err != nil {
		return appmodule.Revision{}, err
	}
	material.DataSchemaIdentities = identities
	value, err := appmodule.NewRevision(material)
	if err != nil {
		return appmodule.Revision{}, fmt.Errorf("%w: validate persisted revision: %v", application.ErrRevisionCorrupt, err)
	}
	return value, nil
}

func loadDataSchemaIdentities(
	ctx context.Context,
	querier revisionQuerier,
	projectID project.ID,
	module string,
	revisionHash string,
) ([]appmodule.DataSchemaIdentity, error) {
	rows, err := querier.Query(ctx, `
		SELECT data_schema_format, data_schema_fingerprint
		FROM panvara_module_revision_data_schema
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
		ORDER BY data_schema_format
	`, projectID.String(), module, revisionHash)
	if err != nil {
		return nil, fmt.Errorf("read PostgreSQL data schema identities: %w", err)
	}
	defer rows.Close()
	identities := make([]appmodule.DataSchemaIdentity, 0, 2)
	for rows.Next() {
		var format int
		var fingerprint string
		if err := rows.Scan(&format, &fingerprint); err != nil {
			return nil, fmt.Errorf("scan PostgreSQL data schema identity: %w", err)
		}
		identity, err := appmodule.NewDataSchemaIdentity(format, fingerprint)
		if err != nil {
			return nil, fmt.Errorf("%w: validate persisted data schema identity: %v", application.ErrRevisionCorrupt, err)
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL data schema identities: %w", err)
	}
	return identities, nil
}

func scanRevisionSummary(row revisionScanner) (appmodule.RevisionSummary, error) {
	var (
		projectText, module, revisionHash, moduleVersion string
		specVersion, sourceFormat, sourceHash            string
		origin, registeredBy, identitiesJSON             string
		irFormat                                         int
		registeredAt                                     time.Time
	)
	if err := row.Scan(
		&projectText, &module, &revisionHash, &moduleVersion, &specVersion, &irFormat,
		&sourceFormat, &sourceHash, &origin, &registeredBy, &registeredAt, &identitiesJSON,
	); err != nil {
		return appmodule.RevisionSummary{}, err
	}
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return appmodule.RevisionSummary{}, fmt.Errorf("%w: parse persisted revision summary project: %v", application.ErrRevisionCorrupt, err)
	}
	var encoded []struct {
		Format      int    `json:"format"`
		Fingerprint string `json:"fingerprint"`
	}
	if err := json.Unmarshal([]byte(identitiesJSON), &encoded); err != nil {
		return appmodule.RevisionSummary{}, fmt.Errorf("%w: decode persisted data schema identities: %v", application.ErrRevisionCorrupt, err)
	}
	identities := make([]appmodule.DataSchemaIdentity, 0, len(encoded))
	for _, value := range encoded {
		identity, err := appmodule.NewDataSchemaIdentity(value.Format, value.Fingerprint)
		if err != nil {
			return appmodule.RevisionSummary{}, fmt.Errorf("%w: validate persisted data schema identity: %v", application.ErrRevisionCorrupt, err)
		}
		identities = append(identities, identity)
	}
	summary, err := appmodule.NewRevisionSummary(appmodule.RevisionSummaryMaterial{
		ProjectID: projectID, ModuleName: module, ModuleVersion: moduleVersion, RevisionHash: revisionHash,
		DataSchemaIdentities: identities, SpecVersion: specVersion, IRFormat: irFormat,
		SourceFormat: appmodule.SourceFormat(sourceFormat), SourceHash: sourceHash,
		Origin: appmodule.RevisionOrigin(origin), RegisteredBy: registeredBy, RegisteredAt: registeredAt,
	})
	if err != nil {
		return appmodule.RevisionSummary{}, fmt.Errorf("%w: validate persisted revision summary: %v", application.ErrRevisionCorrupt, err)
	}
	return summary, nil
}
