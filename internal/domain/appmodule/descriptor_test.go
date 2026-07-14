/*
   Panvara
   internal/domain/appmodule/descriptor_test.go    2026-07-14
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
	"fmt"
	"testing"
)

func TestDescriptorValidateAcceptsReferenceModel(t *testing.T) {
	t.Parallel()

	descriptor := validDescriptor("crm.leads")
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDescriptorValidateAcceptsSemVerPrereleaseAndBuild(t *testing.T) {
	t.Parallel()

	descriptor := validDescriptor("crm.preview")
	descriptor.Version = "1.0.0-alpha.1+build.5"
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("Validate() rejected valid SemVer: %v", err)
	}
}

func TestDescriptorValidateRejectsUnsafeOrAmbiguousModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		descriptor Descriptor
	}{
		{
			name:       "module name",
			descriptor: Descriptor{Name: "CRM Leads", Version: "1.0.0"},
		},
		{
			name:       "semantic version",
			descriptor: Descriptor{Name: "crm", Version: "latest"},
		},
		{
			name:       "semantic version numeric prerelease leading zero",
			descriptor: Descriptor{Name: "crm", Version: "1.0.0-01"},
		},
		{
			name: "self dependency",
			descriptor: Descriptor{
				Name:    "crm",
				Version: "1.0.0",
				Requires: Requirements{Modules: []ModuleRequirement{{
					Name: "crm", Version: "*",
				}}},
			},
		},
		{
			name: "capability without protocol version",
			descriptor: Descriptor{
				Name:     "crm",
				Version:  "1.0.0",
				Provides: []string{"crm.records"},
			},
		},
		{
			name: "duplicate resource",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead"}, {Name: "lead"}},
			},
		},
		{
			name: "reserved field",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{{Name: "project_id", Kind: KindString}}}},
			},
		},
		{
			name: "unknown field kind",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{{Name: "name", Kind: "script"}}}},
			},
		},
		{
			name: "enum without options",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{{Name: "stage", Kind: KindEnum}}}},
			},
		},
		{
			name: "missing reference target",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{{Name: "owner", Kind: KindReference, Target: "user"}}}},
			},
		},
		{
			name: "incompatible metadata",
			descriptor: Descriptor{
				Name:      "crm",
				Version:   "1.0.0",
				Resources: []Resource{{Name: "lead", Fields: []Field{{Name: "name", Kind: KindString, Target: "lead"}}}},
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.descriptor.Validate(); err == nil {
				t.Fatal("Validate() unexpectedly succeeded")
			}
		})
	}
}

func TestDescriptorValidateRejectsOversizedModel(t *testing.T) {
	t.Parallel()

	descriptor := minimalDescriptor("oversized")
	descriptor.Resources = make([]Resource, maxResources+1)
	for index := range descriptor.Resources {
		descriptor.Resources[index].Name = fmt.Sprintf("resource_%d", index)
	}
	if err := descriptor.Validate(); err == nil {
		t.Fatal("Validate() accepted an oversized model")
	}
}

func validDescriptor(name string) Descriptor {
	return Descriptor{
		Name:     name,
		Version:  "1.0.0",
		Provides: []string{"crm.records/v1alpha1"},
		Resources: []Resource{
			{
				Name: "organization",
				Fields: []Field{
					{Name: "name", Kind: KindString, Required: true},
				},
			},
			{
				Name: "lead",
				Fields: []Field{
					{Name: "email", Kind: KindEmail, Unique: true, Constraints: Constraints{MaxLength: intPointer(320)}},
					{Name: "stage", Kind: KindEnum, Options: []string{"new", "qualified", "won"}},
					{Name: "organization", Kind: KindReference, Target: "organization"},
				},
			},
		},
	}
}

func intPointer(value int) *int {
	return &value
}
