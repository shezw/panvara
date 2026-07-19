/*
   Panvara
   internal/interfaces/httpapi/release_handler_test.go    2026-07-19
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const (
	testReleaseID      = "01981234-5678-7abc-8def-0123456789ac"
	testReleaseSource  = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	testAuthCredential = "018f7e93-7b2f-7abc-8def-1234567890ab"
)

func TestReleasePublishReplayAndGetContract(t *testing.T) {
	t.Parallel()
	value := makeHTTPRelease(t, "request-original")
	calls := 0
	service := &fakeReleasePublisherService{
		publish: func(_ context.Context, invocation access.Invocation, module string, input releaseapp.PublishInput) (domainrelease.ModuleRelease, bool, error) {
			if invocation.Execution().Scope().ProjectID().String() != testProjectID ||
				invocation.Execution().Scope().EnvironmentID().String() != testEnvironmentID ||
				invocation.Execution().Actor().ActorID() != "bootstrap-admin" || invocation.RequestID() == "" ||
				module != "crm" || input.PlanID != testPlanID {
				t.Fatalf("publish invocation/module/input = %#v/%q/%#v", invocation, module, input)
			}
			wantKeys := []string{"release-first", "release-first", "release-second-key"}
			if calls >= len(wantKeys) || input.IdempotencyKey != wantKeys[calls] {
				t.Fatalf("publish idempotency key call %d = %q", calls, input.IdempotencyKey)
			}
			calls++
			return value, calls == 1, nil
		},
		get: func(_ context.Context, invocation access.Invocation, module, releaseID string) (domainrelease.ModuleRelease, error) {
			if invocation.RequestID() == "" || module != "crm" || releaseID != testReleaseID {
				t.Fatalf("get invocation/module/release = %#v/%q/%q", invocation, module, releaseID)
			}
			return value, nil
		},
	}
	handler := newTestHandlerWithReleases(t, service)
	collection := "/api/admin/core/v1alpha1/modules/crm/releases"

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, collection, nil))
	if unauthorized.Code != http.StatusUnauthorized || unauthorized.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("unauthorized status/cache/body = %d %q %q", unauthorized.Code, unauthorized.Header().Get("Cache-Control"), unauthorized.Body.String())
	}

	for index, test := range []struct {
		key        string
		wantStatus int
	}{
		{key: "release-first", wantStatus: http.StatusCreated},
		{key: "release-first", wantStatus: http.StatusOK},
		{key: "release-second-key", wantStatus: http.StatusOK},
	} {
		response := performReleaseAdminRequest(handler, http.MethodPost, collection, testPlanID, test.key)
		if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" ||
			response.Header().Get("Location") != releaseLocation("crm", testReleaseID) {
			t.Fatalf("publish %d status/headers/body = %d %#v %q", index, response.Code, response.Header(), response.Body.String())
		}
		assertReleaseResponse(t, response, value)
	}

	get := performReleaseAdminRequest(
		handler, http.MethodGet, releaseLocation("crm", testReleaseID), "", "",
	)
	if get.Code != http.StatusOK || get.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("GET status/headers/body = %d %#v %q", get.Code, get.Header(), get.Body.String())
	}
	assertReleaseResponse(t, get, value)
}

func TestReleasePublishRequiresStrictRequest(t *testing.T) {
	t.Parallel()
	called := false
	service := &fakeReleasePublisherService{publish: func(
		context.Context, access.Invocation, string, releaseapp.PublishInput,
	) (domainrelease.ModuleRelease, bool, error) {
		called = true
		return domainrelease.ModuleRelease{}, false, errors.New("unexpected invalid release Publish")
	}}
	handler := newTestHandlerWithReleases(t, service)
	path := "/api/admin/core/v1alpha1/modules/crm/releases"
	validBody := `{"plan_id":"` + testPlanID + `"}`
	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		keys        []string
		wantStatus  int
		wantCode    string
	}{
		{name: "query", path: path + "?unexpected=1", body: validBody, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_query"},
		{name: "empty query marker", path: path + "?", body: validBody, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_query"},
		{name: "missing key", path: path, body: validBody, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "idempotency_key_required"},
		{name: "duplicate key", path: path, body: validBody, contentType: "application/json", keys: []string{"key-a", "key-b"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_idempotency_key"},
		{name: "unsafe key", path: path, body: validBody, contentType: "application/json", keys: []string{"key with space"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_idempotency_key"},
		{name: "missing content type", path: path, body: validBody, keys: []string{"key"}, wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "unknown field", path: path, body: `{"plan_id":"` + testPlanID + `","activate":true}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "case alias", path: path, body: `{"PLAN_ID":"` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "unicode alias", path: path, body: `{"plan_ıd":"` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "escaped first character", path: path, body: `{"\u0070lan_id":"` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "escaped underscore", path: path, body: `{"plan\u005fid":"` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "duplicate field", path: path, body: `{"plan_id":"` + testPlanID + `","plan_id":"` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "trailing value", path: path, body: validBody + `{}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "array", path: path, body: `[]`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "null", path: path, body: `{"plan_id":null}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "uppercase digest", path: path, body: `{"plan_id":"SHA256:` + strings.Repeat("a", 64) + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "padded digest", path: path, body: `{"plan_id":" ` + testPlanID + `"}`, contentType: "application/json", keys: []string{"key"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Authorization", "Bearer "+testAdminToken)
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			for _, key := range test.keys {
				request.Header.Add("Idempotency-Key", key)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("status/cache/body = %d %q %q, want %d", response.Code, response.Header().Get("Cache-Control"), response.Body.String(), test.wantStatus)
			}
			assertErrorEnvelope(t, response, test.wantCode)
			if called {
				t.Fatal("Publish ran for an invalid HTTP request")
			}
		})
	}
}

func TestReleaseGetRejectsQueryBodyAndWrongMethods(t *testing.T) {
	t.Parallel()
	service := &fakeReleasePublisherService{get: func(
		context.Context, access.Invocation, string, string,
	) (domainrelease.ModuleRelease, error) {
		t.Fatal("Get must not run for an invalid HTTP request")
		return domainrelease.ModuleRelease{}, nil
	}}
	handler := newTestHandlerWithReleases(t, service)
	item := releaseLocation("crm", testReleaseID)
	for _, test := range []struct {
		name, method, path, body string
		wantStatus               int
		wantCode                 string
		allow                    string
	}{
		{name: "query", method: http.MethodGet, path: item + "?detail=true", wantStatus: http.StatusBadRequest, wantCode: "invalid_query"},
		{name: "body", method: http.MethodGet, path: item, body: `{}`, wantStatus: http.StatusBadRequest, wantCode: "unexpected_request_body"},
		{name: "item method", method: http.MethodPost, path: item, wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", allow: http.MethodGet},
		{name: "collection method", method: http.MethodGet, path: "/api/admin/core/v1alpha1/modules/crm/releases", wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", allow: http.MethodPost},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer "+testAdminToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" ||
			(test.allow != "" && response.Header().Get("Allow") != test.allow) {
			t.Fatalf("%s status/headers/body = %d %#v %q", test.name, response.Code, response.Header(), response.Body.String())
		}
		assertErrorEnvelope(t, response, test.wantCode)
	}
}

func TestReleaseApplicationErrorsMapToStableHTTPStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "invalid", err: releaseapp.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: "invalid_release_request"},
		{name: "not found", err: releaseapp.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "release_not_found"},
		{name: "idempotency", err: releaseapp.ErrIdempotencyConflict, wantStatus: http.StatusConflict, wantCode: "idempotency_key_conflict"},
		{name: "stale", err: releaseapp.ErrStale, wantStatus: http.StatusConflict, wantCode: "stale_plan"},
		{name: "not publishable", err: releaseapp.ErrNotPublishable, wantStatus: http.StatusUnprocessableEntity, wantCode: "not_publishable"},
		{name: "corrupt", err: releaseapp.ErrCorrupt, wantStatus: http.StatusServiceUnavailable, wantCode: "release_unavailable"},
		{name: "unavailable", err: releaseapp.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "release_unavailable"},
		{name: "forbidden", err: access.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "scope inactive", err: access.ErrScopeInactive, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "authority unavailable", err: access.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "access_unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeReleasePublisherService{publish: func(
				context.Context, access.Invocation, string, releaseapp.PublishInput,
			) (domainrelease.ModuleRelease, bool, error) {
				return domainrelease.ModuleRelease{}, false, test.err
			}}
			handler := newTestHandlerWithReleases(t, service)
			response := performReleaseAdminRequest(
				handler, http.MethodPost, "/api/admin/core/v1alpha1/modules/crm/releases", testPlanID, "release-error",
			)
			if response.Code != test.wantStatus || response.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("status/cache/body = %d %q %q", response.Code, response.Header().Get("Cache-Control"), response.Body.String())
			}
			assertErrorEnvelope(t, response, test.wantCode)
		})
	}
}

func newTestHandlerWithReleases(t *testing.T, releases ReleasePublisherService) *Handler {
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
		Releases: releases, AdminAuth: newTestAdminAuth(t, scope), AccessAdministration: &fakeAccessAdministration{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func makeHTTPRelease(t *testing.T, requestID string) domainrelease.ModuleRelease {
	t.Helper()
	releaseID, err := domainrelease.ParseID(testReleaseID)
	if err != nil {
		t.Fatal(err)
	}
	draftID, err := domainmodule.ParseDraftID(testDraftID)
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := domainaccess.ParseID(testAuthCredential)
	if err != nil {
		t.Fatal(err)
	}
	value, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: releaseID, Scope: testProjectScope(t), ModuleName: "crm",
		DraftID: draftID, DraftGeneration: 3, ValidationID: testValidationID,
		PlanID: testPlanID, PlanHash: testPlanHash, BaselineRevision: testRevision,
		CandidateRevision: testCandidate, DataSchemaFormat: 1, DataSchemaFingerprint: testDataSchema,
		SourceHash: testReleaseSource, Outcome: domainrelease.OutcomeCompatible, Risk: "low",
		PublishedBy: "bootstrap-admin", PublishedCredentialID: credentialID,
		RequestID: requestID, PublishedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func performReleaseAdminRequest(
	handler http.Handler,
	method, path, planID, idempotencyKey string,
) *httptest.ResponseRecorder {
	body := ""
	if planID != "" {
		body = `{"plan_id":"` + planID + `"}`
	}
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertReleaseResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	want domainrelease.ModuleRelease,
) {
	t.Helper()
	var value releaseResponse
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value.ReleaseID != want.ID().String() || value.Module != want.ModuleName() ||
		value.DraftID != want.DraftID().String() || value.DraftGeneration != want.DraftGeneration() ||
		value.ValidationID != want.ValidationID() || value.PlanID != want.PlanID() || value.PlanHash != want.PlanHash() ||
		value.BaselineRevision == nil || *value.BaselineRevision != want.BaselineRevision() ||
		value.CandidateRevision != want.CandidateRevision() ||
		value.DataSchemaIdentity.Format != want.DataSchemaFormat() ||
		value.DataSchemaIdentity.Fingerprint != want.DataSchemaFingerprint() ||
		value.SourceHash != want.SourceHash() || value.Outcome != want.Outcome() || value.Risk != want.Risk() ||
		value.PublishedBy != want.PublishedBy() || value.PublishedCredentialID != want.PublishedCredentialID().String() ||
		value.RequestID != want.RequestID() || !value.PublishedAt.Equal(want.PublishedAt()) {
		t.Fatalf("release response = %#v, want %#v", value, want)
	}
	if !value.Effects.RevisionRegistered || !value.Effects.Published || value.Effects.Activated ||
		value.Effects.RecordsMigrated || value.Effects.RuntimeChanged || value.Effects.ActivationSupported {
		t.Fatalf("release effects = %#v", value.Effects)
	}
}

type fakeReleasePublisherService struct {
	publish func(context.Context, access.Invocation, string, releaseapp.PublishInput) (domainrelease.ModuleRelease, bool, error)
	get     func(context.Context, access.Invocation, string, string) (domainrelease.ModuleRelease, error)
}

func (service *fakeReleasePublisherService) Publish(
	ctx context.Context,
	invocation access.Invocation,
	module string,
	input releaseapp.PublishInput,
) (domainrelease.ModuleRelease, bool, error) {
	if service.publish == nil {
		return domainrelease.ModuleRelease{}, false, errors.New("unexpected release Publish")
	}
	return service.publish(ctx, invocation, module, input)
}

func (service *fakeReleasePublisherService) Get(
	ctx context.Context,
	invocation access.Invocation,
	module, releaseID string,
) (domainrelease.ModuleRelease, error) {
	if service.get == nil {
		return domainrelease.ModuleRelease{}, errors.New("unexpected release Get")
	}
	return service.get(ctx, invocation, module, releaseID)
}

var _ ReleasePublisherService = (*fakeReleasePublisherService)(nil)
