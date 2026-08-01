/*
   Panvara
   internal/interfaces/httpapi/activation_handler_test.go    2026-08-02
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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

func TestReleaseActivateAndGetActiveContract(t *testing.T) {
	t.Parallel()
	value := makeHTTPActiveSnapshot(t)
	activateCalls := 0
	service := &fakeReleaseActivationService{
		activate: func(
			_ context.Context,
			invocation access.Invocation,
			module string,
			releaseID string,
		) (domainrelease.ActiveSnapshot, bool, error) {
			if invocation.Execution().Actor().ActorID() != "bootstrap-admin" || invocation.RequestID() == "" ||
				module != "crm" || releaseID != testReleaseID {
				t.Fatalf("Activate invocation/module/release = %#v/%q/%q", invocation, module, releaseID)
			}
			activateCalls++
			return value, activateCalls == 1, nil
		},
		getActive: func(
			_ context.Context,
			invocation access.Invocation,
			module string,
		) (domainrelease.ActiveSnapshot, error) {
			if invocation.RequestID() == "" || module != "crm" {
				t.Fatalf("GetActive invocation/module = %#v/%q", invocation, module)
			}
			return value, nil
		},
	}
	handler := newTestHandlerWithActivations(t, service)
	activatePath := releaseLocation("crm", testReleaseID) + "/activate"

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, activatePath, nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unauthorized status/cache/body = %d %q %q", unauthorized.Code, unauthorized.Header().Get("Cache-Control"), unauthorized.Body.String())
	}

	for index, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		response := performActivationAdminRequest(handler, http.MethodPost, activatePath, "")
		if response.Code != wantStatus ||
			response.Header().Get("Location") != activeReleaseLocation("crm") ||
			response.Header().Get("ETag") != `"release-epoch-2"` ||
			response.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("Activate %d status/headers/body = %d %#v %q", index, response.Code, response.Header(), response.Body.String())
		}
		assertActiveSnapshotResponse(t, response, value)
	}

	active := performActivationAdminRequest(handler, http.MethodGet, activeReleaseLocation("crm"), "")
	if active.Code != http.StatusOK || active.Header().Get("ETag") != `"release-epoch-2"` ||
		active.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("GetActive status/headers/body = %d %#v %q", active.Code, active.Header(), active.Body.String())
	}
	assertActiveSnapshotResponse(t, active, value)
}

func TestReleaseActivationRequiresStrictEmptyRequest(t *testing.T) {
	t.Parallel()
	called := false
	service := &fakeReleaseActivationService{
		activate: func(context.Context, access.Invocation, string, string) (domainrelease.ActiveSnapshot, bool, error) {
			called = true
			return domainrelease.ActiveSnapshot{}, false, errors.New("unexpected Activate")
		},
		getActive: func(context.Context, access.Invocation, string) (domainrelease.ActiveSnapshot, error) {
			called = true
			return domainrelease.ActiveSnapshot{}, errors.New("unexpected GetActive")
		},
	}
	handler := newTestHandlerWithActivations(t, service)
	activate := releaseLocation("crm", testReleaseID) + "/activate"
	active := activeReleaseLocation("crm")
	tests := []struct {
		name, method, path, body string
		wantStatus               int
		wantCode, allow          string
	}{
		{name: "activate query", method: http.MethodPost, path: activate + "?force=true", wantStatus: http.StatusBadRequest, wantCode: "invalid_query"},
		{name: "activate body", method: http.MethodPost, path: activate, body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "unexpected_request_body"},
		{name: "activate method", method: http.MethodGet, path: activate, wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", allow: http.MethodPost},
		{name: "active query", method: http.MethodGet, path: active + "?detail=true", wantStatus: http.StatusBadRequest, wantCode: "invalid_query"},
		{name: "active body", method: http.MethodGet, path: active, body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "unexpected_request_body"},
		{name: "active method", method: http.MethodPost, path: active, wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", allow: http.MethodGet},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performActivationAdminRequest(handler, test.method, test.path, test.body)
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" ||
				(test.allow != "" && response.Header().Get("Allow") != test.allow) {
				t.Fatalf("status/headers/body = %d %#v %q", response.Code, response.Header(), response.Body.String())
			}
			assertErrorEnvelope(t, response, test.wantCode)
			if called {
				t.Fatal("activation service ran for invalid HTTP request")
			}
		})
	}
}

func TestReleaseActivationErrorsMapToStableHTTPStatus(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "not activatable", err: releaseapp.ErrNotActivatable, wantStatus: http.StatusUnprocessableEntity, wantCode: "not_activatable"},
		{name: "wrapped conflict", err: fmt.Errorf("activate: %w", releaseapp.ErrActivationConflict), wantStatus: http.StatusConflict, wantCode: "activation_conflict"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeReleaseActivationService{activate: func(
				context.Context, access.Invocation, string, string,
			) (domainrelease.ActiveSnapshot, bool, error) {
				return domainrelease.ActiveSnapshot{}, false, test.err
			}}
			handler := newTestHandlerWithActivations(t, service)
			response := performActivationAdminRequest(
				handler, http.MethodPost, releaseLocation("crm", testReleaseID)+"/activate", "",
			)
			if response.Code != test.wantStatus {
				t.Fatalf("status/body = %d %q, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			assertErrorEnvelope(t, response, test.wantCode)
		})
	}
}

func newTestHandlerWithActivations(t *testing.T, activations ReleaseActivationService) *Handler {
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
		Project: projectContext, Scope: scope,
		PublicActor: publicActor, Module: fakeModule{}, Records: &fakeRecordService{},
		Activations: activations, AdminAuth: newTestAdminAuth(t, scope),
		AccessAdministration: &fakeAccessAdministration{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func makeHTTPActiveSnapshot(t *testing.T) domainrelease.ActiveSnapshot {
	t.Helper()
	releaseID, err := domainrelease.ParseID(testReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := domainaccess.ParseID(testAuthCredential)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := domainrelease.NewModuleBinding(domainrelease.ModuleBindingMaterial{
		ModuleName: "crm", ReleaseID: &releaseID, RuntimeRevision: testCandidate,
		RecordNamespaceRevision: testRevision, DataSchemaFormat: 1,
		DataSchemaFingerprint: testDataSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	value, err := domainrelease.NewActiveSnapshot(domainrelease.ActiveSnapshotMaterial{
		Scope: testProjectScope(t), Epoch: 2, Origin: domainrelease.ActiveSnapshotOriginRelease,
		Binding: binding, ActivatedBy: "bootstrap-admin", ActivatedCredentialID: &credentialID,
		RequestID: "activate-request", ActivatedAt: time.Date(2026, 8, 2, 1, 2, 3, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func performActivationAdminRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertActiveSnapshotResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	want domainrelease.ActiveSnapshot,
) {
	t.Helper()
	var value activeSnapshotResponse
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.Module != want.ModuleName() || value.ReleaseID == nil || *value.ReleaseID != testReleaseID ||
		value.RuntimeRevision != want.RuntimeRevision() ||
		value.RecordNamespaceRevision != want.RecordNamespaceRevision() ||
		value.DataSchemaIdentity.Format != want.DataSchemaFormat() ||
		value.DataSchemaIdentity.Fingerprint != want.DataSchemaFingerprint() ||
		value.Epoch != want.Epoch() || value.Origin != want.Origin() ||
		value.ActivatedBy != want.ActivatedBy() || value.ActivatedCredentialID == nil ||
		*value.ActivatedCredentialID != testAuthCredential || value.RequestID != want.RequestID() ||
		!value.ActivatedAt.Equal(want.ActivatedAt()) {
		t.Fatalf("active snapshot response = %#v, want %#v", value, want)
	}
}

type fakeReleaseActivationService struct {
	activate  func(context.Context, access.Invocation, string, string) (domainrelease.ActiveSnapshot, bool, error)
	getActive func(context.Context, access.Invocation, string) (domainrelease.ActiveSnapshot, error)
}

func (service *fakeReleaseActivationService) Activate(
	ctx context.Context,
	invocation access.Invocation,
	module, releaseID string,
) (domainrelease.ActiveSnapshot, bool, error) {
	if service.activate == nil {
		return domainrelease.ActiveSnapshot{}, false, errors.New("unexpected Release Activate")
	}
	return service.activate(ctx, invocation, module, releaseID)
}

func (service *fakeReleaseActivationService) GetActive(
	ctx context.Context,
	invocation access.Invocation,
	module string,
) (domainrelease.ActiveSnapshot, error) {
	if service.getActive == nil {
		return domainrelease.ActiveSnapshot{}, errors.New("unexpected Release GetActive")
	}
	return service.getActive(ctx, invocation, module)
}

var _ ReleaseActivationService = (*fakeReleaseActivationService)(nil)
