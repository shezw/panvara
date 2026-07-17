/*
   Panvara
   internal/application/appmodule/compiler_test.go    2026-07-14
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
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestCompilerProducesStableCanonicalArtifacts(t *testing.T) {
	t.Parallel()

	compiler := NewCompiler()
	leftDocument := testDocument()
	rightDocument := testDocument()
	slices.Reverse(rightDocument.Spec.Resources)
	for index := range rightDocument.Spec.Resources {
		slices.Reverse(rightDocument.Spec.Resources[index].Fields)
		slices.Reverse(rightDocument.Spec.Resources[index].API.Admin.Operations)
		slices.Reverse(rightDocument.Spec.Resources[index].API.Admin.Writable)
	}
	slices.Reverse(rightDocument.Spec.Provides)

	left, err := compiler.CompileDocument(leftDocument)
	if err != nil {
		t.Fatal(err)
	}
	right, err := compiler.CompileDocument(rightDocument)
	if err != nil {
		t.Fatal(err)
	}
	if left.RevisionHash() != right.RevisionHash() {
		t.Fatalf("equivalent hashes differ: %q != %q", left.RevisionHash(), right.RevisionHash())
	}
	if !bytes.Equal(left.CanonicalIR(), right.CanonicalIR()) {
		t.Fatal("equivalent documents produced different canonical IR")
	}
	if !strings.HasPrefix(left.RevisionHash(), "sha256:") || len(left.RevisionHash()) != 71 {
		t.Fatalf("RevisionHash() = %q", left.RevisionHash())
	}
	if bytes.Contains(left.CanonicalIR(), []byte(`"MaxLength"`)) ||
		!bytes.Contains(left.CanonicalIR(), []byte(`"maxLength":320`)) {
		t.Fatalf("CanonicalIR() leaked or omitted constraint contract: %s", left.CanonicalIR())
	}
	golden, err := os.ReadFile("testdata/crm_leads.ir.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.CanonicalIR(), bytes.TrimSpace(golden)) {
		t.Fatalf("CanonicalIR() differs from golden\n got: %s\nwant: %s", left.CanonicalIR(), golden)
	}
}

func TestCompilerPreservesSemanticDisplayOrderInRevision(t *testing.T) {
	t.Parallel()

	compiler := NewCompiler()
	base := testDocument()
	compiledBase, err := compiler.CompileDocument(base)
	if err != nil {
		t.Fatal(err)
	}

	changedEnum := testDocument()
	changedEnum.Spec.Resources[1].Fields[2].Options = []string{"won", "qualified", "new"}
	compiledEnum, err := compiler.CompileDocument(changedEnum)
	if err != nil {
		t.Fatal(err)
	}
	if compiledBase.RevisionHash() == compiledEnum.RevisionHash() {
		t.Fatal("enum display order did not change revision")
	}

	changedUI := testDocument()
	changedUI.Spec.Resources[1].Manager.List.Columns = []string{"stage", "email", "id", "created_at"}
	compiledUI, err := compiler.CompileDocument(changedUI)
	if err != nil {
		t.Fatal(err)
	}
	if compiledBase.RevisionHash() == compiledUI.RevisionHash() {
		t.Fatal("Manager display order did not change revision")
	}
}

func TestCompilerTreatsEquivalentYAMLAndJSONAsTheSameRevision(t *testing.T) {
	t.Parallel()

	jsonSource := []byte(`{
  "apiVersion": "panvara.dev/v1alpha1",
  "kind": "AppModule",
  "metadata": {"name": "notes", "version": "1.0.0"},
  "spec": {"resources": []}
}`)
	yamlSource := []byte(`# an author comment does not enter canonical IR
apiVersion: panvara.dev/v1alpha1
kind: AppModule
metadata:
  version: 1.0.0
  name: notes
spec:
  resources: []
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
	if fromJSON.RevisionHash() != fromYAML.RevisionHash() ||
		!bytes.Equal(fromJSON.CanonicalIR(), fromYAML.CanonicalIR()) {
		t.Fatal("equivalent YAML and JSON produced different revisions")
	}
}

func TestCompilerRejectsEscapedNULInJSONLabel(t *testing.T) {
	t.Parallel()

	source := []byte(`{
  "apiVersion": "panvara.dev/v1alpha1",
  "kind": "AppModule",
  "metadata": {
    "name": "notes",
    "version": "1.0.0",
    "labels": {"en-US": "Notes\u0000Unsafe"}
  },
  "spec": {"resources": []}
}`)
	if _, err := NewCompiler().Compile(source, spec.FormatJSON); err == nil {
		t.Fatal("Compile() accepted a decoded NUL label")
	}
}

func TestCompilerGeneratesPolicyAlignedOpenAPIAndManagerSchema(t *testing.T) {
	t.Parallel()

	module, err := NewCompiler().CompileDocument(testDocument())
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactHashGolden(t, "testdata/crm_leads.openapi.sha256", module.OpenAPI())
	assertArtifactHashGolden(t, "testdata/crm_leads.manager.sha256", module.ManagerUISchema())
	var openAPI map[string]any
	if err := json.Unmarshal(module.OpenAPI(), &openAPI); err != nil {
		t.Fatal(err)
	}
	paths := openAPI["paths"].(map[string]any)
	publicPath := paths["/api/public/v1alpha1/crm.leads/lead"].(map[string]any)
	if _, secured := publicPath["post"].(map[string]any)["security"]; secured {
		t.Fatal("public operation unexpectedly requires bearer security")
	}
	adminItem := paths["/api/admin/v1alpha1/crm.leads/lead/{id}"].(map[string]any)
	patch := adminItem["patch"].(map[string]any)
	if _, secured := patch["security"]; !secured {
		t.Fatal("admin patch is missing bearer security")
	}
	parameters := patch["parameters"].([]any)
	if len(parameters) != 2 || parameters[1].(map[string]any)["name"] != "If-Match" {
		t.Fatalf("patch parameters = %#v", parameters)
	}

	var manager map[string]any
	if err := json.Unmarshal(module.ManagerUISchema(), &manager); err != nil {
		t.Fatal(err)
	}
	resources := manager["resources"].([]any)
	lead := resources[1].(map[string]any)
	columns := lead["list"].(map[string]any)["columns"].([]any)
	if columns[0].(map[string]any)["name"] != "id" || !columns[0].(map[string]any)["readOnly"].(bool) {
		t.Fatalf("first Manager column = %#v", columns[0])
	}
}

func TestCompiledModuleAccessorsAreDefensiveAndCatalogChecksDependencies(t *testing.T) {
	t.Parallel()

	module, err := NewCompiler().CompileDocument(testDocument())
	if err != nil {
		t.Fatal(err)
	}
	ir := module.CanonicalIR()
	ir[0] = '!'
	if module.CanonicalIR()[0] == '!' {
		t.Fatal("CanonicalIR() leaked mutable bytes")
	}
	descriptor := module.Descriptor()
	descriptor.Resources[0].Fields[0].Name = "mutated"
	if module.Descriptor().Resources[0].Fields[0].Name == "mutated" {
		t.Fatal("Descriptor() leaked mutable state")
	}
	if _, err := NewCatalog(module); err == nil {
		t.Fatal("NewCatalog() accepted a missing base module dependency")
	}
	baseDocument := spec.Document{
		APIVersion: spec.APIVersion, Kind: spec.Kind,
		Metadata: spec.Metadata{Name: "base", Version: "1.5.0"},
		Spec:     spec.Spec{},
	}
	base, err := NewCompiler().CompileDocument(baseDocument)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := NewCatalog(module, base)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := catalog.ModuleNames(), []string{"base", "crm.leads"}; !slices.Equal(got, want) {
		t.Fatalf("Catalog.ModuleNames() = %v, want %v", got, want)
	}
	if _, found := catalog.LookupResource("crm.leads", "lead"); !found {
		t.Fatal("Catalog.LookupResource() did not find lead")
	}
}

func testDocument() spec.Document {
	maxName := 128
	maxEmail := 320
	precision := 12
	scale := 4
	return spec.Document{
		APIVersion: spec.APIVersion,
		Kind:       spec.Kind,
		Metadata: spec.Metadata{
			Name: "crm.leads", Version: "1.2.3", Labels: map[string]string{"zh-CN": "销售线索", "en-US": "CRM Leads"},
		},
		Spec: spec.Spec{
			Requires: spec.Requirements{
				Modules:      []spec.ModuleRequirement{{Name: "base", Version: "^1.0.0"}},
				Capabilities: []string{"notification.email/v1alpha1"},
			},
			Provides: []string{"crm.records/v1alpha1", "crm.leads/v1alpha1"},
			Resources: []spec.Resource{
				{
					Name: "organization", Labels: map[string]string{"en-US": "Organization"},
					Fields: []spec.Field{{
						Name: "name", Type: "string", Required: true, Unique: true,
						Constraints: spec.Constraints{MaxLength: &maxName},
					}},
					API: spec.API{Admin: spec.Access{
						Operations: []string{"list", "get", "create", "patch", "delete"},
						Writable:   []string{"name"}, Filterable: []string{"name"},
					}},
					Manager: spec.Manager{
						List: spec.ListView{Columns: []string{"id", "name", "created_at"}, Filters: []string{"name"}},
						Form: spec.FormView{Fields: []string{"name"}},
					},
				},
				{
					Name: "lead", Labels: map[string]string{"en-US": "Lead"},
					Fields: []spec.Field{
						{Name: "organization", Type: "reference", Target: "organization"},
						{Name: "email", Type: "email", Required: true, Unique: true, Constraints: spec.Constraints{MaxLength: &maxEmail}},
						{Name: "stage", Type: "enum", Required: true, Options: []string{"new", "qualified", "won"}},
						{Name: "score", Type: "decimal", Constraints: spec.Constraints{Precision: &precision, Scale: &scale}},
					},
					API: spec.API{
						Public: spec.Access{
							Operations: []string{"create"},
							Writable:   []string{"email", "stage", "organization"},
						},
						Admin: spec.Access{
							Operations: []string{"list", "get", "create", "patch", "delete"},
							Writable:   []string{"email", "stage", "organization", "score"},
							Filterable: []string{"email", "stage"},
						},
					},
					Manager: spec.Manager{
						List: spec.ListView{Columns: []string{"id", "email", "stage", "created_at"}, Filters: []string{"stage"}},
						Form: spec.FormView{Fields: []string{"email", "stage", "organization", "score"}},
					},
				},
			},
		},
	}
}

func assertArtifactHashGolden(t *testing.T, path string, artifact []byte) {
	t.Helper()
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%x", sha256.Sum256(artifact))
	if got != strings.TrimSpace(string(want)) {
		t.Fatalf("artifact hash = %s, want %s from %s", got, want, path)
	}
}
