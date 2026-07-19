/*
   Panvara
   internal/interfaces/httpapi/identity.go    2026-07-14
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
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

type identityContextKey struct{}

// RequestIdentity is the explicit project and actor boundary for one request.
type RequestIdentity struct {
	Project   project.Context
	Scope     project.Scope
	Actor     actor.Context
	Surface   access.Surface
	execution access.Execution
}

// IdentityFromContext returns the identity selected by the API surface.
func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(RequestIdentity)
	return identity, ok
}

// ExecutionFromContext returns the validated Application authorization input
// selected by trusted route composition rather than request headers.
func ExecutionFromContext(ctx context.Context) (access.Execution, bool) {
	identity, ok := IdentityFromContext(ctx)
	if !ok || identity.execution.Validate() != nil {
		return access.Execution{}, false
	}
	return identity.execution, true
}

func (handler *Handler) withIdentity(surface record.Surface, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		identity := RequestIdentity{Project: handler.project, Scope: handler.executionScope}
		var execution access.Execution
		var err error
		switch surface {
		case record.SurfacePublic:
			identity.Actor = handler.publicActor
			identity.Surface = access.SurfacePublic
			execution, err = access.NewPublicExecution(identity.Scope, identity.Actor)
		case record.SurfaceAdmin:
			authenticated, ok := authenticatedPrincipalFromContext(request.Context())
			if !ok || authenticated.Actor().ProjectID().String() != handler.project.ID().String() {
				writeError(writer, request, http.StatusForbidden, "forbidden", "administrator is outside the project boundary", nil)
				return
			}
			identity.Actor = authenticated.Actor()
			identity.Surface = access.SurfaceAdmin
			execution, err = access.NewAdminExecution(identity.Scope, authenticated)
		default:
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			return
		}
		if err != nil {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			return
		}
		identity.execution = execution
		ctx := context.WithValue(request.Context(), identityContextKey{}, identity)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
