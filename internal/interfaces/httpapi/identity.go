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
	Project project.Context
	Scope   project.Scope
	Actor   actor.Context
	Surface access.Surface
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
	if !ok {
		return access.Execution{}, false
	}
	execution, err := access.NewExecution(identity.Scope, identity.Actor, identity.Surface)
	return execution, err == nil
}

func (handler *Handler) withIdentity(surface record.Surface, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		identity := RequestIdentity{Project: handler.project, Scope: handler.executionScope}
		switch surface {
		case record.SurfacePublic:
			identity.Actor = handler.publicActor
			identity.Surface = access.SurfacePublic
		case record.SurfaceAdmin:
			authenticated, ok := authenticatedActorFromContext(request.Context())
			if !ok || authenticated.ProjectID().String() != handler.project.ID().String() ||
				authenticated.ActorID() != handler.adminActor.ActorID() {
				writeError(writer, request, http.StatusForbidden, "forbidden", "administrator is outside the project boundary", nil)
				return
			}
			identity.Actor = authenticated
			identity.Surface = access.SurfaceAdmin
		default:
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			return
		}
		if _, err := access.NewExecution(identity.Scope, identity.Actor, identity.Surface); err != nil {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			return
		}
		ctx := context.WithValue(request.Context(), identityContextKey{}, identity)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
