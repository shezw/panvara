/*
   Panvara
   internal/application/appmodule/filter_test.go    2026-07-14
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
	"testing"

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestNormalizeFilterValueMatchesJSONBTextRepresentation(t *testing.T) {
	t.Parallel()

	precision := 12
	scale := 4
	document := spec.Document{
		APIVersion: spec.APIVersion, Kind: spec.Kind,
		Metadata: spec.Metadata{Name: "search", Version: "1.0.0"},
		Spec: spec.Spec{Resources: []spec.Resource{{
			Name: "item",
			Fields: []spec.Field{
				{Name: "title", Type: "string"},
				{Name: "count", Type: "int"},
				{Name: "active", Type: "bool"},
				{Name: "amount", Type: "decimal", Constraints: spec.Constraints{Precision: &precision, Scale: &scale}},
				{Name: "status", Type: "enum", Options: []string{"new", "done"}},
				{Name: "day", Type: "date"},
				{Name: "seen_at", Type: "datetime"},
				{Name: "email", Type: "email"},
				{Name: "parent", Type: "reference", Target: "item"},
			},
			API: spec.API{Admin: spec.Access{
				Operations: []string{"list"},
				Filterable: []string{"title", "count", "active", "amount", "status", "day", "seen_at", "email", "parent"},
			}},
		}}},
	}
	module, err := NewCompiler().CompileDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		field string
		raw   string
		want  string
	}{
		{field: "title", raw: "Ada", want: "Ada"},
		{field: "count", raw: "42", want: "42"},
		{field: "active", raw: "true", want: "true"},
		{field: "amount", raw: "1.2300", want: "1.23"},
		{field: "status", raw: "done", want: "done"},
		{field: "day", raw: "2026-07-14", want: "2026-07-14"},
		{field: "seen_at", raw: "2026-07-14T08:00:00+08:00", want: "2026-07-14T00:00:00Z"},
		{field: "email", raw: "Ada@EXAMPLE.COM", want: "Ada@example.com"},
		{field: "parent", raw: "01981234-5678-7ABC-8DEF-0123456789AD", want: "01981234-5678-7abc-8def-0123456789ad"},
	}
	for _, test := range tests {
		got, err := module.NormalizeFilterValue("item", test.field, test.raw)
		if err != nil {
			t.Fatalf("NormalizeFilterValue(%q) error = %v", test.field, err)
		}
		if got != test.want {
			t.Fatalf("NormalizeFilterValue(%q) = %q, want %q", test.field, got, test.want)
		}
	}
	if _, err := module.NormalizeFilterValue("item", "missing", "value"); err == nil {
		t.Fatal("NormalizeFilterValue() accepted an undeclared filter")
	}
}
