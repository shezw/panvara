/*
   Panvara
   internal/interfaces/httpapi/auth_test.go    2026-07-19
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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const testAdminToken = "panvara-test-token-32-bytes-minimum"

var errAuthenticationStore = errors.New("authentication store unavailable")

type fakeCredentialLookup struct {
	candidate applicationaccess.CredentialCandidate
	err       error
}

type fixedPrincipalAuthenticator struct {
	principal applicationaccess.AuthenticatedPrincipal
}

func (authenticator fixedPrincipalAuthenticator) Authenticate(
	context.Context,
	project.Scope,
	string,
) (applicationaccess.AuthenticatedPrincipal, error) {
	return authenticator.principal, nil
}

func (lookup fakeCredentialLookup) LookupCredential(
	context.Context,
	project.Scope,
	applicationaccess.CredentialSelector,
) (applicationaccess.CredentialCandidate, error) {
	return lookup.candidate, lookup.err
}

func TestCredentialAdminAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		headers    []string
		lookupErr  error
		revoked    bool
		wantStatus int
		wantCode   string
	}{
		{name: "valid", headers: []string{"Bearer " + testAdminToken}, wantStatus: http.StatusNoContent},
		{name: "case insensitive scheme", headers: []string{"bearer " + testAdminToken}, wantStatus: http.StatusNoContent},
		{name: "missing", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "unknown", headers: []string{"Bearer " + testAdminToken}, lookupErr: applicationaccess.ErrNotFound, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "revoked", headers: []string{"Bearer " + testAdminToken}, revoked: true, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "disabled principal", headers: []string{"Bearer " + testAdminToken}, lookupErr: applicationaccess.ErrNotFound, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "wrong", headers: []string{"Bearer not-the-token-and-at-least-thirty-two-bytes"}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "basic", headers: []string{"Basic " + testAdminToken}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "empty", headers: []string{"Bearer "}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "extra whitespace", headers: []string{"Bearer  " + testAdminToken}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "tab separator", headers: []string{"Bearer\t" + testAdminToken}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "duplicate", headers: []string{"Bearer " + testAdminToken, "Bearer " + testAdminToken}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "collapsed duplicate", headers: []string{"Bearer " + testAdminToken + ",Bearer" + testAdminToken}, wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "authority failure", headers: []string{"Bearer " + testAdminToken}, lookupErr: errAuthenticationStore, wantStatus: http.StatusServiceUnavailable, wantCode: "authentication_unavailable"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := testProjectScope(t)
			candidate := testCredentialCandidate(
				t, scope, "svc:018f7e93-7b2e-7abc-8def-1234567890ab", test.revoked,
			)
			authenticator, err := applicationaccess.NewCredentialAuthenticator(fakeCredentialLookup{
				candidate: candidate, err: test.lookupErr,
			})
			if err != nil {
				t.Fatal(err)
			}
			auth, err := NewCredentialAdminAuth(scope, authenticator)
			if err != nil {
				t.Fatal(err)
			}
			protected := withRequestID(auth.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				principal, ok := authenticatedPrincipalFromContext(request.Context())
				if !ok || principal.Actor().ActorID() != "svc:018f7e93-7b2e-7abc-8def-1234567890ab" {
					t.Errorf("authenticated principal = %+v, ok = %v", principal, ok)
				}
				writer.WriteHeader(http.StatusNoContent)
			})))

			request := httptest.NewRequest(http.MethodGet, "/api/admin/v1alpha1/crm/leads", nil)
			for _, value := range test.headers {
				request.Header.Add("Authorization", value)
			}
			response := httptest.NewRecorder()
			protected.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status/body = %d %q, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			if test.wantCode == "" {
				return
			}
			var envelope ErrorEnvelope
			if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Code != test.wantCode || envelope.RequestID == "" {
				t.Fatalf("error envelope = %+v", envelope)
			}
			if test.wantStatus == http.StatusUnauthorized {
				if got := response.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") {
					t.Fatalf("WWW-Authenticate = %q", got)
				}
			}
		})
	}
}

func TestCredentialAdminAuthRejectsInvalidConstruction(t *testing.T) {
	t.Parallel()

	if _, err := NewCredentialAdminAuth(project.Scope{}, nil); err == nil {
		t.Fatal("NewCredentialAdminAuth accepted an invalid scope and nil authenticator")
	}
	if _, err := NewCredentialAdminAuth(testProjectScope(t), nil); err == nil {
		t.Fatal("NewCredentialAdminAuth accepted a nil authenticator")
	}
}

func TestCredentialAdminAuthRejectsEvidenceFromAnotherEnvironment(t *testing.T) {
	t.Parallel()

	scope := testProjectScope(t)
	otherEnvironmentID, err := project.ParseEnvironmentID("018f7e93-7b30-7abc-8def-1234567890ab")
	if err != nil {
		t.Fatal(err)
	}
	otherScope, err := project.NewScope(scope.ProjectID(), otherEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	credentialAuthenticator, err := applicationaccess.NewCredentialAuthenticator(fakeCredentialLookup{
		candidate: testCredentialCandidate(
			t, otherScope, "svc:018f7e93-7b2e-7abc-8def-1234567890ab", false,
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := credentialAuthenticator.Authenticate(
		context.Background(), otherScope, testAdminToken,
	)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewCredentialAdminAuth(
		scope, fixedPrincipalAuthenticator{principal: principal},
	)
	if err != nil {
		t.Fatal(err)
	}
	protected := withRequestID(auth.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler ran with evidence from another environment")
	})))

	request := httptest.NewRequest(http.MethodGet, "/api/admin/core/v1alpha1/access/principals", nil)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status/body = %d %q, want 503", response.Code, response.Body.String())
	}
	assertErrorEnvelope(t, response, "authentication_unavailable")
}

func testCredentialCandidate(
	t *testing.T,
	scope project.Scope,
	principalID string,
	revoked bool,
) applicationaccess.CredentialCandidate {
	t.Helper()
	credentialID, err := domainaccess.ParseID("018f7e93-7b2f-7abc-8def-1234567890ab")
	if err != nil {
		t.Fatal(err)
	}
	status := domainaccess.CredentialStatusActive
	var revokedAt *time.Time
	revokedBy := ""
	if revoked {
		status = domainaccess.CredentialStatusRevoked
		value := time.Date(2026, time.July, 19, 8, 1, 0, 0, time.UTC)
		revokedAt = &value
		revokedBy = "bootstrap-admin"
	}
	credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: scope, ID: credentialID,
		PrincipalID: principalID,
		Label:       "test", Hint: "sha256:0123456789ab", Status: status,
		IssuedBy: "bootstrap-admin", IssuedAt: time.Date(2026, time.July, 19, 8, 0, 0, 0, time.UTC),
		RevokedBy: revokedBy, RevokedAt: revokedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return applicationaccess.CredentialCandidate{
		Credential: credential, SecretDigest: sha256.Sum256([]byte(testAdminToken)),
	}
}

func newTestAdminAuth(t *testing.T, scope project.Scope) *CredentialAdminAuth {
	return newTestAdminAuthForPrincipal(t, scope, "bootstrap-admin")
}

func newTestAdminAuthForPrincipal(
	t *testing.T,
	scope project.Scope,
	principalID string,
) *CredentialAdminAuth {
	t.Helper()
	authenticator, err := applicationaccess.NewCredentialAuthenticator(fakeCredentialLookup{
		candidate: testCredentialCandidate(t, scope, principalID, false),
	})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewCredentialAdminAuth(scope, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}
