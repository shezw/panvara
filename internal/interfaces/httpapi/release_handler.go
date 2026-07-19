/*
   Panvara
   internal/interfaces/httpapi/release_handler.go    2026-07-19
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
	"net/http"

	"github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/application/record"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const (
	adminReleaseCollectionPath = "/api/admin/core/v1alpha1/modules/{module}/releases"
	adminReleaseItemPath       = "/api/admin/core/v1alpha1/modules/{module}/releases/{release}"
)

// ReleasePublisherService is the owner-authorized publication boundary consumed by HTTP.
type ReleasePublisherService interface {
	Publish(context.Context, access.Invocation, string, releaseapp.PublishInput) (domainrelease.ModuleRelease, bool, error)
	Get(context.Context, access.Invocation, string, string) (domainrelease.ModuleRelease, error)
}

func (handler *Handler) registerReleaseRoutes(mux *http.ServeMux, auth AdminAuth) {
	admin := func(next http.HandlerFunc) http.Handler {
		return withReleaseNoStore(auth.Middleware(handler.withIdentity(record.SurfaceAdmin, next)))
	}
	mux.Handle(adminReleaseCollectionPath, admin(handler.handleReleaseCollection))
	mux.Handle(adminReleaseItemPath, admin(handler.handleReleaseItem))
}

func withReleaseNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(writer, request)
	})
}

func (handler *Handler) handleReleaseCollection(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if problem := rejectReleaseQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	idempotencyKey, problem := parseReleaseIdempotencyKey(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	input, problem := decodePublishReleaseRequest(writer, request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	value, created, err := handler.releases.Publish(
		request.Context(), invocation, request.PathValue("module"), releaseapp.PublishInput{
			PlanID: input.PlanID, IdempotencyKey: idempotencyKey,
		},
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Location", releaseLocation(value.ModuleName(), value.ID().String()))
	writePrivateJSON(writer, statusCreated(created), makeReleaseResponse(value))
}

func (handler *Handler) handleReleaseItem(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	if problem := rejectReleaseQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if problem := requireReleaseEmptyBody(writer, request); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	value, err := handler.releases.Get(
		request.Context(), invocation, request.PathValue("module"), request.PathValue("release"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makeReleaseResponse(value))
}

var _ ReleasePublisherService = (*releaseapp.Publisher)(nil)
