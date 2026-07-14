/*
   Panvara
   internal/domain/appmodule/policy.go    2026-07-14
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

import "fmt"

const maxTextLength = 1 << 20

const maxUniqueCanonicalBytes = 512

var supportedOperations = map[Operation]struct{}{
	OperationList: {}, OperationGet: {}, OperationCreate: {},
	OperationPatch: {}, OperationDelete: {},
}

var readableSystemFields = map[string]struct{}{
	"id": {}, "version": {}, "created_at": {}, "updated_at": {},
}

func validateConstraints(resource string, field Field) error {
	constraints := field.Constraints
	if constraints.MaxLength != nil {
		if field.Kind != KindString && field.Kind != KindText && field.Kind != KindEmail {
			return fmt.Errorf("field %q.%q maxLength is incompatible with kind %q", resource, field.Name, field.Kind)
		}
		if *constraints.MaxLength < 1 || *constraints.MaxLength > maxTextLength {
			return fmt.Errorf("field %q.%q maxLength must be between 1 and %d", resource, field.Name, maxTextLength)
		}
	}

	if constraints.Precision == nil && constraints.Scale == nil {
		return validateUniqueConstraint(resource, field)
	}
	if field.Kind != KindDecimal {
		return fmt.Errorf("field %q.%q decimal constraints are incompatible with kind %q", resource, field.Name, field.Kind)
	}
	if constraints.Precision == nil {
		return fmt.Errorf("field %q.%q scale requires precision", resource, field.Name)
	}
	if *constraints.Precision < 1 || *constraints.Precision > 38 {
		return fmt.Errorf("field %q.%q precision must be between 1 and 38", resource, field.Name)
	}
	if constraints.Scale != nil && (*constraints.Scale < 0 || *constraints.Scale > *constraints.Precision) {
		return fmt.Errorf("field %q.%q scale must be between 0 and precision", resource, field.Name)
	}
	return validateUniqueConstraint(resource, field)
}

func validateUniqueConstraint(resource string, field Field) error {
	if !field.Unique {
		return nil
	}
	if field.Kind == KindText {
		return fmt.Errorf("field %q.%q text values cannot be unique", resource, field.Name)
	}
	if field.Kind == KindString || field.Kind == KindEmail {
		if field.Constraints.MaxLength == nil {
			return fmt.Errorf("unique field %q.%q requires maxLength", resource, field.Name)
		}
		if *field.Constraints.MaxLength > maxUniqueCanonicalBytes {
			return fmt.Errorf(
				"unique field %q.%q maxLength exceeds canonical %d-byte limit",
				resource,
				field.Name,
				maxUniqueCanonicalBytes,
			)
		}
	}
	return nil
}

func validateResourceConfiguration(resource Resource, fields map[string]Field) error {
	fieldNames := make(map[string]struct{}, len(fields))
	for name := range fields {
		fieldNames[name] = struct{}{}
	}
	if err := validateAccess(resource.Name+" public API", resource.API.Public, fields); err != nil {
		return err
	}
	for _, operation := range resource.API.Public.Operations {
		if operation != OperationCreate {
			return fmt.Errorf("%s public API operation %q is not implemented in alpha.2", resource.Name, operation)
		}
	}
	if err := validateAccess(resource.Name+" admin API", resource.API.Admin, fields); err != nil {
		return err
	}
	if err := validateFieldReferences(resource.Name+" manager list columns", resource.Manager.List.Columns, fieldNames, true); err != nil {
		return err
	}
	if err := validateFieldReferences(resource.Name+" manager list filters", resource.Manager.List.Filters, fieldNames, true); err != nil {
		return err
	}
	if err := validateFieldReferences(resource.Name+" manager form fields", resource.Manager.Form.Fields, fieldNames, false); err != nil {
		return err
	}
	if err := requireSubset(
		resource.Name+" manager list filters",
		resource.Manager.List.Filters,
		resource.API.Admin.Filterable,
	); err != nil {
		return err
	}
	return requireSubset(
		resource.Name+" manager form fields",
		resource.Manager.Form.Fields,
		resource.API.Admin.Writable,
	)
}

func validateAccess(owner string, access Access, fields map[string]Field) error {
	fieldNames := make(map[string]struct{}, len(fields))
	for name := range fields {
		fieldNames[name] = struct{}{}
	}
	operations := make(map[Operation]struct{}, len(access.Operations))
	for _, operation := range access.Operations {
		if _, supported := supportedOperations[operation]; !supported {
			return fmt.Errorf("%s has unsupported operation %q", owner, operation)
		}
		if _, exists := operations[operation]; exists {
			return fmt.Errorf("%s has duplicate operation %q", owner, operation)
		}
		operations[operation] = struct{}{}
	}

	if err := validateFieldReferences(owner+" writable fields", access.Writable, fieldNames, false); err != nil {
		return err
	}
	if err := validateFieldReferences(owner+" filterable fields", access.Filterable, fieldNames, false); err != nil {
		return err
	}
	for _, name := range access.Filterable {
		kind := fields[name].Kind
		if kind == KindText || kind == KindMoney {
			return fmt.Errorf("%s field %q of kind %q cannot use scalar equality filters", owner, name, kind)
		}
	}
	if len(access.Sortable) > 0 {
		return fmt.Errorf("%s declares sortable fields; dynamic sort is not implemented in alpha.2", owner)
	}

	_, canCreate := operations[OperationCreate]
	_, canPatch := operations[OperationPatch]
	if len(access.Writable) > 0 && !canCreate && !canPatch {
		return fmt.Errorf("%s declares writable fields without create or patch", owner)
	}
	_, canList := operations[OperationList]
	if len(access.Filterable) > 0 && !canList {
		return fmt.Errorf("%s declares filterable fields without list", owner)
	}
	if canCreate {
		writable := make(map[string]struct{}, len(access.Writable))
		for _, name := range access.Writable {
			writable[name] = struct{}{}
		}
		for name, field := range fields {
			if field.Required {
				if _, allowed := writable[name]; !allowed {
					return fmt.Errorf("%s create cannot supply required field %q", owner, name)
				}
			}
		}
	}
	return nil
}

func validateFieldReferences(
	owner string,
	references []string,
	fieldNames map[string]struct{},
	allowSystem bool,
) error {
	if len(references) > maxFields {
		return fmt.Errorf("%s has %d entries; maximum is %d", owner, len(references), maxFields)
	}
	seen := make(map[string]struct{}, len(references))
	for _, name := range references {
		if _, exists := fieldNames[name]; !exists {
			_, system := readableSystemFields[name]
			if !allowSystem || !system {
				return fmt.Errorf("%s references unknown field %q", owner, name)
			}
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("%s has duplicate field %q", owner, name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func requireSubset(owner string, values, allowed []string) error {
	allowlist := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowlist[value] = struct{}{}
	}
	for _, value := range values {
		if _, allowed := allowlist[value]; !allowed {
			return fmt.Errorf("%s field %q is not allowed by the admin API", owner, value)
		}
	}
	return nil
}
