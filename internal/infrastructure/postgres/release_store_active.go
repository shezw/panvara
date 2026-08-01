/*
   Panvara
   internal/infrastructure/postgres/release_store_active.go    2026-08-02
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
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

// EnsureBootstrap appends epoch one only when the environment has no active
// pointer. Once a pointer exists, a newly registered bootstrap Revision cannot
// implicitly replace the authoritative active state.
func (store *ReleaseStore) EnsureBootstrap(
	ctx context.Context,
	scope project.Scope,
	module string,
	runtimeRevision string,
	at time.Time,
) (domainrelease.ActiveSnapshot, bool, error) {
	if ctx == nil || scope.Validate() != nil || !domainappmodule.ValidModuleName(module) ||
		!domainappmodule.ValidContentHash(runtimeRevision) || at.IsZero() {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("begin PostgreSQL release bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockAccessProject(ctx, tx, scope.ProjectID()); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	activeEpoch, found, err := lockActiveEpoch(ctx, tx, scope)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if found {
		active, err := readActiveSnapshotAt(ctx, tx, scope, module, activeEpoch)
		if err != nil {
			return domainrelease.ActiveSnapshot{}, false, err
		}
		if err := commitReleaseActivation(ctx, tx, "read existing bootstrap active snapshot"); err != nil {
			return domainrelease.ActiveSnapshot{}, false, err
		}
		return active, false, nil
	}

	persisted, err := getModuleRevision(
		ctx, tx, scope.ProjectID(), module, runtimeRevision,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("read PostgreSQL bootstrap revision: %w", err)
	}
	identity, found := persisted.DataSchemaIdentity(moduleapp.DataSchemaFormatVersion)
	if !found {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: persisted bootstrap data schema identity is missing", releaseapp.ErrCorrupt,
		)
	}

	activatedAt := at.UTC()
	if err := insertBootstrapActiveSnapshot(
		ctx, tx, scope, module, persisted.RevisionHash(), identity, activatedAt,
	); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	active, err := readActiveSnapshotAt(ctx, tx, scope, module, 1)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if err := commitReleaseActivation(ctx, tx, "bootstrap active snapshot"); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	return active, true, nil
}

// GetActive returns the module binding selected by the environment's durable pointer.
func (store *ReleaseStore) GetActive(
	ctx context.Context,
	scope project.Scope,
	module string,
) (domainrelease.ActiveSnapshot, error) {
	if ctx == nil || scope.Validate() != nil || !domainappmodule.ValidModuleName(module) {
		return domainrelease.ActiveSnapshot{}, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ActiveSnapshot{}, err
	}
	var epoch int64
	err := store.pool.QueryRow(ctx, `
		SELECT active_epoch
		FROM panvara_environment_release_pointer
		WHERE project_id = $1 AND environment_id = $2
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ActiveSnapshot{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf("read PostgreSQL active release pointer: %w", err)
	}
	return readActiveSnapshotAt(ctx, store.pool, scope, module, epoch)
}

type activeSnapshotQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readActiveSnapshotAt(
	ctx context.Context,
	querier activeSnapshotQuerier,
	scope project.Scope,
	module string,
	epoch int64,
) (domainrelease.ActiveSnapshot, error) {
	value, err := scanActiveSnapshot(querier.QueryRow(ctx, `
		SELECT snapshot.project_id::text, snapshot.environment_id::text,
		       snapshot.release_epoch, snapshot.cause,
		       snapshot.activated_by_principal_id,
		       snapshot.activated_by_credential_id::text,
		       snapshot.request_id, snapshot.created_at,
		       binding.module_name, binding.source_kind, binding.release_id::text,
		       binding.runtime_revision_hash, binding.record_namespace_revision_hash,
		       binding.data_schema_format, binding.data_schema_fingerprint
		FROM panvara_project_release_snapshot AS snapshot
		JOIN panvara_project_release_snapshot_module AS binding
		  ON binding.project_id = snapshot.project_id
		 AND binding.environment_id = snapshot.environment_id
		 AND binding.release_epoch = snapshot.release_epoch
		WHERE snapshot.project_id = $1 AND snapshot.environment_id = $2
		  AND snapshot.release_epoch = $3 AND binding.module_name = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), epoch, module))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ActiveSnapshot{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf("read PostgreSQL active release snapshot: %w", err)
	}
	if value.Scope().ProjectID().String() != scope.ProjectID().String() ||
		value.Scope().EnvironmentID().String() != scope.EnvironmentID().String() ||
		value.ModuleName() != module || value.Epoch() != uint64(epoch) {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf(
			"%w: active snapshot differs from pointer identity", releaseapp.ErrCorrupt,
		)
	}
	return value, nil
}

type activeSnapshotScanner interface{ Scan(...any) error }

func scanActiveSnapshot(scanner activeSnapshotScanner) (domainrelease.ActiveSnapshot, error) {
	var (
		projectText, environmentText, cause, requestID         string
		module, sourceKind, runtimeRevision, namespaceRevision string
		dataSchemaFingerprint                                  string
		activatedBy, credentialText, releaseText               *string
		epoch                                                  int64
		dataSchemaFormat                                       int
		activatedAt                                            time.Time
	)
	if err := scanner.Scan(
		&projectText, &environmentText, &epoch, &cause,
		&activatedBy, &credentialText, &requestID, &activatedAt,
		&module, &sourceKind, &releaseText, &runtimeRevision, &namespaceRevision,
		&dataSchemaFormat, &dataSchemaFingerprint,
	); err != nil {
		return domainrelease.ActiveSnapshot{}, err
	}
	if epoch <= 0 {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf("%w: invalid active snapshot epoch", releaseapp.ErrCorrupt)
	}
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("parse project", err)
	}
	environmentID, err := project.ParseEnvironmentID(environmentText)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("parse environment", err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("construct scope", err)
	}

	var releaseID *domainrelease.ID
	if releaseText != nil {
		parsed, err := domainrelease.ParseID(*releaseText)
		if err != nil {
			return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("parse release id", err)
		}
		releaseID = &parsed
	}
	if (sourceKind == "bootstrap") != (releaseID == nil) ||
		(sourceKind != "bootstrap" && sourceKind != "release") {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf(
			"%w: active binding source differs from release identity", releaseapp.ErrCorrupt,
		)
	}
	binding, err := domainrelease.NewModuleBinding(domainrelease.ModuleBindingMaterial{
		ModuleName: module, ReleaseID: releaseID, RuntimeRevision: runtimeRevision,
		RecordNamespaceRevision: namespaceRevision, DataSchemaFormat: dataSchemaFormat,
		DataSchemaFingerprint: dataSchemaFingerprint,
	})
	if err != nil {
		return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("construct binding", err)
	}

	origin := domainrelease.ActiveSnapshotOrigin(cause)
	actor := "system:bootstrap"
	var credentialID *domainaccess.ID
	if cause == "activate" {
		origin = domainrelease.ActiveSnapshotOriginRelease
		if activatedBy == nil || credentialText == nil {
			return domainrelease.ActiveSnapshot{}, fmt.Errorf(
				"%w: active release provenance is incomplete", releaseapp.ErrCorrupt,
			)
		}
		actor = *activatedBy
		parsed, err := domainaccess.ParseID(*credentialText)
		if err != nil {
			return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("parse credential id", err)
		}
		credentialID = &parsed
	} else if cause != "bootstrap" || activatedBy != nil || credentialText != nil {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf(
			"%w: active snapshot cause or bootstrap provenance is invalid", releaseapp.ErrCorrupt,
		)
	}
	value, err := domainrelease.NewActiveSnapshot(domainrelease.ActiveSnapshotMaterial{
		Scope: scope, Epoch: uint64(epoch), Origin: origin, Binding: binding,
		ActivatedBy: actor, ActivatedCredentialID: credentialID,
		RequestID: requestID, ActivatedAt: activatedAt,
	})
	if err != nil {
		return domainrelease.ActiveSnapshot{}, corruptActiveSnapshot("construct snapshot", err)
	}
	return value, nil
}

func lockActiveEpoch(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
) (int64, bool, error) {
	var epoch int64
	err := tx.QueryRow(ctx, `
		SELECT active_epoch
		FROM panvara_environment_release_pointer
		WHERE project_id = $1 AND environment_id = $2
		FOR UPDATE
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&epoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("lock PostgreSQL active release pointer: %w", err)
	}
	if epoch <= 0 {
		return 0, false, fmt.Errorf("%w: invalid active release pointer epoch", releaseapp.ErrCorrupt)
	}
	return epoch, true, nil
}

func insertBootstrapActiveSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	module string,
	revisionHash string,
	identity domainappmodule.DataSchemaIdentity,
	at time.Time,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project_release_snapshot (
			project_id, environment_id, release_epoch, cause,
			activated_by_principal_id, activated_by_credential_id,
			request_id, created_at
		) VALUES ($1, $2, 1, 'bootstrap', NULL, NULL, 'system:bootstrap', $3)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), at); err != nil {
		return mapActiveWriteError("insert bootstrap release snapshot", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project_release_snapshot_module (
			project_id, environment_id, release_epoch, module_name,
			source_kind, release_id, runtime_revision_hash,
			record_namespace_revision_hash, data_schema_format,
			data_schema_fingerprint
		) VALUES ($1, $2, 1, $3, 'bootstrap', NULL, $4, $4, $5, $6)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module,
		revisionHash, identity.Format(), identity.Fingerprint()); err != nil {
		return mapActiveWriteError("insert bootstrap release binding", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_environment_release_pointer (
			project_id, environment_id, active_epoch, updated_at
		) VALUES ($1, $2, 1, $3)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), at); err != nil {
		return mapActiveWriteError("insert bootstrap release pointer", err)
	}
	return nil
}

func corruptActiveSnapshot(action string, err error) error {
	return fmt.Errorf("%w: %s: %v", releaseapp.ErrCorrupt, action, err)
}
