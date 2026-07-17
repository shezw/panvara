/*
   Panvara
   internal/application/record/errors.go    2026-07-14
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

import "errors"

var (
	// ErrInvalidArgument reports a malformed scope, identifier, payload, or command.
	ErrInvalidArgument = errors.New("invalid record argument")
	// ErrNotFound reports that a live record does not exist in the exact scope.
	ErrNotFound = errors.New("record not found")
	// ErrAlreadyExists reports a duplicate record identity.
	ErrAlreadyExists = errors.New("record already exists")
	// ErrVersionConflict reports a failed optimistic concurrency check.
	ErrVersionConflict = errors.New("record version conflict")
	// ErrUniqueConflict reports a duplicate model-declared unique value.
	ErrUniqueConflict = errors.New("record unique value conflict")
	// ErrReferenced reports an attempted deletion of a referenced record.
	ErrReferenced = errors.New("record is referenced")
	// ErrOperationForbidden reports that the selected AppModule surface does not
	// declare the requested record operation.
	ErrOperationForbidden = errors.New("record operation is forbidden")
)
