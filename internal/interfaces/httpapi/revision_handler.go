/*
   Panvara
   internal/interfaces/httpapi/revision_handler.go    2026-07-15
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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/appmodule"
)

const (
	adminRevisionCollectionPath = "/api/admin/core/v1alpha1/modules/{module}/revisions"
	adminRevisionItemPath       = "/api/admin/core/v1alpha1/modules/{module}/revisions/{revision}"
	adminRevisionSourcePath     = "/api/admin/core/v1alpha1/modules/{module}/revisions/{revision}/source"
)

// RevisionRegistryService is the owner-authorized read boundary consumed by HTTP.
type RevisionRegistryService interface {
	List(context.Context, access.Execution, string, int) ([]appmodule.RevisionSummary, error)
	Get(context.Context, access.Execution, string, string) (appmodule.Revision, error)
	GetSource(context.Context, access.Execution, string, string) (application.RevisionSource, error)
}

type revisionResponse struct {
	Module               string                       `json:"module"`
	Revision             string                       `json:"revision"`
	ModuleVersion        string                       `json:"module_version"`
	DataSchemaIdentities []dataSchemaIdentityResponse `json:"data_schema_identities"`
	SpecVersion          string                       `json:"spec_version"`
	IRFormat             int                          `json:"ir_format"`
	SourceFormat         appmodule.SourceFormat       `json:"source_format"`
	SourceHash           string                       `json:"source_hash"`
	Origin               appmodule.RevisionOrigin     `json:"origin"`
	RegisteredBy         string                       `json:"registered_by"`
	RegisteredAt         time.Time                    `json:"registered_at"`
}

type dataSchemaIdentityResponse struct {
	Format      int    `json:"format"`
	Fingerprint string `json:"fingerprint"`
}

type revisionListResponse struct {
	Data []revisionResponse `json:"data"`
}

func (handler *Handler) handleRevisionCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	limit, problem := parseRevisionListLimit(request.URL)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	execution, ok := handler.revisionExecution(writer, request)
	if !ok {
		return
	}
	values, err := handler.revisions.List(request.Context(), execution, request.PathValue("module"), limit)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	response := revisionListResponse{Data: make([]revisionResponse, 0, len(values))}
	for _, value := range values {
		response.Data = append(response.Data, makeRevisionSummaryResponse(value))
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, http.StatusOK, response)
}

func (handler *Handler) handleRevisionItem(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	execution, ok := handler.revisionExecution(writer, request)
	if !ok {
		return
	}
	value, err := handler.revisions.Get(
		request.Context(), execution, request.PathValue("module"), request.PathValue("revision"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, http.StatusOK, makeRevisionSummaryResponse(value.Summary()))
}

func (handler *Handler) handleRevisionSource(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	execution, ok := handler.revisionExecution(writer, request)
	if !ok {
		return
	}
	source, err := handler.revisions.GetSource(
		request.Context(), execution, request.PathValue("module"), request.PathValue("revision"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	switch source.Format {
	case appmodule.SourceFormatJSON:
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	case appmodule.SourceFormatYAML:
		writer.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	default:
		handler.writeApplicationError(writer, request, fmt.Errorf("%w: unsupported source format", application.ErrRevisionCorrupt))
		return
	}
	etag := `"` + source.Hash + `"`
	writer.Header().Set("ETag", etag)
	writer.Header().Set("Cache-Control", "private, no-cache")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Header.Get("If-None-Match") == etag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(source.Bytes)
}

func (handler *Handler) revisionExecution(
	writer http.ResponseWriter,
	request *http.Request,
) (access.Execution, bool) {
	execution, ok := ExecutionFromContext(request.Context())
	if !ok || execution.Actor().Anonymous() || execution.Surface() != access.SurfaceAdmin {
		writeError(writer, request, http.StatusForbidden, "forbidden", "project owner access is required", nil)
		return access.Execution{}, false
	}
	return execution, true
}

func makeRevisionSummaryResponse(value appmodule.RevisionSummary) revisionResponse {
	identities := value.DataSchemaIdentities()
	encoded := make([]dataSchemaIdentityResponse, 0, len(identities))
	for _, identity := range identities {
		encoded = append(encoded, dataSchemaIdentityResponse{
			Format: identity.Format(), Fingerprint: identity.Fingerprint(),
		})
	}
	return revisionResponse{
		Module: value.ModuleName(), Revision: value.RevisionHash(), ModuleVersion: value.ModuleVersion(),
		DataSchemaIdentities: encoded, SpecVersion: value.SpecVersion(), IRFormat: value.IRFormat(),
		SourceFormat: value.SourceFormat(),
		SourceHash:   value.SourceHash(), Origin: value.Origin(), RegisteredBy: value.RegisteredBy(),
		RegisteredAt: value.RegisteredAt().UTC(),
	}
}

func parseRevisionListLimit(target *url.URL) (int, *requestProblem) {
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return 0, invalidRevisionQuery()
	}
	for key, values := range query {
		if key != "limit" || len(values) != 1 {
			return 0, invalidRevisionQuery()
		}
	}
	limit := application.DefaultRevisionListLimit
	if values, found := query["limit"]; found {
		value, err := strconv.Atoi(strings.TrimSpace(values[0]))
		if err != nil || value < 1 || value > application.MaxRevisionListLimit {
			return 0, invalidRevisionQuery()
		}
		limit = value
	}
	return limit, nil
}

func invalidRevisionQuery() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_query",
		message: "only one limit query parameter between 1 and 100 is supported",
	}
}
