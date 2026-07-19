/*
   Panvara
   internal/interfaces/httpapi/access_handler.go    2026-07-19
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
	"net/http"

	applicationaccess "github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/application/record"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
)

const (
	adminAccessPrincipalsPath       = "/api/admin/core/v1alpha1/access/principals"
	adminAccessPrincipalDisablePath = "/api/admin/core/v1alpha1/access/principals/{principal}/disable"
	adminAccessCredentialsPath      = "/api/admin/core/v1alpha1/access/principals/{principal}/credentials"
	adminAccessCredentialRevokePath = "/api/admin/core/v1alpha1/access/credentials/{credential}/revoke"
	adminAccessGrantsPath           = "/api/admin/core/v1alpha1/access/principals/{principal}/grants"
	adminAccessProjectOwnerPath     = "/api/admin/core/v1alpha1/access/principals/{principal}/grants/project.owner"
)

func (handler *Handler) registerAccessRoutes(mux *http.ServeMux, auth AdminAuth) {
	admin := func(next http.HandlerFunc) http.Handler {
		return withAccessNoStore(auth.Middleware(handler.withIdentity(record.SurfaceAdmin, next)))
	}
	mux.Handle(adminAccessPrincipalsPath, admin(handler.handleAccessPrincipals))
	mux.Handle(adminAccessPrincipalDisablePath, admin(handler.handleAccessPrincipalDisable))
	mux.Handle(adminAccessCredentialsPath, admin(handler.handleAccessCredentials))
	mux.Handle(adminAccessCredentialRevokePath, admin(handler.handleAccessCredentialRevoke))
	mux.Handle(adminAccessGrantsPath, admin(handler.handleAccessGrants))
	mux.Handle(adminAccessProjectOwnerPath, admin(handler.handleAccessProjectOwner))
}

func withAccessNoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(writer, request)
	})
}

func (handler *Handler) handleAccessPrincipals(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet, http.MethodPost) {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	if request.Method == http.MethodGet {
		values, err := handler.accessAdministration.ListPrincipals(request.Context(), invocation)
		if err != nil {
			handler.writeApplicationError(writer, request, err)
			return
		}
		writePrivateJSON(writer, http.StatusOK, makePrincipalListResponse(values))
		return
	}

	var input createPrincipalRequest
	if problem := decodeStrictAccessJSON(writer, request, &input, "display_name"); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	value, err := handler.accessAdministration.CreatePrincipal(
		request.Context(), invocation,
		applicationaccess.CreatePrincipalInput{DisplayName: input.DisplayName},
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusCreated, makePrincipalResponse(value))
}

func (handler *Handler) handleAccessPrincipalDisable(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if problem := requireEmptyBody(writer, request); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	principalID, problem := parseAccessPrincipalPath(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	value, err := handler.accessAdministration.DisablePrincipal(
		request.Context(), invocation, principalID,
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makePrincipalResponse(value))
}

func (handler *Handler) handleAccessCredentials(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet, http.MethodPost) {
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	principalID, problem := parseAccessPrincipalPath(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if request.Method == http.MethodGet {
		values, err := handler.accessAdministration.ListCredentials(
			request.Context(), invocation, principalID,
		)
		if err != nil {
			handler.writeApplicationError(writer, request, err)
			return
		}
		writePrivateJSON(writer, http.StatusOK, makeCredentialListResponse(values))
		return
	}
	var input issueCredentialRequest
	if problem := decodeStrictAccessJSON(writer, request, &input, "label"); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	value, err := handler.accessAdministration.IssueCredential(
		request.Context(), invocation,
		applicationaccess.IssueCredentialInput{PrincipalID: principalID, Label: input.Label},
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writer.Header().Set("Pragma", "no-cache")
	writePrivateJSON(writer, http.StatusCreated, makeIssuedCredentialResponse(value))
}

func (handler *Handler) handleAccessCredentialRevoke(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPost) {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if problem := requireEmptyBody(writer, request); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	credentialID, err := domainaccess.ParseID(request.PathValue("credential"))
	if err != nil {
		writeError(writer, request, http.StatusBadRequest, "invalid_access_request", "credential id is invalid", nil)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	value, err := handler.accessAdministration.RevokeCredential(
		request.Context(), invocation, credentialID,
	)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makeCredentialResponse(value))
}

func (handler *Handler) handleAccessGrants(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	principalID, problem := parseAccessPrincipalPath(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	values, err := handler.accessAdministration.ListProjectOwners(request.Context(), invocation)
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	filtered := make([]domainaccess.OwnerGrant, 0, 1)
	for _, value := range values {
		if value.PrincipalID() == principalID {
			filtered = append(filtered, value)
		}
	}
	writePrivateJSON(writer, http.StatusOK, makeOwnerGrantListResponse(filtered))
}

func (handler *Handler) handleAccessProjectOwner(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodPut, http.MethodDelete) {
		return
	}
	if problem := rejectAccessQuery(request.URL); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	if problem := requireEmptyBody(writer, request); problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	principalID, problem := parseAccessPrincipalPath(request)
	if problem != nil {
		writeProblem(writer, request, problem)
		return
	}
	invocation, ok := handler.accessInvocation(writer, request)
	if !ok {
		return
	}
	var value domainaccess.OwnerGrant
	var err error
	if request.Method == http.MethodPut {
		value, err = handler.accessAdministration.GrantProjectOwner(
			request.Context(), invocation, principalID,
		)
	} else {
		value, err = handler.accessAdministration.RevokeProjectOwner(
			request.Context(), invocation, principalID,
		)
	}
	if err != nil {
		handler.writeApplicationError(writer, request, err)
		return
	}
	writePrivateJSON(writer, http.StatusOK, makeOwnerGrantResponse(value))
}

func (handler *Handler) accessInvocation(
	writer http.ResponseWriter,
	request *http.Request,
) (applicationaccess.Invocation, bool) {
	execution, ok := ExecutionFromContext(request.Context())
	if !ok || execution.Surface() != applicationaccess.SurfaceAdmin {
		writeError(writer, request, http.StatusForbidden, "forbidden", "administrator access is required", nil)
		return applicationaccess.Invocation{}, false
	}
	invocation, err := applicationaccess.NewInvocation(execution, requestID(request))
	if err != nil {
		writeError(writer, request, http.StatusInternalServerError, "internal_error", "internal server error", nil)
		return applicationaccess.Invocation{}, false
	}
	return invocation, true
}

var _ AccessAdministration = (*applicationaccess.Administration)(nil)
