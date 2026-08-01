/*
   Panvara
   internal/interfaces/httpapi/error_map.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/shezw/panvara/internal/application/access"
	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	releaseapp "github.com/shezw/panvara/internal/application/release"
)

func writeProblem(writer http.ResponseWriter, request *http.Request, problem *requestProblem) {
	writeError(writer, request, problem.status, problem.code, problem.message, nil)
}

func (handler *Handler) writeApplicationError(writer http.ResponseWriter, request *http.Request, err error) {
	var validation *appmodule.ValidationError
	switch {
	case errors.As(err, &validation):
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "record validation failed", validation.Violations)
	case errors.Is(err, access.ErrUnauthenticated):
		writeAuthenticationRequired(writer, request)
	case errors.Is(err, access.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "invalid_access_request", "access request is invalid", nil)
	case errors.Is(err, access.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "access_not_found", "access resource was not found", nil)
	case errors.Is(err, access.ErrLastOwnerPath):
		writeError(writer, request, http.StatusConflict, "last_owner_path", "the last usable project owner path cannot be removed", nil)
	case errors.Is(err, access.ErrConflict):
		writeError(writer, request, http.StatusConflict, "access_conflict", "access resource lifecycle conflicts with the request", nil)
	case errors.Is(err, access.ErrForbidden), errors.Is(err, access.ErrScopeInactive),
		errors.Is(err, record.ErrOperationForbidden):
		writeError(writer, request, http.StatusForbidden, "forbidden", "operation is not allowed", nil)
	case errors.Is(err, access.ErrUnavailable):
		writeError(writer, request, http.StatusServiceUnavailable, "access_unavailable", "authorization is unavailable", nil)
	case errors.Is(err, record.ErrInvalidArgument):
		writeError(writer, request, http.StatusBadRequest, "invalid_argument", "request contains an invalid argument", nil)
	case errors.Is(err, record.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "not_found", "record not found", nil)
	case errors.Is(err, record.ErrVersionConflict):
		writeError(writer, request, http.StatusPreconditionFailed, "precondition_failed", "record ETag no longer matches", nil)
	case errors.Is(err, record.ErrUniqueConflict):
		writeError(writer, request, http.StatusConflict, "unique_conflict", "record conflicts with a unique field", nil)
	case errors.Is(err, record.ErrAlreadyExists):
		writeError(writer, request, http.StatusConflict, "already_exists", "record already exists", nil)
	case errors.Is(err, record.ErrReferenced):
		writeError(writer, request, http.StatusConflict, "record_referenced", "record is still referenced", nil)
	case errors.Is(err, appmodule.ErrRevisionInvalid):
		writeError(writer, request, http.StatusBadRequest, "invalid_revision_request", "module revision request is invalid", nil)
	case errors.Is(err, appmodule.ErrRevisionForbidden):
		writeError(writer, request, http.StatusForbidden, "forbidden", "project owner access is required", nil)
	case errors.Is(err, appmodule.ErrRevisionNotFound):
		writeError(writer, request, http.StatusNotFound, "revision_not_found", "module revision not found", nil)
	case errors.Is(err, appmodule.ErrDraftInvalid):
		writeError(writer, request, http.StatusBadRequest, "invalid_draft_request", "module draft request is invalid", nil)
	case errors.Is(err, appmodule.ErrDraftForbidden):
		writeError(writer, request, http.StatusForbidden, "forbidden", "project owner access is required", nil)
	case errors.Is(err, appmodule.ErrDraftNotFound):
		writeError(writer, request, http.StatusNotFound, "draft_not_found", "module draft not found", nil)
	case errors.Is(err, appmodule.ErrDraftConflict):
		writeError(writer, request, http.StatusPreconditionFailed, "precondition_failed", "draft ETag no longer matches", nil)
	case errors.Is(err, appmodule.ErrDraftIdempotencyConflict):
		writeError(writer, request, http.StatusConflict, "idempotency_key_conflict", "Idempotency-Key was already used for another draft intent", nil)
	case errors.Is(err, appmodule.ErrValidationNotFound):
		writeError(writer, request, http.StatusNotFound, "validation_not_found", "module draft validation not found", nil)
	case errors.Is(err, appmodule.ErrValidationInvalid):
		writeError(writer, request, http.StatusConflict, "draft_not_valid", "a current successful validation is required", nil)
	case errors.Is(err, appmodule.ErrPlanNotFound):
		writeError(writer, request, http.StatusNotFound, "plan_not_found", "module draft plan not found", nil)
	case errors.Is(err, releaseapp.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "invalid_release_request", "module release request is invalid", nil)
	case errors.Is(err, releaseapp.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "release_not_found", "module release or publish plan was not found", nil)
	case errors.Is(err, releaseapp.ErrIdempotencyConflict):
		writeError(writer, request, http.StatusConflict, "idempotency_key_conflict", "Idempotency-Key was already used for another release intent", nil)
	case errors.Is(err, releaseapp.ErrStale):
		writeError(writer, request, http.StatusConflict, "stale_plan", "module release plan is stale", nil)
	case errors.Is(err, releaseapp.ErrNotPublishable):
		writeError(writer, request, http.StatusUnprocessableEntity, "not_publishable", "module release plan is not publishable", nil)
	case errors.Is(err, releaseapp.ErrNotActivatable):
		writeError(writer, request, http.StatusUnprocessableEntity, "not_activatable", "module release cannot be activated by this runtime", nil)
	case errors.Is(err, releaseapp.ErrActivationConflict):
		writeError(writer, request, http.StatusConflict, "activation_conflict", "module release no longer matches the active runtime", nil)
	case errors.Is(err, releaseapp.ErrActivationOutcomeUnknown), errors.Is(err, releaseapp.ErrCorrupt),
		errors.Is(err, releaseapp.ErrUnavailable):
		writeError(writer, request, http.StatusServiceUnavailable, "release_unavailable", "module release authority is unavailable", nil)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(writer, request, http.StatusGatewayTimeout, "deadline_exceeded", "request deadline exceeded", nil)
	case errors.Is(err, context.Canceled):
		writeError(writer, request, http.StatusServiceUnavailable, "request_canceled", "request was canceled", nil)
	default:
		writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
	}
}
