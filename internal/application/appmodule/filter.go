/*
   Panvara
   internal/application/appmodule/filter.go    2026-07-14
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
	"strconv"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

// NormalizeFilterValue validates one model-declared scalar equality filter and
// returns the exact text representation produced by PostgreSQL JSONB ->>.
func (module *CompiledModule) NormalizeFilterValue(
	resourceName string,
	fieldName string,
	raw string,
) (string, error) {
	resource, found := module.resource(resourceName)
	if !found {
		return "", newValidationError(Violation{
			Code: "unknown_resource", Path: "$", Message: "resource is not declared by the module",
		})
	}
	if !containsString(resource.API.Public.Filterable, fieldName) &&
		!containsString(resource.API.Admin.Filterable, fieldName) {
		return "", newValidationError(Violation{
			Code: "filter_not_allowed", Path: "$.filter." + fieldName,
			Message: "field is not declared filterable",
		})
	}
	var field domain.Field
	found = false
	for _, candidate := range resource.Fields {
		if candidate.Name == fieldName {
			field = candidate
			found = true
			break
		}
	}
	if !found || field.Kind == domain.KindText || field.Kind == domain.KindMoney {
		return "", newValidationError(Violation{
			Code: "invalid_filter", Path: "$.filter." + fieldName,
			Message: "field does not support scalar equality filters",
		})
	}

	input := any(raw)
	switch field.Kind {
	case domain.KindInt:
		input = json.Number(raw)
	case domain.KindBool:
		if raw != "true" && raw != "false" {
			return "", newValidationError(Violation{
				Code: "invalid_filter", Path: "$.filter." + fieldName,
				Message: "boolean filter must be true or false",
			})
		}
		input = raw == "true"
	}
	normalized, violation := normalizeField(field, input, "$.filter."+fieldName)
	if violation != nil {
		return "", newValidationError(*violation)
	}
	switch value := normalized.(type) {
	case string:
		return value, nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case bool:
		return strconv.FormatBool(value), nil
	default:
		return "", newValidationError(Violation{
			Code: "invalid_filter", Path: "$.filter." + fieldName,
			Message: "normalized filter is not scalar text",
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
