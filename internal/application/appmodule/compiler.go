/*
   Panvara
   internal/application/appmodule/compiler.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package appmodule compiles author-facing module specifications into stable
// runtime artifacts without giving declarations arbitrary code execution.
package appmodule

import (
	"fmt"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

// Compiler converts a versioned authoring DTO into canonical immutable
// runtime artifacts. Its zero value is ready for use.
type Compiler struct{}

// NewCompiler constructs an AppModule compiler.
func NewCompiler() *Compiler {
	return &Compiler{}
}

// Compile strictly decodes and compiles one explicitly formatted source.
func (compiler *Compiler) Compile(source []byte, format spec.Format) (*CompiledModule, error) {
	document, err := spec.Decode(source, format)
	if err != nil {
		return nil, err
	}
	return compiler.CompileDocument(document)
}

// CompileDocument converts a previously decoded DTO into canonical artifacts.
func (compiler *Compiler) CompileDocument(document spec.Document) (*CompiledModule, error) {
	if document.APIVersion != spec.APIVersion {
		return nil, fmt.Errorf("unsupported apiVersion %q", document.APIVersion)
	}
	if document.Kind != spec.Kind {
		return nil, fmt.Errorf("unsupported kind %q", document.Kind)
	}

	descriptor := convertDocument(document)
	if err := descriptor.Validate(); err != nil {
		return nil, fmt.Errorf("validate appmodule %q: %w", descriptor.Name, err)
	}
	canonicalDescriptor := canonicalizeDescriptor(descriptor)
	canonicalIR, revisionHash, err := marshalCanonicalIR(canonicalDescriptor)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical appmodule IR: %w", err)
	}
	managerSchema, err := marshalManagerSchema(canonicalDescriptor, revisionHash)
	if err != nil {
		return nil, fmt.Errorf("marshal manager UI schema: %w", err)
	}
	openAPI, err := marshalOpenAPI(canonicalDescriptor, revisionHash)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAPI: %w", err)
	}
	return &CompiledModule{
		descriptor:    canonicalDescriptor,
		revisionHash:  revisionHash,
		canonicalIR:   canonicalIR,
		managerSchema: managerSchema,
		openAPI:       openAPI,
	}, nil
}

// CompiledModule is an immutable compilation result. Every byte or descriptor
// accessor returns a defensive copy.
type CompiledModule struct {
	descriptor    domain.Descriptor
	revisionHash  string
	canonicalIR   []byte
	managerSchema []byte
	openAPI       []byte
}

// Name returns the canonical module name.
func (module *CompiledModule) Name() string {
	return module.descriptor.Name
}

// Version returns the module's author-controlled semantic version.
func (module *CompiledModule) Version() string {
	return module.descriptor.Version
}

// RevisionHash returns the sha256-prefixed canonical IR content hash.
func (module *CompiledModule) RevisionHash() string {
	return module.revisionHash
}

// Descriptor returns a deep copy of the canonical domain descriptor.
func (module *CompiledModule) Descriptor() domain.Descriptor {
	return module.descriptor.Clone()
}

// CanonicalIR returns stable JSON bytes for persistence and transport.
func (module *CompiledModule) CanonicalIR() []byte {
	return append([]byte(nil), module.canonicalIR...)
}

// ManagerUISchema returns stable JSON bytes for Manager form/list rendering.
func (module *CompiledModule) ManagerUISchema() []byte {
	return append([]byte(nil), module.managerSchema...)
}

// OpenAPI returns stable OpenAPI 3.1 JSON bytes for generated CRUD surfaces.
func (module *CompiledModule) OpenAPI() []byte {
	return append([]byte(nil), module.openAPI...)
}

func convertDocument(document spec.Document) domain.Descriptor {
	descriptor := domain.Descriptor{
		Name:      document.Metadata.Name,
		Version:   document.Metadata.Version,
		Labels:    cloneStringMap(document.Metadata.Labels),
		Provides:  append([]string(nil), document.Spec.Provides...),
		Conflicts: append([]string(nil), document.Spec.Conflicts...),
		Requires: domain.Requirements{
			Capabilities: append([]string(nil), document.Spec.Requires.Capabilities...),
		},
	}
	for _, requirement := range document.Spec.Requires.Modules {
		descriptor.Requires.Modules = append(descriptor.Requires.Modules, domain.ModuleRequirement{
			Name: requirement.Name, Version: requirement.Version,
		})
	}
	for _, sourceResource := range document.Spec.Resources {
		resource := domain.Resource{
			Name:   sourceResource.Name,
			Labels: cloneStringMap(sourceResource.Labels),
			API: domain.API{
				Public: convertAccess(sourceResource.API.Public),
				Admin:  convertAccess(sourceResource.API.Admin),
			},
			Manager: domain.Manager{
				List: domain.ListView{
					Columns: append([]string(nil), sourceResource.Manager.List.Columns...),
					Filters: append([]string(nil), sourceResource.Manager.List.Filters...),
				},
				Form: domain.FormView{Fields: append([]string(nil), sourceResource.Manager.Form.Fields...)},
			},
		}
		for _, sourceField := range sourceResource.Fields {
			resource.Fields = append(resource.Fields, domain.Field{
				Name:     sourceField.Name,
				Labels:   cloneStringMap(sourceField.Labels),
				Kind:     domain.FieldKind(sourceField.Type),
				Required: sourceField.Required,
				Unique:   sourceField.Unique,
				Target:   sourceField.Target,
				Options:  append([]string(nil), sourceField.Options...),
				Constraints: domain.Constraints{
					MaxLength: cloneInt(sourceField.Constraints.MaxLength),
					Precision: cloneInt(sourceField.Constraints.Precision),
					Scale:     cloneInt(sourceField.Constraints.Scale),
				},
			})
		}
		descriptor.Resources = append(descriptor.Resources, resource)
	}
	return descriptor
}

func convertAccess(access spec.Access) domain.Access {
	result := domain.Access{
		Writable:   append([]string(nil), access.Writable...),
		Filterable: append([]string(nil), access.Filterable...),
		Sortable:   append([]string(nil), access.Sortable...),
	}
	for _, operation := range access.Operations {
		result.Operations = append(result.Operations, domain.Operation(operation))
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
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
