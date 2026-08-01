/*
   Panvara
   internal/application/release/error_normalization.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package release

import (
	"context"
	"errors"
	"fmt"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
)

func normalizeSnapshotError(action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, stable := range []error{
		ErrInvalid, ErrNotFound, ErrStale, ErrNotPublishable,
		ErrNotActivatable, ErrActivationConflict, ErrCorrupt, ErrUnavailable,
	} {
		if errors.Is(err, stable) {
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	switch {
	case errors.Is(err, moduleapp.ErrDraftNotFound), errors.Is(err, moduleapp.ErrValidationNotFound), errors.Is(err, moduleapp.ErrPlanNotFound):
		return fmt.Errorf("%s: %w", action, ErrNotFound)
	case errors.Is(err, moduleapp.ErrDraftConflict):
		return fmt.Errorf("%s: %w", action, ErrStale)
	case errors.Is(err, moduleapp.ErrDraftCorrupt):
		return fmt.Errorf("%s: %w", action, ErrCorrupt)
	default:
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
	}
}

func normalizeStoreError(action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, stable := range []error{
		ErrInvalid, ErrNotFound, ErrStale, ErrNotPublishable, ErrIdempotencyConflict,
		ErrNotActivatable, ErrActivationConflict, ErrCorrupt, ErrUnavailable,
		access.ErrUnauthenticated, access.ErrForbidden, access.ErrScopeInactive,
	} {
		if errors.Is(err, stable) {
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
}

func normalizeGeneratedError(action string, err error) error {
	if errors.Is(err, ErrInvalid) || errors.Is(err, ErrCorrupt) || errors.Is(err, ErrUnavailable) {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
}

func isAuthorizationFailure(err error) bool {
	return errors.Is(err, access.ErrUnauthenticated) || errors.Is(err, access.ErrForbidden) ||
		errors.Is(err, access.ErrScopeInactive)
}

func denialReason(err error) string {
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, access.ErrScopeInactive):
		return "scope_inactive"
	case errors.Is(err, access.ErrUnavailable):
		return "unavailable"
	default:
		return "forbidden"
	}
}
