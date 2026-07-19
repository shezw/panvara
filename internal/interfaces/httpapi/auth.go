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
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/domain/project"
)

// CredentialAdminAuth authenticates admin Bearer credentials against the
// authoritative Application service for one exact execution scope.
type CredentialAdminAuth struct {
	scope         project.Scope
	authenticator access.PrincipalAuthenticator
}

type authenticatedPrincipalContextKey struct{}

// NewCredentialAdminAuth binds database-backed authentication to one Project
// and Environment. The adapter never retains a configured bootstrap token.
func NewCredentialAdminAuth(
	scope project.Scope,
	authenticator access.PrincipalAuthenticator,
) (*CredentialAdminAuth, error) {
	if err := scope.Validate(); err != nil {
		return nil, fmt.Errorf("construct admin authentication: invalid scope: %w", err)
	}
	if authenticator == nil {
		return nil, fmt.Errorf("construct admin authentication: nil credential authenticator")
	}
	return &CredentialAdminAuth{scope: scope, authenticator: authenticator}, nil
}

// Middleware requires exactly one valid Bearer credential before calling the
// protected handler.
func (auth *CredentialAdminAuth) Middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		credential, ok := bearerCredential(request)
		if !ok {
			writeAuthenticationRequired(writer, request)
			return
		}
		if auth == nil || auth.authenticator == nil {
			writeAuthenticationUnavailable(writer, request)
			return
		}
		principal, err := auth.authenticator.Authenticate(request.Context(), auth.scope, credential)
		if err != nil {
			if accessUnavailable(err) {
				writeAuthenticationUnavailable(writer, request)
				return
			}
			writeAuthenticationRequired(writer, request)
			return
		}
		if !principal.Valid() ||
			principal.Scope().ProjectID().String() != auth.scope.ProjectID().String() ||
			principal.Scope().EnvironmentID().String() != auth.scope.EnvironmentID().String() {
			writeAuthenticationUnavailable(writer, request)
			return
		}
		ctx := context.WithValue(
			request.Context(), authenticatedPrincipalContextKey{}, principal,
		)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func authenticatedPrincipalFromContext(
	ctx context.Context,
) (access.AuthenticatedPrincipal, bool) {
	value, ok := ctx.Value(authenticatedPrincipalContextKey{}).(access.AuthenticatedPrincipal)
	return value, ok && value.Valid()
}

func bearerCredential(request *http.Request) (string, bool) {
	if request == nil {
		return "", false
	}
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	scheme, credential, found := strings.Cut(values[0], " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || credential == "" || strings.ContainsAny(credential, " \t\r\n,") {
		return "", false
	}
	return credential, true
}

func accessUnavailable(err error) bool {
	return errors.Is(err, access.ErrUnavailable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func writeAuthenticationRequired(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("WWW-Authenticate", `Bearer realm="panvara-admin"`)
	writeError(
		writer, request, http.StatusUnauthorized, "unauthorized",
		"valid administrator bearer credential required", nil,
	)
}

func writeAuthenticationUnavailable(writer http.ResponseWriter, request *http.Request) {
	writeError(
		writer, request, http.StatusServiceUnavailable, "authentication_unavailable",
		"administrator authentication is unavailable", nil,
	)
}

var _ AdminAuth = (*CredentialAdminAuth)(nil)
