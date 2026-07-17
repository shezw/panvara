/*
   Panvara
   internal/interfaces/httpapi/auth.go    2026-07-14
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
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/shezw/panvara/internal/domain/actor"
)

const minimumBootstrapTokenBytes = 32

// BootstrapAdminAuth protects the temporary administrative API and retains
// only the bootstrap token digest after construction.
type BootstrapAdminAuth struct {
	digest [sha256.Size]byte
	actor  actor.Context
}

type authenticatedActorContextKey struct{}

// NewBootstrapAdminAuth hashes a bootstrap token for subsequent constant-time
// comparisons. Whitespace is rejected to keep Authorization parsing exact.
func NewBootstrapAdminAuth(token string, adminActor actor.Context) (*BootstrapAdminAuth, error) {
	if len(token) < minimumBootstrapTokenBytes {
		return nil, fmt.Errorf("bootstrap admin token must contain at least %d bytes", minimumBootstrapTokenBytes)
	}
	if strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n\t ") {
		return nil, fmt.Errorf("bootstrap admin token must not contain whitespace")
	}
	if !adminActor.Valid() || adminActor.Anonymous() {
		return nil, fmt.Errorf("bootstrap administrator must be an authenticated actor")
	}
	return &BootstrapAdminAuth{digest: sha256.Sum256([]byte(token)), actor: adminActor}, nil
}

// Middleware requires exactly one valid Bearer credential before calling the
// protected handler.
func (auth *BootstrapAdminAuth) Middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if auth == nil || !auth.authorized(request) {
			writer.Header().Set("WWW-Authenticate", `Bearer realm="panvara-admin"`)
			writeError(writer, request, http.StatusUnauthorized, "unauthorized", "valid administrator bearer token required", nil)
			return
		}
		ctx := context.WithValue(request.Context(), authenticatedActorContextKey{}, auth.actor)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func authenticatedActorFromContext(ctx context.Context) (actor.Context, bool) {
	value, ok := ctx.Value(authenticatedActorContextKey{}).(actor.Context)
	return value, ok && value.Valid() && !value.Anonymous()
}

func (auth *BootstrapAdminAuth) authorized(request *http.Request) bool {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return false
	}
	scheme, credential, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || credential == "" || strings.ContainsAny(credential, " \t\r\n") {
		return false
	}
	candidate := sha256.Sum256([]byte(credential))
	return subtle.ConstantTimeCompare(candidate[:], auth.digest[:]) == 1
}
