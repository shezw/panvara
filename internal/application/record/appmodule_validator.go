/*
   Panvara
   internal/application/record/appmodule_validator.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package record

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/shezw/panvara/internal/application/appmodule"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

var _ Validator = (*CompiledModuleValidator)(nil)

// CompiledModuleValidator binds one immutable compiled module revision and one
// explicit Record persistence namespace to the generic record service.
type CompiledModuleValidator struct {
	module    *appmodule.CompiledModule
	namespace string
}

// AuthorizeOperation checks the resource operation allowlist for the exact
// immutable module revision selected by a record use case.
func (validator *CompiledModuleValidator) AuthorizeOperation(
	ctx context.Context,
	input OperationValidationInput,
) error {
	if validator == nil || validator.module == nil {
		return fmt.Errorf("%w: uninitialized compiled module validator", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := input.Scope.Validate(); err != nil {
		return err
	}
	if !input.Surface.Valid() || !input.Operation.Valid() {
		return fmt.Errorf("%w: invalid record policy context", ErrInvalidArgument)
	}
	if input.Scope.ModuleName != validator.module.Name() ||
		input.Scope.RevisionHash != validator.namespace {
		return fmt.Errorf("%w: compiled module or record namespace does not match record scope", ErrInvalidArgument)
	}
	domainOperation, ok := compiledOperation(input.Operation)
	if !ok {
		return fmt.Errorf("%w: unsupported record operation", ErrInvalidArgument)
	}
	_, policy, found := compiledListAccess(
		validator.module.Descriptor(), input.Scope.ResourceName, input.Surface,
	)
	if !found || !containsDomainOperation(policy.Operations, domainOperation) {
		return ErrOperationForbidden
	}
	return nil
}

// ValidateList authorizes declared equality filters for one API surface and
// normalizes each scalar to PostgreSQL jsonb ->> text representation.
func (validator *CompiledModuleValidator) ValidateList(
	ctx context.Context,
	input ListValidationInput,
) ([]ListFilter, error) {
	if validator == nil || validator.module == nil {
		return nil, fmt.Errorf("%w: uninitialized compiled module validator", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := input.Scope.Validate(); err != nil {
		return nil, err
	}
	if input.Scope.ModuleName != validator.module.Name() ||
		input.Scope.RevisionHash != validator.namespace {
		return nil, fmt.Errorf("%w: compiled module or record namespace does not match list scope", ErrInvalidArgument)
	}
	if len(input.Filters) > MaxListFilters {
		return nil, fmt.Errorf("%w: too many list filters", ErrInvalidArgument)
	}
	if err := validator.AuthorizeOperation(ctx, OperationValidationInput{
		Scope: input.Scope, Surface: input.Surface, Operation: OperationList,
	}); err != nil {
		return nil, err
	}

	resource, access, found := compiledListAccess(
		validator.module.Descriptor(), input.Scope.ResourceName, input.Surface,
	)
	if !found || !containsDomainOperation(access.Operations, domain.OperationList) {
		return nil, fmt.Errorf("%w: list operation is not allowed", ErrInvalidArgument)
	}
	allowed := make(map[string]struct{}, len(access.Filterable))
	for _, field := range access.Filterable {
		allowed[field] = struct{}{}
	}
	result := make([]ListFilter, 0, len(input.Filters))
	seen := make(map[string]struct{}, len(input.Filters))
	for _, filter := range input.Filters {
		if _, ok := allowed[filter.Field]; !ok {
			return nil, fmt.Errorf("%w: field %q is not filterable", ErrInvalidArgument, filter.Field)
		}
		if _, duplicate := seen[filter.Field]; duplicate {
			return nil, fmt.Errorf("%w: duplicate filter field %q", ErrInvalidArgument, filter.Field)
		}
		seen[filter.Field] = struct{}{}
		normalized, err := validator.module.NormalizeFilterValue(resource.Name, filter.Field, filter.Value)
		if err != nil {
			return nil, err
		}
		result = append(result, ListFilter{Field: filter.Field, Value: normalized})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Field < result[right].Field })
	return result, nil
}

// NewCompiledModuleValidator constructs the legacy adapter whose runtime
// revision is also its Record persistence namespace.
func NewCompiledModuleValidator(module *appmodule.CompiledModule) (*CompiledModuleValidator, error) {
	if module == nil {
		return NewCompiledModuleValidatorForNamespace(nil, "")
	}
	return NewCompiledModuleValidatorForNamespace(module, module.RevisionHash())
}

// NewCompiledModuleValidatorForNamespace binds runtime validation to an
// explicit Record namespace. Compatible runtime revisions can therefore retain
// their existing records without pretending that the namespace is the active
// module revision.
func NewCompiledModuleValidatorForNamespace(
	module *appmodule.CompiledModule,
	namespace string,
) (*CompiledModuleValidator, error) {
	if module == nil {
		return nil, fmt.Errorf("%w: nil compiled module", ErrInvalidArgument)
	}
	if !domain.ValidContentHash(namespace) {
		return nil, fmt.Errorf("%w: invalid record namespace revision", ErrInvalidArgument)
	}
	return &CompiledModuleValidator{module: module, namespace: namespace}, nil
}

// Validate implements Validator with a two-stage, fail-closed policy:
// authorize only submitted fields, then validate and index the complete record.
func (validator *CompiledModuleValidator) Validate(
	ctx context.Context,
	input ValidationInput,
) (ValidatedData, error) {
	if validator == nil || validator.module == nil {
		return ValidatedData{}, fmt.Errorf("%w: uninitialized compiled module validator", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return ValidatedData{}, err
	}
	if err := input.Scope.Validate(); err != nil {
		return ValidatedData{}, err
	}
	if input.Scope.ModuleName != validator.module.Name() ||
		input.Scope.RevisionHash != validator.namespace {
		return ValidatedData{}, fmt.Errorf("%w: compiled module or record namespace does not match record scope", ErrInvalidArgument)
	}
	surface, ok := compiledSurface(input.Surface)
	if !ok {
		return ValidatedData{}, fmt.Errorf("%w: unsupported write surface", ErrInvalidArgument)
	}
	mutation, ok := compiledMutation(input.Mutation)
	if !ok {
		return ValidatedData{}, fmt.Errorf("%w: unsupported record mutation", ErrInvalidArgument)
	}

	completeInput, err := appmodule.DecodeRecordJSON(input.Data)
	if err != nil {
		return ValidatedData{}, fmt.Errorf("%w: record JSON could not be decoded: %v", ErrInvalidArgument, err)
	}
	switch input.Mutation {
	case MutationCreate:
		if len(input.ExistingData) != 0 || len(input.PatchData) != 0 {
			return ValidatedData{}, fmt.Errorf("%w: create cannot carry patch state", ErrInvalidArgument)
		}
		authorized, err := validator.module.ValidateRecord(
			input.Scope.ResourceName,
			surface,
			mutation,
			completeInput,
		)
		if err != nil {
			return ValidatedData{}, err
		}
		completeInput = authorized
	case MutationPatch:
		patchInput, err := appmodule.DecodeRecordJSON(input.PatchData)
		if err != nil {
			return ValidatedData{}, fmt.Errorf("%w: record patch JSON could not be decoded: %v", ErrInvalidArgument, err)
		}
		if _, err := validator.module.ValidateRecord(
			input.Scope.ResourceName,
			surface,
			mutation,
			patchInput,
		); err != nil {
			return ValidatedData{}, err
		}
		existingInput, err := appmodule.DecodeRecordJSON(input.ExistingData)
		if err != nil {
			return ValidatedData{}, fmt.Errorf("decode existing record: %w", err)
		}
		if !matchesTopLevelMerge(existingInput, patchInput, completeInput) {
			return ValidatedData{}, fmt.Errorf("%w: complete patch result does not match existing data and patch", ErrInvalidArgument)
		}
	default:
		return ValidatedData{}, fmt.Errorf("%w: unsupported record mutation", ErrInvalidArgument)
	}

	complete, err := validator.module.ValidateCompleteRecord(input.Scope.ResourceName, completeInput)
	if err != nil {
		return ValidatedData{}, err
	}
	data, err := json.Marshal(complete.Data)
	if err != nil {
		return ValidatedData{}, fmt.Errorf("encode complete validated record: %w", err)
	}
	result := ValidatedData{Data: json.RawMessage(data)}
	for _, unique := range complete.Uniques {
		result.Uniques = append(result.Uniques, UniqueValue{
			Field: unique.Field, CanonicalValue: unique.CanonicalValue,
		})
	}
	for _, reference := range complete.References {
		targetID, err := ParseID(reference.TargetID)
		if err != nil {
			return ValidatedData{}, fmt.Errorf("parse validated reference target: %w", err)
		}
		result.References = append(result.References, Reference{
			Field:          reference.Field,
			TargetResource: reference.TargetResource,
			TargetID:       targetID,
		})
	}
	return result, nil
}

func compiledSurface(surface Surface) (appmodule.Surface, bool) {
	switch surface {
	case SurfacePublic:
		return appmodule.SurfacePublic, true
	case SurfaceAdmin:
		return appmodule.SurfaceAdmin, true
	default:
		return "", false
	}
}

func compiledMutation(mutation Mutation) (appmodule.Mutation, bool) {
	switch mutation {
	case MutationCreate:
		return appmodule.MutationCreate, true
	case MutationPatch:
		return appmodule.MutationPatch, true
	default:
		return "", false
	}
}

func compiledOperation(operation Operation) (domain.Operation, bool) {
	switch operation {
	case OperationCreate:
		return domain.OperationCreate, true
	case OperationGet:
		return domain.OperationGet, true
	case OperationList:
		return domain.OperationList, true
	case OperationPatch:
		return domain.OperationPatch, true
	case OperationDelete:
		return domain.OperationDelete, true
	default:
		return "", false
	}
}

func compiledListAccess(
	descriptor domain.Descriptor,
	resourceName string,
	surface Surface,
) (domain.Resource, domain.Access, bool) {
	for _, resource := range descriptor.Resources {
		if resource.Name != resourceName {
			continue
		}
		switch surface {
		case SurfacePublic:
			return resource, resource.API.Public, true
		case SurfaceAdmin:
			return resource, resource.API.Admin, true
		default:
			return domain.Resource{}, domain.Access{}, false
		}
	}
	return domain.Resource{}, domain.Access{}, false
}

func containsDomainOperation(operations []domain.Operation, target domain.Operation) bool {
	for _, operation := range operations {
		if operation == target {
			return true
		}
	}
	return false
}

func matchesTopLevelMerge(existing, patch, candidate map[string]any) bool {
	expected := make(map[string]any, len(existing)+len(patch))
	for field, value := range existing {
		expected[field] = value
	}
	for field, value := range patch {
		expected[field] = value
	}
	return reflect.DeepEqual(expected, candidate)
}
