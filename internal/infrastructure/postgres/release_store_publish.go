/*
   Panvara
   internal/infrastructure/postgres/release_store_publish.go    2026-07-19
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
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	accessapp "github.com/shezw/panvara/internal/application/access"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

var releaseIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// ResolveReplay authorizes and resolves stable publication facts before any
// mutable Draft snapshot is read. A new key for an already published scoped
// plan is recorded with its success audit in the same transaction.
func (store *ReleaseStore) ResolveReplay(
	ctx context.Context,
	mutation accessapp.MutationContext,
	module string,
	planID string,
	idempotencyKey string,
	intentHash string,
	resolvedAt time.Time,
) (domainrelease.ModuleRelease, bool, error) {
	if ctx == nil {
		return domainrelease.ModuleRelease{}, false, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := mutation.Validate(); err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf(
			"%w: invalid release replay mutation: %v", releaseapp.ErrInvalid, err,
		)
	}
	if err := validateMutationContext(mutation, accessapp.OperationReleasePublish); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if !domainappmodule.ValidModuleName(module) || !domainappmodule.ValidContentHash(planID) ||
		!releaseIdempotencyKeyPattern.MatchString(idempotencyKey) ||
		!domainappmodule.ValidContentHash(intentHash) || resolvedAt.IsZero() {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf(
			"%w: invalid release replay metadata", releaseapp.ErrInvalid,
		)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("begin PostgreSQL release replay: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	scope := mutation.Scope()
	if err := lockAccessProject(ctx, tx, scope.ProjectID()); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := reauthorizeMutation(ctx, tx, mutation); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}

	stored, storedIntent, found, err := readIdempotentModuleRelease(
		ctx, tx, scope, module, idempotencyKey,
	)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if found {
		if storedIntent != intentHash {
			return domainrelease.ModuleRelease{}, false, releaseapp.ErrIdempotencyConflict
		}
		if stored.PlanID() != planID {
			return domainrelease.ModuleRelease{}, false, fmt.Errorf(
				"%w: idempotency result differs from replay intent", releaseapp.ErrCorrupt,
			)
		}
		if err := commitReleasePublish(ctx, tx, "resolve module release key replay"); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		return stored, true, nil
	}

	existing, found, err := readModuleReleaseByPlan(ctx, tx, scope, module, planID)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if found {
		at := resolvedAt.UTC()
		if err := insertReleaseIdempotency(ctx, tx, existing, idempotencyKey, intentHash, at); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		if err := store.appendReleaseAudit(ctx, tx, mutation, existing.ID().String(), at); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		if err := commitReleasePublish(ctx, tx, "resolve module release plan replay"); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		return existing, true, nil
	}

	if err := commitReleasePublish(ctx, tx, "resolve absent module release replay"); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	return domainrelease.ModuleRelease{}, false, nil
}

// Publish atomically reauthorizes the caller, rechecks the immutable snapshot
// chain against the locked Draft head, registers the Revision, and appends the
// Release, idempotency mapping, and successful security audit.
func (store *ReleaseStore) Publish(
	ctx context.Context,
	mutation accessapp.MutationContext,
	revision domainappmodule.Revision,
	proposed domainrelease.ModuleRelease,
	idempotencyKey string,
	intentHash string,
) (domainrelease.ModuleRelease, bool, error) {
	if ctx == nil {
		return domainrelease.ModuleRelease{}, false, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	validatedRelease, err := rebuildModuleRelease(proposed)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: validate release fact: %v", releaseapp.ErrInvalid, err)
	}
	validatedRevision, err := rebuildRevision(revision)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: validate release revision: %v", releaseapp.ErrInvalid, err)
	}
	if !releaseIdempotencyKeyPattern.MatchString(idempotencyKey) ||
		!domainappmodule.ValidContentHash(intentHash) {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: invalid release idempotency metadata", releaseapp.ErrInvalid)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("begin PostgreSQL module publish: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The project lock serializes owner authority changes and all Release writes
	// before transaction-local validation and reauthorization.
	if err := lockAccessProject(ctx, tx, validatedRelease.Scope().ProjectID()); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := mutation.Validate(); err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: invalid release mutation: %v", releaseapp.ErrInvalid, err)
	}
	if err := validateMutationContext(mutation, accessapp.OperationReleasePublish); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := validateReleaseMutationBoundary(mutation, validatedRevision, validatedRelease); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := reauthorizeMutation(ctx, tx, mutation); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}

	stored, storedIntent, found, err := readIdempotentModuleRelease(
		ctx, tx, validatedRelease.Scope(), validatedRelease.ModuleName(), idempotencyKey,
	)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if found {
		if storedIntent != intentHash {
			return domainrelease.ModuleRelease{}, false, releaseapp.ErrIdempotencyConflict
		}
		if !stored.SamePublishIntent(validatedRelease) {
			return domainrelease.ModuleRelease{}, false, fmt.Errorf(
				"%w: idempotency result differs from publish intent", releaseapp.ErrCorrupt,
			)
		}
		if err := commitReleasePublish(ctx, tx, "replay module release"); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		return stored, false, nil
	}

	existing, found, err := readModuleReleaseByPlan(
		ctx, tx, validatedRelease.Scope(), validatedRelease.ModuleName(), validatedRelease.PlanID(),
	)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if found {
		if !existing.SamePublishIntent(validatedRelease) {
			return domainrelease.ModuleRelease{}, false, fmt.Errorf(
				"%w: existing plan release differs from publish intent", releaseapp.ErrCorrupt,
			)
		}
		if err := insertReleaseIdempotency(
			ctx, tx, existing, idempotencyKey, intentHash, validatedRelease.PublishedAt(),
		); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		if err := store.appendReleaseAudit(ctx, tx, mutation, existing.ID().String(), validatedRelease.PublishedAt()); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		if err := commitReleasePublish(ctx, tx, "alias module release idempotency"); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		return existing, false, nil
	}

	if err := verifyPublishSnapshotTx(ctx, tx, validatedRevision, validatedRelease); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	registered, _, err := registerModuleRevisionTx(ctx, tx, validatedRevision)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("register published PostgreSQL revision: %w", err)
	}
	if registered.RevisionHash() != validatedRelease.CandidateRevision() ||
		!registered.SameParentArtifacts(validatedRevision) {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf(
			"%w: registered revision differs from planned candidate", releaseapp.ErrCorrupt,
		)
	}
	if err := insertModuleRelease(ctx, tx, validatedRelease); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := insertReleaseIdempotency(
		ctx, tx, validatedRelease, idempotencyKey, intentHash, validatedRelease.PublishedAt(),
	); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := store.appendReleaseAudit(
		ctx, tx, mutation, validatedRelease.ID().String(), validatedRelease.PublishedAt(),
	); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := commitReleasePublish(ctx, tx, "publish module release"); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	return validatedRelease, true, nil
}

func validateReleaseMutationBoundary(
	mutation accessapp.MutationContext,
	revision domainappmodule.Revision,
	value domainrelease.ModuleRelease,
) error {
	scope := mutation.Scope()
	if value.Scope() != scope || value.PublishedBy() != mutation.ActorID() ||
		value.PublishedCredentialID() != mutation.CredentialID() ||
		value.RequestID() != mutation.RequestID() {
		return fmt.Errorf("%w: release provenance differs from mutation evidence", releaseapp.ErrInvalid)
	}
	if revision.ProjectID().String() != scope.ProjectID().String() ||
		revision.ModuleName() != value.ModuleName() ||
		revision.RevisionHash() != value.CandidateRevision() ||
		revision.SourceHash() != value.SourceHash() ||
		revision.Origin() != domainappmodule.RevisionOriginPublish ||
		revision.RegisteredBy() != value.PublishedBy() ||
		!revision.RegisteredAt().Equal(value.PublishedAt()) {
		return fmt.Errorf("%w: revision differs from release provenance", releaseapp.ErrInvalid)
	}
	identity, ok := revision.DataSchemaIdentity(value.DataSchemaFormat())
	if !ok || identity.Fingerprint() != value.DataSchemaFingerprint() {
		return fmt.Errorf("%w: revision data schema differs from release", releaseapp.ErrInvalid)
	}
	return nil
}

func verifyPublishSnapshotTx(
	ctx context.Context,
	tx pgx.Tx,
	revision domainappmodule.Revision,
	value domainrelease.ModuleRelease,
) error {
	draft, err := scanDraft(tx.QueryRow(ctx, `
		SELECT `+draftColumns+`
		FROM panvara_module_draft
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		FOR UPDATE
	`, value.Scope().ProjectID().String(), value.ModuleName(), value.DraftID().String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return releaseapp.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock PostgreSQL publish draft: %w", err)
	}
	if draft.Generation() != value.DraftGeneration() ||
		draft.Baseline().RevisionHash() != value.BaselineRevision() ||
		draft.SourceHash() != value.SourceHash() ||
		draft.SourceFormat() != revision.SourceFormat() ||
		!bytes.Equal(draft.Source(), revision.Source()) {
		return releaseapp.ErrStale
	}

	validation, err := scanValidation(tx.QueryRow(ctx, `
		SELECT `+validationColumns+`
		FROM panvara_module_draft_validation
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		  AND validation_id = $4
	`, value.Scope().ProjectID().String(), value.ModuleName(), value.DraftID().String(), value.ValidationID()))
	if errors.Is(err, pgx.ErrNoRows) {
		return releaseapp.ErrNotFound
	}
	if err != nil {
		return snapshotReadError("transaction validation", err)
	}
	plan, err := scanPlan(tx.QueryRow(ctx, `
		SELECT `+planColumns+`
		FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		  AND plan_id = $4
	`, value.Scope().ProjectID().String(), value.ModuleName(), value.DraftID().String(), value.PlanID()))
	if errors.Is(err, pgx.ErrNoRows) {
		return releaseapp.ErrNotFound
	}
	if err != nil {
		return snapshotReadError("transaction plan", err)
	}
	if !validation.Valid || plan.Outcome == "unsupported" {
		return releaseapp.ErrNotPublishable
	}
	if validation.ProjectID.String() != value.Scope().ProjectID().String() ||
		validation.ModuleName != value.ModuleName() ||
		validation.DraftID.String() != value.DraftID().String() ||
		validation.Generation != value.DraftGeneration() ||
		validation.BaselineRevision != value.BaselineRevision() ||
		validation.SourceFormat != revision.SourceFormat() ||
		validation.SourceHash != value.SourceHash() || validation.Candidate == nil ||
		validation.Candidate.RevisionHash != value.CandidateRevision() ||
		validation.Candidate.ModuleVersion != revision.ModuleVersion() ||
		validation.Candidate.DataSchemaFormat != value.DataSchemaFormat() ||
		validation.Candidate.DataSchemaFingerprint != value.DataSchemaFingerprint() ||
		!bytes.Equal(validation.CandidateIR, revision.CanonicalIR()) {
		return fmt.Errorf("%w: validation differs from release candidate", releaseapp.ErrCorrupt)
	}
	if plan.ProjectID.String() != value.Scope().ProjectID().String() ||
		plan.ModuleName != value.ModuleName() || plan.DraftID.String() != value.DraftID().String() ||
		plan.DraftGeneration != value.DraftGeneration() || plan.ValidationID != value.ValidationID() ||
		plan.ID != value.PlanID() || plan.PlanHash != value.PlanHash() ||
		plan.BaselineRevision != value.BaselineRevision() ||
		plan.SourceFormat != revision.SourceFormat() || plan.SourceHash != value.SourceHash() ||
		plan.Candidate.RevisionHash != value.CandidateRevision() ||
		plan.Candidate.ModuleVersion != revision.ModuleVersion() ||
		plan.Candidate.DataSchemaFormat != value.DataSchemaFormat() ||
		plan.Candidate.DataSchemaFingerprint != value.DataSchemaFingerprint() ||
		plan.Outcome != string(value.Outcome()) || plan.Risk != value.Risk() {
		return fmt.Errorf("%w: plan differs from release fact", releaseapp.ErrCorrupt)
	}
	return nil
}

func readIdempotentModuleRelease(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	module string,
	idempotencyKey string,
) (domainrelease.ModuleRelease, string, bool, error) {
	var intentHash, releaseText string
	err := tx.QueryRow(ctx, `
		SELECT intent_hash, release_id::text
		FROM panvara_module_release_idempotency
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND idempotency_key = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(),
		module, idempotencyKey).Scan(&intentHash, &releaseText)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ModuleRelease{}, "", false, nil
	}
	if err != nil {
		return domainrelease.ModuleRelease{}, "", false, fmt.Errorf("read PostgreSQL release idempotency: %w", err)
	}
	stored, err := scanModuleRelease(tx.QueryRow(ctx, `
		SELECT `+moduleReleaseColumns+`
		FROM panvara_module_release
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND release_id = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module, releaseText))
	if err != nil {
		return domainrelease.ModuleRelease{}, "", false, corruptRelease("read idempotency result", err)
	}
	return stored, intentHash, true, nil
}

func readModuleReleaseByPlan(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	module string,
	planID string,
) (domainrelease.ModuleRelease, bool, error) {
	stored, err := scanModuleRelease(tx.QueryRow(ctx, `
		SELECT `+moduleReleaseColumns+`
		FROM panvara_module_release
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND plan_id = $4
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), module, planID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainrelease.ModuleRelease{}, false, nil
	}
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("read PostgreSQL release by plan: %w", err)
	}
	return stored, true, nil
}

func insertModuleRelease(ctx context.Context, tx pgx.Tx, value domainrelease.ModuleRelease) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO panvara_module_release (
			project_id, environment_id, module_name, release_id,
			draft_id, draft_generation, validation_id, plan_id, plan_hash,
			baseline_revision_hash, candidate_revision_hash,
			candidate_data_schema_format, candidate_data_schema_fingerprint,
			source_hash, outcome, risk, published_by_principal_id,
			published_by_credential_id, request_id, published_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''),
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
	`, value.Scope().ProjectID().String(), value.Scope().EnvironmentID().String(),
		value.ModuleName(), value.ID().String(), value.DraftID().String(), value.DraftGeneration(),
		value.ValidationID(), value.PlanID(), value.PlanHash(), value.BaselineRevision(),
		value.CandidateRevision(), value.DataSchemaFormat(), value.DataSchemaFingerprint(),
		value.SourceHash(), string(value.Outcome()), value.Risk(), value.PublishedBy(),
		value.PublishedCredentialID().String(), value.RequestID(), value.PublishedAt())
	if err != nil {
		return mapReleaseWriteError("insert module release", err)
	}
	return nil
}

func insertReleaseIdempotency(
	ctx context.Context,
	tx pgx.Tx,
	value domainrelease.ModuleRelease,
	key string,
	intentHash string,
	createdAt time.Time,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO panvara_module_release_idempotency (
			project_id, environment_id, module_name, idempotency_key,
			intent_hash, release_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, value.Scope().ProjectID().String(), value.Scope().EnvironmentID().String(),
		value.ModuleName(), key, intentHash, value.ID().String(), createdAt)
	if err != nil {
		return mapReleaseWriteError("insert module release idempotency", err)
	}
	return nil
}

func (store *ReleaseStore) appendReleaseAudit(
	ctx context.Context,
	tx pgx.Tx,
	mutation accessapp.MutationContext,
	targetID string,
	at time.Time,
) error {
	auditStore := &AccessAdminStore{pool: store.pool, auditIDs: store.auditIDs}
	return auditStore.appendAudit(ctx, tx, auditFact{
		scope: mutation.Scope(), actorID: mutation.ActorID(), credentialID: mutation.CredentialID(),
		requestID: mutation.RequestID(), action: string(accessapp.OperationReleasePublish),
		outcome: auditOutcomeSuccess, targetKind: "module_release", targetID: targetID,
		occurredAt: at.UTC(),
	})
}

func commitReleasePublish(ctx context.Context, tx pgx.Tx, action string) error {
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL %s: %w", action, err)
	}
	return nil
}

func mapReleaseWriteError(action string, err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "23505":
			return fmt.Errorf("%w: %s", releaseapp.ErrIdempotencyConflict, action)
		case "23503":
			return fmt.Errorf("%w: %s dependency", releaseapp.ErrNotFound, action)
		case "23502", "23514", "22P02":
			return fmt.Errorf("%w: %s", releaseapp.ErrInvalid, action)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}
