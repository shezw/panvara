/*
   Panvara
   internal/domain/appmodule/descriptor.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package appmodule defines the safe, model-driven extension contract.
package appmodule

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxRelations   = 128
	maxResources   = 128
	maxFields      = 256
	maxEnumOptions = 256
	maxNameBytes   = 128
	maxLabels      = 32
	maxLabelBytes  = 256
)

var (
	moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	enumValuePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	labelKeyPattern   = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
	capabilityPattern = regexp.MustCompile(
		`^[a-z][a-z0-9]*(?:\.[a-z][a-z0-9]*)+/v(?:0|[1-9][0-9]*)` +
			`(?:(?:alpha|beta)(?:0|[1-9][0-9]*))?$`,
	)
	semverPattern = regexp.MustCompile(
		`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)` +
			`(?:-((?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)` +
			`(?:\.(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*))*))?` +
			`(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`,
	)
	reservedResourceNames = map[string]struct{}{
		"audit_event": {}, "module_revision": {}, "outbox": {}, "panvara_system": {},
	}
	reservedFieldNames = map[string]struct{}{
		"created_at": {}, "deleted_at": {}, "id": {}, "owner_id": {},
		"project_id": {}, "revision": {}, "updated_at": {},
	}
)

// FieldKind is a stable semantic type in the v1alpha1 model language.
type FieldKind string

const (
	KindString    FieldKind = "string"
	KindText      FieldKind = "text"
	KindInt       FieldKind = "int"
	KindBool      FieldKind = "bool"
	KindDecimal   FieldKind = "decimal"
	KindEnum      FieldKind = "enum"
	KindDate      FieldKind = "date"
	KindDateTime  FieldKind = "datetime"
	KindEmail     FieldKind = "email"
	KindMoney     FieldKind = "money"
	KindReference FieldKind = "reference"
)

var fieldKinds = map[FieldKind]struct{}{
	KindString: {}, KindText: {}, KindInt: {}, KindBool: {}, KindDecimal: {},
	KindEnum: {}, KindDate: {}, KindDateTime: {}, KindEmail: {},
	KindMoney: {}, KindReference: {},
}

// Field declares one resource property.
type Field struct {
	Name        string
	Labels      map[string]string
	Kind        FieldKind
	Required    bool
	Unique      bool
	Target      string
	Options     []string
	Constraints Constraints
}

// Resource declares a managed domain record.
type Resource struct {
	Name    string
	Labels  map[string]string
	Fields  []Field
	API     API
	Manager Manager
}

// Requirements separates module graph edges from runtime capability needs.
type Requirements struct {
	Modules      []ModuleRequirement
	Capabilities []string
}

// ModuleRequirement binds a module dependency to a restricted SemVer range.
type ModuleRequirement struct {
	Name    string
	Version string
}

// Constraints contains bounded field-specific validation rules.
type Constraints struct {
	MaxLength *int
	Precision *int
	Scale     *int
}

// Operation is a generated CRUD operation exposed by an API surface.
type Operation string

const (
	// OperationList permits listing resource records.
	OperationList Operation = "list"
	// OperationGet permits reading one resource record.
	OperationGet Operation = "get"
	// OperationCreate permits creating a resource record.
	OperationCreate Operation = "create"
	// OperationPatch permits partially updating a resource record.
	OperationPatch Operation = "patch"
	// OperationDelete permits deleting a resource record.
	OperationDelete Operation = "delete"
)

// Access is a fail-closed operation and field allowlist.
type Access struct {
	Operations []Operation
	Writable   []string
	Filterable []string
	Sortable   []string
}

// API defines independently deployable public and administrative surfaces.
type API struct {
	Public Access
	Admin  Access
}

// Manager contains explicit display order for generated management views.
type Manager struct {
	List ListView
	Form FormView
}

// ListView configures ordered Manager columns and filters.
type ListView struct {
	Columns []string
	Filters []string
}

// FormView configures ordered Manager form fields.
type FormView struct {
	Fields []string
}

// Descriptor is the source contract from which later Core versions generate a
// canonical IR, storage plan, APIs, and management UI.
type Descriptor struct {
	Name      string
	Version   string
	Labels    map[string]string
	Requires  Requirements
	Provides  []string
	Conflicts []string
	Resources []Resource
}

// Validate rejects ambiguous, oversized, or unsafe v1alpha1 declarations.
func (descriptor Descriptor) Validate() error {
	if !validName(moduleNamePattern, descriptor.Name) {
		return fmt.Errorf("invalid module name %q", descriptor.Name)
	}
	if len(descriptor.Version) > maxNameBytes || !semverPattern.MatchString(descriptor.Version) {
		return fmt.Errorf("invalid semantic version %q", descriptor.Version)
	}
	if err := validateLabels("module", descriptor.Labels); err != nil {
		return err
	}
	if err := validateModuleRequirements(descriptor.Name, descriptor.Requires.Modules); err != nil {
		return err
	}
	if err := validateCapabilities("required capability", descriptor.Requires.Capabilities); err != nil {
		return err
	}
	if err := validateCapabilities("provided capability", descriptor.Provides); err != nil {
		return err
	}
	if err := validateModuleNames("conflicting module", descriptor.Name, descriptor.Conflicts); err != nil {
		return err
	}
	if len(descriptor.Resources) > maxResources {
		return fmt.Errorf("module has %d resources; maximum is %d", len(descriptor.Resources), maxResources)
	}

	resourceNames := make(map[string]struct{}, len(descriptor.Resources))
	for _, resource := range descriptor.Resources {
		if !validName(schemaNamePattern, resource.Name) {
			return fmt.Errorf("invalid resource name %q", resource.Name)
		}
		if _, reserved := reservedResourceNames[resource.Name]; reserved {
			return fmt.Errorf("resource name %q is reserved", resource.Name)
		}
		if _, exists := resourceNames[resource.Name]; exists {
			return fmt.Errorf("duplicate resource %q", resource.Name)
		}
		if err := validateLabels("resource "+resource.Name, resource.Labels); err != nil {
			return err
		}
		resourceNames[resource.Name] = struct{}{}
	}

	for _, resource := range descriptor.Resources {
		if err := validateFields(resource, resourceNames); err != nil {
			return err
		}
	}
	return nil
}

func validateModuleNames(label, self string, names []string) error {
	if len(names) > maxRelations {
		return fmt.Errorf("%s list has %d entries; maximum is %d", label, len(names), maxRelations)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !validName(moduleNamePattern, name) {
			return fmt.Errorf("invalid %s %q", label, name)
		}
		if name == self {
			return fmt.Errorf("module cannot list itself as a %s", label)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate %s %q", label, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateModuleRequirements(self string, requirements []ModuleRequirement) error {
	if len(requirements) > maxRelations {
		return fmt.Errorf("required module list has %d entries; maximum is %d", len(requirements), maxRelations)
	}
	seen := make(map[string]struct{}, len(requirements))
	for _, requirement := range requirements {
		if !validName(moduleNamePattern, requirement.Name) {
			return fmt.Errorf("invalid required module %q", requirement.Name)
		}
		if requirement.Name == self {
			return errors.New("module cannot list itself as a required module")
		}
		if _, exists := seen[requirement.Name]; exists {
			return fmt.Errorf("duplicate required module %q", requirement.Name)
		}
		if err := ValidateVersionRange(requirement.Version); err != nil {
			return fmt.Errorf("required module %q: %w", requirement.Name, err)
		}
		seen[requirement.Name] = struct{}{}
	}
	return nil
}

func validateCapabilities(label string, names []string) error {
	if len(names) > maxRelations {
		return fmt.Errorf("%s list has %d entries; maximum is %d", label, len(names), maxRelations)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if len(name) > maxNameBytes || !capabilityPattern.MatchString(name) {
			return fmt.Errorf("invalid %s %q", label, name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate %s %q", label, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func validateFields(resource Resource, resources map[string]struct{}) error {
	if len(resource.Fields) > maxFields {
		return fmt.Errorf("resource %q has %d fields; maximum is %d", resource.Name, len(resource.Fields), maxFields)
	}
	seen := make(map[string]struct{}, len(resource.Fields))
	fieldDefinitions := make(map[string]Field, len(resource.Fields))
	for _, field := range resource.Fields {
		if !validName(schemaNamePattern, field.Name) {
			return fmt.Errorf("resource %q has invalid field name %q", resource.Name, field.Name)
		}
		if _, reserved := reservedFieldNames[field.Name]; reserved {
			return fmt.Errorf("resource %q field name %q is reserved", resource.Name, field.Name)
		}
		if _, exists := seen[field.Name]; exists {
			return fmt.Errorf("resource %q has duplicate field %q", resource.Name, field.Name)
		}
		seen[field.Name] = struct{}{}
		fieldDefinitions[field.Name] = field
		if err := validateLabels("field "+resource.Name+"."+field.Name, field.Labels); err != nil {
			return err
		}
		if _, exists := fieldKinds[field.Kind]; !exists {
			return fmt.Errorf("resource %q field %q has unknown kind %q", resource.Name, field.Name, field.Kind)
		}

		switch field.Kind {
		case KindEnum:
			if err := validateOptions(resource.Name, field); err != nil {
				return err
			}
			if field.Target != "" {
				return fmt.Errorf("enum field %q.%q cannot have a target", resource.Name, field.Name)
			}
		case KindReference:
			if _, exists := resources[field.Target]; !exists {
				return fmt.Errorf("reference field %q.%q targets unknown resource %q", resource.Name, field.Name, field.Target)
			}
			if len(field.Options) != 0 {
				return fmt.Errorf("reference field %q.%q cannot have options", resource.Name, field.Name)
			}
		default:
			if field.Target != "" || len(field.Options) != 0 {
				return fmt.Errorf("field %q.%q has metadata incompatible with kind %q", resource.Name, field.Name, field.Kind)
			}
		}
		if err := validateConstraints(resource.Name, field); err != nil {
			return err
		}
	}
	return validateResourceConfiguration(resource, fieldDefinitions)
}

func validateOptions(resource string, field Field) error {
	if len(field.Options) == 0 {
		return fmt.Errorf("enum field %q.%q requires options", resource, field.Name)
	}
	if len(field.Options) > maxEnumOptions {
		return fmt.Errorf(
			"enum field %q.%q has %d options; maximum is %d",
			resource,
			field.Name,
			len(field.Options),
			maxEnumOptions,
		)
	}
	seen := make(map[string]struct{}, len(field.Options))
	for _, option := range field.Options {
		if len(option) > maxNameBytes || !enumValuePattern.MatchString(option) {
			return fmt.Errorf("enum field %q.%q has invalid option %q", resource, field.Name, option)
		}
		if _, exists := seen[option]; exists {
			return fmt.Errorf("enum field %q.%q has duplicate option %q", resource, field.Name, option)
		}
		seen[option] = struct{}{}
	}
	return nil
}

func validName(pattern *regexp.Regexp, name string) bool {
	return len(name) <= maxNameBytes && pattern.MatchString(name)
}

func validateLabels(owner string, labels map[string]string) error {
	if len(labels) > maxLabels {
		return fmt.Errorf("%s has %d labels; maximum is %d", owner, len(labels), maxLabels)
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := labels[key]
		if !labelKeyPattern.MatchString(key) {
			return fmt.Errorf("%s has invalid label key %q", owner, key)
		}
		if value == "" || len(value) > maxLabelBytes || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%s label %q must be non-empty NUL-free valid UTF-8 up to %d bytes", owner, key, maxLabelBytes)
		}
	}
	return nil
}
