/*
   Panvara
   internal/interfaces/httpapi/auth_test.go    2026-07-14
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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shezw/panvara/internal/domain/actor"
)

const testAdminToken = "panvara-test-token-32-bytes-minimum"

func TestBootstrapAdminAuth(t *testing.T) {
	t.Parallel()
	adminActor, err := actor.New("018f7e93-7b2c-7abc-8def-1234567890ab", "bootstrap-admin", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewBootstrapAdminAuth(testAdminToken, adminActor)
	if err != nil {
		t.Fatal(err)
	}
	protected := withRequestID(auth.Middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, ok := authenticatedActorFromContext(request.Context()); !ok || got.ActorID() != "bootstrap-admin" {
			t.Errorf("authenticated actor = %+v, ok = %v", got, ok)
		}
		writer.WriteHeader(http.StatusNoContent)
	})))

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{name: "valid", header: "Bearer " + testAdminToken, wantStatus: http.StatusNoContent},
		{name: "case insensitive scheme", header: "bearer " + testAdminToken, wantStatus: http.StatusNoContent},
		{name: "missing", wantStatus: http.StatusUnauthorized},
		{name: "wrong", header: "Bearer not-the-token", wantStatus: http.StatusUnauthorized},
		{name: "basic", header: "Basic " + testAdminToken, wantStatus: http.StatusUnauthorized},
		{name: "extra whitespace", header: "Bearer  " + testAdminToken, wantStatus: http.StatusUnauthorized},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodGet, "/api/admin/v1alpha1/crm/leads", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			response := httptest.NewRecorder()
			protected.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusUnauthorized {
				var envelope ErrorEnvelope
				if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Code != "unauthorized" || envelope.RequestID == "" {
					t.Fatalf("error envelope = %+v", envelope)
				}
				if got := response.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer ") {
					t.Fatalf("WWW-Authenticate = %q", got)
				}
			}
		})
	}
}

func TestBootstrapAdminAuthRejectsWeakOrAmbiguousToken(t *testing.T) {
	t.Parallel()
	adminActor, err := actor.New("018f7e93-7b2c-7abc-8def-1234567890ab", "bootstrap-admin", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"short", strings.Repeat("x", 31), strings.Repeat("x", 32) + " "} {
		if _, err := NewBootstrapAdminAuth(token, adminActor); err == nil {
			t.Fatalf("NewBootstrapAdminAuth(%q) accepted invalid token", token)
		}
	}
}

func TestBootstrapAdminAuthDoesNotTreatActorRolesAsAuthority(t *testing.T) {
	t.Parallel()
	adminActor, err := actor.New(
		"018f7e93-7b2c-7abc-8def-1234567890ab", "bootstrap-admin", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewBootstrapAdminAuth(testAdminToken, adminActor); err != nil {
		t.Fatalf("NewBootstrapAdminAuth(actor without self-reported role) error = %v", err)
	}
}
