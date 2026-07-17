/*
   Panvara
   internal/application/access/policy.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package access

import (
	"context"
	"fmt"
)

// Policy authorizes public record access and authoritative project-owner admin
// access. AppModule policy remains the second, resource-specific public gate.
type Policy struct {
	grants GrantReader
}

// NewPolicy constructs an authorization policy with authoritative grant state.
func NewPolicy(grants GrantReader) (*Policy, error) {
	if grants == nil {
		return nil, fmt.Errorf("%w: nil grant reader", ErrInvalidRequest)
	}
	return &Policy{grants: grants}, nil
}

// Authorize validates scope activity before applying surface and operation
// policy. Actor self-reported roles are intentionally never consulted.
func (policy *Policy) Authorize(
	ctx context.Context,
	execution Execution,
	operation Operation,
) error {
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if policy == nil || policy.grants == nil {
		return fmt.Errorf("%w: policy is not initialized", ErrUnavailable)
	}
	if err := execution.Validate(); err != nil {
		return err
	}

	active, err := policy.grants.ScopeActive(ctx, execution.Scope())
	if err != nil {
		return fmt.Errorf("%w: read scope state: %w", ErrUnavailable, err)
	}
	if !active {
		return ErrScopeInactive
	}
	if !operation.Valid() {
		return fmt.Errorf("%w: unsupported operation %q", ErrForbidden, operation)
	}

	switch execution.Surface() {
	case SurfacePublic:
		if !operation.isRecord() {
			return fmt.Errorf("%w: public surface does not expose %q", ErrForbidden, operation)
		}
		return nil
	case SurfaceAdmin:
		subject := execution.Actor()
		if subject.Anonymous() {
			return ErrUnauthenticated
		}
		granted, err := policy.grants.HasActiveGrant(
			ctx,
			execution.Scope(),
			subject.ActorID(),
			RoleProjectOwner,
		)
		if err != nil {
			return fmt.Errorf("%w: read project owner grant: %w", ErrUnavailable, err)
		}
		if !granted {
			return ErrForbidden
		}
		return nil
	default:
		return fmt.Errorf("%w: invalid surface", ErrInvalidRequest)
	}
}
