/*
   Panvara
   internal/application/appmodule/manager_schema.go    2026-07-14
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
	"encoding/json"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

const managerSchemaVersion = "manager.panvara.dev/v1alpha1"

type managerSchema struct {
	Schema    string            `json:"schema"`
	Module    string            `json:"module"`
	Version   string            `json:"version"`
	Revision  string            `json:"revision"`
	Labels    map[string]string `json:"labels"`
	Resources []managerResource `json:"resources"`
}

type managerResource struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
	List   managerList       `json:"list"`
	Form   managerForm       `json:"form"`
}

type managerList struct {
	Columns []managerField `json:"columns"`
	Filters []managerField `json:"filters"`
}

type managerForm struct {
	Fields []managerField `json:"fields"`
}

type managerField struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels"`
	Kind        domain.FieldKind  `json:"kind"`
	Widget      string            `json:"widget"`
	Required    bool              `json:"required"`
	Unique      bool              `json:"unique"`
	ReadOnly    bool              `json:"readOnly"`
	Target      string            `json:"target,omitempty"`
	Options     []string          `json:"options"`
	Constraints constraintsIR     `json:"constraints"`
}

func marshalManagerSchema(descriptor domain.Descriptor, revision string) ([]byte, error) {
	schema := managerSchema{
		Schema:    managerSchemaVersion,
		Module:    descriptor.Name,
		Version:   descriptor.Version,
		Revision:  revision,
		Labels:    nonNilMap(descriptor.Labels),
		Resources: make([]managerResource, 0, len(descriptor.Resources)),
	}
	for _, resource := range descriptor.Resources {
		fields := make(map[string]domain.Field, len(resource.Fields))
		for _, field := range resource.Fields {
			fields[field.Name] = field
		}
		columns := resource.Manager.List.Columns
		if len(columns) == 0 {
			columns = fieldNames(resource.Fields)
		}
		formFields := resource.Manager.Form.Fields
		if len(formFields) == 0 {
			formFields = resource.API.Admin.Writable
		}
		resourceSchema := managerResource{
			Name:   resource.Name,
			Labels: nonNilMap(resource.Labels),
			List: managerList{
				Columns: makeManagerFields(columns, fields),
				Filters: makeManagerFields(resource.Manager.List.Filters, fields),
			},
			Form: managerForm{Fields: makeManagerFields(formFields, fields)},
		}
		schema.Resources = append(schema.Resources, resourceSchema)
	}
	return json.Marshal(schema)
}

func makeManagerFields(names []string, fields map[string]domain.Field) []managerField {
	result := make([]managerField, 0, len(names))
	for _, name := range names {
		field, declared := fields[name]
		if !declared {
			result = append(result, systemManagerField(name))
			continue
		}
		result = append(result, managerField{
			Name:        field.Name,
			Labels:      nonNilMap(field.Labels),
			Kind:        field.Kind,
			Widget:      widgetFor(field.Kind),
			Required:    field.Required,
			Unique:      field.Unique,
			ReadOnly:    false,
			Target:      field.Target,
			Options:     nonNilStrings(field.Options),
			Constraints: makeConstraintsIR(field.Constraints),
		})
	}
	return result
}

func systemManagerField(name string) managerField {
	kind := domain.KindDateTime
	widget := "datetime"
	switch name {
	case "id":
		kind = domain.KindString
		widget = "text"
	case "version":
		kind = domain.KindInt
		widget = "number"
	}
	return managerField{
		Name: name, Labels: map[string]string{}, Kind: kind, Widget: widget,
		Required: true, ReadOnly: true, Options: []string{}, Constraints: constraintsIR{},
	}
}

func fieldNames(fields []domain.Field) []string {
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		result = append(result, field.Name)
	}
	return result
}

func widgetFor(kind domain.FieldKind) string {
	switch kind {
	case domain.KindText:
		return "textarea"
	case domain.KindInt, domain.KindDecimal:
		return "number"
	case domain.KindBool:
		return "switch"
	case domain.KindEnum:
		return "select"
	case domain.KindDate:
		return "date"
	case domain.KindDateTime:
		return "datetime"
	case domain.KindEmail:
		return "email"
	case domain.KindMoney:
		return "money"
	case domain.KindReference:
		return "reference"
	default:
		return "text"
	}
}
