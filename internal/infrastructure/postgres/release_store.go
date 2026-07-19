/*
   Panvara
   internal/infrastructure/postgres/release_store.go    2026-07-19
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
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

var (
	_ releaseapp.ReleaseStore   = (*ReleaseStore)(nil)
	_ releaseapp.SnapshotReader = (*ReleaseStore)(nil)
)

// ReleaseStore persists immutable environment-scoped module publication facts.
type ReleaseStore struct {
	pool     *pgxpool.Pool
	auditIDs accessAuditIDGenerator
}

// NewReleaseStore constructs the PostgreSQL publication adapter.
func NewReleaseStore(pool *pgxpool.Pool) (*ReleaseStore, error) {
	return newReleaseStore(pool, domainaccess.NewDefaultIDGenerator())
}

func newReleaseStore(
	pool *pgxpool.Pool,
	auditIDs accessAuditIDGenerator,
) (*ReleaseStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL release store: nil pool")
	}
	if auditIDs == nil {
		return nil, fmt.Errorf("construct PostgreSQL release store: nil audit id generator")
	}
	return &ReleaseStore{pool: pool, auditIDs: auditIDs}, nil
}

// Get returns one exact environment/module/release publication fact.
func (store *ReleaseStore) Get(
	ctx context.Context,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	if ctx == nil || scope.Validate() != nil || !domainappmodule.ValidModuleName(module) || !id.Valid() {
		return domainrelease.ModuleRelease{}, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ModuleRelease{}, err
	}
	value, err := scanModuleRelease(store.pool.QueryRow(ctx, `
		SELECT `+moduleReleaseColumns+`
		FROM panvara_module_release
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND release_id = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ModuleRelease{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainrelease.ModuleRelease{}, fmt.Errorf("get PostgreSQL module release: %w", err)
	}
	return value, nil
}

const moduleReleaseColumns = `
	project_id::text, environment_id::text, module_name, release_id::text,
	draft_id::text, draft_generation, validation_id, plan_id, plan_hash,
	baseline_revision_hash, candidate_revision_hash,
	candidate_data_schema_format, candidate_data_schema_fingerprint,
	source_hash, outcome, risk, published_by_principal_id,
	published_by_credential_id::text, request_id, published_at
`

type moduleReleaseScanner interface{ Scan(...any) error }

func scanModuleRelease(scanner moduleReleaseScanner) (domainrelease.ModuleRelease, error) {
	var (
		projectText, environmentText, module, releaseText string
		draftText, validationID, planID, planHash         string
		candidateRevision, dataFingerprint, sourceHash    string
		outcome, risk, publishedBy, credentialText        string
		requestID                                         string
		baselineRevision                                  *string
		draftGeneration                                   uint64
		dataFormat                                        int
		publishedAt                                       time.Time
	)
	if err := scanner.Scan(
		&projectText, &environmentText, &module, &releaseText,
		&draftText, &draftGeneration, &validationID, &planID, &planHash,
		&baselineRevision, &candidateRevision, &dataFormat, &dataFingerprint,
		&sourceHash, &outcome, &risk, &publishedBy, &credentialText,
		&requestID, &publishedAt,
	); err != nil {
		return domainrelease.ModuleRelease{}, err
	}
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("parse project", err)
	}
	environmentID, err := project.ParseEnvironmentID(environmentText)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("parse environment", err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("construct scope", err)
	}
	releaseID, err := domainrelease.ParseID(releaseText)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("parse release id", err)
	}
	draftID, err := domainappmodule.ParseDraftID(draftText)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("parse draft id", err)
	}
	credentialID, err := domainaccess.ParseID(credentialText)
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("parse credential id", err)
	}
	baseline := ""
	if baselineRevision != nil {
		baseline = *baselineRevision
	}
	value, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: releaseID, Scope: scope, ModuleName: module, DraftID: draftID,
		DraftGeneration: draftGeneration, ValidationID: validationID,
		PlanID: planID, PlanHash: planHash, BaselineRevision: baseline,
		CandidateRevision: candidateRevision, DataSchemaFormat: dataFormat,
		DataSchemaFingerprint: dataFingerprint, SourceHash: sourceHash,
		Outcome: domainrelease.Outcome(outcome), Risk: risk, PublishedBy: publishedBy,
		PublishedCredentialID: credentialID, RequestID: requestID, PublishedAt: publishedAt,
	})
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("validate persisted fact", err)
	}
	return value, nil
}

func rebuildModuleRelease(value domainrelease.ModuleRelease) (domainrelease.ModuleRelease, error) {
	return domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: value.ID(), Scope: value.Scope(), ModuleName: value.ModuleName(), DraftID: value.DraftID(),
		DraftGeneration: value.DraftGeneration(), ValidationID: value.ValidationID(),
		PlanID: value.PlanID(), PlanHash: value.PlanHash(), BaselineRevision: value.BaselineRevision(),
		CandidateRevision: value.CandidateRevision(), DataSchemaFormat: value.DataSchemaFormat(),
		DataSchemaFingerprint: value.DataSchemaFingerprint(), SourceHash: value.SourceHash(),
		Outcome: value.Outcome(), Risk: value.Risk(), PublishedBy: value.PublishedBy(),
		PublishedCredentialID: value.PublishedCredentialID(), RequestID: value.RequestID(),
		PublishedAt: value.PublishedAt(),
	})
}

func corruptRelease(action string, err error) error {
	return fmt.Errorf("%w: %s: %v", releaseapp.ErrCorrupt, action, err)
}
