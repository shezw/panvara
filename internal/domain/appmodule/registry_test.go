/*
   Panvara
   internal/domain/appmodule/registry_test.go    2026-07-14
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
	"strings"
	"testing"
)

func TestRegistryOrdersDependenciesDeterministically(t *testing.T) {
	t.Parallel()

	base := minimalDescriptor("base")
	feature := minimalDescriptor("feature")
	feature.Requires.Modules = []string{"base"}
	ui := minimalDescriptor("ui")
	ui.Requires.Modules = []string{"feature"}

	registry, err := NewRegistry([]Descriptor{ui, feature, base})
	if err != nil {
		t.Fatal(err)
	}
	modules := registry.Modules()
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		names = append(names, module.Name)
	}
	if want := []string{"base", "feature", "ui"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("module order = %v, want %v", names, want)
	}
}

func TestRegistryRejectsMissingDependency(t *testing.T) {
	t.Parallel()

	module := minimalDescriptor("feature")
	module.Requires.Modules = []string{"base"}
	if _, err := NewRegistry([]Descriptor{module}); err == nil {
		t.Fatal("NewRegistry() accepted a missing dependency")
	}
}

func TestRegistryRejectsConflicts(t *testing.T) {
	t.Parallel()

	left := minimalDescriptor("left")
	left.Conflicts = []string{"right"}
	right := minimalDescriptor("right")
	if _, err := NewRegistry([]Descriptor{left, right}); err == nil {
		t.Fatal("NewRegistry() accepted conflicting modules")
	}
}

func TestRegistryRejectsDependencyCycle(t *testing.T) {
	t.Parallel()

	left := minimalDescriptor("left")
	left.Requires.Modules = []string{"right"}
	right := minimalDescriptor("right")
	right.Requires.Modules = []string{"left"}
	_, err := NewRegistry([]Descriptor{left, right})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("NewRegistry() error = %v, want cycle", err)
	}
}

func TestRegistryReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	descriptor := validDescriptor("crm")
	descriptor.Requires.Capabilities = []string{"notification.email/v1alpha1"}
	registry, err := NewRegistry([]Descriptor{descriptor})
	if err != nil {
		t.Fatal(err)
	}
	first := registry.Modules()
	first[0].Resources[0].Fields[0].Name = "mutated"
	first[0].Requires.Capabilities[0] = "mutated"
	second := registry.Modules()
	if second[0].Resources[0].Fields[0].Name == "mutated" {
		t.Fatal("registry leaked mutable state")
	}
	if second[0].Requires.Capabilities[0] == "mutated" {
		t.Fatal("registry leaked mutable capability requirements")
	}
}

func minimalDescriptor(name string) Descriptor {
	return Descriptor{APIVersion: APIVersion, Name: name, Version: "1.0.0"}
}
