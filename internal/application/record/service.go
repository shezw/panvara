/*
   Panvara
   internal/application/record/service.go    2026-07-14
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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/shezw/panvara/internal/application/access"
)

// Service coordinates schema validation, identity generation, and persistence.
type Service struct {
	store      Store
	validator  Validator
	authorizer access.Authorizer
	clock      Clock
	ids        IDGenerator
}

// NewService constructs a record application service with explicit dependencies.
func NewService(
	store Store,
	validator Validator,
	authorizer access.Authorizer,
	clock Clock,
	ids IDGenerator,
) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: nil record store", ErrInvalidArgument)
	}
	if validator == nil {
		return nil, fmt.Errorf("%w: nil record validator", ErrInvalidArgument)
	}
	if authorizer == nil {
		return nil, fmt.Errorf("%w: nil record authorizer", ErrInvalidArgument)
	}
	if clock == nil {
		return nil, fmt.Errorf("%w: nil record clock", ErrInvalidArgument)
	}
	if ids == nil {
		return nil, fmt.Errorf("%w: nil record id generator", ErrInvalidArgument)
	}
	return &Service{
		store: store, validator: validator, authorizer: authorizer, clock: clock, ids: ids,
	}, nil
}

// NewDefaultService uses the system clock and cryptographic UUIDv7 generator.
func NewDefaultService(store Store, validator Validator, authorizer access.Authorizer) (*Service, error) {
	return NewService(store, validator, authorizer, SystemClock{}, NewDefaultUUIDv7Generator())
}

// Create validates and atomically persists a new record and its derived indexes.
func (service *Service) Create(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	data json.RawMessage,
) (Record, error) {
	surface, err := service.authorize(
		ctx, execution, scope, access.OperationRecordCreate, OperationCreate,
	)
	if err != nil {
		return Record{}, err
	}
	validated, err := service.validate(ctx, ValidationInput{
		Scope: scope, Surface: surface, Mutation: MutationCreate, Data: data,
	})
	if err != nil {
		return Record{}, err
	}
	at := service.clock.Now().UTC()
	id, err := service.ids.New(at)
	if err != nil {
		return Record{}, fmt.Errorf("generate record id: %w", err)
	}
	record, err := service.store.Create(ctx, CreateCommand{
		Scope: scope, ID: id, Data: validated.Data,
		Uniques: validated.Uniques, References: validated.References, At: at,
	})
	if err != nil {
		return Record{}, fmt.Errorf("create record: %w", err)
	}
	return cloneRecord(record), nil
}

// Get returns one live record from the exact schema revision scope.
func (service *Service) Get(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	id ID,
) (Record, error) {
	if err := validateIdentity(scope, id); err != nil {
		return Record{}, err
	}
	if _, err := service.authorize(
		ctx, execution, scope, access.OperationRecordGet, OperationGet,
	); err != nil {
		return Record{}, err
	}
	record, err := service.store.Get(ctx, scope, id)
	if err != nil {
		return Record{}, fmt.Errorf("get record: %w", err)
	}
	return cloneRecord(record), nil
}

// List returns a bounded stable page of live records.
func (service *Service) List(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	options ListOptions,
) (ListResult, error) {
	surface, err := service.authorize(
		ctx, execution, scope, access.OperationRecordList, OperationList,
	)
	if err != nil {
		return ListResult{}, err
	}
	if options.Limit < 0 || options.Limit > MaxListLimit {
		return ListResult{}, fmt.Errorf("%w: list limit must be between 0 and %d", ErrInvalidArgument, MaxListLimit)
	}
	if options.Limit == 0 {
		options.Limit = DefaultListLimit
	}
	if len(options.Filters) > MaxListFilters {
		return ListResult{}, fmt.Errorf("%w: list supports at most %d filters", ErrInvalidArgument, MaxListFilters)
	}
	filters, err := service.validator.ValidateList(ctx, ListValidationInput{
		Scope: scope, Surface: surface, Filters: append([]ListFilter(nil), options.Filters...),
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("validate list policy: %w", err)
	}
	if err := validateListFilters(filters); err != nil {
		return ListResult{}, err
	}
	sort.Slice(filters, func(left, right int) bool { return filters[left].Field < filters[right].Field })
	options.Filters = filters
	queryHash := listQueryHash(scope, surface, filters)
	if options.Cursor != nil {
		if options.Cursor.CreatedAt.IsZero() || !options.Cursor.ID.Valid() {
			return ListResult{}, fmt.Errorf("%w: invalid list cursor", ErrInvalidArgument)
		}
		if options.Cursor.QueryHash != queryHash {
			return ListResult{}, fmt.Errorf("%w: list cursor does not match filters", ErrInvalidArgument)
		}
		cursor := *options.Cursor
		cursor.CreatedAt = cursor.CreatedAt.UTC()
		options.Cursor = &cursor
	}
	result, err := service.store.List(ctx, scope, options)
	if err != nil {
		return ListResult{}, fmt.Errorf("list records: %w", err)
	}
	for index := range result.Records {
		result.Records[index] = cloneRecord(result.Records[index])
	}
	if result.Next != nil {
		result.Next.QueryHash = queryHash
	}
	return result, nil
}

// Update applies a top-level JSON merge patch, validates the resulting complete
// record against the selected surface policy, and applies optimistic concurrency.
// Nested values are replaced. Explicit null is unsupported in alpha.2.
func (service *Service) Update(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	id ID,
	expectedVersion uint64,
	patch json.RawMessage,
) (Record, error) {
	if err := validateMutation(scope, id, expectedVersion); err != nil {
		return Record{}, err
	}
	surface, err := service.authorize(
		ctx, execution, scope, access.OperationRecordPatch, OperationPatch,
	)
	if err != nil {
		return Record{}, err
	}
	current, err := service.store.Get(ctx, scope, id)
	if err != nil {
		return Record{}, fmt.Errorf("read record before patch: %w", err)
	}
	if current.Version != expectedVersion {
		return Record{}, ErrVersionConflict
	}
	merged, err := mergeTopLevelPatch(current.Data, patch)
	if err != nil {
		return Record{}, err
	}
	validated, err := service.validate(ctx, ValidationInput{
		Scope:        scope,
		Surface:      surface,
		Mutation:     MutationPatch,
		ExistingData: current.Data,
		PatchData:    patch,
		Data:         merged,
	})
	if err != nil {
		return Record{}, err
	}
	record, err := service.store.Update(ctx, UpdateCommand{
		Scope: scope, ID: id, ExpectedVersion: expectedVersion,
		Data: validated.Data, Uniques: validated.Uniques,
		References: validated.References, At: service.clock.Now().UTC(),
	})
	if err != nil {
		return Record{}, fmt.Errorf("update record: %w", err)
	}
	return cloneRecord(record), nil
}

// Delete soft-deletes a record, releases its unique entries and outbound
// references, and rejects deletion while live records still reference it.
func (service *Service) Delete(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	id ID,
	expectedVersion uint64,
) (Record, error) {
	if err := validateMutation(scope, id, expectedVersion); err != nil {
		return Record{}, err
	}
	if _, err := service.authorize(
		ctx, execution, scope, access.OperationRecordDelete, OperationDelete,
	); err != nil {
		return Record{}, err
	}
	record, err := service.store.Delete(ctx, DeleteCommand{
		Scope: scope, ID: id, ExpectedVersion: expectedVersion, At: service.clock.Now().UTC(),
	})
	if err != nil {
		return Record{}, fmt.Errorf("delete record: %w", err)
	}
	return cloneRecord(record), nil
}

func (service *Service) authorize(
	ctx context.Context,
	execution access.Execution,
	scope Scope,
	accessOperation access.Operation,
	recordOperation Operation,
) (Surface, error) {
	if service == nil || service.authorizer == nil || service.validator == nil {
		return "", fmt.Errorf("%w: uninitialized record service", ErrInvalidArgument)
	}
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if err := execution.Validate(); err != nil {
		return "", err
	}
	if execution.Scope().ProjectID() != scope.ProjectID {
		return "", access.ErrForbidden
	}
	if err := service.authorizer.Authorize(ctx, execution, accessOperation); err != nil {
		return "", err
	}
	surface, err := recordSurface(execution.Surface())
	if err != nil {
		return "", err
	}
	if err := service.validator.AuthorizeOperation(ctx, OperationValidationInput{
		Scope: scope, Surface: surface, Operation: recordOperation,
	}); err != nil {
		return "", fmt.Errorf("authorize record operation: %w", err)
	}
	return surface, nil
}

func recordSurface(surface access.Surface) (Surface, error) {
	switch surface {
	case access.SurfacePublic:
		return SurfacePublic, nil
	case access.SurfaceAdmin:
		return SurfaceAdmin, nil
	default:
		return "", fmt.Errorf("%w: invalid access surface %q", ErrInvalidArgument, surface)
	}
}

func (service *Service) validate(
	ctx context.Context,
	input ValidationInput,
) (ValidatedData, error) {
	if !input.Surface.Valid() || !input.Mutation.Valid() {
		return ValidatedData{}, fmt.Errorf("%w: invalid validator policy context", ErrInvalidArgument)
	}
	input.Data = append(json.RawMessage(nil), input.Data...)
	input.ExistingData = append(json.RawMessage(nil), input.ExistingData...)
	input.PatchData = append(json.RawMessage(nil), input.PatchData...)
	validated, err := service.validator.Validate(ctx, input)
	if err != nil {
		return ValidatedData{}, fmt.Errorf("validate record against module revision: %w", err)
	}
	validated, err = normalizeValidatedData(validated)
	if err != nil {
		return ValidatedData{}, fmt.Errorf("validate model output: %w", err)
	}
	return validated, nil
}

func validateIdentity(scope Scope, id ID) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if !id.Valid() {
		return fmt.Errorf("%w: invalid record id", ErrInvalidArgument)
	}
	return nil
}

func validateMutation(scope Scope, id ID, expectedVersion uint64) error {
	if err := validateIdentity(scope, id); err != nil {
		return err
	}
	if expectedVersion == 0 {
		return fmt.Errorf("%w: expected version must be greater than zero", ErrInvalidArgument)
	}
	return nil
}

func cloneRecord(value Record) Record {
	value.Data = append(json.RawMessage(nil), value.Data...)
	if value.DeletedAt != nil {
		deletedAt := *value.DeletedAt
		value.DeletedAt = &deletedAt
	}
	return value
}

func validateListFilters(filters []ListFilter) error {
	if len(filters) > MaxListFilters {
		return fmt.Errorf("%w: list supports at most %d filters", ErrInvalidArgument, MaxListFilters)
	}
	seen := make(map[string]struct{}, len(filters))
	for _, filter := range filters {
		if !resourceNamePattern.MatchString(filter.Field) {
			return fmt.Errorf("%w: invalid list filter field %q", ErrInvalidArgument, filter.Field)
		}
		if len(filter.Value) > MaxListFilterValueBytes {
			return fmt.Errorf("%w: list filter value is too large", ErrInvalidArgument)
		}
		if _, exists := seen[filter.Field]; exists {
			return fmt.Errorf("%w: duplicate list filter field %q", ErrInvalidArgument, filter.Field)
		}
		seen[filter.Field] = struct{}{}
	}
	return nil
}

func listQueryHash(scope Scope, surface Surface, filters []ListFilter) string {
	payload, _ := json.Marshal(struct {
		ProjectID    string       `json:"project_id"`
		ModuleName   string       `json:"module"`
		ResourceName string       `json:"resource"`
		RevisionHash string       `json:"revision"`
		Surface      Surface      `json:"surface"`
		Filters      []ListFilter `json:"filters"`
	}{
		ProjectID: scope.ProjectID.String(), ModuleName: scope.ModuleName,
		ResourceName: scope.ResourceName, RevisionHash: scope.RevisionHash,
		Surface: surface, Filters: filters,
	})
	hash := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", hash[:])
}
