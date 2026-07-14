/*
   Panvara
   internal/spec/appmodule/v1alpha1/document.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package v1alpha1 defines the author-facing AppModule v1alpha1 contract.
package v1alpha1

const (
	// APIVersion identifies the first AppModule authoring contract.
	APIVersion = "panvara.dev/v1alpha1"
	// Kind identifies an AppModule document.
	Kind = "AppModule"
)

// Document is the strict authoring DTO. It intentionally stays separate from
// the canonical domain model so future source versions can use converters.
type Document struct {
	APIVersion string   `json:"apiVersion" yaml:"apiVersion"`
	Kind       string   `json:"kind" yaml:"kind"`
	Metadata   Metadata `json:"metadata" yaml:"metadata"`
	Spec       Spec     `json:"spec" yaml:"spec"`
}

// Metadata identifies a module and its author-controlled semantic version.
type Metadata struct {
	Name    string            `json:"name" yaml:"name"`
	Version string            `json:"version" yaml:"version"`
	Labels  map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
}

// Spec contains declarative module relationships and managed resources.
type Spec struct {
	Requires  Requirements `json:"requires,omitempty" yaml:"requires,omitempty"`
	Provides  []string     `json:"provides,omitempty" yaml:"provides,omitempty"`
	Conflicts []string     `json:"conflicts,omitempty" yaml:"conflicts,omitempty"`
	Resources []Resource   `json:"resources,omitempty" yaml:"resources,omitempty"`
}

// Requirements separates concrete modules from abstract capabilities.
type Requirements struct {
	Modules      []ModuleRequirement `json:"modules,omitempty" yaml:"modules,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// ModuleRequirement declares a module and a deterministic semantic-version
// range: wildcard, exact, caret, or space-separated comparison predicates.
type ModuleRequirement struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version" yaml:"version"`
}

// Resource describes one managed record type.
type Resource struct {
	Name    string            `json:"name" yaml:"name"`
	Labels  map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Fields  []Field           `json:"fields,omitempty" yaml:"fields,omitempty"`
	API     API               `json:"api,omitempty" yaml:"api,omitempty"`
	Manager Manager           `json:"manager,omitempty" yaml:"manager,omitempty"`
}

// Field describes one resource property in protocol-neutral terms.
type Field struct {
	Name        string            `json:"name" yaml:"name"`
	Labels      map[string]string `json:"labels,omitempty" yaml:"labels,omitempty"`
	Type        string            `json:"type" yaml:"type"`
	Required    bool              `json:"required,omitempty" yaml:"required,omitempty"`
	Unique      bool              `json:"unique,omitempty" yaml:"unique,omitempty"`
	Target      string            `json:"target,omitempty" yaml:"target,omitempty"`
	Options     []string          `json:"options,omitempty" yaml:"options,omitempty"`
	Constraints Constraints       `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

// Constraints contains deterministic field limits supported by alpha.2.
type Constraints struct {
	MaxLength *int `json:"maxLength,omitempty" yaml:"maxLength,omitempty"`
	Precision *int `json:"precision,omitempty" yaml:"precision,omitempty"`
	Scale     *int `json:"scale,omitempty" yaml:"scale,omitempty"`
}

// API controls generated public and administrative CRUD surfaces.
type API struct {
	Public Access `json:"public,omitempty" yaml:"public,omitempty"`
	Admin  Access `json:"admin,omitempty" yaml:"admin,omitempty"`
}

// Access is a fail-closed operation and field allowlist.
type Access struct {
	Operations []string `json:"operations,omitempty" yaml:"operations,omitempty"`
	Writable   []string `json:"writable,omitempty" yaml:"writable,omitempty"`
	Filterable []string `json:"filterable,omitempty" yaml:"filterable,omitempty"`
	Sortable   []string `json:"sortable,omitempty" yaml:"sortable,omitempty"`
}

// Manager controls explicit display order for generated management views.
type Manager struct {
	List ListView `json:"list,omitempty" yaml:"list,omitempty"`
	Form FormView `json:"form,omitempty" yaml:"form,omitempty"`
}

// ListView configures ordered columns and filters.
type ListView struct {
	Columns []string `json:"columns,omitempty" yaml:"columns,omitempty"`
	Filters []string `json:"filters,omitempty" yaml:"filters,omitempty"`
}

// FormView configures ordered editable fields.
type FormView struct {
	Fields []string `json:"fields,omitempty" yaml:"fields,omitempty"`
}
