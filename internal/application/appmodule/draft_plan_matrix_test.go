/*
   Panvara
   internal/application/appmodule/draft_plan_matrix_test.go    2026-07-16
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
	"reflect"
	"testing"
	"time"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

type planChangeExpectation struct {
	code      string
	risk      string
	migration bool
}

func TestBuildPlanChangesDecisionMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*moduleIR)
		want       []planChangeExpectation
		wantImpact string
	}{
		{
			name: "resource add",
			mutate: func(value *moduleIR) {
				value.Resources = append(value.Resources, resourceIR{Name: "folder"})
			},
			want: []planChangeExpectation{{"resource.added", "review", true}},
		},
		{
			name: "resource remove",
			mutate: func(value *moduleIR) {
				value.Resources = nil
			},
			want: []planChangeExpectation{{"resource.removed", "destructive", true}},
		},
		{
			name: "resource rename is remove and add",
			mutate: func(value *moduleIR) {
				value.Resources[0].Name = "entry"
			},
			want: []planChangeExpectation{
				{"resource.added", "review", true},
				{"resource.removed", "destructive", true},
			},
		},
		{
			name: "optional field add",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields = append(value.Resources[0].Fields, fieldIR{Name: "summary", Kind: domain.KindString})
			},
			want: []planChangeExpectation{{"field.added", "low", false}},
		},
		{
			name: "required field add",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields = append(value.Resources[0].Fields, fieldIR{Name: "summary", Kind: domain.KindString, Required: true})
			},
			want: []planChangeExpectation{{"field.added", "review", true}},
		},
		{
			name: "field remove",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields = value.Resources[0].Fields[1:]
			},
			want: []planChangeExpectation{{"field.removed", "destructive", true}},
		},
		{
			name: "field rename is remove and add",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Name = "headline"
			},
			want: []planChangeExpectation{
				{"field.added", "review", true},
				{"field.removed", "destructive", true},
			},
		},
		{
			name: "field kind",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Kind = domain.KindText
			},
			want: []planChangeExpectation{{"field.kind_changed", "destructive", true}},
		},
		{
			name: "reference target",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[2].Target = "folder"
			},
			want: []planChangeExpectation{{"field.target_changed", "destructive", true}},
		},
		{
			name: "required enabled",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[1].Required = true
			},
			want: []planChangeExpectation{{"field.required_changed", "review", true}},
		},
		{
			name: "required disabled",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Required = false
			},
			want: []planChangeExpectation{{"field.required_changed", "low", false}},
		},
		{
			name: "unique enabled",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Unique = true
			},
			want: []planChangeExpectation{{"field.unique_changed", "review", true}},
		},
		{
			name: "unique disabled",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[1].Unique = false
			},
			want: []planChangeExpectation{{"field.unique_changed", "low", false}},
		},
		{
			name: "enum expand",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[1].Options = append(value.Resources[0].Fields[1].Options, "archived")
			},
			want: []planChangeExpectation{{"field.options_changed", "low", false}},
		},
		{
			name: "enum contract",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[1].Options = []string{"new"}
			},
			want: []planChangeExpectation{{"field.options_changed", "destructive", true}},
		},
		{
			name: "enum display order",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[1].Options = []string{"done", "new"}
			},
			want:       []planChangeExpectation{{"field.options_changed", "low", false}},
			wantImpact: "enum option display order changes",
		},
		{
			name: "constraint tighten",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Constraints.MaxLength = planMatrixInt(40)
			},
			want: []planChangeExpectation{{"field.constraints_changed", "review", true}},
		},
		{
			name: "constraint relax",
			mutate: func(value *moduleIR) {
				value.Resources[0].Fields[0].Constraints.MaxLength = planMatrixInt(200)
			},
			want: []planChangeExpectation{{"field.constraints_changed", "low", false}},
		},
		{
			name: "api",
			mutate: func(value *moduleIR) {
				value.Resources[0].API.Admin.Operations = append(value.Resources[0].API.Admin.Operations, domain.OperationDelete)
			},
			want: []planChangeExpectation{{"resource.api_changed", "review", false}},
		},
		{
			name: "manager",
			mutate: func(value *moduleIR) {
				value.Resources[0].Manager.List.Columns = []string{"state", "title"}
			},
			want: []planChangeExpectation{{"resource.manager_changed", "review", false}},
		},
		{
			name: "requires",
			mutate: func(value *moduleIR) {
				value.Requires.Capabilities = append(value.Requires.Capabilities, "search.query/v1")
			},
			want: []planChangeExpectation{{"module.requires_changed", "review", false}},
		},
		{
			name: "provides",
			mutate: func(value *moduleIR) {
				value.Provides = append(value.Provides, "notes.export/v1")
			},
			want: []planChangeExpectation{{"module.provides_changed", "review", false}},
		},
		{
			name: "conflicts",
			mutate: func(value *moduleIR) {
				value.Conflicts = append(value.Conflicts, "legacy.notes")
			},
			want: []planChangeExpectation{{"module.conflicts_changed", "review", false}},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			before := planMatrixModuleIR()
			after := planMatrixModuleIR()
			test.mutate(&after)
			changes := buildPlanChanges(before, after, true)
			got := make([]planChangeExpectation, 0, len(changes))
			for _, change := range changes {
				got = append(got, planChangeExpectation{change.Code, change.Risk, change.RequiresMigration})
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("buildPlanChanges() = %#v, want %#v; full changes=%#v", got, test.want, changes)
			}
			if test.wantImpact != "" && (len(changes) != 1 || changes[0].Impact != test.wantImpact) {
				t.Fatalf("change impact = %#v, want %q", changes, test.wantImpact)
			}
		})
	}
}

func TestValidateDraftPlanRejectsUnknownChangeClassification(t *testing.T) {
	t.Parallel()

	valid := planMatrixValidSnapshot(t)
	if err := ValidateDraftPlan(valid); err != nil {
		t.Fatalf("ValidateDraftPlan(valid) error = %v", err)
	}
	for _, mutation := range []func(*DraftPlan){
		func(value *DraftPlan) { value.Changes[0].Kind = "execute" },
		func(value *DraftPlan) { value.Changes[0].Risk = "critical" },
	} {
		corrupt := valid
		corrupt.Changes = append([]PlanChange(nil), valid.Changes...)
		mutation(&corrupt)
		corrupt.RiskSummary, corrupt.Outcome, corrupt.Risk = summarizePlan(corrupt.Changes)
		corrupt.PlanHash = planHashIdentity(corrupt)
		corrupt.ID = planSnapshotIdentity(corrupt)
		if err := ValidateDraftPlan(corrupt); err == nil {
			t.Fatalf("ValidateDraftPlan() accepted unknown classification: %#v", corrupt.Changes[0])
		}
	}
}

func TestBuildPlanChangesSortsByPathAndCode(t *testing.T) {
	t.Parallel()

	before := planMatrixModuleIR()
	after := planMatrixModuleIR()
	after.Version = "2.0.0"
	after.Provides = append(after.Provides, "notes.export/v1")
	after.Resources[0].API.Admin.Operations = append(after.Resources[0].API.Admin.Operations, domain.OperationDelete)
	after.Resources[0].Fields[0].Kind = domain.KindText
	after.Resources[0].Fields[0].Required = false
	after.Resources[0].Fields = append(after.Resources[0].Fields, fieldIR{Name: "abstract", Kind: domain.KindText})

	changes := buildPlanChanges(before, after, true)
	for index := 1; index < len(changes); index++ {
		previous, current := changes[index-1], changes[index]
		if previous.Path > current.Path || (previous.Path == current.Path && previous.Code > current.Code) {
			t.Fatalf("changes are not sorted at %d: %#v then %#v", index, previous, current)
		}
	}
}

func TestBuildPlanChangesWithoutBaselineFailsClosed(t *testing.T) {
	t.Parallel()

	changes := buildPlanChanges(moduleIR{}, planMatrixModuleIR(), false)
	if len(changes) < 2 || changes[0].Code != "module.baseline_absent" || changes[0].Risk != "review" ||
		changes[0].RequiresMigration || changes[1].Code != "resource.added" || changes[1].Risk != "review" ||
		changes[1].RequiresMigration {
		t.Fatalf("no-baseline changes = %#v", changes)
	}
	_, outcome, risk := summarizePlan(changes)
	if outcome != "review_required" || risk != "medium" {
		t.Fatalf("no-baseline summary = %s/%s, want review_required/medium", outcome, risk)
	}
}

func TestPlanHashIsStableAcrossSourceFormatAndDeclarationOrder(t *testing.T) {
	t.Parallel()

	jsonSource := []byte(`{
  "apiVersion": "panvara.dev/v1alpha1",
  "kind": "AppModule",
  "metadata": {"name": "notes", "version": "1.0.0"},
  "spec": {"resources": [
    {"name": "folder", "fields": [{"name": "name", "type": "string"}]},
    {"name": "note", "fields": [
      {"name": "state", "type": "enum", "options": ["new", "done"]},
      {"name": "title", "type": "string", "required": true}
    ]}
  ]}
}`)
	yamlSource := []byte(`# syntax, comments, and declaration order are not plan semantics
kind: AppModule
apiVersion: panvara.dev/v1alpha1
metadata:
  version: 1.0.0
  name: notes
spec:
  resources:
    - name: note
      fields:
        - name: title
          required: true
          type: string
        - options: [new, done]
          type: enum
          name: state
    - fields:
        - type: string
          name: name
      name: folder
`)
	compiler := NewCompiler()
	fromJSON, err := compiler.Compile(jsonSource, spec.FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	fromYAML, err := compiler.Compile(yamlSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	if fromJSON.RevisionHash() != fromYAML.RevisionHash() {
		t.Fatalf("equivalent source revisions differ: %s != %s", fromJSON.RevisionHash(), fromYAML.RevisionHash())
	}
	jsonPlan := planMatrixInitialPlan(t, fromJSON)
	yamlPlan := planMatrixInitialPlan(t, fromYAML)
	jsonHash, yamlHash := planHashIdentity(jsonPlan), planHashIdentity(yamlPlan)
	if jsonHash != yamlHash {
		t.Fatalf("equivalent source plan hashes differ: %s != %s", jsonHash, yamlHash)
	}
	const golden = "sha256:e554c33785a96c1edd603ddc2c18e3f5b02dafda761cc8af71b3df5432bbcd1b"
	if jsonHash != golden {
		t.Fatalf("plan hash = %q, want golden %q", jsonHash, golden)
	}
}

func planMatrixModuleIR() moduleIR {
	return moduleIR{
		FormatVersion: IRFormatVersion,
		SpecVersion:   spec.APIVersion,
		Name:          "notes",
		Version:       "1.0.0",
		Labels:        map[string]string{"en-US": "Notes"},
		Requires: requirementsIR{
			Modules:      []moduleRequirementIR{},
			Capabilities: []string{"identity.user/v1"},
		},
		Provides:  []string{"notes.records/v1"},
		Conflicts: []string{},
		Resources: []resourceIR{{
			Name:   "note",
			Labels: map[string]string{"en-US": "Note"},
			Fields: []fieldIR{
				{Name: "title", Labels: map[string]string{}, Kind: domain.KindString, Required: true, Options: []string{}, Constraints: constraintsIR{MaxLength: planMatrixInt(100)}},
				{Name: "state", Labels: map[string]string{}, Kind: domain.KindEnum, Unique: true, Options: []string{"new", "done"}},
				{Name: "parent", Labels: map[string]string{}, Kind: domain.KindReference, Target: "note", Options: []string{}},
			},
			API: apiIR{Admin: accessIR{
				Operations: []domain.Operation{domain.OperationList, domain.OperationGet},
				Writable:   []string{"title", "state", "parent"},
				Filterable: []string{"state"},
				Sortable:   []string{"title"},
			}},
			Manager: managerIR{
				List: listViewIR{Columns: []string{"title", "state"}, Filters: []string{"state"}},
				Form: formViewIR{Fields: []string{"title", "state", "parent"}},
			},
		}},
	}
}

func planMatrixInitialPlan(t *testing.T, compiled *CompiledModule) DraftPlan {
	t.Helper()
	ir, err := decodeCanonicalModuleIR(compiled.CanonicalIR())
	if err != nil {
		t.Fatal(err)
	}
	changes := buildPlanChanges(moduleIR{}, ir, false)
	summary, outcome, risk := summarizePlan(changes)
	return DraftPlan{
		FormatVersion: DraftPlanFormatVersion,
		ModuleName:    compiled.Name(),
		Candidate: CandidateIdentity{
			RevisionHash:          compiled.RevisionHash(),
			ModuleVersion:         compiled.Version(),
			DataSchemaFormat:      compiled.DataSchemaFormat(),
			DataSchemaFingerprint: compiled.DataSchemaFingerprint(),
		},
		Changes: changes, RiskSummary: summary, Outcome: outcome, Risk: risk,
		DataSchemaChanged: true, RecordNamespaceChanged: true,
	}
}

func planMatrixValidSnapshot(t *testing.T) DraftPlan {
	t.Helper()
	projectID := draftTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	draftID, err := domain.ParseDraftID("01981234-5678-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	plan := DraftPlan{
		FormatVersion:   DraftPlanFormatVersion,
		ProjectID:       projectID,
		ModuleName:      "notes",
		DraftID:         draftID,
		DraftGeneration: 1,
		ValidationID:    "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		SourceFormat:    domain.SourceFormatYAML,
		SourceHash:      "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		Candidate: CandidateIdentity{
			RevisionHash:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ModuleVersion:         "1.0.0",
			DataSchemaFormat:      DataSchemaFormatVersion,
			DataSchemaFingerprint: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Changes:                []PlanChange{{Code: "module.baseline_absent", Path: "/baseline", Kind: "change", Risk: "review", Impact: "baseline absent", RequiresMigration: false}},
		RiskSummary:            PlanRiskSummary{Review: 1},
		Outcome:                "review_required",
		Risk:                   "medium",
		DataSchemaChanged:      true,
		RecordNamespaceChanged: true,
		CreatedBy:              "owner",
		CreatedAt:              time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC),
	}
	plan.PlanHash = planHashIdentity(plan)
	plan.ID = planSnapshotIdentity(plan)
	return plan
}

func planMatrixInt(value int) *int { return &value }
