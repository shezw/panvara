/*
   Panvara
   internal/interfaces/httpapi/draft_handler.go    2026-07-16
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

	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	adminDraftCollectionPath           = "/api/admin/core/v1alpha1/modules/{module}/drafts"
	adminDraftItemPath                 = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}"
	adminDraftSourcePath               = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/source"
	adminDraftValidationCollectionPath = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/validations"
	adminDraftValidationItemPath       = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/validations/{validation}"
	adminDraftPlanCollectionPath       = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/plans"
	adminDraftPlanItemPath             = "/api/admin/core/v1alpha1/modules/{module}/drafts/{draft}/plans/{plan}"
)

// DraftWorkflowService is the project-owner application boundary consumed by HTTP.
type DraftWorkflowService interface {
	Create(context.Context, project.ID, actor.Context, string, application.CreateDraftInput) (domain.Draft, bool, error)
	Get(context.Context, project.ID, actor.Context, string, string) (domain.Draft, error)
	GetSource(context.Context, project.ID, actor.Context, string, string) (application.DraftSource, error)
	Replace(context.Context, project.ID, actor.Context, string, string, uint64, application.ReplaceDraftInput) (domain.Draft, bool, error)
	Validate(context.Context, project.ID, actor.Context, string, string, uint64) (application.DraftValidation, bool, error)
	GetValidation(context.Context, project.ID, actor.Context, string, string, string) (application.DraftValidation, error)
	Plan(context.Context, project.ID, actor.Context, string, string, string, uint64) (application.DraftPlan, bool, error)
	GetPlan(context.Context, project.ID, actor.Context, string, string, string) (application.DraftPlan, error)
}

func (handler *Handler) registerDraftRoutes(mux *http.ServeMux, auth *BootstrapAdminAuth) {
	admin := func(next http.HandlerFunc) http.Handler {
		return auth.Middleware(handler.withIdentity(record.SurfaceAdmin, next))
	}
	mux.Handle(adminDraftCollectionPath, admin(handler.handleDraftCollection))
	mux.Handle(adminDraftItemPath, admin(handler.handleDraftItem))
	mux.Handle(adminDraftSourcePath, admin(handler.handleDraftSource))
	mux.Handle(adminDraftValidationCollectionPath, admin(handler.handleDraftValidationCollection))
	mux.Handle(adminDraftValidationItemPath, admin(handler.handleDraftValidationItem))
	mux.Handle(adminDraftPlanCollectionPath, admin(handler.handleDraftPlanCollection))
	mux.Handle(adminDraftPlanItemPath, admin(handler.handleDraftPlanItem))
}

func (handler *Handler) handleDraftCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	baseline, problem := parseDraftBaseline(request.URL)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	idempotencyKey, problem := parseIdempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	source, format, problem := readDraftSource(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, created, err := handler.drafts.Create(request.Context(), projectID, owner, request.PathValue("module"), application.CreateDraftInput{
		BaselineRevision: baseline, Format: format, Source: source, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	location := draftLocation(value.ModuleName(), value.ID().String())
	writer.Header().Set("Location", location)
	writeDraftMetadata(writer, statusCreated(created), value)
}

func (handler *Handler) handleDraftItem(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, err := handler.drafts.Get(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writeDraftMetadata(writer, http.StatusOK, value)
}

func (handler *Handler) handleDraftSource(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet, http.MethodPut) {
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	if request.Method == http.MethodGet {
		handler.getDraftSource(writer, request, projectID, owner)
		return
	}
	expected, problem := parseDraftIfMatch(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	source, format, problem := readDraftSource(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	value, _, err := handler.drafts.Replace(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"), expected,
		application.ReplaceDraftInput{Format: format, Source: source},
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writeDraftMetadata(writer, http.StatusOK, value)
}

func (handler *Handler) getDraftSource(
	writer http.ResponseWriter,
	request *http.Request,
	projectID project.ID,
	owner actor.Context,
) {
	value, err := handler.drafts.GetSource(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	if !setDraftSourceContentType(writer, value.Format) {
		handler.writeApplicationError(writer, request, fmt.Errorf("%w: unsupported draft source format", application.ErrDraftCorrupt))
		return
	}
	etag := formatVersionETag(value.Generation)
	writer.Header().Set("ETag", etag)
	writer.Header().Set("X-Panvara-Source-Hash", value.Hash)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Header.Get("If-None-Match") == etag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(value.Bytes)
}

func (handler *Handler) handleDraftValidationCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	expected, problem := parseDraftIfMatch(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if problem := requireEmptyBody(writer, request); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, created, err := handler.drafts.Validate(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"), expected,
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Location", validationLocation(value.ModuleName, value.DraftID.String(), value.ID))
	writer.Header().Set("ETag", formatVersionETag(value.Generation))
	writePrivateJSON(writer, statusCreated(created), makeValidationResponse(value))
}

func (handler *Handler) handleDraftValidationItem(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, err := handler.drafts.GetValidation(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"),
		request.PathValue("validation"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makeValidationResponse(value))
}

func (handler *Handler) handleDraftPlanCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	expected, problem := parseDraftIfMatch(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	body, problem := readJSONBody(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	input, problem := decodePlanRequest(body)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, created, err := handler.drafts.Plan(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"),
		input.ValidationID, expected,
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Location", planLocation(value.ModuleName, value.DraftID.String(), value.ID))
	writer.Header().Set("ETag", formatVersionETag(value.DraftGeneration))
	writePrivateJSON(writer, statusCreated(created), makePlanResponse(value))
}

func (handler *Handler) handleDraftPlanItem(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	projectID, owner, ok := handler.revisionIdentity(writer, request)
	if !ok {
		return
	}
	value, err := handler.drafts.GetPlan(
		request.Context(), projectID, owner, request.PathValue("module"), request.PathValue("draft"), request.PathValue("plan"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makePlanResponse(value))
}

var _ DraftWorkflowService = (*application.DraftWorkflow)(nil)
