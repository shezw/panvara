/*
   Panvara
   internal/interfaces/httpapi/record_handler.go    2026-07-14
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
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/application/record"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
)

var filterFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,127}$`)

type recordResponse struct {
	ID        string          `json:"id"`
	Version   uint64          `json:"version"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt *time.Time      `json:"deleted_at,omitempty"`
}

type listResponse struct {
	Data       []recordResponse `json:"data"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (handler *Handler) handlePublicCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if !handler.validateRoute(writer, request, record.SurfacePublic, domainmodule.OperationCreate) {
		return
	}
	handler.createRecord(writer, request, record.SurfacePublic)
}

func (handler *Handler) handleAdminCollection(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodPost:
		if !handler.validateRoute(writer, request, record.SurfaceAdmin, domainmodule.OperationCreate) {
			return
		}
		handler.createRecord(writer, request, record.SurfaceAdmin)
	case http.MethodGet:
		if !handler.validateRoute(writer, request, record.SurfaceAdmin, domainmodule.OperationList) {
			return
		}
		handler.listRecords(writer, request)
	default:
		requireMethod(writer, request, http.MethodGet, http.MethodPost)
	}
}

func (handler *Handler) handleAdminRecord(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		if !handler.validateRoute(writer, request, record.SurfaceAdmin, domainmodule.OperationGet) {
			return
		}
		handler.getRecord(writer, request)
	case http.MethodPatch:
		if !handler.validateRoute(writer, request, record.SurfaceAdmin, domainmodule.OperationPatch) {
			return
		}
		handler.patchRecord(writer, request)
	case http.MethodDelete:
		if !handler.validateRoute(writer, request, record.SurfaceAdmin, domainmodule.OperationDelete) {
			return
		}
		handler.deleteRecord(writer, request)
	default:
		requireMethod(writer, request, http.MethodDelete, http.MethodGet, http.MethodPatch)
	}
}

func (handler *Handler) validateRoute(
	writer http.ResponseWriter,
	request *http.Request,
	surface record.Surface,
	operation domainmodule.Operation,
) bool {
	if request.PathValue("module") != handler.module.Name() {
		writeError(writer, request, http.StatusNotFound, "not_found", "module not found", nil)
		return false
	}
	resourceName := request.PathValue("resource")
	if _, exists := handler.resources[resourceName]; !exists {
		writeError(writer, request, http.StatusNotFound, "not_found", "resource not found", nil)
		return false
	}
	if !handler.operationAllowed(resourceName, surface, operation) {
		writeError(writer, request, http.StatusForbidden, "operation_forbidden", "operation is not enabled for this API surface", nil)
		return false
	}
	return true
}

func (handler *Handler) createRecord(writer http.ResponseWriter, request *http.Request, surface record.Surface) {
	body, problem := readJSONBody(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	scope, err := handler.scope(request)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	created, err := handler.records.Create(request.Context(), scope, surface, body)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Location", request.URL.Path+"/"+created.ID.String())
	writeRecord(writer, http.StatusCreated, created)
}

func (handler *Handler) getRecord(writer http.ResponseWriter, request *http.Request) {
	scope, id, ok := handler.scopeAndID(writer, request)
	if !ok {
		return
	}
	result, err := handler.records.Get(request.Context(), scope, id)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writeRecord(writer, http.StatusOK, result)
}

func (handler *Handler) listRecords(writer http.ResponseWriter, request *http.Request) {
	options, problem := parseListOptions(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	scope, err := handler.scope(request)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	result, err := handler.records.List(request.Context(), scope, record.SurfaceAdmin, options)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	response := listResponse{Data: make([]recordResponse, 0, len(result.Records))}
	for _, item := range result.Records {
		response.Data = append(response.Data, makeRecordResponse(item))
	}
	if result.Next != nil {
		response.NextCursor = encodeListCursor(*result.Next)
	}
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) patchRecord(writer http.ResponseWriter, request *http.Request) {
	expectedVersion, problem := parseIfMatch(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	body, problem := readJSONBody(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	scope, id, ok := handler.scopeAndID(writer, request)
	if !ok {
		return
	}
	updated, err := handler.records.Update(
		request.Context(), scope, id, expectedVersion, record.SurfaceAdmin, body,
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writeRecord(writer, http.StatusOK, updated)
}

func (handler *Handler) deleteRecord(writer http.ResponseWriter, request *http.Request) {
	expectedVersion, problem := parseIfMatch(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	scope, id, ok := handler.scopeAndID(writer, request)
	if !ok {
		return
	}
	deleted, err := handler.records.Delete(request.Context(), scope, id, expectedVersion)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("ETag", formatVersionETag(deleted.Version))
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) scopeAndID(
	writer http.ResponseWriter,
	request *http.Request,
) (record.Scope, record.ID, bool) {
	scope, err := handler.scope(request)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return record.Scope{}, record.ID{}, false
	}
	id, err := record.ParseID(request.PathValue("id"))
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return record.Scope{}, record.ID{}, false
	}
	return scope, id, true
}

func writeRecord(writer http.ResponseWriter, status int, value record.Record) {
	writer.Header().Set("ETag", formatVersionETag(value.Version))
	writeJSON(writer, status, makeRecordResponse(value))
}

func makeRecordResponse(value record.Record) recordResponse {
	return recordResponse{
		ID: value.ID.String(), Version: value.Version, Data: append(json.RawMessage(nil), value.Data...),
		CreatedAt: value.CreatedAt.UTC(), UpdatedAt: value.UpdatedAt.UTC(), DeletedAt: cloneTime(value.DeletedAt),
	}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}

func parseListOptions(request *http.Request) (record.ListOptions, *requestProblem) {
	query, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return record.ListOptions{}, &requestProblem{
			status: http.StatusBadRequest, code: "invalid_query", message: "query string is invalid",
		}
	}
	filters := make([]record.ListFilter, 0)
	for key, values := range query {
		if len(values) != 1 {
			return record.ListOptions{}, &requestProblem{
				status: http.StatusBadRequest, code: "invalid_query",
				message: "query parameters must not be repeated",
			}
		}
		if key == "limit" || key == "cursor" {
			continue
		}
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			return record.ListOptions{}, &requestProblem{
				status: http.StatusBadRequest, code: "invalid_query",
				message: "only limit, cursor, and filter[field] query parameters are supported",
			}
		}
		field := strings.TrimSuffix(strings.TrimPrefix(key, "filter["), "]")
		if !filterFieldPattern.MatchString(field) || values[0] == "" || len(values[0]) > record.MaxListFilterValueBytes {
			return record.ListOptions{}, &requestProblem{
				status: http.StatusBadRequest, code: "invalid_filter",
				message: "filter field or value is invalid",
			}
		}
		filters = append(filters, record.ListFilter{Field: field, Value: values[0]})
	}
	if len(filters) > record.MaxListFilters {
		return record.ListOptions{}, &requestProblem{
			status: http.StatusBadRequest, code: "invalid_filter", message: "too many filters",
		}
	}
	options := record.ListOptions{Limit: record.DefaultListLimit, Filters: filters}
	if values, present := query["limit"]; present {
		value := values[0]
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > record.MaxListLimit {
			return record.ListOptions{}, &requestProblem{
				status: http.StatusBadRequest, code: "invalid_limit",
				message: "limit must be an integer between 1 and 100",
			}
		}
		options.Limit = limit
	}
	if values, present := query["cursor"]; present {
		value := values[0]
		cursor, err := decodeListCursor(value)
		if err != nil {
			return record.ListOptions{}, &requestProblem{
				status: http.StatusBadRequest, code: "invalid_cursor", message: "cursor is invalid",
			}
		}
		options.Cursor = &cursor
	}
	return options, nil
}
