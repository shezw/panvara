/*
   Panvara
   internal/application/appmodule/draft_workflow_test.go    2026-07-16
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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestDraftWorkflowGoldenPathIdempotencyCASStalenessAndInvalidResult(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := appmoduleTestExecution(t, projectID, "owner", nil)
	store := newDraftTestStore()
	workflow := draftTestWorkflow(t, store, &draftTestRevisionReader{})
	source := draftTestSource("notes", "1.0.0", "")

	draft, created, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML, Source: source, IdempotencyKey: "create-1",
	})
	if err != nil || !created || draft.Generation() != 1 {
		t.Fatalf("Create() = %#v, %v, %v", draft, created, err)
	}
	replayed, created, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML, Source: source, IdempotencyKey: "create-1",
	})
	if err != nil || created || replayed.ID() != draft.ID() {
		t.Fatalf("replayed Create() = %#v, %v, %v", replayed, created, err)
	}
	if _, _, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML, Source: []byte("different"), IdempotencyKey: "create-1",
	}); !errors.Is(err, ErrDraftIdempotencyConflict) {
		t.Fatalf("conflicting Create() error = %v", err)
	}

	noOp, changed, err := workflow.Replace(context.Background(), execution, "notes", draft.ID().String(), 1,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: source})
	if err != nil || changed || noOp.Generation() != 1 || noOp.UpdatedAt() != draft.UpdatedAt() {
		t.Fatalf("no-op Replace() = generation %d changed %v error %v", noOp.Generation(), changed, err)
	}
	validation, created, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 1)
	if err != nil || !created || !validation.Valid || validation.Candidate == nil || validation.ID != validation.ValidationHash {
		t.Fatalf("Validate(valid) = %#v, %v, %v", validation, created, err)
	}
	replayedValidation, created, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 1)
	if err != nil || created || replayedValidation.ID != validation.ID {
		t.Fatalf("replayed Validate() = %#v, %v, %v", replayedValidation, created, err)
	}
	plan, created, err := workflow.Plan(context.Background(), execution, "notes", draft.ID().String(), validation.ID, 1)
	if err != nil || !created || plan.Outcome != "review_required" || plan.MigrationExecutionSupported || len(plan.Changes) != 1 || plan.Changes[0].Code != "module.baseline_absent" {
		t.Fatalf("Plan(initial) = %#v, %v, %v", plan, created, err)
	}
	replayedPlan, created, err := workflow.Plan(context.Background(), execution, "notes", draft.ID().String(), validation.ID, 1)
	if err != nil || created || replayedPlan.PlanHash != plan.PlanHash {
		t.Fatalf("replayed Plan() = %#v, %v, %v", replayedPlan, created, err)
	}
	equivalentSource := append([]byte("# same compiled semantics\n"), source...)
	equivalentDraft, changed, err := workflow.Replace(context.Background(), execution, "notes", draft.ID().String(), 1,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: equivalentSource})
	if err != nil || !changed || equivalentDraft.Generation() != 2 {
		t.Fatalf("Replace(equivalent source) = %#v, %v, %v", equivalentDraft, changed, err)
	}
	equivalentValidation, _, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 2)
	if err != nil || !equivalentValidation.Valid || equivalentValidation.ID == validation.ID {
		t.Fatalf("Validate(equivalent generation) = %#v, %v", equivalentValidation, err)
	}
	equivalentPlan, created, err := workflow.Plan(
		context.Background(), execution, "notes", draft.ID().String(), equivalentValidation.ID, 2,
	)
	if err != nil || !created || equivalentPlan.PlanHash != plan.PlanHash || equivalentPlan.ID == plan.ID {
		t.Fatalf("Plan(equivalent generation) = %#v, %v, %v; first = %#v", equivalentPlan, created, err, plan)
	}
	if replay, created, err := workflow.Plan(
		context.Background(), execution, "notes", draft.ID().String(), equivalentValidation.ID, 2,
	); err != nil || created || replay.ID != equivalentPlan.ID || replay.PlanHash != equivalentPlan.PlanHash {
		t.Fatalf("Plan(equivalent replay) = %#v, %v, %v", replay, created, err)
	}

	updated, changed, err := workflow.Replace(context.Background(), execution, "notes", draft.ID().String(), 2,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: nil})
	if err != nil || !changed || updated.Generation() != 3 {
		t.Fatalf("Replace(empty) = %#v, %v, %v", updated, changed, err)
	}
	stale, err := workflow.GetValidation(context.Background(), execution, "notes", draft.ID().String(), validation.ID)
	if err != nil || !stale.Stale {
		t.Fatalf("GetValidation(stale) = %#v, %v", stale, err)
	}
	if _, _, err := workflow.Plan(context.Background(), execution, "notes", draft.ID().String(), validation.ID, 1); !errors.Is(err, ErrDraftConflict) {
		t.Fatalf("Plan(stale generation) error = %v", err)
	}
	invalid, created, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 3)
	if err != nil || !created || invalid.Valid || len(invalid.Issues) != 1 || invalid.Candidate != nil {
		t.Fatalf("Validate(invalid) = %#v, %v, %v", invalid, created, err)
	}
	if _, _, err := workflow.Plan(context.Background(), execution, "notes", draft.ID().String(), invalid.ID, 3); !errors.Is(err, ErrValidationInvalid) {
		t.Fatalf("Plan(invalid validation) error = %v", err)
	}
}

func TestDraftWorkflowAuthorizesEveryUseCaseBeforeStoreAccess(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := appmoduleTestExecution(t, projectID, "self-reported-owner", []string{"project.owner"})
	store := newDraftTestStore()
	revisions := &draftTestRevisionReader{}
	denied := &recordingAuthorizer{err: access.ErrForbidden}
	workflow := draftTestWorkflowWithAuthorizer(t, store, revisions, denied)
	unknown := "sha256:" + strings.Repeat("f", 64)

	if _, _, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("Create() error = %v, want access.ErrForbidden", err)
	}
	if _, err := workflow.Get(context.Background(), execution, "notes", "invalid"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("Get() error = %v, want access.ErrForbidden", err)
	}
	if _, err := workflow.GetSource(context.Background(), execution, "notes", "invalid"); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("GetSource() error = %v, want access.ErrForbidden", err)
	}
	if _, _, err := workflow.Replace(context.Background(), execution, "notes", "invalid", 0, ReplaceDraftInput{}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("Replace() error = %v, want access.ErrForbidden", err)
	}
	if _, _, err := workflow.Validate(context.Background(), execution, "notes", "invalid", 0); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("Validate() error = %v, want access.ErrForbidden", err)
	}
	if _, _, err := workflow.Plan(context.Background(), execution, "notes", "invalid", unknown, 0); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("Plan() error = %v, want access.ErrForbidden", err)
	}
	if _, err := workflow.GetValidation(context.Background(), execution, "notes", "invalid", unknown); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("GetValidation() error = %v, want access.ErrForbidden", err)
	}
	if _, err := workflow.GetPlan(context.Background(), execution, "notes", "invalid", unknown); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("GetPlan() error = %v, want access.ErrForbidden", err)
	}
	wantOperations := []access.Operation{
		access.OperationDraftCreate,
		access.OperationDraftGet,
		access.OperationDraftGetSource,
		access.OperationDraftReplace,
		access.OperationDraftValidate,
		access.OperationDraftPlan,
		access.OperationDraftGetValidation,
		access.OperationDraftGetPlan,
	}
	if got := denied.Operations(); !equalOperations(got, wantOperations) {
		t.Fatalf("authorization operations = %#v, want %#v", got, wantOperations)
	}
	if calls := store.Calls(); calls != 0 {
		t.Fatalf("denied draft store calls = %d, want 0", calls)
	}
	if revisions.calls != 0 {
		t.Fatalf("denied draft revision calls = %d, want 0", revisions.calls)
	}
}

func TestNewDraftWorkflowRequiresAuthorizer(t *testing.T) {
	t.Parallel()
	_, err := NewDraftWorkflow(
		newDraftTestStore(),
		&draftTestRevisionReader{},
		nil,
		draftTestClock{at: time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)},
		&draftTestIDGenerator{},
	)
	if !errors.Is(err, ErrDraftInvalid) {
		t.Fatalf("NewDraftWorkflow(nil authorizer) error = %v, want ErrDraftInvalid", err)
	}
}

func TestDraftValidationTreatsEscapedNULLabelAsAuthorError(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := appmoduleTestExecution(t, projectID, "owner", nil)
	workflow := draftTestWorkflow(t, newDraftTestStore(), &draftTestRevisionReader{})
	source := []byte(`{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","metadata":{"name":"notes","version":"1.0.0","labels":{"en-US":"Bad\u0000Label"}},"spec":{"resources":[]}}`)
	draft, _, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatJSON, Source: source, IdempotencyKey: "escaped-nul",
	})
	if err != nil {
		t.Fatalf("Create(escaped NUL source) error = %v", err)
	}
	validation, created, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 1)
	if err != nil || !created || validation.Valid || len(validation.Issues) == 0 {
		t.Fatalf("Validate(escaped NUL label) = %#v, %v, %v", validation, created, err)
	}
}

func TestDraftWorkflowGenerationBoundaryPreventsBigintOverflow(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := appmoduleTestExecution(t, projectID, "owner", nil)
	id, _ := domain.ParseDraftID("01981234-5678-7abc-8def-0123456789b0")
	baseline, _ := domain.NewDraftBaseline("")
	source := draftTestSource("notes", "1.0.0", "")
	at := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	draft, err := domain.NewDraft(domain.DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: "notes", Baseline: baseline,
		SourceFormat: domain.SourceFormatYAML, SourceHash: domain.DraftSourceHash(source), Source: source,
		Generation: domain.MaxDraftGeneration, CreatedBy: "owner", CreatedAt: at, UpdatedBy: "owner", UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newDraftTestStore()
	store.drafts[draftTestKey(projectID, "notes", id)] = draft
	workflow := draftTestWorkflow(t, store, &draftTestRevisionReader{})
	if got, changed, err := workflow.Replace(context.Background(), execution, "notes", id.String(), domain.MaxDraftGeneration,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: source}); err != nil || changed || got.Generation() != domain.MaxDraftGeneration {
		t.Fatalf("Replace(max no-op) = %#v, %v, %v", got, changed, err)
	}
	if _, _, err := workflow.Replace(context.Background(), execution, "notes", id.String(), domain.MaxDraftGeneration,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: append(source, '#')}); !errors.Is(err, ErrDraftConflict) {
		t.Fatalf("Replace(max change) error = %v", err)
	}
	if _, _, err := workflow.Replace(context.Background(), execution, "notes", id.String(), domain.MaxDraftGeneration+1,
		ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: source}); !errors.Is(err, ErrDraftInvalid) {
		t.Fatalf("Replace(overflow ETag) error = %v", err)
	}
}

func TestDraftWorkflowFixesAndReverifiesExactBaseline(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := appmoduleTestExecution(t, projectID, "owner", nil)
	source := draftTestSource("notes", "1.0.0", "")
	baseline := draftTestRevision(t, projectID, source)
	reader := &draftTestRevisionReader{revision: baseline}
	workflow := draftTestWorkflow(t, newDraftTestStore(), reader)
	draft, created, err := workflow.Create(context.Background(), execution, "notes", CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domain.SourceFormatYAML,
		Source: source, IdempotencyKey: "baseline-create",
	})
	if err != nil || !created || draft.Baseline().RevisionHash() != baseline.RevisionHash() {
		t.Fatalf("Create(baseline) = %#v, %v, %v", draft, created, err)
	}
	validation, _, err := workflow.Validate(context.Background(), execution, "notes", draft.ID().String(), 1)
	if err != nil || !validation.Valid {
		t.Fatalf("Validate(baseline) = %#v, %v", validation, err)
	}
	plan, _, err := workflow.Plan(context.Background(), execution, "notes", draft.ID().String(), validation.ID, 1)
	if err != nil || len(plan.Changes) != 0 || plan.DataSchemaChanged || plan.RecordNamespaceChanged || plan.Risk != "none" {
		t.Fatalf("Plan(unchanged baseline) = %#v, %v", plan, err)
	}
	if reader.calls != 3 {
		t.Fatalf("baseline verification calls = %d, want create+validate+plan", reader.calls)
	}
}

func TestPlanHashExcludesWorkflowMetadataAndChangeMatrixIsDirectional(t *testing.T) {
	t.Parallel()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	draftID, _ := domain.ParseDraftID("01981234-5678-7abc-8def-0123456789ac")
	candidate := CandidateIdentity{
		RevisionHash: "sha256:" + strings.Repeat("a", 64), ModuleVersion: "2.0.0",
		DataSchemaFormat: 1, DataSchemaFingerprint: "sha256:" + strings.Repeat("b", 64),
	}
	plan := DraftPlan{
		FormatVersion: 1, ProjectID: projectID, ModuleName: "notes", DraftID: draftID, DraftGeneration: 7,
		ValidationID: "sha256:" + strings.Repeat("c", 64), SourceFormat: domain.SourceFormatYAML,
		SourceHash: "sha256:" + strings.Repeat("d", 64), Candidate: candidate,
		Changes:     []PlanChange{{Code: "module.baseline_absent", Path: "/baseline", Kind: "change", Risk: "review", Impact: "baseline absent", RequiresMigration: false}},
		RiskSummary: PlanRiskSummary{Review: 1}, Outcome: "review_required", Risk: "medium",
		DataSchemaChanged: true, RecordNamespaceChanged: true,
	}
	first := planHashIdentity(plan)
	plan.ProjectID = draftTestProject(t, "01981234-5678-7abc-8def-0123456789ad")
	plan.DraftGeneration = 42
	plan.ValidationID = "sha256:" + strings.Repeat("e", 64)
	plan.SourceFormat = domain.SourceFormatJSON
	plan.SourceHash = "sha256:" + strings.Repeat("f", 64)
	plan.CreatedBy = "another"
	if second := planHashIdentity(plan); second != first {
		t.Fatalf("plan hash changed with non-semantic metadata: %s != %s", second, first)
	}

	before := moduleIR{FormatVersion: 1, Name: "notes", Version: "1.0.0", Resources: []resourceIR{{
		Name: "note", Fields: []fieldIR{{Name: "state", Kind: domain.KindEnum, Required: true, Unique: true, Options: []string{"new", "done"}, Constraints: constraintsIR{MaxLength: intPointer(20)}}},
	}}}
	after := before
	after.Resources = []resourceIR{{Name: "note", Fields: []fieldIR{
		{Name: "state", Kind: domain.KindEnum, Required: false, Unique: false, Options: []string{"new", "done", "archived"}, Constraints: constraintsIR{MaxLength: intPointer(30)}},
		{Name: "summary", Kind: domain.KindString},
	}}}
	changes := buildPlanChanges(before, after, true)
	for _, change := range changes {
		if change.RequiresMigration {
			t.Fatalf("relaxing/additive change %#v unexpectedly requires migration", change)
		}
	}
	after.Resources[0].Fields[0].Options = []string{"new"}
	after.Resources[0].Fields[0].Constraints.MaxLength = intPointer(10)
	changes = buildPlanChanges(before, after, true)
	if !containsMigrationChange(changes, "field.options_changed") || !containsMigrationChange(changes, "field.constraints_changed") {
		t.Fatalf("tightening changes = %#v", changes)
	}
}

func TestDraftUUIDv7Generator(t *testing.T) {
	t.Parallel()
	generator, err := NewDraftUUIDv7Generator(strings.NewReader(strings.Repeat("x", 10)))
	if err != nil {
		t.Fatal(err)
	}
	id, err := generator.New(time.UnixMilli(1_720_000_000_000))
	if err != nil || !id.Valid() || id.String()[14] != '7' || !strings.Contains("89ab", string(id.String()[19])) {
		t.Fatalf("New() = %q, %v", id.String(), err)
	}
}

func TestCandidateDataSchemaFingerprintUsesCanonicalEnumOptionOrder(t *testing.T) {
	t.Parallel()
	source := []byte(`apiVersion: panvara.dev/v1alpha1
kind: AppModule
metadata:
  name: notes
  version: 1.0.0
spec:
  resources:
    - name: note
      fields:
        - name: state
          type: enum
          options: [won, new, contacted]
`)
	compiled, err := NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	ir, err := decodeCanonicalModuleIR(compiled.CanonicalIR())
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := dataSchemaFingerprintFromModuleIR(ir)
	if err != nil || fingerprint != compiled.DataSchemaFingerprint() {
		t.Fatalf("data schema fingerprint = %q, %v; want %q", fingerprint, err, compiled.DataSchemaFingerprint())
	}
}

func containsMigrationChange(changes []PlanChange, code string) bool {
	for _, change := range changes {
		if change.Code == code && change.RequiresMigration {
			return true
		}
	}
	return false
}

func intPointer(value int) *int { return &value }

type draftTestClock struct{ at time.Time }

func (clock draftTestClock) Now() time.Time { return clock.at }

type draftTestIDGenerator struct{ next int }

func (generator *draftTestIDGenerator) New(time.Time) (domain.DraftID, error) {
	values := []string{
		"01981234-5678-7abc-8def-0123456789b0", "01981234-5678-7abc-8def-0123456789b1",
		"01981234-5678-7abc-8def-0123456789b2", "01981234-5678-7abc-8def-0123456789b3",
	}
	value := values[generator.next%len(values)]
	generator.next++
	return domain.ParseDraftID(value)
}

type draftTestIdempotency struct {
	intent string
	draft  domain.Draft
}

type draftTestStore struct {
	drafts      map[string]domain.Draft
	idempotency map[string]draftTestIdempotency
	validations map[string]DraftValidation
	plans       map[string]DraftPlan
	calls       int
}

func newDraftTestStore() *draftTestStore {
	return &draftTestStore{drafts: map[string]domain.Draft{}, idempotency: map[string]draftTestIdempotency{},
		validations: map[string]DraftValidation{}, plans: map[string]DraftPlan{}}
}

func (store *draftTestStore) Create(_ context.Context, value domain.Draft, key, intent string) (domain.Draft, bool, error) {
	store.calls++
	scope := value.ProjectID().String() + "/" + value.ModuleName() + "/" + key
	if existing, found := store.idempotency[scope]; found {
		if existing.intent != intent {
			return domain.Draft{}, false, ErrDraftIdempotencyConflict
		}
		return existing.draft, false, nil
	}
	store.drafts[draftTestKey(value.ProjectID(), value.ModuleName(), value.ID())] = value
	store.idempotency[scope] = draftTestIdempotency{intent: intent, draft: value}
	return value, true, nil
}

func (store *draftTestStore) Get(_ context.Context, projectID project.ID, module string, id domain.DraftID) (domain.Draft, error) {
	store.calls++
	value, found := store.drafts[draftTestKey(projectID, module, id)]
	if !found {
		return domain.Draft{}, ErrDraftNotFound
	}
	return value, nil
}

func (store *draftTestStore) Replace(_ context.Context, projectID project.ID, module string, id domain.DraftID, expected uint64, replacement domain.DraftReplacement) (domain.Draft, bool, error) {
	store.calls++
	current, err := store.Get(context.Background(), projectID, module, id)
	if err != nil {
		return domain.Draft{}, false, err
	}
	if current.Generation() != expected {
		return domain.Draft{}, false, ErrDraftConflict
	}
	if current.SameSource(replacement) {
		return current, false, nil
	}
	updated, err := domain.NewDraft(domain.DraftMaterial{
		ID: current.ID(), ProjectID: current.ProjectID(), ModuleName: current.ModuleName(), Baseline: current.Baseline(),
		SourceFormat: replacement.Format(), SourceHash: replacement.SourceHash(), Source: replacement.Source(),
		Generation: current.Generation() + 1, CreatedBy: current.CreatedBy(), CreatedAt: current.CreatedAt(),
		UpdatedBy: replacement.ActorID(), UpdatedAt: replacement.At(),
	})
	if err != nil {
		return domain.Draft{}, false, err
	}
	store.drafts[draftTestKey(projectID, module, id)] = updated
	return updated, true, nil
}

func (store *draftTestStore) SaveValidation(_ context.Context, value DraftValidation) (DraftValidation, bool, error) {
	store.calls++
	current, err := store.Get(context.Background(), value.ProjectID, value.ModuleName, value.DraftID)
	if err != nil {
		return DraftValidation{}, false, err
	}
	if current.Generation() != value.Generation || current.SourceHash() != value.SourceHash {
		return DraftValidation{}, false, ErrDraftConflict
	}
	key := draftTestKey(value.ProjectID, value.ModuleName, value.DraftID) + "/validation/" + value.ID
	if existing, found := store.validations[key]; found {
		return cloneValidation(existing), false, nil
	}
	store.validations[key] = cloneValidation(value)
	return cloneValidation(value), true, nil
}

func (store *draftTestStore) GetValidation(_ context.Context, projectID project.ID, module string, id domain.DraftID, validationID string) (DraftValidation, error) {
	store.calls++
	value, found := store.validations[draftTestKey(projectID, module, id)+"/validation/"+validationID]
	if !found {
		return DraftValidation{}, ErrValidationNotFound
	}
	return cloneValidation(value), nil
}

func (store *draftTestStore) SavePlan(_ context.Context, value DraftPlan, expected uint64) (DraftPlan, bool, error) {
	store.calls++
	current, err := store.Get(context.Background(), value.ProjectID, value.ModuleName, value.DraftID)
	if err != nil {
		return DraftPlan{}, false, err
	}
	if current.Generation() != expected || current.SourceHash() != value.SourceHash {
		return DraftPlan{}, false, ErrDraftConflict
	}
	key := draftTestKey(value.ProjectID, value.ModuleName, value.DraftID) + "/plan/" + value.ID
	if existing, found := store.plans[key]; found {
		return clonePlan(existing), false, nil
	}
	store.plans[key] = clonePlan(value)
	return clonePlan(value), true, nil
}

func (store *draftTestStore) GetPlan(_ context.Context, projectID project.ID, module string, id domain.DraftID, planID string) (DraftPlan, error) {
	store.calls++
	value, found := store.plans[draftTestKey(projectID, module, id)+"/plan/"+planID]
	if !found {
		return DraftPlan{}, ErrPlanNotFound
	}
	return clonePlan(value), nil
}

func (store *draftTestStore) Calls() int { return store.calls }

type draftTestRevisionReader struct {
	revision domain.Revision
	calls    int
}

func (reader *draftTestRevisionReader) Get(_ context.Context, execution access.Execution, module, revision string) (domain.Revision, error) {
	reader.calls++
	projectID := execution.Scope().ProjectID()
	if reader.revision.ProjectID().String() != projectID.String() || reader.revision.ModuleName() != module || reader.revision.RevisionHash() != revision {
		return domain.Revision{}, ErrRevisionNotFound
	}
	return reader.revision, nil
}

func draftTestWorkflow(t *testing.T, store DraftStore, revisions DraftRevisionReader) *DraftWorkflow {
	t.Helper()
	return draftTestWorkflowWithAuthorizer(t, store, revisions, &recordingAuthorizer{})
}

func draftTestWorkflowWithAuthorizer(
	t *testing.T,
	store DraftStore,
	revisions DraftRevisionReader,
	authorizer access.Authorizer,
) *DraftWorkflow {
	t.Helper()
	value, err := NewDraftWorkflow(store, revisions, authorizer,
		draftTestClock{at: time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)}, &draftTestIDGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func draftTestProject(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func draftTestOwner(t *testing.T, projectID project.ID, id string) actor.Context {
	t.Helper()
	return draftTestActor(t, projectID, id, []string{"project.owner"})
}

func draftTestActor(t *testing.T, projectID project.ID, id string, roles []string) actor.Context {
	t.Helper()
	value, err := actor.New(projectID.String(), id, roles)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func draftTestKey(projectID project.ID, module string, id domain.DraftID) string {
	return projectID.String() + "/" + module + "/" + id.String()
}

func draftTestSource(module, version, resources string) []byte {
	if resources == "" {
		resources = "[]"
	}
	return []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: " + module +
		"\n  version: " + version + "\nspec:\n  resources: " + resources + "\n")
}

func draftTestRevision(t *testing.T, projectID project.ID, source []byte) domain.Revision {
	t.Helper()
	compiled, err := NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domain.NewDataSchemaIdentity(compiled.DataSchemaFormat(), compiled.DataSchemaFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	value, err := domain.NewRevision(domain.RevisionMaterial{
		ProjectID: projectID, ModuleName: compiled.Name(), ModuleVersion: compiled.Version(),
		RevisionHash: compiled.RevisionHash(), DataSchemaIdentities: []domain.DataSchemaIdentity{identity},
		SpecVersion: spec.APIVersion, IRFormat: IRFormatVersion, SourceFormat: domain.SourceFormatYAML,
		SourceHash: domain.DraftSourceHash(source), Source: source, CanonicalIR: compiled.CanonicalIR(),
		OpenAPI: compiled.OpenAPI(), ManagerSchema: compiled.ManagerUISchema(),
		Origin: domain.RevisionOriginBootstrap, RegisteredBy: "system:bootstrap",
		RegisteredAt: time.Date(2026, 7, 16, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
