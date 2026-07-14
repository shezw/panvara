/*
   Panvara
   internal/application/record/appmodule_validator_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package record

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shezw/panvara/internal/application/appmodule"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestCompiledModuleValidatorCreateBuildsIndexes(t *testing.T) {
	t.Parallel()
	module := testCompiledModule(t)
	validator, err := NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatalf("NewCompiledModuleValidator() error = %v", err)
	}
	targetID := "01981234-5678-7abc-8def-0123456789ac"
	validated, err := validator.Validate(context.Background(), ValidationInput{
		Scope:    testModuleScope(t, module, "lead"),
		Surface:  SurfacePublic,
		Mutation: MutationCreate,
		Data: json.RawMessage(
			`{"name":"Ada","email":"ADA@Example.COM","owner":"` + targetID + `"}`,
		),
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := string(validated.Data); got !=
		`{"email":"ADA@example.com","name":"Ada","owner":"`+targetID+`"}` {
		t.Fatalf("Validate().Data = %s", got)
	}
	if len(validated.Uniques) != 1 || validated.Uniques[0].CanonicalValue != `"ADA@example.com"` {
		t.Fatalf("Validate().Uniques = %#v", validated.Uniques)
	}
	if len(validated.References) != 1 || validated.References[0].TargetID.String() != targetID {
		t.Fatalf("Validate().References = %#v", validated.References)
	}
}

func TestCompiledModuleValidatorPatchSeparatesAllowlistFromCompleteValidation(t *testing.T) {
	t.Parallel()
	module := testCompiledModule(t)
	validator, err := NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatalf("NewCompiledModuleValidator() error = %v", err)
	}
	targetID := "01981234-5678-7abc-8def-0123456789ac"
	existing := json.RawMessage(
		`{"email":"old@example.com","name":"Ada","owner":"` + targetID + `"}`,
	)
	patch := json.RawMessage(`{"email":"NEW@Example.COM"}`)
	complete := json.RawMessage(
		`{"email":"NEW@Example.COM","name":"Ada","owner":"` + targetID + `"}`,
	)
	validated, err := validator.Validate(context.Background(), ValidationInput{
		Scope:        testModuleScope(t, module, "lead"),
		Surface:      SurfaceAdmin,
		Mutation:     MutationPatch,
		ExistingData: existing,
		PatchData:    patch,
		Data:         complete,
	})
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got := string(validated.Data); got !=
		`{"email":"NEW@example.com","name":"Ada","owner":"`+targetID+`"}` {
		t.Fatalf("Validate().Data = %s", got)
	}

	_, err = validator.Validate(context.Background(), ValidationInput{
		Scope:        testModuleScope(t, module, "lead"),
		Surface:      SurfaceAdmin,
		Mutation:     MutationPatch,
		ExistingData: existing,
		PatchData:    json.RawMessage(`{"name":"Grace"}`),
		Data: json.RawMessage(
			`{"email":"old@example.com","name":"Grace","owner":"` + targetID + `"}`,
		),
	})
	var validationError *appmodule.ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("Validate(non-writable) error = %v, want ValidationError", err)
	}
}

func TestCompiledModuleValidatorPatchRejectsExplicitNull(t *testing.T) {
	t.Parallel()
	module := testCompiledModule(t)
	validator, err := NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatalf("NewCompiledModuleValidator() error = %v", err)
	}
	targetID := "01981234-5678-7abc-8def-0123456789ac"
	_, err = validator.Validate(context.Background(), ValidationInput{
		Scope:        testModuleScope(t, module, "lead"),
		Surface:      SurfaceAdmin,
		Mutation:     MutationPatch,
		ExistingData: json.RawMessage(`{"email":"old@example.com","name":"Ada","owner":"` + targetID + `"}`),
		PatchData:    json.RawMessage(`{"email":null}`),
		Data:         json.RawMessage(`{"email":null,"name":"Ada","owner":"` + targetID + `"}`),
	})
	var validationError *appmodule.ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("Validate(null patch) error = %v, want ValidationError", err)
	}
}

func TestCompiledModuleValidatorClassifiesInvalidClientJSON(t *testing.T) {
	t.Parallel()
	module := testCompiledModule(t)
	validator, err := NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatalf("NewCompiledModuleValidator() error = %v", err)
	}
	deep := strings.Repeat(`{"nested":`, 18) + `true` + strings.Repeat(`}`, 18)
	for name, source := range map[string]string{
		"malformed":      `{"name":`,
		"duplicate key":  `{"name":"Ada","name":"Grace"}`,
		"trailing value": `{"name":"Ada"}{}`,
		"too deep":       deep,
	} {
		name, source := name, source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := validator.Validate(context.Background(), ValidationInput{
				Scope: testModuleScope(t, module, "lead"), Surface: SurfacePublic,
				Mutation: MutationCreate, Data: json.RawMessage(source),
			})
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Validate(%s) error = %v, want ErrInvalidArgument", source, err)
			}
		})
	}
}

func TestCompiledModuleValidatorListAuthorizesAndNormalizesFilters(t *testing.T) {
	t.Parallel()
	module := testCompiledModule(t)
	validator, err := NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatalf("NewCompiledModuleValidator() error = %v", err)
	}
	filters, err := validator.ValidateList(context.Background(), ListValidationInput{
		Scope:   testModuleScope(t, module, "lead"),
		Surface: SurfaceAdmin,
		Filters: []ListFilter{{Field: "email", Value: "ADA@Example.COM"}},
	})
	if err != nil {
		t.Fatalf("ValidateList() error = %v", err)
	}
	if len(filters) != 1 || filters[0].Value != "ADA@example.com" {
		t.Fatalf("ValidateList() = %#v", filters)
	}
	if _, err := validator.ValidateList(context.Background(), ListValidationInput{
		Scope:   testModuleScope(t, module, "lead"),
		Surface: SurfacePublic,
		Filters: []ListFilter{{Field: "email", Value: "ADA@Example.COM"}},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ValidateList(public) error = %v, want ErrInvalidArgument", err)
	}
}

func testCompiledModule(t *testing.T) *appmodule.CompiledModule {
	t.Helper()
	emailMaxLength := 320
	compiler := appmodule.NewCompiler()
	module, err := compiler.CompileDocument(spec.Document{
		APIVersion: spec.APIVersion,
		Kind:       spec.Kind,
		Metadata:   spec.Metadata{Name: "crm.leads", Version: "0.1.0"},
		Spec: spec.Spec{Resources: []spec.Resource{
			{
				Name: "user",
				Fields: []spec.Field{
					{Name: "name", Type: "string", Required: true},
				},
				API: spec.API{Admin: spec.Access{
					Operations: []string{"create", "patch"}, Writable: []string{"name"},
				}},
			},
			{
				Name: "lead",
				Fields: []spec.Field{
					{Name: "name", Type: "string", Required: true},
					{
						Name: "email", Type: "email", Required: true, Unique: true,
						Constraints: spec.Constraints{MaxLength: &emailMaxLength},
					},
					{Name: "owner", Type: "reference", Required: true, Target: "user"},
				},
				API: spec.API{
					Public: spec.Access{
						Operations: []string{"create"},
						Writable:   []string{"name", "email", "owner"},
					},
					Admin: spec.Access{
						Operations: []string{"list", "patch"},
						Writable:   []string{"email"},
						Filterable: []string{"email"},
					},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("CompileDocument() error = %v", err)
	}
	return module
}

func testModuleScope(t *testing.T, module *appmodule.CompiledModule, resource string) Scope {
	t.Helper()
	scope, err := NewScope(testScope(t, resource).ProjectID, module.Name(), resource, module.RevisionHash())
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	return scope
}
