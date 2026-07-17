/*
   Panvara
   internal/application/record/types.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package record owns the application boundary for model-driven flex records.
package record

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

const (
	// DefaultListLimit is used when callers do not select a page size.
	DefaultListLimit = 20
	// MaxListLimit bounds one storage read independently of transport limits.
	MaxListLimit = 100
	// MaxListFilters bounds dynamic JSONB predicates in one list query.
	MaxListFilters = 16
	// MaxListFilterValueBytes bounds a normalized scalar equality value.
	MaxListFilterValueBytes = 512
)

// Surface identifies the caller-facing field allowlist selected by AppModule IR.
type Surface string

const (
	// SurfacePublic applies the least-privileged public API write policy.
	SurfacePublic Surface = "public"
	// SurfaceAdmin applies the authenticated Manager/admin write policy.
	SurfaceAdmin Surface = "admin"
)

// Valid reports whether the surface is an explicit supported value.
func (surface Surface) Valid() bool {
	return surface == SurfacePublic || surface == SurfaceAdmin
}

// Mutation identifies the model policy for one validator invocation.
type Mutation string

const (
	// MutationCreate validates a new complete record.
	MutationCreate Mutation = "create"
	// MutationPatch validates the complete record produced by applying a patch.
	MutationPatch Mutation = "patch"
)

// Valid reports whether the mutation is an explicit supported value.
func (mutation Mutation) Valid() bool {
	return mutation == MutationCreate || mutation == MutationPatch
}

// Operation identifies the fixed record use case whose AppModule policy must
// be checked by the application layer. Callers do not select this value; each
// Service method binds its own operation.
type Operation string

const (
	// OperationCreate authorizes creation of a record.
	OperationCreate Operation = "create"
	// OperationGet authorizes reading one record.
	OperationGet Operation = "get"
	// OperationList authorizes listing records.
	OperationList Operation = "list"
	// OperationPatch authorizes patching one record.
	OperationPatch Operation = "patch"
	// OperationDelete authorizes deleting one record.
	OperationDelete Operation = "delete"
)

// Valid reports whether operation is one of the explicit record use cases.
func (operation Operation) Valid() bool {
	switch operation {
	case OperationCreate, OperationGet, OperationList, OperationPatch, OperationDelete:
		return true
	default:
		return false
	}
}

var (
	recordIDPattern = regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	moduleNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)
	resourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	revisionHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ID is a UUIDv7 flex-record identity.
type ID struct {
	value string
}

// ParseID validates and normalizes a UUIDv7 record identity.
func ParseID(value string) (ID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !recordIDPattern.MatchString(value) {
		return ID{}, fmt.Errorf("%w: invalid UUIDv7 record id %q", ErrInvalidArgument, value)
	}
	return ID{value: value}, nil
}

// String returns the normalized UUIDv7 string.
func (id ID) String() string {
	return id.value
}

// Valid reports whether the ID was created by ParseID or a trusted generator.
func (id ID) Valid() bool {
	return recordIDPattern.MatchString(id.value)
}

// Scope is the complete isolation boundary of one record operation.
// RevisionHash is mandatory: callers never read or mutate an implicit schema.
type Scope struct {
	ProjectID    project.ID
	ModuleName   string
	ResourceName string
	RevisionHash string
}

// NewScope validates and constructs an exact project/module/resource/revision scope.
func NewScope(
	projectID project.ID,
	moduleName string,
	resourceName string,
	revisionHash string,
) (Scope, error) {
	scope := Scope{
		ProjectID:    projectID,
		ModuleName:   strings.TrimSpace(moduleName),
		ResourceName: strings.TrimSpace(resourceName),
		RevisionHash: strings.ToLower(strings.TrimSpace(revisionHash)),
	}
	if err := scope.Validate(); err != nil {
		return Scope{}, err
	}
	return scope, nil
}

// Validate rejects incomplete or ambiguous isolation boundaries.
func (scope Scope) Validate() error {
	if !scope.ProjectID.Valid() {
		return fmt.Errorf("%w: invalid project id", ErrInvalidArgument)
	}
	if len(scope.ModuleName) > 128 || !moduleNamePattern.MatchString(scope.ModuleName) {
		return fmt.Errorf("%w: invalid module name %q", ErrInvalidArgument, scope.ModuleName)
	}
	if len(scope.ResourceName) > 128 || !resourceNamePattern.MatchString(scope.ResourceName) {
		return fmt.Errorf("%w: invalid resource name %q", ErrInvalidArgument, scope.ResourceName)
	}
	if !revisionHashPattern.MatchString(scope.RevisionHash) {
		return fmt.Errorf("%w: invalid revision hash %q", ErrInvalidArgument, scope.RevisionHash)
	}
	return nil
}

// Record is one live or just-deleted flex record returned by the application.
type Record struct {
	Scope     Scope
	ID        ID
	Version   uint64
	Data      json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// UniqueValue is a schema-derived unique index entry. CanonicalValue must be
// stable for logically equal values; its representation is owned by Validator.
type UniqueValue struct {
	Field          string
	CanonicalValue string
}

// Reference is a same-project, same-module relation. Scope supplies the
// project and module so adapters cannot accidentally accept a cross-boundary target.
type Reference struct {
	Field          string
	TargetResource string
	TargetID       ID
}

// ValidationInput asks the model layer to validate one complete record body.
type ValidationInput struct {
	Scope        Scope
	Surface      Surface
	Mutation     Mutation
	ExistingData json.RawMessage
	PatchData    json.RawMessage
	Data         json.RawMessage
}

// ListFilter is one declared scalar equality predicate. Value is the
// schema-normalized text representation stored by PostgreSQL's jsonb ->> operator.
type ListFilter struct {
	Field string
	Value string
}

// ListValidationInput asks the compiled module to authorize and normalize list filters.
type ListValidationInput struct {
	Scope   Scope
	Surface Surface
	Filters []ListFilter
}

// OperationValidationInput asks the immutable module revision to authorize a
// fixed record use case before any record persistence is accessed.
type OperationValidationInput struct {
	Scope     Scope
	Surface   Surface
	Operation Operation
}

// ValidatedData is the trusted complete-record output of a model validator.
// For MutationPatch it must contain the full merged record and all unique and
// reference indexes, never only the patch. Service applies a second structural
// check before handing it to persistence.
type ValidatedData struct {
	Data       json.RawMessage
	Uniques    []UniqueValue
	References []Reference
}

// Validator is implemented by the AppModule IR adapter owned by the runtime
// composition root. The record application layer does not depend on IR layout.
type Validator interface {
	AuthorizeOperation(context.Context, OperationValidationInput) error
	Validate(context.Context, ValidationInput) (ValidatedData, error)
	ValidateList(context.Context, ListValidationInput) ([]ListFilter, error)
}

// CreateCommand is the complete transaction input for Store.Create.
type CreateCommand struct {
	Scope      Scope
	ID         ID
	Data       json.RawMessage
	Uniques    []UniqueValue
	References []Reference
	At         time.Time
}

// UpdateCommand replaces a complete record body and all derived indexes.
type UpdateCommand struct {
	Scope           Scope
	ID              ID
	ExpectedVersion uint64
	Data            json.RawMessage
	Uniques         []UniqueValue
	References      []Reference
	At              time.Time
}

// DeleteCommand soft-deletes a record and releases its outbound constraints.
type DeleteCommand struct {
	Scope           Scope
	ID              ID
	ExpectedVersion uint64
	At              time.Time
}

// ListCursor continues a stable ascending (created_at, record_id) scan.
type ListCursor struct {
	CreatedAt time.Time
	ID        ID
	QueryHash string
}

// ListOptions bounds one list request.
type ListOptions struct {
	Limit   int
	Cursor  *ListCursor
	Filters []ListFilter
}

// ListResult returns live records and a cursor only when another page exists.
type ListResult struct {
	Records []Record
	Next    *ListCursor
}

// Store is the persistence port consumed by Service. Every method receives a
// complete Scope; implementations must repeat the project predicate in all SQL.
type Store interface {
	Create(context.Context, CreateCommand) (Record, error)
	Get(context.Context, Scope, ID) (Record, error)
	List(context.Context, Scope, ListOptions) (ListResult, error)
	Update(context.Context, UpdateCommand) (Record, error)
	Delete(context.Context, DeleteCommand) (Record, error)
}
