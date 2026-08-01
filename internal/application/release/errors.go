/*
   Panvara
   internal/application/release/errors.go    2026-07-19
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

import "errors"

var (
	// ErrInvalid reports malformed publication input or incomplete dependencies.
	ErrInvalid = errors.New("invalid module release request")
	// ErrNotFound reports an absent plan snapshot or release in the exact scope.
	ErrNotFound = errors.New("module release fact not found")
	// ErrStale reports that the draft no longer matches the immutable plan chain.
	ErrStale = errors.New("module release plan is stale")
	// ErrNotPublishable reports a valid plan whose outcome cannot be published.
	ErrNotPublishable = errors.New("module release plan is not publishable")
	// ErrIdempotencyConflict reports reuse of a key for another publish intent.
	ErrIdempotencyConflict = errors.New("module release idempotency conflict")
	// ErrNotActivatable reports a published Release that cannot safely become active.
	ErrNotActivatable = errors.New("module release is not activatable")
	// ErrActivationConflict reports a stale active baseline or prohibited reactivation.
	ErrActivationConflict = errors.New("module release activation conflict")
	// ErrCorrupt reports persisted facts that fail identity or chain verification.
	ErrCorrupt = errors.New("module release storage is corrupt")
	// ErrUnavailable reports an inability to read or atomically persist release facts.
	ErrUnavailable = errors.New("module release state unavailable")
)
