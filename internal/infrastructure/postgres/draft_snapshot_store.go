/*
   Panvara
   internal/infrastructure/postgres/draft_snapshot_store.go    2026-07-16
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

	"github.com/jackc/pgx/v5"
	application "github.com/shezw/panvara/internal/application/appmodule"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

// SaveValidation atomically verifies the current draft generation/source and
// appends one immutable deterministic validation snapshot.
func (store *DraftStore) SaveValidation(ctx context.Context, value application.DraftValidation) (application.DraftValidation, bool, error) {
	if err := application.ValidateDraftValidation(value); err != nil {
		return application.DraftValidation{}, false, err
	}
	issuesJSON, err := json.Marshal(value.Issues)
	if err != nil {
		return application.DraftValidation{}, false, fmt.Errorf("marshal PostgreSQL validation issues: %w", err)
	}
	candidateRevision, candidateVersion, candidateFormat, candidateFingerprint := validationCandidateArguments(value.Candidate)
	row := store.pool.QueryRow(ctx, `
		INSERT INTO panvara_module_draft_validation (
			project_id, module_name, draft_id, validation_id, format_version,
			draft_generation, baseline_revision_hash, source_format, source_hash,
			valid, issues, issues_hash, candidate_revision_hash, candidate_module_version,
			candidate_data_schema_format, candidate_data_schema_fingerprint, candidate_ir_bytes,
			created_by, created_at
		)
		SELECT $1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, $9,
		       $10, $11::jsonb, $12, $13, $14, $15, $16, $17, $18, $19
		FROM panvara_module_draft AS draft
		WHERE draft.project_id = $1 AND draft.module_name = $2 AND draft.draft_id = $3
		  AND draft.generation = $6
		  AND draft.baseline_revision_hash IS NOT DISTINCT FROM NULLIF($7, '')
		  AND draft.source_format = $8 AND draft.source_hash = $9
		ON CONFLICT DO NOTHING
		RETURNING `+validationColumns,
		value.ProjectID.String(), value.ModuleName, value.DraftID.String(), value.ID, value.FormatVersion,
		value.Generation, value.BaselineRevision, string(value.SourceFormat), value.SourceHash,
		value.Valid, string(issuesJSON), value.IssuesHash, candidateRevision, candidateVersion,
		candidateFormat, candidateFingerprint, nullableBytes(value.CandidateIR), value.CreatedBy, value.CreatedAt)
	stored, err := scanValidation(row)
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.DraftValidation{}, false, fmt.Errorf("insert PostgreSQL draft validation: %w", err)
	}
	current, getErr := store.Get(ctx, value.ProjectID, value.ModuleName, value.DraftID)
	if getErr != nil {
		return application.DraftValidation{}, false, getErr
	}
	if current.Generation() != value.Generation || current.SourceHash() != value.SourceHash ||
		current.SourceFormat() != value.SourceFormat || current.Baseline().RevisionHash() != value.BaselineRevision {
		return application.DraftValidation{}, false, application.ErrDraftConflict
	}
	existing, err := scanValidation(store.pool.QueryRow(ctx, `
		SELECT `+validationColumns+`
		FROM panvara_module_draft_validation
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		  AND draft_generation = $4 AND format_version = $5
	`, value.ProjectID.String(), value.ModuleName, value.DraftID.String(), value.Generation, value.FormatVersion))
	if err != nil {
		return application.DraftValidation{}, false, fmt.Errorf("read deterministic PostgreSQL draft validation: %w", err)
	}
	if existing.ID != value.ID {
		return application.DraftValidation{}, false, fmt.Errorf("%w: validation generation has conflicting identity", application.ErrDraftCorrupt)
	}
	return existing, false, nil
}

// GetValidation returns one exact immutable validation snapshot.
func (store *DraftStore) GetValidation(ctx context.Context, projectID project.ID, module string, draftID domain.DraftID, validationID string) (application.DraftValidation, error) {
	value, err := scanValidation(store.pool.QueryRow(ctx, `
		SELECT `+validationColumns+`
		FROM panvara_module_draft_validation
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3 AND validation_id = $4
	`, projectID.String(), module, draftID.String(), validationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return application.DraftValidation{}, application.ErrValidationNotFound
	}
	if err != nil {
		return application.DraftValidation{}, fmt.Errorf("get PostgreSQL draft validation: %w", err)
	}
	return value, nil
}

// SavePlan atomically verifies the exact current generation and successful
// validation before appending one immutable deterministic plan.
func (store *DraftStore) SavePlan(ctx context.Context, value application.DraftPlan, expected uint64) (application.DraftPlan, bool, error) {
	if expected == 0 || expected > domain.MaxDraftGeneration || expected != value.DraftGeneration {
		return application.DraftPlan{}, false, application.ErrDraftConflict
	}
	if err := application.ValidateDraftPlan(value); err != nil {
		return application.DraftPlan{}, false, err
	}
	changesJSON, err := json.Marshal(value.Changes)
	if err != nil {
		return application.DraftPlan{}, false, fmt.Errorf("marshal PostgreSQL plan changes: %w", err)
	}
	row := store.pool.QueryRow(ctx, `
		INSERT INTO panvara_module_draft_plan (
			project_id, module_name, draft_id, plan_id, plan_hash, format_version, draft_generation,
			validation_id, baseline_revision_hash, baseline_data_schema_format,
			baseline_data_schema_fingerprint, source_format, source_hash,
			candidate_revision_hash, candidate_module_version, candidate_data_schema_format,
			candidate_data_schema_fingerprint, changes, risk_low, risk_review, risk_destructive,
			outcome, risk, data_schema_changed, record_namespace_changed,
			migration_execution_supported, created_by, created_at
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), NULLIF($10, 0), NULLIF($11, ''),
		       $12, $13, $14, $15, $16, $17, $18::jsonb, $19, $20, $21,
		       $22, $23, $24, $25, $26, $27, $28
		FROM panvara_module_draft AS draft
		JOIN panvara_module_draft_validation AS validation
		  ON validation.project_id = draft.project_id
		 AND validation.module_name = draft.module_name
		 AND validation.draft_id = draft.draft_id
		 AND validation.validation_id = $8
		WHERE draft.project_id = $1 AND draft.module_name = $2 AND draft.draft_id = $3
		  AND draft.generation = $7 AND draft.source_format = $12 AND draft.source_hash = $13
		  AND validation.valid AND validation.draft_generation = $7
		  AND validation.source_format = $12 AND validation.source_hash = $13
		  AND validation.baseline_revision_hash IS NOT DISTINCT FROM NULLIF($9, '')
		  AND validation.candidate_revision_hash = $14
		  AND validation.candidate_module_version = $15
		  AND validation.candidate_data_schema_format = $16
		  AND validation.candidate_data_schema_fingerprint = $17
		  AND (
		      (NULLIF($9, '') IS NULL AND $10 = 0 AND NULLIF($11, '') IS NULL)
		      OR EXISTS (
		          SELECT 1
		          FROM panvara_module_revision_data_schema AS baseline_identity
		          WHERE baseline_identity.project_id = $1
		            AND baseline_identity.module_name = $2
		            AND baseline_identity.revision_hash = $9
		            AND baseline_identity.data_schema_format = $10
		            AND baseline_identity.data_schema_fingerprint = $11
		      )
		  )
		ON CONFLICT DO NOTHING
		RETURNING `+planColumns,
		value.ProjectID.String(), value.ModuleName, value.DraftID.String(), value.ID, value.PlanHash, value.FormatVersion,
		value.DraftGeneration, value.ValidationID, value.BaselineRevision, value.BaselineDataSchemaFormat,
		value.BaselineDataSchemaFingerprint, string(value.SourceFormat), value.SourceHash,
		value.Candidate.RevisionHash, value.Candidate.ModuleVersion, value.Candidate.DataSchemaFormat,
		value.Candidate.DataSchemaFingerprint, string(changesJSON), value.RiskSummary.Low,
		value.RiskSummary.Review, value.RiskSummary.Destructive, value.Outcome, value.Risk,
		value.DataSchemaChanged, value.RecordNamespaceChanged, value.MigrationExecutionSupported,
		value.CreatedBy, value.CreatedAt)
	stored, err := scanPlan(row)
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.DraftPlan{}, false, fmt.Errorf("insert PostgreSQL draft plan: %w", err)
	}
	current, getErr := store.Get(ctx, value.ProjectID, value.ModuleName, value.DraftID)
	if getErr != nil {
		return application.DraftPlan{}, false, getErr
	}
	if current.Generation() != expected || current.SourceHash() != value.SourceHash || current.SourceFormat() != value.SourceFormat {
		return application.DraftPlan{}, false, application.ErrDraftConflict
	}
	if _, getErr := store.GetValidation(ctx, value.ProjectID, value.ModuleName, value.DraftID, value.ValidationID); getErr != nil {
		return application.DraftPlan{}, false, getErr
	}
	existing, err := scanPlan(store.pool.QueryRow(ctx, `
		SELECT `+planColumns+`
		FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		  AND validation_id = $4 AND format_version = $5
	`, value.ProjectID.String(), value.ModuleName, value.DraftID.String(), value.ValidationID, value.FormatVersion))
	if err != nil {
		return application.DraftPlan{}, false, fmt.Errorf("read deterministic PostgreSQL draft plan: %w", err)
	}
	if existing.ID != value.ID {
		return application.DraftPlan{}, false, fmt.Errorf("%w: validation has conflicting plan identity", application.ErrDraftCorrupt)
	}
	return existing, false, nil
}

// GetPlan returns one exact immutable plan snapshot.
func (store *DraftStore) GetPlan(ctx context.Context, projectID project.ID, module string, draftID domain.DraftID, planID string) (application.DraftPlan, error) {
	value, err := scanPlan(store.pool.QueryRow(ctx, `
		SELECT `+planColumns+`
		FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3 AND plan_id = $4
	`, projectID.String(), module, draftID.String(), planID))
	if errors.Is(err, pgx.ErrNoRows) {
		return application.DraftPlan{}, application.ErrPlanNotFound
	}
	if err != nil {
		return application.DraftPlan{}, fmt.Errorf("get PostgreSQL draft plan: %w", err)
	}
	return value, nil
}

const validationColumns = `
	project_id::text, module_name, draft_id::text, validation_id, format_version,
	draft_generation, baseline_revision_hash, source_format, source_hash, valid,
	issues::text, issues_hash, candidate_revision_hash, candidate_module_version,
	candidate_data_schema_format, candidate_data_schema_fingerprint, candidate_ir_bytes,
	created_by, created_at
`

const planColumns = `
	project_id::text, module_name, draft_id::text, plan_id, plan_hash, format_version,
	draft_generation, validation_id, baseline_revision_hash, baseline_data_schema_format,
	baseline_data_schema_fingerprint, source_format, source_hash,
	candidate_revision_hash, candidate_module_version, candidate_data_schema_format,
	candidate_data_schema_fingerprint, changes::text, risk_low, risk_review, risk_destructive,
	outcome, risk, data_schema_changed, record_namespace_changed,
	migration_execution_supported, created_by, created_at
`

func validationCandidateArguments(candidate *application.CandidateIdentity) (any, any, any, any) {
	if candidate == nil {
		return nil, nil, nil, nil
	}
	return candidate.RevisionHash, candidate.ModuleVersion, candidate.DataSchemaFormat, candidate.DataSchemaFingerprint
}

func nullableBytes(value []byte) any {
	if value == nil {
		return nil
	}
	return value
}

func scanValidation(row draftScanner) (application.DraftValidation, error) {
	var value application.DraftValidation
	var projectText, draftText, issuesText string
	var baseline, candidateRevision, candidateVersion, candidateFingerprint *string
	var candidateFormat *int
	var sourceFormat string
	if err := row.Scan(
		&projectText, &value.ModuleName, &draftText, &value.ID, &value.FormatVersion,
		&value.Generation, &baseline, &sourceFormat, &value.SourceHash, &value.Valid,
		&issuesText, &value.IssuesHash, &candidateRevision, &candidateVersion, &candidateFormat,
		&candidateFingerprint, &value.CandidateIR, &value.CreatedBy, &value.CreatedAt,
	); err != nil {
		return application.DraftValidation{}, err
	}
	projectID, draftID, err := parseSnapshotScope(projectText, draftText)
	if err != nil {
		return application.DraftValidation{}, err
	}
	value.ProjectID, value.DraftID = projectID, draftID
	value.ValidationHash = value.ID
	value.SourceFormat = domain.SourceFormat(sourceFormat)
	if baseline != nil {
		value.BaselineRevision = *baseline
	}
	if err := json.Unmarshal([]byte(issuesText), &value.Issues); err != nil {
		return application.DraftValidation{}, fmt.Errorf("%w: decode persisted validation issues: %v", application.ErrDraftCorrupt, err)
	}
	if candidateRevision != nil && candidateVersion != nil && candidateFormat != nil && candidateFingerprint != nil {
		value.Candidate = &application.CandidateIdentity{
			RevisionHash: *candidateRevision, ModuleVersion: *candidateVersion,
			DataSchemaFormat: *candidateFormat, DataSchemaFingerprint: *candidateFingerprint,
		}
	}
	if err := application.ValidateDraftValidation(value); err != nil {
		return application.DraftValidation{}, err
	}
	return value, nil
}

func scanPlan(row draftScanner) (application.DraftPlan, error) {
	var value application.DraftPlan
	var projectText, draftText, changesText, sourceFormat string
	var baseline, baselineFingerprint *string
	var baselineFormat *int
	if err := row.Scan(
		&projectText, &value.ModuleName, &draftText, &value.ID, &value.PlanHash, &value.FormatVersion,
		&value.DraftGeneration, &value.ValidationID, &baseline, &baselineFormat, &baselineFingerprint,
		&sourceFormat, &value.SourceHash, &value.Candidate.RevisionHash, &value.Candidate.ModuleVersion,
		&value.Candidate.DataSchemaFormat, &value.Candidate.DataSchemaFingerprint, &changesText,
		&value.RiskSummary.Low, &value.RiskSummary.Review, &value.RiskSummary.Destructive,
		&value.Outcome, &value.Risk, &value.DataSchemaChanged, &value.RecordNamespaceChanged,
		&value.MigrationExecutionSupported, &value.CreatedBy, &value.CreatedAt,
	); err != nil {
		return application.DraftPlan{}, err
	}
	projectID, draftID, err := parseSnapshotScope(projectText, draftText)
	if err != nil {
		return application.DraftPlan{}, err
	}
	value.ProjectID, value.DraftID = projectID, draftID
	value.SourceFormat = domain.SourceFormat(sourceFormat)
	if baseline != nil {
		value.BaselineRevision = *baseline
	}
	if baselineFormat != nil {
		value.BaselineDataSchemaFormat = *baselineFormat
	}
	if baselineFingerprint != nil {
		value.BaselineDataSchemaFingerprint = *baselineFingerprint
	}
	if err := json.Unmarshal([]byte(changesText), &value.Changes); err != nil {
		return application.DraftPlan{}, fmt.Errorf("%w: decode persisted plan changes: %v", application.ErrDraftCorrupt, err)
	}
	if err := application.ValidateDraftPlan(value); err != nil {
		return application.DraftPlan{}, err
	}
	return value, nil
}

func parseSnapshotScope(projectText, draftText string) (project.ID, domain.DraftID, error) {
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return project.ID{}, domain.DraftID{}, fmt.Errorf("%w: parse persisted snapshot project: %v", application.ErrDraftCorrupt, err)
	}
	draftID, err := domain.ParseDraftID(draftText)
	if err != nil {
		return project.ID{}, domain.DraftID{}, fmt.Errorf("%w: parse persisted snapshot draft: %v", application.ErrDraftCorrupt, err)
	}
	return projectID, draftID, nil
}
