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

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

// RoleProjectOwner is the exact persisted grant required by admin operations.
const RoleProjectOwner = domainaccess.RoleProjectOwner

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
	// OperationReleasePublish authorizes creating an immutable module release fact.
	OperationReleasePublish Operation = "release.publish"
	// OperationReleaseGet authorizes reading one immutable module release fact.
	OperationReleaseGet Operation = "release.get"
	// OperationReleaseActivate authorizes atomically advancing the active Release epoch.
	OperationReleaseActivate Operation = "release.activate"
	// OperationReleaseGetActive authorizes reading the current active Release snapshot.
	OperationReleaseGetActive Operation = "release.get_active"

	// OperationPrincipalList authorizes listing project-local principals.
	OperationPrincipalList Operation = "access.principal.list"
	// OperationPrincipalCreate authorizes creating a service principal.
	OperationPrincipalCreate Operation = "access.principal.create"
	// OperationPrincipalDisable authorizes terminally disabling a principal.
	OperationPrincipalDisable Operation = "access.principal.disable"
	// OperationCredentialList authorizes listing non-secret credential metadata.
	OperationCredentialList Operation = "access.credential.list"
	// OperationCredentialIssue authorizes issuing a one-time API credential token.
	OperationCredentialIssue Operation = "access.credential.issue"
	// OperationCredentialRevoke authorizes terminally revoking an API credential.
	OperationCredentialRevoke Operation = "access.credential.revoke"
	// OperationCredentialBootstrap identifies the first bootstrap credential audit.
	OperationCredentialBootstrap Operation = "access.credential.bootstrap"
	// OperationProjectOwnerList authorizes listing project-owner grants.
	OperationProjectOwnerList Operation = "access.project_owner.list"
	// OperationProjectOwnerGrant authorizes granting project-owner access.
	OperationProjectOwnerGrant Operation = "access.project_owner.grant"
	// OperationProjectOwnerRevoke authorizes revoking project-owner access.
	OperationProjectOwnerRevoke Operation = "access.project_owner.revoke"
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
		OperationDraftGetPlan,
		OperationReleasePublish,
		OperationReleaseGet,
		OperationReleaseActivate,
		OperationReleaseGetActive,
		OperationPrincipalList,
		OperationPrincipalCreate,
		OperationPrincipalDisable,
		OperationCredentialList,
		OperationCredentialIssue,
		OperationCredentialRevoke,
		OperationCredentialBootstrap,
		OperationProjectOwnerList,
		OperationProjectOwnerGrant,
		OperationProjectOwnerRevoke:
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

// AuthenticatedPrincipal is credential-backed actor evidence. Its fields are
// intentionally private so protocol adapters cannot assemble trusted identity.
type AuthenticatedPrincipal struct {
	scope        project.Scope
	actor        actor.Context
	credentialID domainaccess.ID
}

func newAuthenticatedPrincipal(
	scope project.Scope,
	subject actor.Context,
	credentialID domainaccess.ID,
) (AuthenticatedPrincipal, error) {
	value := AuthenticatedPrincipal{scope: scope, actor: subject, credentialID: credentialID}
	if !value.Valid() {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: invalid authenticated principal", ErrInvalid)
	}
	return value, nil
}

// Scope returns the exact project/environment boundary proven by the credential.
func (principal AuthenticatedPrincipal) Scope() project.Scope { return principal.scope }

// Actor returns the authenticated project-local actor.
func (principal AuthenticatedPrincipal) Actor() actor.Context { return principal.actor }

// CredentialID returns the authoritative credential evidence identity.
func (principal AuthenticatedPrincipal) CredentialID() domainaccess.ID {
	return principal.credentialID
}

// Valid reports whether exact scope, non-anonymous actor, and credential evidence exist.
func (principal AuthenticatedPrincipal) Valid() bool {
	return principal.scope.Validate() == nil &&
		principal.actor.Valid() &&
		!principal.actor.Anonymous() &&
		principal.actor.ProjectID().String() == principal.scope.ProjectID().String() &&
		principal.credentialID.Valid()
}

// Execution carries the complete immutable authorization input for one use case.
type Execution struct {
	scope        project.Scope
	actor        actor.Context
	credentialID domainaccess.ID
	surface      Surface
}

// NewExecution validates and constructs a public anonymous execution. Admin
// callers must use NewAdminExecution with credential-backed evidence.
func NewExecution(
	scope project.Scope,
	subject actor.Context,
	surface Surface,
) (Execution, error) {
	execution := Execution{scope: scope, actor: subject, surface: surface}
	if surface != SurfacePublic {
		return Execution{}, fmt.Errorf("%w: admin execution requires authenticated principal evidence", ErrInvalid)
	}
	if err := execution.Validate(); err != nil {
		return Execution{}, err
	}
	return execution, nil
}

// NewPublicExecution constructs an anonymous public execution.
func NewPublicExecution(scope project.Scope, subject actor.Context) (Execution, error) {
	return NewExecution(scope, subject, SurfacePublic)
}

// NewAdminExecution constructs an admin execution from credential-backed evidence.
func NewAdminExecution(
	scope project.Scope,
	principal AuthenticatedPrincipal,
) (Execution, error) {
	if !principal.Valid() {
		return Execution{}, fmt.Errorf("%w: invalid authenticated principal", ErrInvalid)
	}
	if !sameProjectScope(scope, principal.Scope()) {
		return Execution{}, fmt.Errorf("%w: authenticated principal and execution scopes differ", ErrInvalid)
	}
	execution := Execution{
		scope: scope, actor: principal.Actor(), credentialID: principal.CredentialID(), surface: SurfaceAdmin,
	}
	if err := execution.Validate(); err != nil {
		return Execution{}, err
	}
	return execution, nil
}

func sameProjectScope(left, right project.Scope) bool {
	return left.ProjectID().String() == right.ProjectID().String() &&
		left.EnvironmentID().String() == right.EnvironmentID().String()
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
	switch execution.surface {
	case SurfacePublic:
		if !execution.actor.Anonymous() || execution.credentialID.Valid() {
			return fmt.Errorf("%w: public execution must be anonymous", ErrInvalid)
		}
	case SurfaceAdmin:
		if execution.actor.Anonymous() || !execution.credentialID.Valid() {
			return fmt.Errorf("%w: admin execution requires credential evidence", ErrInvalid)
		}
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

// CredentialID returns admin credential evidence or a zero ID for public calls.
func (execution Execution) CredentialID() domainaccess.ID { return execution.credentialID }

// Surface returns the public or admin authorization boundary.
func (execution Execution) Surface() Surface {
	return execution.surface
}

// Authorizer guards an application use case before business or storage work.
type Authorizer interface {
	Authorize(context.Context, Execution, Operation) error
}

// AuthorityReader reads authoritative scope, credential, and role state.
// Implementations must evaluate exact project/environment/principal identities.
type AuthorityReader interface {
	ScopeActive(context.Context, project.Scope) (bool, error)
	CredentialActive(context.Context, project.Scope, string, domainaccess.ID) (bool, error)
	HasActiveGrant(context.Context, project.Scope, string, string) (bool, error)
}

// GrantReader is retained as an alias while adapters migrate to AuthorityReader.
type GrantReader = AuthorityReader
