/*
   Panvara
   internal/application/access/errors.go    2026-07-18
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

import "errors"

var (
	// ErrInvalid reports malformed access input or an invalid lifecycle request.
	ErrInvalid = errors.New("invalid access request")
	// ErrInvalidRequest is the backwards-compatible name for ErrInvalid.
	ErrInvalidRequest = ErrInvalid
	// ErrNotFound reports an absent access-domain fact in the exact scope.
	ErrNotFound = errors.New("access fact not found")
	// ErrConflict reports a duplicate or stale access lifecycle transition.
	ErrConflict = errors.New("access lifecycle conflict")
	// ErrLastOwnerPath rejects removal of the final usable project-owner path.
	ErrLastOwnerPath = errors.New("last project owner path")
	// ErrUnauthenticated reports an admin request without an authenticated principal.
	ErrUnauthenticated = errors.New("access authentication required")
	// ErrForbidden reports a recognized principal or surface without permission.
	ErrForbidden = errors.New("access forbidden")
	// ErrScopeInactive reports a project/environment boundary that is not active.
	ErrScopeInactive = errors.New("access scope inactive")
	// ErrUnavailable reports a failure to read authoritative access state.
	ErrUnavailable = errors.New("access state unavailable")
)
