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

	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
)

func writeProblem(writer http.ResponseWriter, request *http.Request, problem *requestProblem) {
	writeError(writer, request, problem.status, problem.code, problem.message, nil)
}

func (handler *Handler) writeApplicationError(writer http.ResponseWriter, request *http.Request, err error) {
	var validation *appmodule.ValidationError
	switch {
	case errors.As(err, &validation):
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_failed", "record validation failed", validation.Violations)
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
	case errors.Is(err, context.DeadlineExceeded):
		writeError(writer, request, http.StatusGatewayTimeout, "deadline_exceeded", "request deadline exceeded", nil)
	case errors.Is(err, context.Canceled):
		writeError(writer, request, http.StatusServiceUnavailable, "request_canceled", "request was canceled", nil)
	default:
		writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
	}
}
