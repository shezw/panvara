/*
   Panvara
   internal/interfaces/httpapi/activation_handler.go    2026-08-02
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
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const (
	adminReleaseActivatePath = "/api/admin/core/v1alpha1/modules/{module}/releases/{release}/activate"
	adminActiveReleasePath   = "/api/admin/core/v1alpha1/modules/{module}/active"
)

// ReleaseActivationService is the owner-authorized active-snapshot boundary
// consumed by HTTP. Publish and Activate remain separate use cases.
type ReleaseActivationService interface {
	Activate(context.Context, access.Invocation, string, string) (domainrelease.ActiveSnapshot, bool, error)
	GetActive(context.Context, access.Invocation, string) (domainrelease.ActiveSnapshot, error)
}

func (handler *Handler) registerActivationRoutes(mux *http.ServeMux, auth AdminAuth) {
	admin := func(next http.HandlerFunc) http.Handler {
		return withReleaseNoStore(auth.Middleware(handler.withIdentity(record.SurfaceAdmin, next)))
	}
	mux.Handle(adminReleaseActivatePath, admin(handler.handleReleaseActivate))
	mux.Handle(adminActiveReleasePath, admin(handler.handleActiveRelease))
}

func (handler *Handler) handleReleaseActivate(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
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
	value, created, err := handler.activations.Activate(
		request.Context(), invocation, request.PathValue("module"), request.PathValue("release"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Location", activeReleaseLocation(value.ModuleName()))
	writeActiveSnapshot(writer, statusCreated(created), value)
}

func (handler *Handler) handleActiveRelease(writer http.ResponseWriter, request *http.Request) {
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
	value, err := handler.activations.GetActive(
		request.Context(), invocation, request.PathValue("module"),
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writeActiveSnapshot(writer, http.StatusOK, value)
}
