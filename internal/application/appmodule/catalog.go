/*
   Panvara
   internal/application/appmodule/catalog.go    2026-07-14
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

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

// Catalog is an immutable, dependency-checked lookup of compiled modules.
type Catalog struct {
	modules map[string]*CompiledModule
	ordered []string
}

// NewCatalog validates module dependency ranges and rejects duplicate names.
func NewCatalog(modules ...*CompiledModule) (*Catalog, error) {
	byName := make(map[string]*CompiledModule, len(modules))
	descriptors := make([]domain.Descriptor, 0, len(modules))
	for index, module := range modules {
		if module == nil {
			return nil, fmt.Errorf("compiled module at index %d is nil", index)
		}
		if _, exists := byName[module.Name()]; exists {
			return nil, fmt.Errorf("duplicate compiled module %q", module.Name())
		}
		byName[module.Name()] = module
		descriptors = append(descriptors, module.Descriptor())
	}
	registry, err := domain.NewRegistry(descriptors)
	if err != nil {
		return nil, fmt.Errorf("build compiled module catalog: %w", err)
	}
	ordered := make([]string, 0, len(modules))
	for _, descriptor := range registry.Modules() {
		ordered = append(ordered, descriptor.Name)
	}
	return &Catalog{modules: byName, ordered: ordered}, nil
}

// Lookup returns an immutable compiled module by canonical name.
func (catalog *Catalog) Lookup(name string) (*CompiledModule, bool) {
	if catalog == nil {
		return nil, false
	}
	module, found := catalog.modules[name]
	return module, found
}

// LookupResource returns a defensive copy of one canonical resource.
func (catalog *Catalog) LookupResource(moduleName, resourceName string) (domain.Resource, bool) {
	module, found := catalog.Lookup(moduleName)
	if !found {
		return domain.Resource{}, false
	}
	return module.resource(resourceName)
}

// ModuleNames returns dependency-first module names.
func (catalog *Catalog) ModuleNames() []string {
	if catalog == nil {
		return []string{}
	}
	return append([]string(nil), catalog.ordered...)
}
