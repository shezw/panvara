/*
   Panvara
   internal/application/appmodule/record_validator_test.go    2026-07-14
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
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestValidateCompleteRecordNormalizesAndBuildsIndexes(t *testing.T) {
	t.Parallel()

	module := compiledTestModule(t)
	input := map[string]any{
		"email":        "Ada@EXAMPLE.COM",
		"stage":        "qualified",
		"score":        "1.2300",
		"organization": "01981234-5678-7ABC-8DEF-0123456789AD",
	}
	validated, err := module.ValidateCompleteRecord("lead", input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := validated.Data["email"], "Ada@example.com"; got != want {
		t.Fatalf("normalized email = %v, want %v", got, want)
	}
	if got, want := validated.Data["score"], "1.23"; got != want {
		t.Fatalf("normalized decimal = %v, want %v", got, want)
	}
	if got, want := validated.Uniques, []UniqueValue{{Field: "email", CanonicalValue: `"Ada@example.com"`}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("unique values = %#v, want %#v", got, want)
	}
	wantReference := []ReferenceValue{{
		Field: "organization", TargetResource: "organization",
		TargetID: "01981234-5678-7abc-8def-0123456789ad",
	}}
	if !reflect.DeepEqual(validated.References, wantReference) {
		t.Fatalf("references = %#v, want %#v", validated.References, wantReference)
	}
	if input["email"] != "Ada@EXAMPLE.COM" {
		t.Fatal("ValidateCompleteRecord() mutated its input")
	}
}

func TestValidateRecordSeparatesPatchPolicyFromCompleteValidation(t *testing.T) {
	t.Parallel()

	module := compiledTestModule(t)
	patch, err := module.ValidateRecord("lead", SurfaceAdmin, MutationPatch, map[string]any{"stage": "won"})
	if err != nil {
		t.Fatal(err)
	}
	if patch["stage"] != "won" {
		t.Fatalf("patch = %#v", patch)
	}
	complete, err := module.ValidateCompleteRecord("lead", map[string]any{
		"email": "a@example.com", "stage": patch["stage"], "score": "-0.0000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if complete.Data["score"] != "0" {
		t.Fatalf("normalized negative zero = %v, want 0", complete.Data["score"])
	}
	if _, err := module.ValidateRecord("lead", SurfacePublic, MutationPatch, map[string]any{"stage": "won"}); err == nil {
		t.Fatal("public patch unexpectedly passed a create-only policy")
	}
}

func TestValidateCompleteRecordRejectsNULStringsBeforePostgres(t *testing.T) {
	t.Parallel()
	module := allFieldKindsModule(t)
	for field, value := range map[string]string{
		"string_value": "text\x00value",
		"email_value":  "ada\x00@example.com",
	} {
		_, err := module.ValidateCompleteRecord("item", map[string]any{field: value})
		var validation *ValidationError
		if !errors.As(err, &validation) || len(validation.Violations) != 1 || validation.Violations[0].Code != "invalid_value" {
			t.Fatalf("ValidateCompleteRecord(%s NUL) error = %#v", field, err)
		}
	}
}

func TestValidateRecordRejectsUnknownSystemNullAndNonWritableFieldsDeterministically(t *testing.T) {
	t.Parallel()

	module := compiledTestModule(t)
	_, err := module.ValidateRecord("lead", SurfacePublic, MutationCreate, map[string]any{
		"version": 1,
		"unknown": true,
		"score":   "1.0",
		"stage":   nil,
	})
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("ValidateRecord() error = %v, want ValidationError", err)
	}
	wantCodes := []string{"required", "field_not_writable", "null_not_allowed", "unknown_field", "system_field"}
	gotCodes := make([]string, 0, len(validation.Violations))
	for _, violation := range validation.Violations {
		gotCodes = append(gotCodes, violation.Code)
	}
	if !reflect.DeepEqual(gotCodes, wantCodes) {
		t.Fatalf("violation codes = %v, want %v; violations=%#v", gotCodes, wantCodes, validation.Violations)
	}
}

func TestDecodeRecordJSONRejectsDuplicateTrailingAndOversizedInput(t *testing.T) {
	t.Parallel()

	for _, source := range [][]byte{
		[]byte(`{"email":"a@example.com","email":"b@example.com"}`),
		[]byte(`{} {}`),
		append([]byte(`{"value":"`), append(make([]byte, maxRecordBytes), []byte(`"}`)...)...),
	} {
		if _, err := DecodeRecordJSON(source); err == nil {
			t.Fatal("DecodeRecordJSON() unexpectedly succeeded")
		}
	}
	decoded, err := DecodeRecordJSON([]byte(`{"number":9223372036854775807}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["number"].(json.Number); !ok {
		t.Fatalf("decoded number type = %T, want json.Number", decoded["number"])
	}
}

func TestValidateCompleteRecordRejectsEquivalentDecimalOverflowBeforeNormalization(t *testing.T) {
	t.Parallel()

	module := compiledTestModule(t)
	_, err := module.ValidateCompleteRecord("lead", map[string]any{
		"email": "a@example.com", "stage": "new", "score": "1.23000",
	})
	if err == nil {
		t.Fatal("ValidateCompleteRecord() accepted decimal beyond declared scale")
	}
}

func TestValidateCompleteRecordCanonicalizesEquivalentUniqueDecimals(t *testing.T) {
	t.Parallel()

	precision := 8
	scale := 3
	document := spec.Document{
		APIVersion: spec.APIVersion, Kind: spec.Kind,
		Metadata: spec.Metadata{Name: "billing", Version: "1.0.0"},
		Spec: spec.Spec{Resources: []spec.Resource{{
			Name: "rate",
			Fields: []spec.Field{{
				Name: "amount", Type: "decimal", Required: true, Unique: true,
				Constraints: spec.Constraints{Precision: &precision, Scale: &scale},
			}},
		}}},
	}
	module, err := NewCompiler().CompileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	left, err := module.ValidateCompleteRecord("rate", map[string]any{"amount": "1.0"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := module.ValidateCompleteRecord("rate", map[string]any{"amount": "1.00"})
	if err != nil {
		t.Fatal(err)
	}
	if left.Uniques[0].CanonicalValue != right.Uniques[0].CanonicalValue ||
		left.Uniques[0].CanonicalValue != `"1"` {
		t.Fatalf("equivalent canonical values = %q / %q", left.Uniques[0].CanonicalValue, right.Uniques[0].CanonicalValue)
	}
}

func TestValidateCompleteRecordCoversEveryV1Alpha1FieldKind(t *testing.T) {
	t.Parallel()

	module := allFieldKindsModule(t)
	valid := map[string]any{
		"string_value":    "short",
		"text_value":      "long form",
		"int_value":       json.Number("42"),
		"bool_value":      true,
		"decimal_value":   "12.340",
		"enum_value":      "open",
		"date_value":      "2026-07-14",
		"datetime_value":  "2026-07-14T08:00:00+08:00",
		"email_value":     "Ada@EXAMPLE.COM",
		"money_value":     map[string]any{"minor": json.Number("1234"), "currency": "usd"},
		"reference_value": "01981234-5678-7ABC-8DEF-0123456789AD",
	}
	validated, err := module.ValidateCompleteRecord("item", valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated.Data) != 11 || validated.Data["decimal_value"] != "12.34" ||
		validated.Data["datetime_value"] != "2026-07-14T00:00:00Z" {
		t.Fatalf("validated all-kind data = %#v", validated.Data)
	}

	invalid := map[string]any{
		"string_value":    1,
		"text_value":      false,
		"int_value":       1.2,
		"bool_value":      "true",
		"decimal_value":   12.34,
		"enum_value":      "missing",
		"date_value":      "2026-02-30",
		"datetime_value":  "yesterday",
		"email_value":     "not-an-email",
		"money_value":     map[string]any{"minor": 1},
		"reference_value": "not-a-uuid",
	}
	for field, value := range invalid {
		if _, err := module.ValidateCompleteRecord("item", map[string]any{field: value}); err == nil {
			t.Fatalf("ValidateCompleteRecord() accepted invalid %s", field)
		}
	}
}

func allFieldKindsModule(t *testing.T) *CompiledModule {
	t.Helper()
	maxLength := 128
	precision := 12
	scale := 4
	document := spec.Document{
		APIVersion: spec.APIVersion, Kind: spec.Kind,
		Metadata: spec.Metadata{Name: "all.kinds", Version: "1.0.0"},
		Spec: spec.Spec{Resources: []spec.Resource{{
			Name: "item",
			Fields: []spec.Field{
				{Name: "string_value", Type: "string", Constraints: spec.Constraints{MaxLength: &maxLength}},
				{Name: "text_value", Type: "text"},
				{Name: "int_value", Type: "int"},
				{Name: "bool_value", Type: "bool"},
				{Name: "decimal_value", Type: "decimal", Constraints: spec.Constraints{Precision: &precision, Scale: &scale}},
				{Name: "enum_value", Type: "enum", Options: []string{"open", "closed"}},
				{Name: "date_value", Type: "date"},
				{Name: "datetime_value", Type: "datetime"},
				{Name: "email_value", Type: "email"},
				{Name: "money_value", Type: "money"},
				{Name: "reference_value", Type: "reference", Target: "item"},
			},
		}}},
	}
	module, err := NewCompiler().CompileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func compiledTestModule(t *testing.T) *CompiledModule {
	t.Helper()
	module, err := NewCompiler().CompileDocument(testDocument())
	if err != nil {
		t.Fatal(err)
	}
	return module
}
