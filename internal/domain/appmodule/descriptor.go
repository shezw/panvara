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
	"fmt"
	"regexp"
)

const APIVersion = "panvara.dev/v1alpha1"

const (
	maxRelations   = 128
	maxResources   = 128
	maxFields      = 256
	maxEnumOptions = 256
	maxNameBytes   = 128
)

var (
	moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	schemaNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	enumValuePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
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
	Name     string
	Kind     FieldKind
	Required bool
	Unique   bool
	Target   string
	Options  []string
}

// Resource declares a managed domain record.
type Resource struct {
	Name   string
	Fields []Field
}

// Requirements separates module graph edges from runtime capability needs.
type Requirements struct {
	Modules      []string
	Capabilities []string
}

// Descriptor is the source contract from which later Core versions generate a
// canonical IR, storage plan, APIs, and management UI.
type Descriptor struct {
	APIVersion string
	Name       string
	Version    string
	Requires   Requirements
	Provides   []string
	Conflicts  []string
	Resources  []Resource
}

// Validate rejects ambiguous, oversized, or unsafe v1alpha1 declarations.
func (descriptor Descriptor) Validate() error {
	if descriptor.APIVersion != APIVersion {
		return fmt.Errorf("unsupported apiVersion %q", descriptor.APIVersion)
	}
	if !validName(moduleNamePattern, descriptor.Name) {
		return fmt.Errorf("invalid module name %q", descriptor.Name)
	}
	if len(descriptor.Version) > maxNameBytes || !semverPattern.MatchString(descriptor.Version) {
		return fmt.Errorf("invalid semantic version %q", descriptor.Version)
	}
	if err := validateModuleNames("required module", descriptor.Name, descriptor.Requires.Modules); err != nil {
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
	}
	return nil
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
