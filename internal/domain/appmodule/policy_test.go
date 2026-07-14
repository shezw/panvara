/*
   Panvara
   internal/domain/appmodule/policy_test.go    2026-07-14
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

import "testing"

func TestDescriptorValidateAllowsReadOnlySystemFields(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor{
		Name: "crm", Version: "1.0.0",
		Resources: []Resource{{
			Name:   "lead",
			Fields: []Field{{Name: "name", Kind: KindString, Required: true}},
			API: API{Admin: Access{
				Operations: []Operation{OperationList, OperationCreate},
				Writable:   []string{"name"}, Filterable: []string{"name"},
			}},
			Manager: Manager{List: ListView{Columns: []string{"id", "name", "created_at"}, Filters: []string{"name"}}},
		}},
	}
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDescriptorValidateRejectsImpossibleCreateAndSystemWrites(t *testing.T) {
	t.Parallel()

	for _, writable := range [][]string{{}, {"id"}} {
		descriptor := Descriptor{
			Name: "crm", Version: "1.0.0",
			Resources: []Resource{{
				Name: "lead", Fields: []Field{{Name: "name", Kind: KindString, Required: true}},
				API: API{Public: Access{Operations: []Operation{OperationCreate}, Writable: writable}},
			}},
		}
		if err := descriptor.Validate(); err == nil {
			t.Fatalf("Validate() accepted impossible writable allowlist %v", writable)
		}
	}
}

func TestDescriptorValidateRejectsUnimplementedPublicReadOperations(t *testing.T) {
	t.Parallel()

	descriptor := Descriptor{
		Name: "crm", Version: "1.0.0",
		Resources: []Resource{{
			Name: "lead",
			API:  API{Public: Access{Operations: []Operation{OperationList}}},
		}},
	}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("Validate() accepted an unimplemented public list operation")
	}
}

func TestDescriptorValidateRejectsDynamicSortAndNonScalarFilters(t *testing.T) {
	t.Parallel()

	tests := []Access{
		{Operations: []Operation{OperationList}, Sortable: []string{"name"}},
		{Operations: []Operation{OperationList}, Filterable: []string{"notes"}},
		{Operations: []Operation{OperationList}, Filterable: []string{"price"}},
	}
	for _, access := range tests {
		descriptor := Descriptor{
			Name: "crm", Version: "1.0.0",
			Resources: []Resource{{
				Name: "lead",
				Fields: []Field{
					{Name: "name", Kind: KindString},
					{Name: "notes", Kind: KindText},
					{Name: "price", Kind: KindMoney},
				},
				API: API{Admin: access},
			}},
		}
		if err := descriptor.Validate(); err == nil {
			t.Fatalf("Validate() accepted unsupported access %#v", access)
		}
	}
}

func TestDescriptorValidateEnforcesBoundedUniqueValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field Field
		want  bool
	}{
		{name: "text", field: Field{Name: "value", Kind: KindText, Unique: true}},
		{name: "unbounded string", field: Field{Name: "value", Kind: KindString, Unique: true}},
		{name: "oversized email", field: Field{Name: "value", Kind: KindEmail, Unique: true, Constraints: Constraints{MaxLength: intPointer(513)}}},
		{name: "bounded string", field: Field{Name: "value", Kind: KindString, Unique: true, Constraints: Constraints{MaxLength: intPointer(128)}}, want: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			descriptor := Descriptor{
				Name: "crm", Version: "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{test.field}}},
			}
			if got := descriptor.Validate() == nil; got != test.want {
				t.Fatalf("Validate() success = %t, want %t", got, test.want)
			}
		})
	}
}
