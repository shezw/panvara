/*
   Panvara
   internal/interfaces/httpapi/access_handler_test.go    2026-07-19
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	testServicePrincipalID = "svc:018f7e93-7b2e-7abc-8def-1234567890ab"
	testCredentialID       = "018f7e93-7b30-7abc-8def-1234567890ab"
	testIssuedToken        = "pvk1.018f7e93-7b30-7abc-8def-1234567890ab.not-a-real-secret"
)

type fakeAccessAdministration struct {
	listPrincipals     func(context.Context, applicationaccess.Invocation) ([]domainaccess.Principal, error)
	createPrincipal    func(context.Context, applicationaccess.Invocation, applicationaccess.CreatePrincipalInput) (domainaccess.Principal, error)
	disablePrincipal   func(context.Context, applicationaccess.Invocation, string) (domainaccess.Principal, error)
	listCredentials    func(context.Context, applicationaccess.Invocation, string) ([]domainaccess.Credential, error)
	issueCredential    func(context.Context, applicationaccess.Invocation, applicationaccess.IssueCredentialInput) (applicationaccess.IssuedCredential, error)
	revokeCredential   func(context.Context, applicationaccess.Invocation, domainaccess.ID) (domainaccess.Credential, error)
	listProjectOwners  func(context.Context, applicationaccess.Invocation) ([]domainaccess.OwnerGrant, error)
	grantProjectOwner  func(context.Context, applicationaccess.Invocation, string) (domainaccess.OwnerGrant, error)
	revokeProjectOwner func(context.Context, applicationaccess.Invocation, string) (domainaccess.OwnerGrant, error)
}

func (service *fakeAccessAdministration) ListPrincipals(
	ctx context.Context,
	invocation applicationaccess.Invocation,
) ([]domainaccess.Principal, error) {
	if service.listPrincipals == nil {
		return []domainaccess.Principal{}, nil
	}
	return service.listPrincipals(ctx, invocation)
}

func (service *fakeAccessAdministration) CreatePrincipal(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	input applicationaccess.CreatePrincipalInput,
) (domainaccess.Principal, error) {
	if service.createPrincipal == nil {
		return domainaccess.Principal{}, errors.New("unexpected CreatePrincipal")
	}
	return service.createPrincipal(ctx, invocation, input)
}

func (service *fakeAccessAdministration) DisablePrincipal(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	principalID string,
) (domainaccess.Principal, error) {
	if service.disablePrincipal == nil {
		return domainaccess.Principal{}, errors.New("unexpected DisablePrincipal")
	}
	return service.disablePrincipal(ctx, invocation, principalID)
}

func (service *fakeAccessAdministration) ListCredentials(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	principalID string,
) ([]domainaccess.Credential, error) {
	if service.listCredentials == nil {
		return []domainaccess.Credential{}, nil
	}
	return service.listCredentials(ctx, invocation, principalID)
}

func (service *fakeAccessAdministration) IssueCredential(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	input applicationaccess.IssueCredentialInput,
) (applicationaccess.IssuedCredential, error) {
	if service.issueCredential == nil {
		return applicationaccess.IssuedCredential{}, errors.New("unexpected IssueCredential")
	}
	return service.issueCredential(ctx, invocation, input)
}

func (service *fakeAccessAdministration) RevokeCredential(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	credentialID domainaccess.ID,
) (domainaccess.Credential, error) {
	if service.revokeCredential == nil {
		return domainaccess.Credential{}, errors.New("unexpected RevokeCredential")
	}
	return service.revokeCredential(ctx, invocation, credentialID)
}

func (service *fakeAccessAdministration) ListProjectOwners(
	ctx context.Context,
	invocation applicationaccess.Invocation,
) ([]domainaccess.OwnerGrant, error) {
	if service.listProjectOwners == nil {
		return []domainaccess.OwnerGrant{}, nil
	}
	return service.listProjectOwners(ctx, invocation)
}

func (service *fakeAccessAdministration) GrantProjectOwner(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	principalID string,
) (domainaccess.OwnerGrant, error) {
	if service.grantProjectOwner == nil {
		return domainaccess.OwnerGrant{}, errors.New("unexpected GrantProjectOwner")
	}
	return service.grantProjectOwner(ctx, invocation, principalID)
}

func (service *fakeAccessAdministration) RevokeProjectOwner(
	ctx context.Context,
	invocation applicationaccess.Invocation,
	principalID string,
) (domainaccess.OwnerGrant, error) {
	if service.revokeProjectOwner == nil {
		return domainaccess.OwnerGrant{}, errors.New("unexpected RevokeProjectOwner")
	}
	return service.revokeProjectOwner(ctx, invocation, principalID)
}

func TestAccessAdministrationRoutes(t *testing.T) {
	principal := testAccessPrincipal(t, testServicePrincipalID, false)
	credential := testAccessCredential(t, testCredentialID, testServicePrincipalID, false)
	grant := testOwnerGrant(t, testServicePrincipalID, false)
	otherGrant := testOwnerGrant(t, "svc:018f7e93-7b31-7abc-8def-1234567890ab", false)
	seenInvocation := func(invocation applicationaccess.Invocation) {
		if invocation.RequestID() != "access-request-42" ||
			invocation.Execution().Actor().ActorID() != testServicePrincipalID ||
			!invocation.Execution().CredentialID().Valid() {
			t.Fatalf("invocation = %+v actor=%q", invocation, invocation.Execution().Actor().ActorID())
		}
	}
	service := &fakeAccessAdministration{
		listPrincipals: func(_ context.Context, invocation applicationaccess.Invocation) ([]domainaccess.Principal, error) {
			seenInvocation(invocation)
			return []domainaccess.Principal{principal}, nil
		},
		createPrincipal: func(_ context.Context, invocation applicationaccess.Invocation, input applicationaccess.CreatePrincipalInput) (domainaccess.Principal, error) {
			seenInvocation(invocation)
			if input.DisplayName != "deploy bot" {
				t.Fatalf("create principal input = %+v", input)
			}
			return principal, nil
		},
		disablePrincipal: func(_ context.Context, invocation applicationaccess.Invocation, principalID string) (domainaccess.Principal, error) {
			seenInvocation(invocation)
			if principalID != testServicePrincipalID {
				t.Fatalf("disable principal id = %q", principalID)
			}
			return testAccessPrincipal(t, principalID, true), nil
		},
		listCredentials: func(_ context.Context, invocation applicationaccess.Invocation, principalID string) ([]domainaccess.Credential, error) {
			seenInvocation(invocation)
			if principalID != testServicePrincipalID {
				t.Fatalf("credential principal id = %q", principalID)
			}
			return []domainaccess.Credential{credential}, nil
		},
		issueCredential: func(_ context.Context, invocation applicationaccess.Invocation, input applicationaccess.IssueCredentialInput) (applicationaccess.IssuedCredential, error) {
			seenInvocation(invocation)
			if input.PrincipalID != testServicePrincipalID || input.Label != "deployment" {
				t.Fatalf("issue credential input = %+v", input)
			}
			return applicationaccess.IssuedCredential{Credential: credential, Token: testIssuedToken}, nil
		},
		revokeCredential: func(_ context.Context, invocation applicationaccess.Invocation, id domainaccess.ID) (domainaccess.Credential, error) {
			seenInvocation(invocation)
			if id.String() != testCredentialID {
				t.Fatalf("revoke credential id = %q", id.String())
			}
			return testAccessCredential(t, id.String(), testServicePrincipalID, true), nil
		},
		listProjectOwners: func(_ context.Context, invocation applicationaccess.Invocation) ([]domainaccess.OwnerGrant, error) {
			seenInvocation(invocation)
			return []domainaccess.OwnerGrant{grant, otherGrant}, nil
		},
		grantProjectOwner: func(_ context.Context, invocation applicationaccess.Invocation, principalID string) (domainaccess.OwnerGrant, error) {
			seenInvocation(invocation)
			if principalID != testServicePrincipalID {
				t.Fatalf("grant principal id = %q", principalID)
			}
			return grant, nil
		},
		revokeProjectOwner: func(_ context.Context, invocation applicationaccess.Invocation, principalID string) (domainaccess.OwnerGrant, error) {
			seenInvocation(invocation)
			if principalID != testServicePrincipalID {
				t.Fatalf("revoke grant principal id = %q", principalID)
			}
			return testOwnerGrant(t, principalID, true), nil
		},
	}
	handler := newAccessTestHandler(t, service, testServicePrincipalID)
	base := "/api/admin/core/v1alpha1/access"

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		contains   string
	}{
		{name: "list principals", method: http.MethodGet, path: base + "/principals", wantStatus: http.StatusOK, contains: `"display_name":"deploy bot"`},
		{name: "create principal", method: http.MethodPost, path: base + "/principals", body: `{"display_name":"deploy bot"}`, wantStatus: http.StatusCreated, contains: `"id":"` + testServicePrincipalID + `"`},
		{name: "disable principal", method: http.MethodPost, path: base + "/principals/" + testServicePrincipalID + "/disable", wantStatus: http.StatusOK, contains: `"status":"disabled"`},
		{name: "list credentials", method: http.MethodGet, path: base + "/principals/" + testServicePrincipalID + "/credentials", wantStatus: http.StatusOK, contains: `"hint":"sha256:0123456789ab"`},
		{name: "issue credential", method: http.MethodPost, path: base + "/principals/" + testServicePrincipalID + "/credentials", body: `{"label":"deployment"}`, wantStatus: http.StatusCreated, contains: `"token":"` + testIssuedToken + `"`},
		{name: "revoke credential", method: http.MethodPost, path: base + "/credentials/" + testCredentialID + "/revoke", wantStatus: http.StatusOK, contains: `"status":"revoked"`},
		{name: "list grants", method: http.MethodGet, path: base + "/principals/" + testServicePrincipalID + "/grants", wantStatus: http.StatusOK, contains: `"role":"project.owner"`},
		{name: "grant owner", method: http.MethodPut, path: base + "/principals/" + testServicePrincipalID + "/grants/project.owner", wantStatus: http.StatusOK, contains: `"active":true`},
		{name: "revoke owner", method: http.MethodDelete, path: base + "/principals/" + testServicePrincipalID + "/grants/project.owner", wantStatus: http.StatusOK, contains: `"active":false`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			response := performAccessRequest(handler, test.method, test.path, test.body, "access-request-42")
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.contains) {
				t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("Cache-Control = %q", response.Header().Get("Cache-Control"))
			}
			if test.name == "issue credential" {
				if response.Header().Get("Pragma") != "no-cache" {
					t.Fatalf("Pragma = %q", response.Header().Get("Pragma"))
				}
			} else if strings.Contains(response.Body.String(), testIssuedToken) ||
				strings.Contains(response.Body.String(), "secret_digest") {
				t.Fatalf("non-issuance response leaked secret material: %q", response.Body.String())
			}
			if test.name == "list grants" && strings.Contains(response.Body.String(), otherGrant.PrincipalID()) {
				t.Fatalf("principal grant route leaked another principal: %q", response.Body.String())
			}
		})
	}
}

func TestAccessAdministrationRejectsAuthenticationAndUntrustedInput(t *testing.T) {
	createCalls := 0
	issueCalls := 0
	service := &fakeAccessAdministration{
		createPrincipal: func(context.Context, applicationaccess.Invocation, applicationaccess.CreatePrincipalInput) (domainaccess.Principal, error) {
			createCalls++
			return domainaccess.Principal{}, nil
		},
		issueCredential: func(context.Context, applicationaccess.Invocation, applicationaccess.IssueCredentialInput) (applicationaccess.IssuedCredential, error) {
			issueCalls++
			return applicationaccess.IssuedCredential{}, nil
		},
	}
	handler := newAccessTestHandler(t, service, testServicePrincipalID)
	base := "/api/admin/core/v1alpha1/access"

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, base+"/principals", nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unauthorized status/cache/body = %d %q %q", unauthorized.Code, unauthorized.Header().Get("Cache-Control"), unauthorized.Body.String())
	}
	invalidPrincipal := performAccessRequest(
		handler, http.MethodGet, base+"/principals/bad%20principal/grants", "", "invalid-principal",
	)
	if invalidPrincipal.Code != http.StatusBadRequest {
		t.Fatalf("invalid principal status/body = %d %q", invalidPrincipal.Code, invalidPrincipal.Body.String())
	}

	deepBody := strings.Repeat(`{"nested":`, maxAccessJSONDepth+1) + `true` +
		strings.Repeat(`}`, maxAccessJSONDepth+1)
	principalBodies := []string{
		`{"display_name":"bot","project_id":"` + testProjectID + `"}`,
		`{"display_name":"bot","environment_id":"` + testEnvironmentID + `"}`,
		`{"display_name":"bot","status":"active"}`,
		`{"display_name":"bot","role":"project.owner"}`,
		`{"display_name":"bot","secret":"hidden"}`,
		`{"display_name":"bot","display_name":"duplicate"}`,
		`{"display_name":"bot","DISPLAY_NAME":"case duplicate"}`,
		`{"display_name":"bot","diſplay_name":"unicode fold duplicate"}`,
		`{"display_name":"bot"}{}`,
		`[]`,
		deepBody,
	}
	for _, body := range principalBodies {
		response := performAccessRequest(handler, http.MethodPost, base+"/principals", body, "strict-principal")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("principal body %q status/body = %d %q", body, response.Code, response.Body.String())
		}
	}
	credentialBodies := []string{
		`{"label":"deployment","principal_id":"` + testServicePrincipalID + `"}`,
		`{"label":"deployment","project_id":"` + testProjectID + `"}`,
		`{"label":"deployment","secret":"hidden"}`,
		`{"label":"deployment","status":"active"}`,
		`{"label":"deployment","role":"project.owner"}`,
		`{"label":"deployment","label":"duplicate"}`,
		`{"label":"deployment","LABEL":"case duplicate"}`,
		`{"label":"deployment"}{}`,
	}
	for _, body := range credentialBodies {
		response := performAccessRequest(
			handler, http.MethodPost,
			base+"/principals/"+testServicePrincipalID+"/credentials",
			body, "strict-credential",
		)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("credential body %q status/body = %d %q", body, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{
		base + "/principals/" + testServicePrincipalID + "/disable",
		base + "/credentials/" + testCredentialID + "/revoke",
		base + "/principals/" + testServicePrincipalID + "/grants/project.owner",
	} {
		method := http.MethodPost
		if strings.Contains(path, "project.owner") {
			method = http.MethodPut
		}
		response := performAccessRequest(handler, method, path, `{}`, "strict-empty")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("non-empty mutation %s status/body = %d %q", path, response.Code, response.Body.String())
		}
	}
	if createCalls != 0 || issueCalls != 0 {
		t.Fatalf("strictly rejected input reached Application: create=%d issue=%d", createCalls, issueCalls)
	}
}

func TestAccessAdministrationMethodAndErrorContract(t *testing.T) {
	base := "/api/admin/core/v1alpha1/access"
	methods := []struct {
		path  string
		allow string
	}{
		{path: base + "/principals", allow: "GET, POST"},
		{path: base + "/principals/" + testServicePrincipalID + "/disable", allow: "POST"},
		{path: base + "/principals/" + testServicePrincipalID + "/credentials", allow: "GET, POST"},
		{path: base + "/credentials/" + testCredentialID + "/revoke", allow: "POST"},
		{path: base + "/principals/" + testServicePrincipalID + "/grants", allow: "GET"},
		{path: base + "/principals/" + testServicePrincipalID + "/grants/project.owner", allow: "PUT, DELETE"},
	}
	handler := newAccessTestHandler(t, &fakeAccessAdministration{}, testServicePrincipalID)
	for _, test := range methods {
		response := performAccessRequest(handler, http.MethodPatch, test.path, "", "method-check")
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != test.allow {
			t.Fatalf("PATCH %s status/allow = %d %q", test.path, response.Code, response.Header().Get("Allow"))
		}
	}

	for _, test := range []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{err: applicationaccess.ErrUnauthenticated, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{err: applicationaccess.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{err: applicationaccess.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "access_not_found"},
		{err: applicationaccess.ErrConflict, wantStatus: http.StatusConflict, wantCode: "access_conflict"},
		{err: applicationaccess.ErrLastOwnerPath, wantStatus: http.StatusConflict, wantCode: "last_owner_path"},
		{err: applicationaccess.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "access_unavailable"},
	} {
		test := test
		t.Run(test.wantCode, func(t *testing.T) {
			errorHandler := newAccessTestHandler(t, &fakeAccessAdministration{
				listPrincipals: func(context.Context, applicationaccess.Invocation) ([]domainaccess.Principal, error) {
					return nil, test.err
				},
			}, testServicePrincipalID)
			response := performAccessRequest(errorHandler, http.MethodGet, base+"/principals", "", "error-check")
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("status/cache/body = %d %q %q", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
			}
			assertErrorEnvelope(t, response, test.wantCode)
			if strings.Contains(response.Body.String(), "token") || strings.Contains(response.Body.String(), "digest") {
				t.Fatalf("error leaked secret fields: %q", response.Body.String())
			}
		})
	}
}

func newAccessTestHandler(
	t *testing.T,
	administration AccessAdministration,
	principalID string,
) *Handler {
	t.Helper()
	projectContext, err := project.NewContext(testProjectID, "crm", "en-US", "UTC", "USD")
	if err != nil {
		t.Fatal(err)
	}
	publicActor, err := actor.NewAnonymous(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	scope := testProjectScope(t)
	handler, err := New(Config{
		Project: projectContext, Scope: scope, PublicActor: publicActor,
		Module: fakeModule{}, Records: &fakeRecordService{},
		AdminAuth:            newTestAdminAuthForPrincipal(t, scope, principalID),
		AccessAdministration: administration,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func performAccessRequest(
	handler http.Handler,
	method string,
	path string,
	body string,
	requestIDValue string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	request.Header.Set(requestIDHeader, requestIDValue)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func testAccessPrincipal(t *testing.T, principalID string, disabled bool) domainaccess.Principal {
	t.Helper()
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	status := domainaccess.PrincipalStatusActive
	updatedAt := issuedAt
	var disabledAt *time.Time
	if disabled {
		status = domainaccess.PrincipalStatusDisabled
		value := issuedAt.Add(time.Minute)
		updatedAt = value
		disabledAt = &value
	}
	value, err := domainaccess.NewPrincipal(domainaccess.PrincipalMaterial{
		ProjectID: projectID, ID: principalID, Kind: domainaccess.PrincipalKindService,
		DisplayName: "deploy bot", Status: status,
		CreatedAt: issuedAt, UpdatedAt: updatedAt, DisabledAt: disabledAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testAccessCredential(
	t *testing.T,
	credentialIDText string,
	principalID string,
	revoked bool,
) domainaccess.Credential {
	t.Helper()
	credentialID, err := domainaccess.ParseID(credentialIDText)
	if err != nil {
		t.Fatal(err)
	}
	issuedAt := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	status := domainaccess.CredentialStatusActive
	revokedBy := ""
	var revokedAt *time.Time
	if revoked {
		status = domainaccess.CredentialStatusRevoked
		value := issuedAt.Add(time.Minute)
		revokedBy = testServicePrincipalID
		revokedAt = &value
	}
	value, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: testProjectScope(t), ID: credentialID, PrincipalID: principalID,
		Label: "deployment", Hint: "sha256:0123456789ab", Status: status,
		IssuedBy: testServicePrincipalID, IssuedAt: issuedAt,
		RevokedBy: revokedBy, RevokedAt: revokedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testOwnerGrant(t *testing.T, principalID string, revoked bool) domainaccess.OwnerGrant {
	t.Helper()
	grantedAt := time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC)
	revokedBy := ""
	var revokedAt *time.Time
	if revoked {
		value := grantedAt.Add(time.Minute)
		revokedBy = testServicePrincipalID
		revokedAt = &value
	}
	value, err := domainaccess.NewOwnerGrant(domainaccess.OwnerGrantMaterial{
		Scope: testProjectScope(t), PrincipalID: principalID,
		GrantedBy: testServicePrincipalID, GrantedAt: grantedAt,
		RevokedBy: revokedBy, RevokedAt: revokedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

var _ AccessAdministration = (*fakeAccessAdministration)(nil)
