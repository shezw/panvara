/*
   Panvara
   internal/interfaces/httpapi/handler_test.go    2026-07-14
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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

const (
	testProjectID     = "018f7e93-7b2c-7abc-8def-1234567890ab"
	testEnvironmentID = "018f7e93-7b2d-7abc-8def-1234567890ab"
	testRecordID      = "018f7e93-7b2c-7abc-8def-1234567890ac"
	testRevision      = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestSchemaEndpoints(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, &fakeRecordService{})

	tests := []struct {
		path     string
		contains string
	}{
		{path: "/api/core/v1alpha1/modules/crm/openapi.json", contains: `"openapi":"3.1.0"`},
		{path: "/api/core/v1alpha1/modules/crm/ui-schema.json", contains: `"schema":"manager.panvara.dev/v1alpha1"`},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("GET %s status/body = %d %q", test.path, response.Code, response.Body.String())
		}
		digest := sha256.Sum256(response.Body.Bytes())
		if got, want := response.Header().Get("ETag"), fmt.Sprintf(`"sha256:%x"`, digest); got != want {
			t.Fatalf("ETag = %q, want representation digest %q", got, want)
		}
	}
}

func TestSchemaArtifactETagsAreRepresentationBound(t *testing.T) {
	t.Parallel()
	handler := newTestHandler(t, &fakeRecordService{})
	paths := []string{
		"/api/core/v1alpha1/modules/crm/openapi.json",
		"/api/core/v1alpha1/modules/crm/ui-schema.json",
	}
	etags := make([]string, len(paths))
	for index, path := range paths {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status/body = %d %q", path, response.Code, response.Body.String())
		}
		etags[index] = response.Header().Get("ETag")
		digest := sha256.Sum256(response.Body.Bytes())
		if want := fmt.Sprintf(`"sha256:%x"`, digest); etags[index] != want {
			t.Fatalf("GET %s ETag = %q, want %q", path, etags[index], want)
		}
	}
	if etags[0] == etags[1] {
		t.Fatalf("OpenAPI and UI schema unexpectedly share ETag %q", etags[0])
	}
	for index, path := range paths {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("If-None-Match", etags[index])
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotModified || response.Body.Len() != 0 {
			t.Fatalf("conditional GET %s status/body = %d %q", path, response.Code, response.Body.String())
		}

		crossRequest := httptest.NewRequest(http.MethodGet, path, nil)
		crossRequest.Header.Set("If-None-Match", etags[1-index])
		crossResponse := httptest.NewRecorder()
		handler.ServeHTTP(crossResponse, crossRequest)
		if crossResponse.Code != http.StatusOK || crossResponse.Body.Len() == 0 {
			t.Fatalf("cross-artifact conditional GET %s status/body = %d %q", path, crossResponse.Code, crossResponse.Body.String())
		}
	}
}

func TestSchemaEndpointsExposeRealCompiledArtifacts(t *testing.T) {
	t.Parallel()
	module, err := appmodule.NewCompiler().Compile([]byte(`{
  "apiVersion":"panvara.dev/v1alpha1",
  "kind":"AppModule",
  "metadata":{"name":"crm","version":"1.0.0"},
  "spec":{"resources":[{
    "name":"leads",
    "fields":[{"name":"email","type":"email","required":true}],
    "api":{
      "public":{"operations":["create"],"writable":["email"]},
      "admin":{"operations":["list","get","create","patch","delete"],"writable":["email"]}
    }
  }]}
}`), spec.FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	handler := newTestHandlerWithModule(t, module, &fakeRecordService{})

	uiRequest := httptest.NewRequest(http.MethodGet, "/api/core/v1alpha1/modules/crm/ui-schema.json", nil)
	uiResponse := httptest.NewRecorder()
	handler.ServeHTTP(uiResponse, uiRequest)
	var uiSchema map[string]any
	if err := json.NewDecoder(uiResponse.Body).Decode(&uiSchema); err != nil {
		t.Fatal(err)
	}
	if uiResponse.Code != http.StatusOK || uiSchema["schema"] != "manager.panvara.dev/v1alpha1" ||
		uiSchema["revision"] != module.RevisionHash() {
		t.Fatalf("compiled UI schema = %#v", uiSchema)
	}

	openAPIRequest := httptest.NewRequest(http.MethodGet, "/api/core/v1alpha1/modules/crm/openapi.json", nil)
	openAPIResponse := httptest.NewRecorder()
	handler.ServeHTTP(openAPIResponse, openAPIRequest)
	var openAPI map[string]any
	if err := json.NewDecoder(openAPIResponse.Body).Decode(&openAPI); err != nil {
		t.Fatal(err)
	}
	paths := openAPI["paths"].(map[string]any)
	if _, exists := paths["/api/public/v1alpha1/crm/leads"]; !exists {
		t.Fatalf("compiled OpenAPI paths = %#v", paths)
	}
}

func TestPublicCreateUsesAnonymousProjectIdentity(t *testing.T) {
	t.Parallel()
	service := &fakeRecordService{}
	service.create = func(ctx context.Context, scope record.Scope, surface record.Surface, body json.RawMessage) (record.Record, error) {
		identity, ok := IdentityFromContext(ctx)
		if !ok || !identity.Actor.Anonymous() || identity.Project.ID().String() != testProjectID {
			t.Fatalf("request identity = %+v, ok = %v", identity, ok)
		}
		if surface != record.SurfacePublic || scope.ModuleName != "crm" || scope.ResourceName != "leads" {
			t.Fatalf("scope/surface = %+v/%q", scope, surface)
		}
		if string(body) != `{"name":"Ada"}` {
			t.Fatalf("body = %s", body)
		}
		return makeTestRecord(t, scope, 1), nil
	}
	handler := newTestHandler(t, service)
	request := httptest.NewRequest(
		http.MethodPost, "/api/public/v1alpha1/crm/leads", strings.NewReader(`{"name":"Ada"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("ETag"); got != `"1"` {
		t.Fatalf("ETag = %q", got)
	}
	if got := response.Header().Get("Location"); !strings.HasSuffix(got, "/"+testRecordID) {
		t.Fatalf("Location = %q", got)
	}
	var result recordResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.ID != testRecordID || result.Version != 1 || string(result.Data) != `{"name":"Ada"}` {
		t.Fatalf("record = %+v", result)
	}
}

func TestRecordRequestsUseExplicitNamespaceInsteadOfRuntimeRevision(t *testing.T) {
	t.Parallel()
	const namespace = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	service := &fakeRecordService{create: func(
		_ context.Context,
		scope record.Scope,
		_ record.Surface,
		_ json.RawMessage,
	) (record.Record, error) {
		if scope.RevisionHash != namespace || scope.RevisionHash == testRevision {
			t.Fatalf("Record scope revision = %q, want namespace %q", scope.RevisionHash, namespace)
		}
		return makeTestRecord(t, scope, 1), nil
	}}
	handler := newTestHandlerWithModuleAndNamespace(t, fakeModule{}, namespace, service)
	request := httptest.NewRequest(
		http.MethodPost, "/api/public/v1alpha1/crm/leads", strings.NewReader(`{"name":"Ada"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
}

func TestAdminCRUDContract(t *testing.T) {
	service := &fakeRecordService{}
	handler := newTestHandler(t, service)

	t.Run("authentication required", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/admin/v1alpha1/crm/leads", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		assertErrorEnvelope(t, response, "unauthorized")
	})

	t.Run("list", func(t *testing.T) {
		service.list = func(
			ctx context.Context, scope record.Scope, surface record.Surface, options record.ListOptions,
		) (record.ListResult, error) {
			assertAdminIdentity(t, ctx)
			if surface != record.SurfaceAdmin || options.Limit != record.DefaultListLimit ||
				len(options.Filters) != 1 || options.Filters[0] != (record.ListFilter{Field: "name", Value: "Ada"}) {
				t.Fatalf("surface/options = %q/%+v", surface, options)
			}
			item := makeTestRecord(t, scope, 3)
			return record.ListResult{
				Records: []record.Record{item},
				Next:    &record.ListCursor{CreatedAt: item.CreatedAt, ID: item.ID, QueryHash: testRevision},
			}, nil
		}
		response := performAdminRequest(
			handler, http.MethodGet, "/api/admin/v1alpha1/crm/leads?filter[name]=Ada", "", "",
		)
		if response.Code != http.StatusOK {
			t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
		}
		var result listResponse
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if len(result.Data) != 1 || result.NextCursor == "" {
			t.Fatalf("list = %+v", result)
		}
		if _, err := decodeListCursor(result.NextCursor); err != nil {
			t.Fatalf("next cursor: %v", err)
		}
	})

	t.Run("get", func(t *testing.T) {
		service.get = func(ctx context.Context, scope record.Scope, id record.ID) (record.Record, error) {
			assertAdminIdentity(t, ctx)
			return makeTestRecord(t, scope, 3), nil
		}
		response := performAdminRequest(
			handler, http.MethodGet, "/api/admin/v1alpha1/crm/leads/"+testRecordID, "", "",
		)
		if response.Code != http.StatusOK || response.Header().Get("ETag") != `"3"` {
			t.Fatalf("status/etag = %d %q", response.Code, response.Header().Get("ETag"))
		}
	})

	t.Run("patch requires If-Match", func(t *testing.T) {
		response := performAdminRequest(
			handler, http.MethodPatch, "/api/admin/v1alpha1/crm/leads/"+testRecordID, `{"name":"Grace"}`, "",
		)
		if response.Code != http.StatusPreconditionRequired {
			t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
		}
		assertErrorEnvelope(t, response, "if_match_required")
	})

	t.Run("patch", func(t *testing.T) {
		service.update = func(
			ctx context.Context,
			scope record.Scope,
			id record.ID,
			expected uint64,
			surface record.Surface,
			patch json.RawMessage,
		) (record.Record, error) {
			assertAdminIdentity(t, ctx)
			if expected != 3 || surface != record.SurfaceAdmin || string(patch) != `{"name":"Grace"}` {
				t.Fatalf("update expected/surface/patch = %d/%q/%s", expected, surface, patch)
			}
			result := makeTestRecord(t, scope, 4)
			result.Data = json.RawMessage(`{"name":"Grace"}`)
			return result, nil
		}
		response := performAdminRequest(
			handler, http.MethodPatch, "/api/admin/v1alpha1/crm/leads/"+testRecordID,
			`{"name":"Grace"}`, `"3"`,
		)
		if response.Code != http.StatusOK || response.Header().Get("ETag") != `"4"` {
			t.Fatalf("status/etag/body = %d %q %q", response.Code, response.Header().Get("ETag"), response.Body.String())
		}
	})

	t.Run("delete conflict maps to precondition failed", func(t *testing.T) {
		service.delete = func(context.Context, record.Scope, record.ID, uint64) (record.Record, error) {
			return record.Record{}, record.ErrVersionConflict
		}
		response := performAdminRequest(
			handler, http.MethodDelete, "/api/admin/v1alpha1/crm/leads/"+testRecordID, "", `"3"`,
		)
		if response.Code != http.StatusPreconditionFailed {
			t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
		}
		assertErrorEnvelope(t, response, "precondition_failed")
	})

	t.Run("delete returns no content", func(t *testing.T) {
		service.delete = func(ctx context.Context, scope record.Scope, id record.ID, expected uint64) (record.Record, error) {
			assertAdminIdentity(t, ctx)
			if expected != 4 {
				t.Fatalf("expected version = %d", expected)
			}
			deleted := makeTestRecord(t, scope, 5)
			at := deleted.UpdatedAt.Add(time.Second)
			deleted.DeletedAt = &at
			return deleted, nil
		}
		response := performAdminRequest(
			handler, http.MethodDelete, "/api/admin/v1alpha1/crm/leads/"+testRecordID, "", `"4"`,
		)
		if response.Code != http.StatusNoContent || response.Body.Len() != 0 || response.Header().Get("ETag") != `"5"` {
			t.Fatalf("status/etag/body = %d %q %q", response.Code, response.Header().Get("ETag"), response.Body.String())
		}
	})
}

func TestListQueryRejectsAmbiguousFilters(t *testing.T) {
	t.Parallel()

	for _, rawQuery := range []string{
		"filter[name]=Ada&filter[name]=Grace",
		"filter[name]=",
		"filter[Bad-Name]=Ada",
		"sort=name",
		"limit=0",
		"cursor=",
	} {
		request := httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
		if _, problem := parseListOptions(request); problem == nil {
			t.Fatalf("parseListOptions(%q) accepted an ambiguous query", rawQuery)
		}
	}
}

func TestTransportInputErrorsUseClientStatuses(t *testing.T) {
	t.Parallel()
	service := &fakeRecordService{
		list: func(context.Context, record.Scope, record.Surface, record.ListOptions) (record.ListResult, error) {
			return record.ListResult{}, record.ErrInvalidArgument
		},
	}
	handler := newTestHandler(t, service)
	for _, test := range []struct {
		name       string
		method     string
		path       string
		body       string
		ifMatch    string
		wantStatus int
		wantCode   string
	}{
		{
			name: "invalid filter syntax", method: http.MethodGet,
			path:       "/api/admin/v1alpha1/crm/leads?filter[Bad-Name]=Ada",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_filter",
		},
		{
			name: "filter rejected by model", method: http.MethodGet,
			path:       "/api/admin/v1alpha1/crm/leads?filter[unknown]=Ada",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_argument",
		},
		{
			name: "invalid cursor", method: http.MethodGet,
			path:       "/api/admin/v1alpha1/crm/leads?cursor=not-base64!",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_cursor",
		},
		{
			name: "weak If-Match", method: http.MethodPatch,
			path: "/api/admin/v1alpha1/crm/leads/" + testRecordID,
			body: `{"name":"Grace"}`, ifMatch: `W/"1"`,
			wantStatus: http.StatusBadRequest, wantCode: "invalid_if_match",
		},
		{
			name: "invalid UUID", method: http.MethodGet,
			path:       "/api/admin/v1alpha1/crm/leads/not-a-uuid",
			wantStatus: http.StatusBadRequest, wantCode: "invalid_argument",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := performAdminRequest(handler, test.method, test.path, test.body, test.ifMatch)
			if response.Code != test.wantStatus {
				t.Fatalf("status/body = %d %q, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
			assertErrorEnvelope(t, response, test.wantCode)
		})
	}
}

func TestValidationErrorUsesStableEnvelope(t *testing.T) {
	t.Parallel()
	service := &fakeRecordService{
		create: func(context.Context, record.Scope, record.Surface, json.RawMessage) (record.Record, error) {
			return record.Record{}, &appmodule.ValidationError{Violations: []appmodule.Violation{
				{Code: "required", Path: "$.name", Message: "required field is missing"},
			}}
		},
	}
	handler := newTestHandler(t, service)
	request := httptest.NewRequest(http.MethodPost, "/api/public/v1alpha1/crm/leads", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
	assertErrorEnvelope(t, response, "validation_failed")
}

func TestCreateInvalidJSONUsesClientErrorEnvelope(t *testing.T) {
	t.Parallel()
	module, err := appmodule.NewCompiler().Compile([]byte(`{
  "apiVersion":"panvara.dev/v1alpha1",
  "kind":"AppModule",
  "metadata":{"name":"crm","version":"1.0.0"},
  "spec":{"resources":[{
    "name":"leads",
    "fields":[{"name":"email","type":"email","required":true}],
    "api":{"public":{"operations":["create"],"writable":["email"]}}
  }]}
}`), spec.FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	validator, err := record.NewCompiledModuleValidator(module)
	if err != nil {
		t.Fatal(err)
	}
	service := &fakeRecordService{create: func(
		ctx context.Context,
		scope record.Scope,
		surface record.Surface,
		body json.RawMessage,
	) (record.Record, error) {
		_, err := validator.Validate(ctx, record.ValidationInput{
			Scope: scope, Surface: surface, Mutation: record.MutationCreate, Data: body,
		})
		return record.Record{}, err
	}}
	handler := newTestHandlerWithModule(t, module, service)
	deep := strings.Repeat(`{"nested":`, 18) + `true` + strings.Repeat(`}`, 18)
	for name, body := range map[string]string{
		"malformed":      `{"email":`,
		"duplicate key":  `{"email":"ada@example.com","email":"grace@example.com"}`,
		"trailing value": `{"email":"ada@example.com"}{}`,
		"too deep":       deep,
	} {
		name, body := name, body
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/public/v1alpha1/crm/leads",
				strings.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status/body = %d %q, want 400", response.Code, response.Body.String())
			}
			assertErrorEnvelope(t, response, "invalid_argument")
		})
	}
}

func newTestHandler(t *testing.T, records RecordService) *Handler {
	return newTestHandlerWithModule(t, fakeModule{}, records)
}

func newTestHandlerWithModule(t *testing.T, module Module, records RecordService) *Handler {
	return newTestHandlerWithModuleAndNamespace(t, module, "", records)
}

func newTestHandlerWithModuleAndNamespace(
	t *testing.T,
	module Module,
	namespace string,
	records RecordService,
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
	auth := newTestAdminAuth(t, scope)
	handler, err := New(Config{
		Project: projectContext, Scope: scope,
		PublicActor: publicActor, Module: module,
		RecordNamespaceRevision: namespace, Records: records, AdminAuth: auth,
		AccessAdministration: &fakeAccessAdministration{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

type fakeModule struct{}

func (fakeModule) Name() string         { return "crm" }
func (fakeModule) RevisionHash() string { return testRevision }
func (fakeModule) ManagerUISchema() []byte {
	return []byte(`{"schema":"manager.panvara.dev/v1alpha1"}`)
}
func (fakeModule) OpenAPI() []byte { return []byte(`{"openapi":"3.1.0"}`) }
func (fakeModule) Descriptor() domainmodule.Descriptor {
	return domainmodule.Descriptor{
		Name: "crm", Version: "1.0.0",
		Resources: []domainmodule.Resource{{
			Name: "leads",
			API: domainmodule.API{
				Public: domainmodule.Access{
					Operations: []domainmodule.Operation{domainmodule.OperationCreate}, Writable: []string{"name"},
				},
				Admin: domainmodule.Access{
					Operations: []domainmodule.Operation{
						domainmodule.OperationList, domainmodule.OperationGet, domainmodule.OperationCreate,
						domainmodule.OperationPatch, domainmodule.OperationDelete,
					},
					Writable: []string{"name"},
				},
			},
		}},
	}
}

type fakeRecordService struct {
	create func(context.Context, record.Scope, record.Surface, json.RawMessage) (record.Record, error)
	get    func(context.Context, record.Scope, record.ID) (record.Record, error)
	list   func(context.Context, record.Scope, record.Surface, record.ListOptions) (record.ListResult, error)
	update func(context.Context, record.Scope, record.ID, uint64, record.Surface, json.RawMessage) (record.Record, error)
	delete func(context.Context, record.Scope, record.ID, uint64) (record.Record, error)
}

func (service *fakeRecordService) Create(
	ctx context.Context, execution access.Execution, scope record.Scope, body json.RawMessage,
) (record.Record, error) {
	if service.create == nil {
		return record.Record{}, errors.New("unexpected Create")
	}
	return service.create(ctx, scope, testRecordSurface(execution), body)
}
func (service *fakeRecordService) Get(
	ctx context.Context, _ access.Execution, scope record.Scope, id record.ID,
) (record.Record, error) {
	if service.get == nil {
		return record.Record{}, errors.New("unexpected Get")
	}
	return service.get(ctx, scope, id)
}
func (service *fakeRecordService) List(
	ctx context.Context, execution access.Execution, scope record.Scope, options record.ListOptions,
) (record.ListResult, error) {
	if service.list == nil {
		return record.ListResult{}, errors.New("unexpected List")
	}
	return service.list(ctx, scope, testRecordSurface(execution), options)
}
func (service *fakeRecordService) Update(
	ctx context.Context,
	execution access.Execution,
	scope record.Scope,
	id record.ID,
	expected uint64,
	patch json.RawMessage,
) (record.Record, error) {
	if service.update == nil {
		return record.Record{}, errors.New("unexpected Update")
	}
	return service.update(ctx, scope, id, expected, testRecordSurface(execution), patch)
}
func (service *fakeRecordService) Delete(
	ctx context.Context, _ access.Execution, scope record.Scope, id record.ID, expected uint64,
) (record.Record, error) {
	if service.delete == nil {
		return record.Record{}, errors.New("unexpected Delete")
	}
	return service.delete(ctx, scope, id, expected)
}

func testProjectScope(t *testing.T) project.Scope {
	t.Helper()
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	environmentID, err := project.ParseEnvironmentID(testEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func testRecordSurface(execution access.Execution) record.Surface {
	if execution.Surface() == access.SurfacePublic {
		return record.SurfacePublic
	}
	return record.SurfaceAdmin
}

func makeTestRecord(t *testing.T, scope record.Scope, version uint64) record.Record {
	t.Helper()
	id, err := record.ParseID(testRecordID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	return record.Record{
		Scope: scope, ID: id, Version: version, Data: json.RawMessage(`{"name":"Ada"}`),
		CreatedAt: now, UpdatedAt: now,
	}
}

func performAdminRequest(handler http.Handler, method, path, body, ifMatch string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertAdminIdentity(t *testing.T, ctx context.Context) {
	t.Helper()
	identity, ok := IdentityFromContext(ctx)
	if !ok || identity.Actor.ActorID() != "bootstrap-admin" || len(identity.Actor.Roles()) != 0 ||
		identity.Project.ID().String() != testProjectID ||
		identity.Scope.EnvironmentID().String() != testEnvironmentID ||
		identity.Surface != access.SurfaceAdmin {
		t.Fatalf("admin identity = %+v, ok = %v", identity, ok)
	}
}

func assertErrorEnvelope(t *testing.T, response *httptest.ResponseRecorder, code string) {
	t.Helper()
	var envelope ErrorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != code || envelope.Message == "" || envelope.RequestID == "" {
		t.Fatalf("error envelope = %+v", envelope)
	}
}
