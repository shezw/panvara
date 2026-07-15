/*
   Panvara
   internal/application/appmodule/data_schema_test.go    2026-07-15
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
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestDataSchemaProjectionV1GoldenAndFingerprint(t *testing.T) {
	t.Parallel()

	projection, err := marshalDataSchemaProjection(convertDocument(testDocument()))
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/data_schema_v1.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(golden) {
		t.Fatal("data schema v1 golden is not strict JSON")
	}
	wantProjection := bytes.TrimSpace(golden)
	if !bytes.Equal(projection, wantProjection) {
		t.Fatalf("data schema projection differs from v1 golden\n got: %s\nwant: %s", projection, wantProjection)
	}

	const wantFingerprint = "sha256:ae81fd7f5db8dbe6d7fbe0dfd57171bdff4e0d8158ddbde3e5e415a63e121d30"
	if got := fingerprintDataSchema(projection); got != wantFingerprint {
		t.Fatalf("fingerprintDataSchema() = %q, want %q", got, wantFingerprint)
	}
	module, err := NewCompiler().CompileDocument(testDocument())
	if err != nil {
		t.Fatal(err)
	}
	if module.DataSchemaFormat() != DataSchemaFormatVersion {
		t.Fatalf("DataSchemaFormat() = %d, want %d", module.DataSchemaFormat(), DataSchemaFormatVersion)
	}
	if got := module.DataSchemaFingerprint(); got != wantFingerprint {
		t.Fatalf("DataSchemaFingerprint() = %q, want %q", got, wantFingerprint)
	}
}

func TestDataSchemaFingerprintExcludesNonDataModelChanges(t *testing.T) {
	t.Parallel()
	base := compileMutatedTestDocument(t, testDocumentMutation{})

	documents := []struct {
		name   string
		change func(*testDocumentMutation)
	}{
		{name: "semantic version", change: func(value *testDocumentMutation) { value.version = "2.0.0" }},
		{name: "labels", change: func(value *testDocumentMutation) { value.label = "Updated" }},
		{name: "manager", change: func(value *testDocumentMutation) { value.managerFirst = "stage" }},
		{name: "api", change: func(value *testDocumentMutation) { value.removeDelete = true }},
		{name: "capability", change: func(value *testDocumentMutation) { value.capability = "notification.sms/v1alpha1" }},
		{name: "dependency", change: func(value *testDocumentMutation) { value.dependencyRange = "^2.0.0" }},
	}
	for _, test := range documents {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mutation := testDocumentMutation{}
			test.change(&mutation)
			changed := compileMutatedTestDocument(t, mutation)
			if changed.RevisionHash() == base.RevisionHash() {
				t.Fatalf("RevisionHash() did not change for %s", test.name)
			}
			if changed.DataSchemaFingerprint() != base.DataSchemaFingerprint() {
				t.Fatalf("DataSchemaFingerprint() changed for %s", test.name)
			}
		})
	}
	if base.DataSchemaFormat() != DataSchemaFormatVersion ||
		!strings.HasPrefix(base.DataSchemaFingerprint(), "sha256:") ||
		len(base.DataSchemaFingerprint()) != 71 {
		t.Fatalf("data schema identity = format %d fingerprint %q", base.DataSchemaFormat(), base.DataSchemaFingerprint())
	}
}

func TestDataSchemaFingerprintChangesWithStructuralFields(t *testing.T) {
	t.Parallel()
	base := compileMutatedTestDocument(t, testDocumentMutation{})
	tests := []struct {
		name   string
		change func(*testDocumentMutation)
	}{
		{name: "field type", change: func(value *testDocumentMutation) { value.fieldType = "email" }},
		{name: "required", change: func(value *testDocumentMutation) { value.notRequired = true }},
		{name: "unique", change: func(value *testDocumentMutation) { value.notUnique = true }},
		{name: "reference target", change: func(value *testDocumentMutation) { value.referenceTarget = "lead" }},
		{name: "enum candidate", change: func(value *testDocumentMutation) { value.enumOption = "lost" }},
		{name: "max length", change: func(value *testDocumentMutation) { value.maxLength = 319 }},
		{name: "precision", change: func(value *testDocumentMutation) { value.precision = 13 }},
		{name: "scale", change: func(value *testDocumentMutation) { value.scale = 3 }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mutation := testDocumentMutation{}
			test.change(&mutation)
			changed := compileMutatedTestDocument(t, mutation)
			if changed.DataSchemaFingerprint() == base.DataSchemaFingerprint() {
				t.Fatalf("DataSchemaFingerprint() did not change for %s", test.name)
			}
		})
	}
}

func TestDataSchemaFingerprintChangesWithModuleResourceAndFieldShape(t *testing.T) {
	t.Parallel()

	base := compileMutatedTestDocument(t, testDocumentMutation{})
	tests := []struct {
		name   string
		change func(*spec.Document)
	}{
		{name: "module rename", change: func(document *spec.Document) {
			document.Metadata.Name = "crm.pipeline"
		}},
		{name: "resource add", change: func(document *spec.Document) {
			document.Spec.Resources = append(document.Spec.Resources, spec.Resource{
				Name:   "campaign",
				Fields: []spec.Field{{Name: "code", Type: "string"}},
			})
		}},
		{name: "resource delete", change: func(document *spec.Document) {
			document.Spec.Resources = document.Spec.Resources[:1]
		}},
		{name: "resource rename", change: func(document *spec.Document) {
			document.Spec.Resources[0].Name = "account"
			document.Spec.Resources[1].Fields[0].Target = "account"
		}},
		{name: "field add", change: func(document *spec.Document) {
			document.Spec.Resources[0].Fields = append(
				document.Spec.Resources[0].Fields,
				spec.Field{Name: "domain", Type: "string"},
			)
		}},
		{name: "field delete", change: func(document *spec.Document) {
			lead := &document.Spec.Resources[1]
			lead.Fields = lead.Fields[:3]
			lead.API.Admin.Writable = lead.API.Admin.Writable[:3]
			lead.Manager.Form.Fields = lead.Manager.Form.Fields[:3]
		}},
		{name: "field rename", change: func(document *spec.Document) {
			lead := &document.Spec.Resources[1]
			lead.Fields[3].Name = "rating"
			lead.API.Admin.Writable[3] = "rating"
			lead.Manager.Form.Fields[3] = "rating"
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			document := testDocument()
			test.change(&document)
			changed, err := NewCompiler().CompileDocument(document)
			if err != nil {
				t.Fatal(err)
			}
			if changed.DataSchemaFingerprint() == base.DataSchemaFingerprint() {
				t.Fatalf("DataSchemaFingerprint() did not change for %s", test.name)
			}
		})
	}
}

func TestDataSchemaProjectionIgnoresResourceAndFieldOrder(t *testing.T) {
	t.Parallel()

	ordered := testDocument()
	reordered := testDocument()
	slices.Reverse(reordered.Spec.Resources)
	for index := range reordered.Spec.Resources {
		slices.Reverse(reordered.Spec.Resources[index].Fields)
	}
	orderedProjection, err := marshalDataSchemaProjection(convertDocument(ordered))
	if err != nil {
		t.Fatal(err)
	}
	reorderedProjection, err := marshalDataSchemaProjection(convertDocument(reordered))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orderedProjection, reorderedProjection) {
		t.Fatalf("resource or field declaration order changed projection\nordered:   %s\nreordered: %s", orderedProjection, reorderedProjection)
	}
	if fingerprintDataSchema(orderedProjection) != fingerprintDataSchema(reorderedProjection) {
		t.Fatal("resource or field declaration order changed data schema fingerprint")
	}
}

func TestDataSchemaFingerprintTreatsEnumCandidatesAsASet(t *testing.T) {
	t.Parallel()
	base := testDocument()
	changed := testDocument()
	changed.Spec.Resources[1].Fields[2].Options = []string{"won", "new", "qualified"}
	baseModule, err := NewCompiler().CompileDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	changedModule, err := NewCompiler().CompileDocument(changed)
	if err != nil {
		t.Fatal(err)
	}
	if baseModule.RevisionHash() == changedModule.RevisionHash() {
		t.Fatal("enum display order did not change complete revision")
	}
	if baseModule.DataSchemaFingerprint() != changedModule.DataSchemaFingerprint() {
		t.Fatal("enum display order changed data schema fingerprint")
	}
}

type testDocumentMutation struct {
	version         string
	label           string
	managerFirst    string
	removeDelete    bool
	capability      string
	dependencyRange string
	fieldType       string
	notRequired     bool
	notUnique       bool
	referenceTarget string
	enumOption      string
	maxLength       int
	precision       int
	scale           int
}

func compileMutatedTestDocument(t *testing.T, mutation testDocumentMutation) *CompiledModule {
	t.Helper()
	document := testDocument()
	if mutation.version != "" {
		document.Metadata.Version = mutation.version
	}
	if mutation.label != "" {
		document.Metadata.Labels["en-US"] = mutation.label
	}
	if mutation.managerFirst != "" {
		document.Spec.Resources[1].Manager.List.Columns = []string{mutation.managerFirst, "email", "id", "created_at"}
	}
	if mutation.removeDelete {
		document.Spec.Resources[1].API.Admin.Operations = []string{"list", "get", "create", "patch"}
	}
	if mutation.capability != "" {
		document.Spec.Requires.Capabilities = []string{mutation.capability}
	}
	if mutation.dependencyRange != "" {
		document.Spec.Requires.Modules[0].Version = mutation.dependencyRange
	}
	if mutation.fieldType != "" {
		document.Spec.Resources[0].Fields[0].Type = mutation.fieldType
	}
	if mutation.notRequired {
		document.Spec.Resources[0].Fields[0].Required = false
	}
	if mutation.notUnique {
		document.Spec.Resources[0].Fields[0].Unique = false
	}
	if mutation.referenceTarget != "" {
		document.Spec.Resources[1].Fields[0].Target = mutation.referenceTarget
	}
	if mutation.enumOption != "" {
		document.Spec.Resources[1].Fields[2].Options = append(document.Spec.Resources[1].Fields[2].Options, mutation.enumOption)
	}
	if mutation.maxLength != 0 {
		document.Spec.Resources[0].Fields[0].Constraints.MaxLength = &mutation.maxLength
	}
	if mutation.precision != 0 {
		document.Spec.Resources[1].Fields[3].Constraints.Precision = &mutation.precision
	}
	if mutation.scale != 0 {
		document.Spec.Resources[1].Fields[3].Constraints.Scale = &mutation.scale
	}
	module, err := NewCompiler().CompileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	return module
}
