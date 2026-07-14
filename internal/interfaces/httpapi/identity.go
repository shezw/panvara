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

	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

type identityContextKey struct{}

// RequestIdentity is the explicit project and actor boundary for one request.
type RequestIdentity struct {
	Project project.Context
	Actor   actor.Context
}

// IdentityFromContext returns the identity selected by the API surface.
func IdentityFromContext(ctx context.Context) (RequestIdentity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(RequestIdentity)
	return identity, ok
}

func (handler *Handler) withIdentity(surface record.Surface, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		identity := RequestIdentity{Project: handler.project}
		switch surface {
		case record.SurfacePublic:
			identity.Actor = handler.publicActor
		case record.SurfaceAdmin:
			authenticated, ok := authenticatedActorFromContext(request.Context())
			if !ok || authenticated.ProjectID().String() != handler.project.ID().String() ||
				authenticated.ActorID() != handler.adminActor.ActorID() || !authenticated.HasRole("project.owner") {
				writeError(writer, request, http.StatusForbidden, "forbidden", "administrator is outside the project boundary", nil)
				return
			}
			identity.Actor = authenticated
		default:
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
			return
		}
		ctx := context.WithValue(request.Context(), identityContextKey{}, identity)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
