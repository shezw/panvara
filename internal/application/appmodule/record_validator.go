/*
   Panvara
   internal/application/appmodule/record_validator.go    2026-07-14
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
	"math"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/value"
)

var (
	decimalPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
	uuidV7Pattern  = regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	systemFields = map[string]struct{}{
		"id": {}, "version": {}, "project_id": {}, "owner_id": {}, "revision": {},
		"created_at": {}, "updated_at": {}, "deleted_at": {},
	}
)

// Surface selects the fail-closed API allowlist used for record validation.
type Surface string

const (
	// SurfacePublic applies the public resource policy.
	SurfacePublic Surface = "public"
	// SurfaceAdmin applies the administrative resource policy.
	SurfaceAdmin Surface = "admin"
)

// Mutation selects a model-driven write operation.
type Mutation string

const (
	// MutationCreate validates a complete create payload.
	MutationCreate Mutation = "create"
	// MutationPatch validates a non-empty partial update payload.
	MutationPatch Mutation = "patch"
)

// Violation is one stable, field-addressable validation failure.
type Violation struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError contains deterministically ordered model violations.
type ValidationError struct {
	Violations []Violation `json:"violations"`
}

// UniqueValue is one model-declared unique index entry with stable canonical
// JSON suitable for the record application adapter.
type UniqueValue struct {
	Field          string
	CanonicalValue string
}

// ReferenceValue is one normalized same-project, same-module reference.
type ReferenceValue struct {
	Field          string
	TargetResource string
	TargetID       string
}

// ValidatedRecord is a complete normalized record and all derived indexes.
type ValidatedRecord struct {
	Data       map[string]any
	Uniques    []UniqueValue
	References []ReferenceValue
}

// Error implements error without exposing submitted field values.
func (validation *ValidationError) Error() string {
	if validation == nil || len(validation.Violations) == 0 {
		return "record validation failed"
	}
	first := validation.Violations[0]
	return fmt.Sprintf("record validation failed at %s: %s", first.Path, first.Message)
}

// ValidateRecord validates and normalizes a model-driven create or patch. It
// rejects null, unknown/system fields, non-writable fields, and operations not
// enabled for the selected surface. Input maps are never mutated.
func (module *CompiledModule) ValidateRecord(
	resourceName string,
	surface Surface,
	mutation Mutation,
	input map[string]any,
) (map[string]any, error) {
	resource, found := module.resource(resourceName)
	if !found {
		return nil, newValidationError(Violation{
			Code: "unknown_resource", Path: "$", Message: "resource is not declared by the module",
		})
	}
	access, ok := accessForSurface(resource, surface)
	if !ok {
		return nil, newValidationError(Violation{
			Code: "unsupported_surface", Path: "$", Message: "API surface is not supported",
		})
	}
	operation, ok := operationForMutation(mutation)
	if !ok {
		return nil, newValidationError(Violation{
			Code: "unsupported_mutation", Path: "$", Message: "record mutation is not supported",
		})
	}
	if !containsOperation(access.Operations, operation) {
		return nil, newValidationError(Violation{
			Code: "operation_not_allowed", Path: "$", Message: "record mutation is not allowed on this surface",
		})
	}
	if input == nil {
		input = map[string]any{}
	}

	fields := make(map[string]domain.Field, len(resource.Fields))
	for _, field := range resource.Fields {
		fields[field.Name] = field
	}
	writable := make(map[string]struct{}, len(access.Writable))
	for _, name := range access.Writable {
		writable[name] = struct{}{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalized := make(map[string]any, len(input))
	violations := make([]Violation, 0)
	for _, key := range keys {
		path := "$." + key
		field, declared := fields[key]
		if !declared {
			code := "unknown_field"
			message := "field is not declared by the resource"
			if _, system := systemFields[key]; system {
				code = "system_field"
				message = "system-managed field cannot be written"
			}
			violations = append(violations, Violation{Code: code, Path: path, Message: message})
			continue
		}
		if _, allowed := writable[key]; !allowed {
			violations = append(violations, Violation{
				Code: "field_not_writable", Path: path, Message: "field is not writable on this surface",
			})
			continue
		}
		if input[key] == nil {
			violations = append(violations, Violation{
				Code: "null_not_allowed", Path: path, Message: "null is not supported",
			})
			continue
		}
		value, violation := normalizeField(field, input[key], path)
		if violation != nil {
			violations = append(violations, *violation)
			continue
		}
		normalized[key] = value
	}

	if mutation == MutationCreate {
		for _, field := range resource.Fields {
			if !field.Required {
				continue
			}
			if _, present := input[field.Name]; !present {
				violations = append(violations, Violation{
					Code: "required", Path: "$." + field.Name, Message: "required field is missing",
				})
			}
		}
	} else if len(input) == 0 {
		violations = append(violations, Violation{
			Code: "empty_patch", Path: "$", Message: "patch must contain at least one field",
		})
	}
	if len(violations) > 0 {
		return nil, newValidationError(violations...)
	}
	return normalized, nil
}

// ValidateRecordJSON decodes strict JSON, then validates and normalizes it.
func (module *CompiledModule) ValidateRecordJSON(
	resource string,
	surface Surface,
	mutation Mutation,
	source []byte,
) (map[string]any, error) {
	input, err := DecodeRecordJSON(source)
	if err != nil {
		return nil, err
	}
	return module.ValidateRecord(resource, surface, mutation, input)
}

// ValidateCompleteRecord validates a full merged record without reapplying a
// surface writable allowlist. Runtime adapters use it after ValidateRecord has
// authorized a patch, so unchanged fields cannot be mistaken for patch input.
func (module *CompiledModule) ValidateCompleteRecord(
	resourceName string,
	input map[string]any,
) (ValidatedRecord, error) {
	resource, found := module.resource(resourceName)
	if !found {
		return ValidatedRecord{}, newValidationError(Violation{
			Code: "unknown_resource", Path: "$", Message: "resource is not declared by the module",
		})
	}
	if input == nil {
		input = map[string]any{}
	}
	fields := make(map[string]domain.Field, len(resource.Fields))
	for _, field := range resource.Fields {
		fields[field.Name] = field
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := ValidatedRecord{Data: make(map[string]any, len(input))}
	violations := make([]Violation, 0)
	for _, key := range keys {
		path := "$." + key
		field, declared := fields[key]
		if !declared {
			code := "unknown_field"
			message := "field is not declared by the resource"
			if _, system := systemFields[key]; system {
				code = "system_field"
				message = "system-managed field cannot be written"
			}
			violations = append(violations, Violation{Code: code, Path: path, Message: message})
			continue
		}
		if input[key] == nil {
			violations = append(violations, Violation{
				Code: "null_not_allowed", Path: path, Message: "null is not supported",
			})
			continue
		}
		normalized, violation := normalizeField(field, input[key], path)
		if violation != nil {
			violations = append(violations, *violation)
			continue
		}
		result.Data[key] = normalized
		if field.Unique {
			canonical, err := json.Marshal(normalized)
			if err != nil || len(canonical) > 512 {
				violations = append(violations, Violation{
					Code: "unique_value_too_large", Path: path,
					Message: "unique canonical value exceeds 512 bytes",
				})
				continue
			}
			result.Uniques = append(result.Uniques, UniqueValue{
				Field: key, CanonicalValue: string(canonical),
			})
		}
		if field.Kind == domain.KindReference {
			result.References = append(result.References, ReferenceValue{
				Field: key, TargetResource: field.Target, TargetID: normalized.(string),
			})
		}
	}
	for _, field := range resource.Fields {
		if field.Required {
			if _, present := input[field.Name]; !present {
				violations = append(violations, Violation{
					Code: "required", Path: "$." + field.Name, Message: "required field is missing",
				})
			}
		}
	}
	if len(violations) > 0 {
		return ValidatedRecord{}, newValidationError(violations...)
	}
	if result.Uniques == nil {
		result.Uniques = []UniqueValue{}
	}
	if result.References == nil {
		result.References = []ReferenceValue{}
	}
	return result, nil
}

func (module *CompiledModule) resource(name string) (domain.Resource, bool) {
	for _, resource := range module.descriptor.Resources {
		if resource.Name == name {
			copyDescriptor := domain.Descriptor{Resources: []domain.Resource{resource}}.Clone()
			return copyDescriptor.Resources[0], true
		}
	}
	return domain.Resource{}, false
}

func accessForSurface(resource domain.Resource, surface Surface) (domain.Access, bool) {
	switch surface {
	case SurfacePublic:
		return resource.API.Public, true
	case SurfaceAdmin:
		return resource.API.Admin, true
	default:
		return domain.Access{}, false
	}
}

func operationForMutation(mutation Mutation) (domain.Operation, bool) {
	switch mutation {
	case MutationCreate:
		return domain.OperationCreate, true
	case MutationPatch:
		return domain.OperationPatch, true
	default:
		return "", false
	}
}

func containsOperation(operations []domain.Operation, target domain.Operation) bool {
	for _, operation := range operations {
		if operation == target {
			return true
		}
	}
	return false
}

func newValidationError(violations ...Violation) *ValidationError {
	sort.SliceStable(violations, func(left, right int) bool {
		if violations[left].Path != violations[right].Path {
			return violations[left].Path < violations[right].Path
		}
		return violations[left].Code < violations[right].Code
	})
	return &ValidationError{Violations: violations}
}

func normalizeField(field domain.Field, input any, path string) (any, *Violation) {
	switch field.Kind {
	case domain.KindString, domain.KindText:
		return normalizeString(field, input, path)
	case domain.KindInt:
		value, ok := normalizeInt64(input)
		if !ok {
			return nil, invalidType(path, "expected a signed 64-bit integer")
		}
		return value, nil
	case domain.KindBool:
		value, ok := input.(bool)
		if !ok {
			return nil, invalidType(path, "expected a boolean")
		}
		return value, nil
	case domain.KindDecimal:
		return normalizeDecimal(field, input, path)
	case domain.KindEnum:
		value, ok := input.(string)
		if !ok {
			return nil, invalidType(path, "expected an enum string")
		}
		for _, option := range field.Options {
			if value == option {
				return value, nil
			}
		}
		return nil, invalidValue(path, "value is not an allowed enum option")
	case domain.KindDate:
		value, ok := input.(string)
		if !ok {
			return nil, invalidType(path, "expected an ISO 8601 date string")
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return nil, invalidValue(path, "date must use YYYY-MM-DD")
		}
		return value, nil
	case domain.KindDateTime:
		value, ok := input.(string)
		if !ok {
			return nil, invalidType(path, "expected an RFC 3339 timestamp")
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, invalidValue(path, "timestamp must use RFC 3339")
		}
		return parsed.UTC().Format(time.RFC3339Nano), nil
	case domain.KindEmail:
		return normalizeEmail(field, input, path)
	case domain.KindMoney:
		return normalizeMoney(input, path)
	case domain.KindReference:
		value, ok := input.(string)
		if !ok {
			return nil, invalidType(path, "expected a UUIDv7 reference string")
		}
		value = strings.ToLower(value)
		if !uuidV7Pattern.MatchString(value) {
			return nil, invalidValue(path, "reference must be a UUIDv7 in the current project and module")
		}
		return value, nil
	default:
		return nil, invalidValue(path, "field kind is not supported by the runtime")
	}
}

func normalizeString(field domain.Field, input any, path string) (any, *Violation) {
	value, ok := input.(string)
	if !ok {
		return nil, invalidType(path, "expected a string")
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return nil, invalidValue(path, "string must be NUL-free valid UTF-8")
	}
	if field.Constraints.MaxLength != nil && utf8.RuneCountInString(value) > *field.Constraints.MaxLength {
		return nil, &Violation{Code: "too_long", Path: path, Message: "string exceeds maxLength"}
	}
	if field.Unique && len(value) > 512 {
		return nil, &Violation{Code: "unique_value_too_large", Path: path, Message: "unique value exceeds 512 bytes"}
	}
	return value, nil
}

func normalizeEmail(field domain.Field, input any, path string) (any, *Violation) {
	value, ok := input.(string)
	if !ok {
		return nil, invalidType(path, "expected an email address string")
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return nil, invalidValue(path, "email must be NUL-free valid UTF-8")
	}
	if field.Constraints.MaxLength != nil && utf8.RuneCountInString(value) > *field.Constraints.MaxLength {
		return nil, &Violation{Code: "too_long", Path: path, Message: "email exceeds maxLength"}
	}
	if field.Unique && len(value) > 512 {
		return nil, &Violation{Code: "unique_value_too_large", Path: path, Message: "unique value exceeds 512 bytes"}
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Name != "" || address.Address != value {
		return nil, invalidValue(path, "email address is invalid")
	}
	at := strings.LastIndexByte(value, '@')
	if at <= 0 || at == len(value)-1 {
		return nil, invalidValue(path, "email address is invalid")
	}
	return value[:at+1] + strings.ToLower(value[at+1:]), nil
}

func normalizeDecimal(field domain.Field, input any, path string) (any, *Violation) {
	value, ok := input.(string)
	if !ok {
		return nil, invalidType(path, "decimal must be encoded as a string")
	}
	if !decimalPattern.MatchString(value) {
		return nil, invalidValue(path, "decimal string has an invalid format")
	}
	unsigned := strings.TrimPrefix(value, "-")
	integer, fraction, hasFraction := strings.Cut(unsigned, ".")
	significantInteger := strings.TrimLeft(integer, "0")
	precision := len(significantInteger)
	if hasFraction {
		precision += len(fraction)
	}
	if precision == 0 {
		precision = 1
	}
	if field.Constraints.Precision != nil && precision > *field.Constraints.Precision {
		return nil, &Violation{Code: "out_of_range", Path: path, Message: "decimal exceeds precision"}
	}
	if field.Constraints.Scale != nil && len(fraction) > *field.Constraints.Scale {
		return nil, &Violation{Code: "out_of_range", Path: path, Message: "decimal exceeds scale"}
	}
	if hasFraction {
		fraction = strings.TrimRight(fraction, "0")
	}
	normalized := integer
	if fraction != "" {
		normalized += "." + fraction
	}
	if strings.Trim(normalized, "0.") == "" {
		return "0", nil
	}
	if strings.HasPrefix(value, "-") {
		normalized = "-" + normalized
	}
	return normalized, nil
}

func normalizeMoney(input any, path string) (any, *Violation) {
	object, ok := input.(map[string]any)
	if !ok {
		return nil, invalidType(path, "money must be an object with minor and currency")
	}
	if len(object) != 2 {
		return nil, invalidValue(path, "money must contain only minor and currency")
	}
	minorInput, hasMinor := object["minor"]
	currencyInput, hasCurrency := object["currency"]
	if !hasMinor || !hasCurrency || minorInput == nil || currencyInput == nil {
		return nil, invalidValue(path, "money requires non-null minor and currency")
	}
	minor, ok := normalizeInt64(minorInput)
	if !ok {
		return nil, invalidType(path+".minor", "money minor must be a signed 64-bit integer")
	}
	currencyString, ok := currencyInput.(string)
	if !ok {
		return nil, invalidType(path+".currency", "money currency must be a string")
	}
	if len(currencyString) != 3 {
		return nil, invalidValue(path+".currency", "money currency must be a three-letter code")
	}
	for _, char := range currencyString {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return nil, invalidValue(path+".currency", "money currency must be a three-letter code")
		}
	}
	currency, err := value.ParseCurrency(currencyString)
	if err != nil {
		return nil, invalidValue(path+".currency", "money currency must be a three-letter code")
	}
	return map[string]any{"minor": minor, "currency": currency.String()}, nil
}

func normalizeInt64(input any) (int64, bool) {
	switch value := input.(type) {
	case json.Number:
		parsed, err := value.Int64()
		return parsed, err == nil
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		if uint64(value) <= math.MaxInt64 {
			return int64(value), true
		}
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value <= math.MaxInt64 {
			return int64(value), true
		}
	}
	return 0, false
}

func invalidType(path, message string) *Violation {
	return &Violation{Code: "invalid_type", Path: path, Message: message}
}

func invalidValue(path, message string) *Violation {
	return &Violation{Code: "invalid_value", Path: path, Message: message}
}
