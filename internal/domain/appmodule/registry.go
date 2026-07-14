/*
   Panvara
   internal/domain/appmodule/registry.go    2026-07-14
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
	"sort"
	"strings"
)

// Registry is an immutable, dependency-ordered set of validated modules.
type Registry struct {
	ordered []Descriptor
	byName  map[string]Descriptor
}

// NewRegistry validates modules, rejects missing/conflicting module
// dependencies, and produces a deterministic dependency-first order.
// Capability requirements are resolved separately by bootstrap.
func NewRegistry(descriptors []Descriptor) (*Registry, error) {
	byName := make(map[string]Descriptor, len(descriptors))
	for _, descriptor := range descriptors {
		if err := descriptor.Validate(); err != nil {
			return nil, fmt.Errorf("module %q: %w", descriptor.Name, err)
		}
		if _, exists := byName[descriptor.Name]; exists {
			return nil, fmt.Errorf("duplicate module %q", descriptor.Name)
		}
		byName[descriptor.Name] = cloneDescriptor(descriptor)
	}

	for name, descriptor := range byName {
		for _, dependency := range descriptor.Requires.Modules {
			required, exists := byName[dependency.Name]
			if !exists {
				return nil, fmt.Errorf("module %q requires missing module %q", name, dependency.Name)
			}
			matches, err := SatisfiesVersionRange(required.Version, dependency.Version)
			if err != nil {
				return nil, fmt.Errorf("module %q dependency %q: %w", name, dependency.Name, err)
			}
			if !matches {
				return nil, fmt.Errorf(
					"module %q requires module %q version %q; installed version is %q",
					name,
					dependency.Name,
					dependency.Version,
					required.Version,
				)
			}
		}
		for _, conflict := range descriptor.Conflicts {
			if _, exists := byName[conflict]; exists {
				return nil, fmt.Errorf("module %q conflicts with module %q", name, conflict)
			}
		}
	}

	order, err := dependencyOrder(byName)
	if err != nil {
		return nil, err
	}
	ordered := make([]Descriptor, 0, len(order))
	for _, name := range order {
		ordered = append(ordered, cloneDescriptor(byName[name]))
	}
	return &Registry{ordered: ordered, byName: byName}, nil
}

// Modules returns a defensive copy in dependency-first order.
func (registry *Registry) Modules() []Descriptor {
	result := make([]Descriptor, 0, len(registry.ordered))
	for _, descriptor := range registry.ordered {
		result = append(result, cloneDescriptor(descriptor))
	}
	return result
}

// Lookup returns a defensive copy of a module.
func (registry *Registry) Lookup(name string) (Descriptor, bool) {
	descriptor, ok := registry.byName[name]
	if !ok {
		return Descriptor{}, false
	}
	return cloneDescriptor(descriptor), true
}

func dependencyOrder(byName map[string]Descriptor) ([]string, error) {
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(names))
	stack := make([]string, 0, len(names))
	order := make([]string, 0, len(names))

	var visit func(string) error
	visit = func(name string) error {
		switch state[name] {
		case visiting:
			return fmt.Errorf("module dependency cycle: %s -> %s", strings.Join(stack, " -> "), name)
		case visited:
			return nil
		}
		state[name] = visiting
		stack = append(stack, name)
		dependencies := make([]string, 0, len(byName[name].Requires.Modules))
		for _, dependency := range byName[name].Requires.Modules {
			dependencies = append(dependencies, dependency.Name)
		}
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = visited
		order = append(order, name)
		return nil
	}

	for _, name := range names {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return order, nil
}

// Clone returns a deep copy safe for use across registry and compiler boundaries.
func (descriptor Descriptor) Clone() Descriptor {
	return cloneDescriptor(descriptor)
}

func cloneDescriptor(descriptor Descriptor) Descriptor {
	descriptor.Labels = cloneLabels(descriptor.Labels)
	descriptor.Requires.Modules = append([]ModuleRequirement(nil), descriptor.Requires.Modules...)
	descriptor.Requires.Capabilities = append(
		[]string(nil),
		descriptor.Requires.Capabilities...,
	)
	descriptor.Provides = append([]string(nil), descriptor.Provides...)
	descriptor.Conflicts = append([]string(nil), descriptor.Conflicts...)
	descriptor.Resources = append([]Resource(nil), descriptor.Resources...)
	for index := range descriptor.Resources {
		descriptor.Resources[index].Labels = cloneLabels(descriptor.Resources[index].Labels)
		descriptor.Resources[index].Fields = append([]Field(nil), descriptor.Resources[index].Fields...)
		descriptor.Resources[index].API.Public = cloneAccess(descriptor.Resources[index].API.Public)
		descriptor.Resources[index].API.Admin = cloneAccess(descriptor.Resources[index].API.Admin)
		descriptor.Resources[index].Manager.List.Columns = append(
			[]string(nil),
			descriptor.Resources[index].Manager.List.Columns...,
		)
		descriptor.Resources[index].Manager.List.Filters = append(
			[]string(nil),
			descriptor.Resources[index].Manager.List.Filters...,
		)
		descriptor.Resources[index].Manager.Form.Fields = append(
			[]string(nil),
			descriptor.Resources[index].Manager.Form.Fields...,
		)
		for fieldIndex := range descriptor.Resources[index].Fields {
			field := &descriptor.Resources[index].Fields[fieldIndex]
			field.Labels = cloneLabels(field.Labels)
			field.Options = append(
				[]string(nil),
				field.Options...,
			)
			field.Constraints.MaxLength = cloneInt(field.Constraints.MaxLength)
			field.Constraints.Precision = cloneInt(field.Constraints.Precision)
			field.Constraints.Scale = cloneInt(field.Constraints.Scale)
		}
	}
	return descriptor
}

func cloneAccess(access Access) Access {
	access.Operations = append([]Operation(nil), access.Operations...)
	access.Writable = append([]string(nil), access.Writable...)
	access.Filterable = append([]string(nil), access.Filterable...)
	access.Sortable = append([]string(nil), access.Sortable...)
	return access
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}
