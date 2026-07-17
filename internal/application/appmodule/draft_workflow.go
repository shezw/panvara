/*
   Panvara
   internal/application/appmodule/draft_workflow.go    2026-07-16
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package appmodule

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// DraftWorkflow coordinates authorized drafting, deterministic validation, and
// side-effect-free planning. It has no publication, activation, or Record port.
type DraftWorkflow struct {
	store     DraftStore
	revisions DraftRevisionReader
	clock     DraftClock
	ids       DraftIDGenerator
	compiler  *Compiler
}

// NewDraftWorkflow constructs the draft application boundary.
func NewDraftWorkflow(store DraftStore, revisions DraftRevisionReader, clock DraftClock, ids DraftIDGenerator) (*DraftWorkflow, error) {
	if store == nil || revisions == nil || clock == nil || ids == nil {
		return nil, fmt.Errorf("%w: draft workflow dependency is nil", ErrDraftInvalid)
	}
	return &DraftWorkflow{store: store, revisions: revisions, clock: clock, ids: ids, compiler: NewCompiler()}, nil
}

// Create persists a raw, possibly invalid source with a fixed explicit baseline.
func (workflow *DraftWorkflow) Create(ctx context.Context, projectID project.ID, owner actor.Context, module string, input CreateDraftInput) (domain.Draft, bool, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return domain.Draft{}, false, err
	}
	if err := ctx.Err(); err != nil {
		return domain.Draft{}, false, err
	}
	module = strings.TrimSpace(module)
	if !domain.ValidModuleName(module) || !idempotencyKeyPattern.MatchString(strings.TrimSpace(input.IdempotencyKey)) {
		return domain.Draft{}, false, fmt.Errorf("%w: module or idempotency key is invalid", ErrDraftInvalid)
	}
	baselineText := strings.TrimSpace(input.BaselineRevision)
	if baselineText == "none" {
		baselineText = ""
	}
	baseline, err := domain.NewDraftBaseline(baselineText)
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("%w: %v", ErrDraftInvalid, err)
	}
	if !baseline.None() {
		if _, err := workflow.revisions.Get(ctx, projectID, owner, module, baseline.RevisionHash()); err != nil {
			return domain.Draft{}, false, fmt.Errorf("verify draft baseline: %w", err)
		}
	}
	at := workflow.clock.Now().UTC()
	id, err := workflow.ids.New(at)
	if err != nil {
		return domain.Draft{}, false, err
	}
	draft, err := domain.NewDraft(domain.DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: module, Baseline: baseline,
		SourceFormat: input.Format, SourceHash: domain.DraftSourceHash(input.Source), Source: input.Source,
		Generation: 1, CreatedBy: owner.ActorID(), CreatedAt: at, UpdatedBy: owner.ActorID(), UpdatedAt: at,
	})
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("%w: %v", ErrDraftInvalid, err)
	}
	intentHash := hashCanonical(struct {
		Format   int                 `json:"formatVersion"`
		Project  string              `json:"project"`
		Module   string              `json:"module"`
		Baseline string              `json:"baseline"`
		Source   domain.SourceFormat `json:"sourceFormat"`
		Hash     string              `json:"sourceHash"`
	}{1, projectID.String(), module, baseline.RevisionHash(), input.Format, draft.SourceHash()})
	stored, created, err := workflow.store.Create(ctx, draft, strings.TrimSpace(input.IdempotencyKey), intentHash)
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("create module draft: %w", err)
	}
	if err := verifyDraftScope(stored, projectID, module, stored.ID()); err != nil {
		return domain.Draft{}, false, err
	}
	return stored, created, nil
}

// Get returns draft metadata and source identity after owner authorization.
func (workflow *DraftWorkflow) Get(ctx context.Context, projectID project.ID, owner actor.Context, module, idText string) (domain.Draft, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return domain.Draft{}, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil {
		return domain.Draft{}, err
	}
	return workflow.get(ctx, projectID, strings.TrimSpace(module), id)
}

// GetSource returns raw bytes bound to the current generation.
func (workflow *DraftWorkflow) GetSource(ctx context.Context, projectID project.ID, owner actor.Context, module, idText string) (DraftSource, error) {
	draft, err := workflow.Get(ctx, projectID, owner, module, idText)
	if err != nil {
		return DraftSource{}, err
	}
	return DraftSource{Format: draft.SourceFormat(), Hash: draft.SourceHash(), Bytes: draft.Source(), Generation: draft.Generation()}, nil
}

// Replace atomically replaces complete raw source at an expected generation.
// Byte-identical replacements are successful no-ops that retain generation and audit metadata.
func (workflow *DraftWorkflow) Replace(ctx context.Context, projectID project.ID, owner actor.Context, module, idText string, expectedGeneration uint64, input ReplaceDraftInput) (domain.Draft, bool, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return domain.Draft{}, false, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil || !validDraftGeneration(expectedGeneration) {
		return domain.Draft{}, false, fmt.Errorf("%w: invalid draft identity or generation", ErrDraftInvalid)
	}
	replacement, err := domain.NewDraftReplacement(input.Format, input.Source, owner.ActorID(), workflow.clock.Now())
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("%w: %v", ErrDraftInvalid, err)
	}
	if expectedGeneration == domain.MaxDraftGeneration {
		current, err := workflow.get(ctx, projectID, strings.TrimSpace(module), id)
		if err != nil {
			return domain.Draft{}, false, err
		}
		if current.Generation() != expectedGeneration || !current.SameSource(replacement) {
			return domain.Draft{}, false, ErrDraftConflict
		}
		return current, false, nil
	}
	stored, changed, err := workflow.store.Replace(ctx, projectID, strings.TrimSpace(module), id, expectedGeneration, replacement)
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("replace module draft: %w", err)
	}
	if err := verifyDraftScope(stored, projectID, strings.TrimSpace(module), id); err != nil {
		return domain.Draft{}, false, err
	}
	return stored, changed, nil
}

// Validate compiles one exact generation and persists a deterministic immutable result.
// Invalid AppModule source is a successful use-case result with Valid=false.
func (workflow *DraftWorkflow) Validate(ctx context.Context, projectID project.ID, owner actor.Context, module, idText string, expectedGeneration uint64) (DraftValidation, bool, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return DraftValidation{}, false, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil || !validDraftGeneration(expectedGeneration) {
		return DraftValidation{}, false, fmt.Errorf("%w: invalid validation identity or generation", ErrDraftInvalid)
	}
	module = strings.TrimSpace(module)
	draft, err := workflow.get(ctx, projectID, module, id)
	if err != nil {
		return DraftValidation{}, false, err
	}
	if draft.Generation() != expectedGeneration {
		return DraftValidation{}, false, ErrDraftConflict
	}
	if !draft.Baseline().None() {
		if _, err := workflow.revisions.Get(ctx, projectID, owner, module, draft.Baseline().RevisionHash()); err != nil {
			return DraftValidation{}, false, fmt.Errorf("reverify draft baseline: %w", err)
		}
	}
	validation := DraftValidation{
		FormatVersion: DraftValidationFormatVersion, ProjectID: projectID, ModuleName: module,
		DraftID: id, Generation: draft.Generation(), BaselineRevision: draft.Baseline().RevisionHash(),
		SourceFormat: draft.SourceFormat(), SourceHash: draft.SourceHash(), CreatedBy: owner.ActorID(), CreatedAt: workflow.clock.Now().UTC(),
	}
	compiled, compileErr := workflow.compiler.Compile(draft.Source(), draftSpecFormat(draft.SourceFormat()))
	if compileErr != nil {
		validation.Issues = []ValidationIssue{makeValidationIssue(compileErr)}
	} else if compiled.Name() != module {
		validation.Issues = []ValidationIssue{{
			Code: "module_name_mismatch", Stage: "policy", Path: "/metadata/name",
			Message: fmt.Sprintf("compiled module name %q does not match route module %q", compiled.Name(), module), Severity: "error",
		}}
	} else {
		validation.Valid = true
		validation.Issues = []ValidationIssue{}
		validation.Candidate = &CandidateIdentity{
			RevisionHash: compiled.RevisionHash(), ModuleVersion: compiled.Version(),
			DataSchemaFormat: compiled.DataSchemaFormat(), DataSchemaFingerprint: compiled.DataSchemaFingerprint(),
		}
		validation.CandidateIR = compiled.CanonicalIR()
	}
	sortValidationIssues(validation.Issues)
	validation.IssuesHash = hashCanonical(validation.Issues)
	validation.ID = validationIdentity(validation)
	validation.ValidationHash = validation.ID
	if err := ValidateDraftValidation(validation); err != nil {
		return DraftValidation{}, false, err
	}
	stored, created, err := workflow.store.SaveValidation(ctx, validation)
	if err != nil {
		return DraftValidation{}, false, fmt.Errorf("save module draft validation: %w", err)
	}
	if err := ValidateDraftValidation(stored); err != nil {
		return DraftValidation{}, false, err
	}
	return cloneValidation(stored), created, nil
}

// Plan deterministically compares one exact successful validation with its fixed baseline.
// It never registers, publishes, activates, migrates, or changes Record state.
func (workflow *DraftWorkflow) Plan(ctx context.Context, projectID project.ID, owner actor.Context, module, idText, validationID string, expectedGeneration uint64) (DraftPlan, bool, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return DraftPlan{}, false, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil || !validDraftGeneration(expectedGeneration) || !domain.ValidContentHash(strings.TrimSpace(validationID)) {
		return DraftPlan{}, false, fmt.Errorf("%w: invalid plan identity or generation", ErrDraftInvalid)
	}
	module = strings.TrimSpace(module)
	validation, err := workflow.store.GetValidation(ctx, projectID, module, id, strings.TrimSpace(validationID))
	if err != nil {
		return DraftPlan{}, false, fmt.Errorf("get plan validation: %w", err)
	}
	if err := ValidateDraftValidation(validation); err != nil {
		return DraftPlan{}, false, err
	}
	if !validation.Valid {
		return DraftPlan{}, false, ErrValidationInvalid
	}
	if validation.Generation != expectedGeneration {
		return DraftPlan{}, false, ErrDraftConflict
	}
	draft, err := workflow.get(ctx, projectID, module, id)
	if err != nil {
		return DraftPlan{}, false, err
	}
	if draft.Generation() != expectedGeneration || draft.SourceHash() != validation.SourceHash || draft.SourceFormat() != validation.SourceFormat {
		return DraftPlan{}, false, ErrDraftConflict
	}
	candidateIR, err := decodeCanonicalModuleIR(validation.CandidateIR)
	if err != nil || candidateIR.Name != module {
		return DraftPlan{}, false, fmt.Errorf("%w: invalid candidate IR: %v", ErrDraftCorrupt, err)
	}
	var baselineIR moduleIR
	var baselineIdentity domain.DataSchemaIdentity
	baselineExists := validation.BaselineRevision != ""
	if baselineExists {
		baseline, err := workflow.revisions.Get(ctx, projectID, owner, module, validation.BaselineRevision)
		if err != nil {
			return DraftPlan{}, false, fmt.Errorf("reverify plan baseline: %w", err)
		}
		baselineIR, err = decodeCanonicalModuleIR(baseline.CanonicalIR())
		if err != nil {
			return DraftPlan{}, false, fmt.Errorf("%w: decode baseline IR: %v", ErrDraftCorrupt, err)
		}
		var found bool
		baselineIdentity, found = baseline.DataSchemaIdentity(DataSchemaFormatVersion)
		if !found {
			return DraftPlan{}, false, fmt.Errorf("%w: baseline data schema identity missing", ErrDraftCorrupt)
		}
	}
	changes := buildPlanChanges(baselineIR, candidateIR, baselineExists)
	summary, outcome, risk := summarizePlan(changes)
	plan := DraftPlan{
		FormatVersion: DraftPlanFormatVersion, ProjectID: projectID, ModuleName: module, DraftID: id,
		DraftGeneration: expectedGeneration, ValidationID: validation.ID, BaselineRevision: validation.BaselineRevision,
		SourceFormat: validation.SourceFormat, SourceHash: validation.SourceHash, Candidate: *validation.Candidate,
		Changes: changes, RiskSummary: summary, Outcome: outcome, Risk: risk,
		DataSchemaChanged:           !baselineExists || baselineIdentity.Fingerprint() != validation.Candidate.DataSchemaFingerprint,
		RecordNamespaceChanged:      !baselineExists || validation.BaselineRevision != validation.Candidate.RevisionHash,
		MigrationExecutionSupported: false, CreatedBy: owner.ActorID(), CreatedAt: workflow.clock.Now().UTC(),
	}
	if baselineExists {
		plan.BaselineDataSchemaFormat = baselineIdentity.Format()
		plan.BaselineDataSchemaFingerprint = baselineIdentity.Fingerprint()
	}
	plan.PlanHash = planHashIdentity(plan)
	plan.ID = planSnapshotIdentity(plan)
	if err := ValidateDraftPlan(plan); err != nil {
		return DraftPlan{}, false, err
	}
	stored, created, err := workflow.store.SavePlan(ctx, plan, expectedGeneration)
	if err != nil {
		return DraftPlan{}, false, fmt.Errorf("save module draft plan: %w", err)
	}
	if err := ValidateDraftPlan(stored); err != nil {
		return DraftPlan{}, false, err
	}
	return clonePlan(stored), created, nil
}

// GetValidation returns one immutable snapshot and computes current staleness.
func (workflow *DraftWorkflow) GetValidation(ctx context.Context, projectID project.ID, owner actor.Context, module, idText, validationID string) (DraftValidation, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return DraftValidation{}, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil || !domain.ValidContentHash(strings.TrimSpace(validationID)) {
		return DraftValidation{}, fmt.Errorf("%w: invalid validation identity", ErrDraftInvalid)
	}
	module = strings.TrimSpace(module)
	value, err := workflow.store.GetValidation(ctx, projectID, module, id, strings.TrimSpace(validationID))
	if err != nil {
		return DraftValidation{}, err
	}
	if err := ValidateDraftValidation(value); err != nil {
		return DraftValidation{}, err
	}
	current, err := workflow.get(ctx, projectID, module, id)
	if err != nil {
		return DraftValidation{}, err
	}
	value.Stale = current.Generation() != value.Generation || current.SourceHash() != value.SourceHash
	return cloneValidation(value), nil
}

// GetPlan returns one immutable snapshot and computes current staleness.
func (workflow *DraftWorkflow) GetPlan(ctx context.Context, projectID project.ID, owner actor.Context, module, idText, planID string) (DraftPlan, error) {
	if err := authorizeDraft(projectID, owner); err != nil {
		return DraftPlan{}, err
	}
	id, err := parseDraftRequest(module, idText)
	if err != nil || !domain.ValidContentHash(strings.TrimSpace(planID)) {
		return DraftPlan{}, fmt.Errorf("%w: invalid plan identity", ErrDraftInvalid)
	}
	module = strings.TrimSpace(module)
	value, err := workflow.store.GetPlan(ctx, projectID, module, id, strings.TrimSpace(planID))
	if err != nil {
		return DraftPlan{}, err
	}
	if err := ValidateDraftPlan(value); err != nil {
		return DraftPlan{}, err
	}
	current, err := workflow.get(ctx, projectID, module, id)
	if err != nil {
		return DraftPlan{}, err
	}
	value.Stale = current.Generation() != value.DraftGeneration || current.SourceHash() != value.SourceHash
	return clonePlan(value), nil
}

func (workflow *DraftWorkflow) get(ctx context.Context, projectID project.ID, module string, id domain.DraftID) (domain.Draft, error) {
	value, err := workflow.store.Get(ctx, projectID, module, id)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("get module draft: %w", err)
	}
	if err := verifyDraftScope(value, projectID, module, id); err != nil {
		return domain.Draft{}, err
	}
	return value, nil
}

func authorizeDraft(projectID project.ID, owner actor.Context) error {
	if !projectID.Valid() {
		return fmt.Errorf("%w: invalid project", ErrDraftInvalid)
	}
	if !owner.Valid() || owner.Anonymous() || owner.ProjectID().String() != projectID.String() || !owner.HasRole("project.owner") {
		return ErrDraftForbidden
	}
	return nil
}

func parseDraftRequest(module, idText string) (domain.DraftID, error) {
	if !domain.ValidModuleName(strings.TrimSpace(module)) {
		return domain.DraftID{}, fmt.Errorf("%w: invalid module", ErrDraftInvalid)
	}
	id, err := domain.ParseDraftID(idText)
	if err != nil {
		return domain.DraftID{}, fmt.Errorf("%w: %v", ErrDraftInvalid, err)
	}
	return id, nil
}

func verifyDraftScope(value domain.Draft, projectID project.ID, module string, id domain.DraftID) error {
	if !value.ID().Valid() || value.ProjectID().String() != projectID.String() || value.ModuleName() != module || value.ID().String() != id.String() {
		return fmt.Errorf("%w: stored draft crossed requested scope", ErrDraftCorrupt)
	}
	return nil
}

func draftSpecFormat(format domain.SourceFormat) spec.Format {
	if format == domain.SourceFormatJSON {
		return spec.FormatJSON
	}
	return spec.FormatYAML
}

func validDraftGeneration(value uint64) bool {
	return value > 0 && value <= domain.MaxDraftGeneration
}
