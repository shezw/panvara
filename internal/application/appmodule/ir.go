/*
   Panvara
   internal/application/appmodule/ir.go    2026-07-14
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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

// IRFormatVersion is the persisted canonical Module IR format version.
const IRFormatVersion = 1

type moduleIR struct {
	FormatVersion int               `json:"formatVersion"`
	SpecVersion   string            `json:"specVersion"`
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Labels        map[string]string `json:"labels"`
	Requires      requirementsIR    `json:"requires"`
	Provides      []string          `json:"provides"`
	Conflicts     []string          `json:"conflicts"`
	Resources     []resourceIR      `json:"resources"`
}

type requirementsIR struct {
	Modules      []moduleRequirementIR `json:"modules"`
	Capabilities []string              `json:"capabilities"`
}

type moduleRequirementIR struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type resourceIR struct {
	Name    string            `json:"name"`
	Labels  map[string]string `json:"labels"`
	Fields  []fieldIR         `json:"fields"`
	API     apiIR             `json:"api"`
	Manager managerIR         `json:"manager"`
}

type fieldIR struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	Kind        domain.FieldKind  `json:"kind"`
	Required    bool              `json:"required"`
	Unique      bool              `json:"unique"`
	Target      string            `json:"target,omitempty"`
	Options     []string          `json:"options"`
	Constraints constraintsIR     `json:"constraints"`
}

type constraintsIR struct {
	MaxLength *int `json:"maxLength,omitempty"`
	Precision *int `json:"precision,omitempty"`
	Scale     *int `json:"scale,omitempty"`
}

type apiIR struct {
	Public accessIR `json:"public"`
	Admin  accessIR `json:"admin"`
}

type accessIR struct {
	Operations []domain.Operation `json:"operations"`
	Writable   []string           `json:"writable"`
	Filterable []string           `json:"filterable"`
	Sortable   []string           `json:"sortable"`
}

type managerIR struct {
	List listViewIR `json:"list"`
	Form formViewIR `json:"form"`
}

type listViewIR struct {
	Columns []string `json:"columns"`
	Filters []string `json:"filters"`
}

type formViewIR struct {
	Fields []string `json:"fields"`
}

func canonicalizeDescriptor(descriptor domain.Descriptor) domain.Descriptor {
	result := descriptor.Clone()
	sort.Slice(result.Requires.Modules, func(left, right int) bool {
		if result.Requires.Modules[left].Name != result.Requires.Modules[right].Name {
			return result.Requires.Modules[left].Name < result.Requires.Modules[right].Name
		}
		return result.Requires.Modules[left].Version < result.Requires.Modules[right].Version
	})
	sort.Strings(result.Requires.Capabilities)
	sort.Strings(result.Provides)
	sort.Strings(result.Conflicts)
	sort.Slice(result.Resources, func(left, right int) bool {
		return result.Resources[left].Name < result.Resources[right].Name
	})
	for resourceIndex := range result.Resources {
		resource := &result.Resources[resourceIndex]
		sort.Slice(resource.Fields, func(left, right int) bool {
			return resource.Fields[left].Name < resource.Fields[right].Name
		})
		canonicalizeAccess(&resource.API.Public)
		canonicalizeAccess(&resource.API.Admin)
	}
	return result
}

func canonicalizeAccess(access *domain.Access) {
	sort.Slice(access.Operations, func(left, right int) bool {
		return access.Operations[left] < access.Operations[right]
	})
	sort.Strings(access.Writable)
	sort.Strings(access.Filterable)
	sort.Strings(access.Sortable)
}

func marshalCanonicalIR(descriptor domain.Descriptor) ([]byte, string, error) {
	ir := moduleIR{
		FormatVersion: IRFormatVersion,
		SpecVersion:   spec.APIVersion,
		Name:          descriptor.Name,
		Version:       descriptor.Version,
		Labels:        nonNilMap(descriptor.Labels),
		Requires: requirementsIR{
			Modules:      makeModuleRequirementsIR(descriptor.Requires.Modules),
			Capabilities: nonNilStrings(descriptor.Requires.Capabilities),
		},
		Provides:  nonNilStrings(descriptor.Provides),
		Conflicts: nonNilStrings(descriptor.Conflicts),
		Resources: make([]resourceIR, 0, len(descriptor.Resources)),
	}
	for _, resource := range descriptor.Resources {
		resourceValue := resourceIR{
			Name:   resource.Name,
			Labels: nonNilMap(resource.Labels),
			Fields: make([]fieldIR, 0, len(resource.Fields)),
			API: apiIR{
				Public: makeAccessIR(resource.API.Public),
				Admin:  makeAccessIR(resource.API.Admin),
			},
			Manager: managerIR{
				List: listViewIR{
					Columns: nonNilStrings(resource.Manager.List.Columns),
					Filters: nonNilStrings(resource.Manager.List.Filters),
				},
				Form: formViewIR{Fields: nonNilStrings(resource.Manager.Form.Fields)},
			},
		}
		for _, field := range resource.Fields {
			resourceValue.Fields = append(resourceValue.Fields, fieldIR{
				Name:        field.Name,
				Labels:      nonNilMap(field.Labels),
				Kind:        field.Kind,
				Required:    field.Required,
				Unique:      field.Unique,
				Target:      field.Target,
				Options:     nonNilStrings(field.Options),
				Constraints: makeConstraintsIR(field.Constraints),
			})
		}
		ir.Resources = append(ir.Resources, resourceValue)
	}
	encoded, err := json.Marshal(ir)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return encoded, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func makeAccessIR(access domain.Access) accessIR {
	operations := append([]domain.Operation(nil), access.Operations...)
	if operations == nil {
		operations = []domain.Operation{}
	}
	return accessIR{
		Operations: operations,
		Writable:   nonNilStrings(access.Writable),
		Filterable: nonNilStrings(access.Filterable),
		Sortable:   nonNilStrings(access.Sortable),
	}
}

func nonNilStrings(values []string) []string {
	result := append([]string(nil), values...)
	if result == nil {
		return []string{}
	}
	return result
}

func makeModuleRequirementsIR(values []domain.ModuleRequirement) []moduleRequirementIR {
	result := make([]moduleRequirementIR, 0, len(values))
	for _, value := range values {
		result = append(result, moduleRequirementIR{Name: value.Name, Version: value.Version})
	}
	return result
}

func makeConstraintsIR(value domain.Constraints) constraintsIR {
	return constraintsIR{
		MaxLength: cloneInt(value.MaxLength),
		Precision: cloneInt(value.Precision),
		Scale:     cloneInt(value.Scale),
	}
}

func nonNilMap(values map[string]string) map[string]string {
	result := cloneStringMap(values)
	if result == nil {
		return map[string]string{}
	}
	return result
}
