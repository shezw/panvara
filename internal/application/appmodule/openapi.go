/*
   Panvara
   internal/application/appmodule/openapi.go    2026-07-14
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
	"fmt"
	"strings"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

func marshalOpenAPI(descriptor domain.Descriptor, revision string) ([]byte, error) {
	paths := map[string]any{}
	schemas := map[string]any{
		"PanvaraError": map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"code", "message", "request_id"},
			"properties": map[string]any{
				"code":       map[string]any{"type": "string"},
				"message":    map[string]any{"type": "string"},
				"request_id": map[string]any{"type": "string"},
				"details": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object", "additionalProperties": false,
						"required": []string{"code", "path", "message"},
						"properties": map[string]any{
							"code":    map[string]any{"type": "string"},
							"path":    map[string]any{"type": "string"},
							"message": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}
	for _, resource := range descriptor.Resources {
		addOpenAPIResource(paths, schemas, descriptor, resource, "public", resource.API.Public)
		addOpenAPIResource(paths, schemas, descriptor, resource, "admin", resource.API.Admin)
	}
	document := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   descriptor.Name + " generated API",
			"version": descriptor.Version,
		},
		"paths": paths,
		"components": map[string]any{
			"schemas": schemas,
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"},
			},
		},
		"x-panvara-ir-format": IRFormatVersion,
		"x-panvara-module":    descriptor.Name,
		"x-panvara-revision":  revision,
	}
	return json.Marshal(document)
}

func addOpenAPIResource(
	paths map[string]any,
	schemas map[string]any,
	descriptor domain.Descriptor,
	resource domain.Resource,
	surface string,
	access domain.Access,
) {
	if len(access.Operations) == 0 {
		return
	}
	prefix := "/api/public/v1alpha1/"
	if surface == "admin" {
		prefix = "/api/admin/v1alpha1/"
	}
	collectionPath := prefix + descriptor.Name + "/" + resource.Name
	itemPath := collectionPath + "/{id}"
	componentPrefix := componentName(descriptor.Name + "_" + resource.Name + "_" + surface)
	createName := componentPrefix + "Create"
	patchName := componentPrefix + "Patch"
	recordName := componentPrefix + "Record"
	schemas[createName] = objectSchema(resource, access.Writable, true)
	schemas[patchName] = objectSchema(resource, access.Writable, false)
	schemas[recordName] = recordSchema(resource)

	collection := map[string]any{}
	item := map[string]any{}
	for _, operation := range access.Operations {
		switch operation {
		case domain.OperationList:
			collection["get"] = listOperation(descriptor.Name, resource.Name, surface, recordName, access)
		case domain.OperationCreate:
			collection["post"] = writeOperation(
				descriptor.Name, resource.Name, surface, "create", createName, recordName, "201",
			)
		case domain.OperationGet:
			item["get"] = itemReadOperation(descriptor.Name, resource.Name, surface, recordName)
		case domain.OperationPatch:
			item["patch"] = writeOperation(
				descriptor.Name, resource.Name, surface, "patch", patchName, recordName, "200",
			)
		case domain.OperationDelete:
			item["delete"] = deleteOperation(descriptor.Name, resource.Name, surface)
		}
	}
	if len(collection) > 0 {
		paths[collectionPath] = collection
	}
	if len(item) > 0 {
		paths[itemPath] = item
	}
}

func objectSchema(resource domain.Resource, writable []string, create bool) map[string]any {
	fields := make(map[string]domain.Field, len(resource.Fields))
	for _, field := range resource.Fields {
		fields[field.Name] = field
	}
	properties := map[string]any{}
	required := make([]string, 0, len(writable))
	for _, name := range writable {
		field := fields[name]
		properties[name] = fieldJSONSchema(field)
		if create && field.Required {
			required = append(required, name)
		}
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	if !create {
		schema["minProperties"] = 1
	}
	return schema
}

func recordSchema(resource domain.Resource) map[string]any {
	dataProperties := map[string]any{}
	dataRequired := make([]string, 0, len(resource.Fields))
	for _, field := range resource.Fields {
		dataProperties[field.Name] = fieldJSONSchema(field)
		if field.Required {
			dataRequired = append(dataRequired, field.Name)
		}
	}
	dataSchema := map[string]any{
		"type": "object", "additionalProperties": false, "properties": dataProperties,
	}
	if len(dataRequired) > 0 {
		dataSchema["required"] = dataRequired
	}
	properties := map[string]any{
		"id":         map[string]any{"type": "string", "format": "uuid", "x-panvara-uuid-version": 7},
		"version":    map[string]any{"type": "integer", "format": "int64", "minimum": 1},
		"data":       dataSchema,
		"created_at": map[string]any{"type": "string", "format": "date-time"},
		"updated_at": map[string]any{"type": "string", "format": "date-time"},
		"deleted_at": map[string]any{"type": "string", "format": "date-time"},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": properties,
		"required":   []string{"id", "version", "data", "created_at", "updated_at"},
	}
}

func fieldJSONSchema(field domain.Field) map[string]any {
	schema := map[string]any{}
	switch field.Kind {
	case domain.KindInt:
		schema["type"] = "integer"
		schema["format"] = "int64"
	case domain.KindBool:
		schema["type"] = "boolean"
	case domain.KindDecimal:
		schema["type"] = "string"
		schema["pattern"] = `^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`
		if field.Constraints.Precision != nil {
			schema["x-panvara-precision"] = *field.Constraints.Precision
		}
		if field.Constraints.Scale != nil {
			schema["x-panvara-scale"] = *field.Constraints.Scale
		}
	case domain.KindEnum:
		schema["type"] = "string"
		schema["enum"] = nonNilStrings(field.Options)
	case domain.KindDate:
		schema["type"] = "string"
		schema["format"] = "date"
	case domain.KindDateTime:
		schema["type"] = "string"
		schema["format"] = "date-time"
	case domain.KindEmail:
		schema["type"] = "string"
		schema["format"] = "email"
	case domain.KindMoney:
		schema = map[string]any{
			"type": "object", "additionalProperties": false,
			"required": []string{"minor", "currency"},
			"properties": map[string]any{
				"minor":    map[string]any{"type": "integer", "format": "int64"},
				"currency": map[string]any{"type": "string", "pattern": "^[A-Za-z]{3}$"},
			},
		}
	case domain.KindReference:
		schema["type"] = "string"
		schema["format"] = "uuid"
		schema["x-panvara-uuid-version"] = 7
		schema["x-panvara-resource"] = field.Target
	default:
		schema["type"] = "string"
	}
	if field.Constraints.MaxLength != nil {
		schema["maxLength"] = *field.Constraints.MaxLength
	}
	return schema
}

func listOperation(module, resource, surface, recordName string, access domain.Access) map[string]any {
	parameters := make([]any, 0, len(access.Filterable)+2)
	for _, field := range access.Filterable {
		parameters = append(parameters, map[string]any{
			"name": "filter[" + field + "]", "in": "query", "required": false,
			"schema": map[string]any{"type": "string"},
		})
	}
	parameters = append(parameters,
		map[string]any{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}},
		map[string]any{"name": "cursor", "in": "query", "schema": map[string]any{"type": "string"}},
	)
	operation := map[string]any{
		"operationId": operationID(surface, "list", module, resource),
		"tags":        []string{module + "/" + resource},
		"parameters":  parameters,
		"responses": map[string]any{
			"200": response("List records", map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"data"},
				"properties": map[string]any{
					"data":        map[string]any{"type": "array", "items": referenceSchema(recordName)},
					"next_cursor": map[string]any{"type": "string"},
				},
			}),
			"default": errorResponse(),
		},
	}
	applySecurity(operation, surface)
	return operation
}

func writeOperation(module, resource, surface, action, inputName, recordName, status string) map[string]any {
	operation := map[string]any{
		"operationId": operationID(surface, action, module, resource),
		"tags":        []string{module + "/" + resource},
		"requestBody": map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": referenceSchema(inputName)}},
		},
		"responses": map[string]any{
			status:    recordResponse("Record", referenceSchema(recordName)),
			"default": errorResponse(),
		},
	}
	if action == "patch" {
		operation["parameters"] = []any{idParameter(), ifMatchParameter()}
		responses := operation["responses"].(map[string]any)
		responses["412"] = errorResponse()
		responses["428"] = errorResponse()
	}
	applySecurity(operation, surface)
	return operation
}

func itemReadOperation(module, resource, surface, recordName string) map[string]any {
	operation := map[string]any{
		"operationId": operationID(surface, "get", module, resource),
		"tags":        []string{module + "/" + resource},
		"parameters":  []any{idParameter()},
		"responses": map[string]any{
			"200":     recordResponse("Record", referenceSchema(recordName)),
			"default": errorResponse(),
		},
	}
	applySecurity(operation, surface)
	return operation
}

func deleteOperation(module, resource, surface string) map[string]any {
	operation := map[string]any{
		"operationId": operationID(surface, "delete", module, resource),
		"tags":        []string{module + "/" + resource},
		"parameters":  []any{idParameter(), ifMatchParameter()},
		"responses": map[string]any{
			"204":     map[string]any{"description": "Deleted"},
			"412":     errorResponse(),
			"428":     errorResponse(),
			"default": errorResponse(),
		},
	}
	applySecurity(operation, surface)
	return operation
}

func idParameter() map[string]any {
	return map[string]any{
		"name": "id", "in": "path", "required": true,
		"schema": map[string]any{"type": "string", "format": "uuid", "x-panvara-uuid-version": 7},
	}
}

func ifMatchParameter() map[string]any {
	return map[string]any{
		"name": "If-Match", "in": "header", "required": true,
		"description": "Strong ETag returned by the latest record response.",
		"schema":      map[string]any{"type": "string", "pattern": `^\"[1-9][0-9]*\"$`},
	}
}

func response(description string, schema map[string]any) map[string]any {
	return map[string]any{
		"description": description,
		"content":     map[string]any{"application/json": map[string]any{"schema": schema}},
	}
}

func recordResponse(description string, schema map[string]any) map[string]any {
	result := response(description, schema)
	result["headers"] = map[string]any{
		"ETag": map[string]any{
			"description": "Strong record version ETag.",
			"schema":      map[string]any{"type": "string", "pattern": `^\"[1-9][0-9]*\"$`},
		},
	}
	return result
}

func errorResponse() map[string]any {
	return response("Panvara error", referenceSchema("PanvaraError"))
}

func referenceSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func applySecurity(operation map[string]any, surface string) {
	if surface == "admin" {
		operation["security"] = []any{map[string]any{"bearerAuth": []string{}}}
	}
}

func operationID(surface, action, module, resource string) string {
	return componentName(strings.Join([]string{surface, action, module, resource}, "_"))
}

func componentName(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return fmt.Sprintf("Component_%x", value)
	}
	return builder.String()
}
