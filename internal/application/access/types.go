/*
   Panvara
   internal/application/access/types.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package access defines the authoritative application authorization boundary.
package access

import (
	"context"
	"fmt"

	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

// RoleProjectOwner is the exact persisted grant required by admin operations.
const RoleProjectOwner = "project.owner"

// Surface identifies the caller-facing authorization boundary.
type Surface string

const (
	// SurfacePublic is the unauthenticated or end-user application surface.
	SurfacePublic Surface = "public"
	// SurfaceAdmin is the authenticated Manager and administration surface.
	SurfaceAdmin Surface = "admin"
)

// Valid reports whether the surface is explicitly supported.
func (surface Surface) Valid() bool {
	return surface == SurfacePublic || surface == SurfaceAdmin
}

// Operation is one explicit Server use-case authorization action.
type Operation string

const (
	// OperationRecordList authorizes listing model-driven records.
	OperationRecordList Operation = "record.list"
	// OperationRecordGet authorizes reading one model-driven record.
	OperationRecordGet Operation = "record.get"
	// OperationRecordCreate authorizes creating one model-driven record.
	OperationRecordCreate Operation = "record.create"
	// OperationRecordPatch authorizes patching one model-driven record.
	OperationRecordPatch Operation = "record.patch"
	// OperationRecordDelete authorizes deleting one model-driven record.
	OperationRecordDelete Operation = "record.delete"

	// OperationRevisionList authorizes listing immutable module revisions.
	OperationRevisionList Operation = "revision.list"
	// OperationRevisionGet authorizes reading one immutable module revision.
	OperationRevisionGet Operation = "revision.get"
	// OperationRevisionGetSource authorizes reading immutable revision source.
	OperationRevisionGetSource Operation = "revision.get_source"

	// OperationDraftCreate authorizes creating a module draft.
	OperationDraftCreate Operation = "draft.create"
	// OperationDraftGet authorizes reading one module draft.
	OperationDraftGet Operation = "draft.get"
	// OperationDraftGetSource authorizes reading module draft source.
	OperationDraftGetSource Operation = "draft.get_source"
	// OperationDraftReplace authorizes replacing module draft source.
	OperationDraftReplace Operation = "draft.replace"
	// OperationDraftValidate authorizes validating a module draft.
	OperationDraftValidate Operation = "draft.validate"
	// OperationDraftPlan authorizes producing a deterministic draft change plan.
	OperationDraftPlan Operation = "draft.plan"
	// OperationDraftGetValidation authorizes reading a draft validation snapshot.
	OperationDraftGetValidation Operation = "draft.get_validation"
	// OperationDraftGetPlan authorizes reading a draft change-plan snapshot.
	OperationDraftGetPlan Operation = "draft.get_plan"
)

// Valid reports whether the operation is an explicit supported use case.
func (operation Operation) Valid() bool {
	switch operation {
	case OperationRecordList,
		OperationRecordGet,
		OperationRecordCreate,
		OperationRecordPatch,
		OperationRecordDelete,
		OperationRevisionList,
		OperationRevisionGet,
		OperationRevisionGetSource,
		OperationDraftCreate,
		OperationDraftGet,
		OperationDraftGetSource,
		OperationDraftReplace,
		OperationDraftValidate,
		OperationDraftPlan,
		OperationDraftGetValidation,
		OperationDraftGetPlan:
		return true
	default:
		return false
	}
}

func (operation Operation) isRecord() bool {
	switch operation {
	case OperationRecordList,
		OperationRecordGet,
		OperationRecordCreate,
		OperationRecordPatch,
		OperationRecordDelete:
		return true
	default:
		return false
	}
}

// Execution carries the complete immutable authorization input for one use case.
type Execution struct {
	scope   project.Scope
	actor   actor.Context
	surface Surface
}

// NewExecution validates and constructs one authorization execution.
func NewExecution(
	scope project.Scope,
	subject actor.Context,
	surface Surface,
) (Execution, error) {
	execution := Execution{scope: scope, actor: subject, surface: surface}
	if err := execution.Validate(); err != nil {
		return Execution{}, err
	}
	return execution, nil
}

// Validate rejects incomplete or contradictory authorization boundaries.
func (execution Execution) Validate() error {
	if err := execution.scope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if !execution.actor.Valid() {
		return fmt.Errorf("%w: invalid actor context", ErrInvalidRequest)
	}
	if execution.actor.ProjectID().String() != execution.scope.ProjectID().String() {
		return fmt.Errorf("%w: actor and scope projects differ", ErrInvalidRequest)
	}
	if !execution.surface.Valid() {
		return fmt.Errorf("%w: invalid surface %q", ErrInvalidRequest, execution.surface)
	}
	return nil
}

// Scope returns the exact project/environment authorization boundary.
func (execution Execution) Scope() project.Scope {
	return execution.scope
}

// Actor returns the authenticated or anonymous use-case principal.
func (execution Execution) Actor() actor.Context {
	return execution.actor
}

// Surface returns the public or admin authorization boundary.
func (execution Execution) Surface() Surface {
	return execution.surface
}

// Authorizer guards an application use case before business or storage work.
type Authorizer interface {
	Authorize(context.Context, Execution, Operation) error
}

// GrantReader reads authoritative scope and role state. Implementations must
// evaluate the complete project/environment scope and active grant state.
type GrantReader interface {
	ScopeActive(context.Context, project.Scope) (bool, error)
	HasActiveGrant(context.Context, project.Scope, string, string) (bool, error)
}
