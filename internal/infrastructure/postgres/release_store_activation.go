/*
   Panvara
   internal/infrastructure/postgres/release_store_activation.go    2026-08-02
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	accessapp "github.com/shezw/panvara/internal/application/access"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const maxPostgresReleaseEpoch uint64 = 1<<63 - 1

var _ releaseapp.ActivationStore = (*ReleaseStore)(nil)

// ActivateCompatible atomically advances the active environment epoch only
// when the immutable Release, prepared Candidate, expected active state, and
// persistent Record namespace still prove the same compatible transition.
func (store *ReleaseStore) ActivateCompatible(
	ctx context.Context,
	mutation accessapp.MutationContext,
	command releaseapp.ActivateCompatibleCommand,
) (domainrelease.ActiveSnapshot, bool, error) {
	validatedRelease, validatedCandidate, err := validateActivationCommand(ctx, mutation, command)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("begin PostgreSQL release activation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scope := mutation.Scope()
	if err := lockAccessProject(ctx, tx, scope.ProjectID()); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if err := reauthorizeMutation(ctx, tx, mutation); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	activeEpoch, found, err := lockActiveEpoch(ctx, tx, scope)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if !found {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrNotFound
	}
	current, err := readActiveSnapshotAt(
		ctx, tx, scope, validatedRelease.ModuleName(), activeEpoch,
	)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	persistedRelease, err := readExactModuleRelease(
		ctx, tx, scope, validatedRelease.ModuleName(), validatedRelease.ID(),
	)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if !sameModuleReleaseFact(persistedRelease, validatedRelease) {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: activation Release differs from persisted fact", releaseapp.ErrInvalid,
		)
	}
	persistedCandidate, err := getModuleRevision(
		ctx, tx, scope.ProjectID(), validatedRelease.ModuleName(), validatedRelease.CandidateRevision(),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: activated Release candidate revision is missing", releaseapp.ErrCorrupt,
		)
	}
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("read PostgreSQL activation candidate: %w", err)
	}
	if !sameActivationRevision(persistedCandidate, validatedCandidate) {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: activation Candidate differs from persisted revision", releaseapp.ErrInvalid,
		)
	}
	if err := validatePersistedActivationCandidate(persistedRelease, persistedCandidate); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if persistedRelease.Outcome() != domainrelease.OutcomeCompatible ||
		persistedRelease.BaselineRevision() == "" {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrNotActivatable
	}
	if err := verifyActiveNamespaceIdentity(ctx, tx, current, persistedRelease); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	if activeRelease, ok := current.ReleaseID(); ok && activeRelease == persistedRelease.ID() {
		if err := commitReleaseActivation(ctx, tx, "replay active module release"); err != nil {
			return domainrelease.ActiveSnapshot{}, false, err
		}
		return current, false, nil
	}
	if !sameActiveSnapshotFact(current, command.Expected) ||
		command.ActivatedAt.UTC().Before(current.ActivatedAt()) {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrActivationConflict
	}
	historical, err := releaseWasActivated(
		ctx, tx, scope, persistedRelease.ModuleName(), persistedRelease.ID(),
	)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if historical || persistedRelease.BaselineRevision() != current.RuntimeRevision() {
		return domainrelease.ActiveSnapshot{}, false, releaseapp.ErrActivationConflict
	}
	if current.Epoch() >= maxPostgresReleaseEpoch {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: active release epoch is exhausted", releaseapp.ErrActivationConflict,
		)
	}

	nextEpoch := current.Epoch() + 1
	if err := appendActivatedSnapshot(
		ctx, tx, mutation, current, persistedRelease, nextEpoch, command.ActivatedAt.UTC(),
	); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	active, err := readActiveSnapshotAt(
		ctx, tx, scope, persistedRelease.ModuleName(), int64(nextEpoch),
	)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	auditStore := &AccessAdminStore{pool: store.pool, auditIDs: store.auditIDs}
	if err := auditStore.appendMutationAudit(
		ctx, tx, mutation, "module_release", persistedRelease.ID().String(), command.ActivatedAt.UTC(),
	); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if err := commitReleaseActivation(ctx, tx, "activate compatible module release"); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	return active, true, nil
}

func validateActivationCommand(
	ctx context.Context,
	mutation accessapp.MutationContext,
	command releaseapp.ActivateCompatibleCommand,
) (domainrelease.ModuleRelease, domainappmodule.Revision, error) {
	if ctx == nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, err
	}
	if err := mutation.Validate(); err != nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: invalid activation mutation: %v", releaseapp.ErrInvalid, err,
		)
	}
	if err := validateMutationContext(mutation, accessapp.OperationReleaseActivate); err != nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, err
	}
	if command.Expected.Validate() != nil || command.ActivatedAt.IsZero() {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: invalid expected active snapshot or activation time", releaseapp.ErrInvalid,
		)
	}
	value, err := rebuildModuleRelease(command.Release)
	if err != nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: validate activation Release: %v", releaseapp.ErrInvalid, err,
		)
	}
	candidate, err := rebuildRevision(command.Candidate)
	if err != nil {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: validate activation Candidate: %v", releaseapp.ErrInvalid, err,
		)
	}
	scope := mutation.Scope()
	if !sameProjectScope(scope, command.Expected.Scope()) || !sameProjectScope(scope, value.Scope()) ||
		command.Expected.ModuleName() != value.ModuleName() ||
		candidate.ProjectID().String() != scope.ProjectID().String() ||
		candidate.ModuleName() != value.ModuleName() ||
		candidate.RevisionHash() != value.CandidateRevision() ||
		candidate.SourceHash() != value.SourceHash() {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: activation facts cross scope, module, or candidate identity", releaseapp.ErrInvalid,
		)
	}
	identity, found := candidate.DataSchemaIdentity(value.DataSchemaFormat())
	if !found || identity.Fingerprint() != value.DataSchemaFingerprint() {
		return domainrelease.ModuleRelease{}, domainappmodule.Revision{}, fmt.Errorf(
			"%w: activation Candidate data schema differs from Release", releaseapp.ErrInvalid,
		)
	}
	return value, candidate, nil
}

func readExactModuleRelease(
	ctx context.Context,
	querier activeSnapshotQuerier,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	value, err := scanModuleRelease(querier.QueryRow(ctx, `
		SELECT `+moduleReleaseColumns+`
		FROM panvara_module_release
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND release_id = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ModuleRelease{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainrelease.ModuleRelease{}, corruptRelease("read activation Release", err)
	}
	return value, nil
}

func validatePersistedActivationCandidate(
	value domainrelease.ModuleRelease,
	candidate domainappmodule.Revision,
) error {
	if candidate.ProjectID().String() != value.Scope().ProjectID().String() ||
		candidate.ModuleName() != value.ModuleName() ||
		candidate.RevisionHash() != value.CandidateRevision() ||
		candidate.SourceHash() != value.SourceHash() {
		return fmt.Errorf("%w: persisted Release candidate identity differs", releaseapp.ErrCorrupt)
	}
	identity, found := candidate.DataSchemaIdentity(value.DataSchemaFormat())
	if !found || identity.Fingerprint() != value.DataSchemaFingerprint() {
		return fmt.Errorf("%w: persisted Release candidate schema differs", releaseapp.ErrCorrupt)
	}
	return nil
}

func verifyActiveNamespaceIdentity(
	ctx context.Context,
	querier activeSnapshotQuerier,
	current domainrelease.ActiveSnapshot,
	value domainrelease.ModuleRelease,
) error {
	var fingerprint string
	err := querier.QueryRow(ctx, `
		SELECT data_schema_fingerprint
		FROM panvara_module_revision_data_schema
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
		  AND data_schema_format = $4
	`, current.Scope().ProjectID().String(), current.ModuleName(),
		current.RecordNamespaceRevision(), value.DataSchemaFormat()).Scan(&fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: active Record namespace schema identity is missing", releaseapp.ErrCorrupt)
	}
	if err != nil {
		return fmt.Errorf("read PostgreSQL active Record namespace identity: %w", err)
	}
	if current.DataSchemaFormat() != value.DataSchemaFormat() ||
		current.DataSchemaFingerprint() != fingerprint ||
		value.DataSchemaFingerprint() != fingerprint {
		return releaseapp.ErrNotActivatable
	}
	return nil
}

func releaseWasActivated(
	ctx context.Context,
	querier activeSnapshotQuerier,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) (bool, error) {
	var found bool
	if err := querier.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM panvara_project_release_snapshot_module
			WHERE project_id = $1 AND environment_id = $2
			  AND module_name = $3 AND release_id = $4
		)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module, id.String()).Scan(&found); err != nil {
		return false, fmt.Errorf("read PostgreSQL release activation history: %w", err)
	}
	return found, nil
}

func appendActivatedSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	mutation accessapp.MutationContext,
	current domainrelease.ActiveSnapshot,
	value domainrelease.ModuleRelease,
	nextEpoch uint64,
	at time.Time,
) error {
	scope := mutation.Scope()
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project_release_snapshot (
			project_id, environment_id, release_epoch, cause,
			activated_by_principal_id, activated_by_credential_id,
			request_id, created_at
		) VALUES ($1, $2, $3, 'activate', $4, $5, $6, $7)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), nextEpoch,
		mutation.ActorID(), mutation.CredentialID().String(), mutation.RequestID(), at); err != nil {
		return mapActiveWriteError("insert activated release snapshot", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project_release_snapshot_module (
			project_id, environment_id, release_epoch, module_name,
			source_kind, release_id, runtime_revision_hash,
			record_namespace_revision_hash, data_schema_format,
			data_schema_fingerprint
		)
		SELECT project_id, environment_id, $4, module_name,
		       source_kind, release_id, runtime_revision_hash,
		       record_namespace_revision_hash, data_schema_format,
		       data_schema_fingerprint
		FROM panvara_project_release_snapshot_module
		WHERE project_id = $1 AND environment_id = $2
		  AND release_epoch = $3 AND module_name <> $5
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), current.Epoch(), nextEpoch,
		value.ModuleName()); err != nil {
		return mapActiveWriteError("copy active release bindings", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project_release_snapshot_module (
			project_id, environment_id, release_epoch, module_name,
			source_kind, release_id, runtime_revision_hash,
			record_namespace_revision_hash, data_schema_format,
			data_schema_fingerprint
		) VALUES ($1, $2, $3, $4, 'release', $5, $6, $7, $8, $9)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), nextEpoch,
		value.ModuleName(), value.ID().String(), value.CandidateRevision(),
		current.RecordNamespaceRevision(), value.DataSchemaFormat(),
		value.DataSchemaFingerprint()); err != nil {
		return mapActiveWriteError("insert activated release binding", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE panvara_environment_release_pointer
		SET active_epoch = $3, updated_at = $4
		WHERE project_id = $1 AND environment_id = $2 AND active_epoch = $5
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), nextEpoch, at, current.Epoch())
	if err != nil {
		return mapActiveWriteError("advance active release pointer", err)
	}
	if tag.RowsAffected() != 1 {
		return releaseapp.ErrActivationConflict
	}
	return nil
}

func sameActivationRevision(left, right domainappmodule.Revision) bool {
	return left.SameArtifacts(right) && left.SourceFormat() == right.SourceFormat() &&
		left.SourceHash() == right.SourceHash() && bytes.Equal(left.Source(), right.Source())
}

func sameModuleReleaseFact(left, right domainrelease.ModuleRelease) bool {
	return left.ID() == right.ID() && left.SamePublishIntent(right) &&
		sameProjectScope(left.Scope(), right.Scope()) &&
		left.PublishedBy() == right.PublishedBy() &&
		left.PublishedCredentialID() == right.PublishedCredentialID() &&
		left.RequestID() == right.RequestID() && left.PublishedAt().Equal(right.PublishedAt())
}

func sameActiveSnapshotFact(left, right domainrelease.ActiveSnapshot) bool {
	leftRelease, leftHasRelease := left.ReleaseID()
	rightRelease, rightHasRelease := right.ReleaseID()
	leftCredential, leftHasCredential := left.ActivatedCredentialID()
	rightCredential, rightHasCredential := right.ActivatedCredentialID()
	return sameProjectScope(left.Scope(), right.Scope()) && left.Epoch() == right.Epoch() &&
		left.Origin() == right.Origin() && left.ModuleName() == right.ModuleName() &&
		leftHasRelease == rightHasRelease && (!leftHasRelease || leftRelease == rightRelease) &&
		left.RuntimeRevision() == right.RuntimeRevision() &&
		left.RecordNamespaceRevision() == right.RecordNamespaceRevision() &&
		left.DataSchemaFormat() == right.DataSchemaFormat() &&
		left.DataSchemaFingerprint() == right.DataSchemaFingerprint() &&
		left.ActivatedBy() == right.ActivatedBy() &&
		leftHasCredential == rightHasCredential && (!leftHasCredential || leftCredential == rightCredential) &&
		left.RequestID() == right.RequestID() && left.ActivatedAt().Equal(right.ActivatedAt())
}

func sameProjectScope(left, right project.Scope) bool {
	return left.ProjectID().String() == right.ProjectID().String() &&
		left.EnvironmentID().String() == right.EnvironmentID().String()
}

func commitReleaseActivation(ctx context.Context, tx pgx.Tx, action string) error {
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf(
			"%w: commit PostgreSQL %s: %w",
			releaseapp.ErrActivationOutcomeUnknown,
			action,
			err,
		)
	}
	return nil
}

func mapActiveWriteError(action string, err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "23505", "55000", "40001", "40P01":
			return fmt.Errorf("%w: %s", releaseapp.ErrActivationConflict, action)
		case "23503":
			return fmt.Errorf("%w: %s dependency", releaseapp.ErrCorrupt, action)
		case "23502", "23514", "22P02":
			return fmt.Errorf("%w: %s", releaseapp.ErrCorrupt, action)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
