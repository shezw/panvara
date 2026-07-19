/*
   Panvara
   internal/interfaces/httpapi/revision_handler_test.go    2026-07-15
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_/\/_/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestRevisionRegistryEndpointsRequireAuthenticationAndReturnStableRepresentations(t *testing.T) {
	t.Parallel()
	revision, source := testHTTPRevision(t)
	service := &fakeRevisionRegistryService{revision: revision, source: application.RevisionSource{
		Format: domain.SourceFormatYAML, Hash: revision.SourceHash(), Bytes: source,
	}}
	handler := newTestHandlerWithRevisionService(t, service)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet,
		"/api/admin/core/v1alpha1/modules/crm/revisions", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status/body = %d %q", unauthorized.Code, unauthorized.Body.String())
	}

	list := performAdminRequest(handler, http.MethodGet,
		"/api/admin/core/v1alpha1/modules/crm/revisions?limit=100", "", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status/body = %d %q", list.Code, list.Body.String())
	}
	var listed revisionListResponse
	if err := json.NewDecoder(list.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Revision != revision.RevisionHash() || service.limit != 100 {
		t.Fatalf("list response = %#v, limit = %d", listed, service.limit)
	}
	if list.Header().Get("Cache-Control") != "private, no-store" ||
		len(listed.Data[0].DataSchemaIdentities) != 2 ||
		listed.Data[0].DataSchemaIdentities[0].Format != 1 ||
		listed.Data[0].DataSchemaIdentities[1].Format != 2 {
		t.Fatalf("list cache/identities = %q %#v", list.Header().Get("Cache-Control"), listed.Data[0].DataSchemaIdentities)
	}

	detailPath := "/api/admin/core/v1alpha1/modules/crm/revisions/" + revision.RevisionHash()
	detail := performAdminRequest(handler, http.MethodGet, detailPath, "", "")
	if detail.Code != http.StatusOK || detail.Header().Get("Cache-Control") != "private, no-store" ||
		!strings.Contains(detail.Body.String(), `"registered_by":"system:bootstrap"`) {
		t.Fatalf("detail status/body = %d %q", detail.Code, detail.Body.String())
	}
	method := performAdminRequest(handler, http.MethodDelete, detailPath, "", "")
	if method.Code != http.StatusMethodNotAllowed || method.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("DELETE detail status/allow = %d %q", method.Code, method.Header().Get("Allow"))
	}

	sourceResponse := performAdminRequest(handler, http.MethodGet, detailPath+"/source", "", "")
	if sourceResponse.Code != http.StatusOK || sourceResponse.Body.String() != string(source) ||
		sourceResponse.Header().Get("Content-Type") != "application/yaml; charset=utf-8" ||
		sourceResponse.Header().Get("Cache-Control") != "private, no-cache" ||
		sourceResponse.Header().Get("ETag") != `"`+revision.SourceHash()+`"` {
		t.Fatalf("source status/headers/body = %d %#v %q", sourceResponse.Code, sourceResponse.Header(), sourceResponse.Body.String())
	}

	notModifiedRequest := httptest.NewRequest(http.MethodGet, detailPath+"/source", nil)
	notModifiedRequest.Header.Set("Authorization", "Bearer "+testAdminToken)
	notModifiedRequest.Header.Set("If-None-Match", `"`+revision.SourceHash()+`"`)
	notModified := httptest.NewRecorder()
	handler.ServeHTTP(notModified, notModifiedRequest)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 ||
		notModified.Header().Get("ETag") != `"`+revision.SourceHash()+`"` {
		t.Fatalf("conditional source status/etag/body = %d %q %q", notModified.Code, notModified.Header().Get("ETag"), notModified.Body.String())
	}
}

func TestRevisionRegistrySourceUsesJSONContentType(t *testing.T) {
	t.Parallel()
	revision, source := testHTTPRevision(t)
	service := &fakeRevisionRegistryService{revision: revision, source: application.RevisionSource{
		Format: domain.SourceFormatJSON, Hash: revision.SourceHash(), Bytes: source,
	}}
	handler := newTestHandlerWithRevisionService(t, service)
	path := "/api/admin/core/v1alpha1/modules/crm/revisions/" + revision.RevisionHash() + "/source"
	response := performAdminRequest(handler, http.MethodGet, path, "", "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("JSON source status/content-type = %d %q", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestRevisionRegistrySourceRejectsUnknownFormatBeforeWritingValidators(t *testing.T) {
	t.Parallel()
	revision, source := testHTTPRevision(t)
	service := &fakeRevisionRegistryService{revision: revision, source: application.RevisionSource{
		Format: domain.SourceFormat("toml"), Hash: revision.SourceHash(), Bytes: source,
	}}
	handler := newTestHandlerWithRevisionService(t, service)
	path := "/api/admin/core/v1alpha1/modules/crm/revisions/" + revision.RevisionHash() + "/source"
	response := performAdminRequest(handler, http.MethodGet, path, "", "")
	if response.Code != http.StatusInternalServerError || response.Header().Get("ETag") != "" {
		t.Fatalf("unknown format status/etag = %d %q", response.Code, response.Header().Get("ETag"))
	}
}

func TestRevisionRegistryEndpointMapsNotFoundAndInvalidLimit(t *testing.T) {
	t.Parallel()
	service := &fakeRevisionRegistryService{err: application.ErrRevisionNotFound}
	handler := newTestHandlerWithRevisionService(t, service)
	missing := performAdminRequest(handler, http.MethodGet,
		"/api/admin/core/v1alpha1/modules/crm/revisions/sha256:"+strings.Repeat("a", 64), "", "")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing status/body = %d %q", missing.Code, missing.Body.String())
	}
	invalid := performAdminRequest(handler, http.MethodGet,
		"/api/admin/core/v1alpha1/modules/crm/revisions?limit=101", "", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit status/body = %d %q", invalid.Code, invalid.Body.String())
	}
	service.err = application.ErrRevisionInvalid
	invalidIdentity := performAdminRequest(handler, http.MethodGet,
		"/api/admin/core/v1alpha1/modules/Bad_Module/revisions/not-a-hash", "", "")
	if invalidIdentity.Code != http.StatusBadRequest {
		t.Fatalf("invalid identity status/body = %d %q", invalidIdentity.Code, invalidIdentity.Body.String())
	}
}

type fakeRevisionRegistryService struct {
	revision domain.Revision
	source   application.RevisionSource
	err      error
	limit    int
}

func (service *fakeRevisionRegistryService) List(
	_ context.Context, execution access.Execution, _ string, limit int,
) ([]domain.RevisionSummary, error) {
	service.limit = limit
	if !execution.Scope().ProjectID().Valid() || execution.Actor().ActorID() != "bootstrap-admin" {
		return nil, errors.New("missing owner identity")
	}
	if service.err != nil {
		return nil, service.err
	}
	return []domain.RevisionSummary{service.revision.Summary()}, nil
}

func (service *fakeRevisionRegistryService) Get(
	_ context.Context, _ access.Execution, _, _ string,
) (domain.Revision, error) {
	if service.err != nil {
		return domain.Revision{}, service.err
	}
	return service.revision, nil
}

func (service *fakeRevisionRegistryService) GetSource(
	_ context.Context, _ access.Execution, _, _ string,
) (application.RevisionSource, error) {
	if service.err != nil {
		return application.RevisionSource{}, service.err
	}
	return service.source, nil
}

func newTestHandlerWithRevisionService(t *testing.T, revisions RevisionRegistryService) *Handler {
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
		PublicActor: publicActor, Module: fakeModule{}, Records: &fakeRecordService{},
		Revisions: revisions, AdminAuth: auth, AccessAdministration: &fakeAccessAdministration{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func testHTTPRevision(t *testing.T) (domain.Revision, []byte) {
	t.Helper()
	source := []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: crm\n  version: 1.0.0\nspec:\n  resources: []\n")
	compiled, err := application.NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := domain.NewRevision(domain.RevisionMaterial{
		ProjectID: projectID, ModuleName: compiled.Name(), ModuleVersion: compiled.Version(),
		RevisionHash: compiled.RevisionHash(), DataSchemaIdentities: httpDataSchemaIdentities(t, compiled),
		SpecVersion: spec.APIVersion,
		IRFormat:    application.IRFormatVersion, SourceFormat: domain.SourceFormatYAML,
		SourceHash: httpHashBytes(source), Source: source, CanonicalIR: compiled.CanonicalIR(),
		OpenAPI: compiled.OpenAPI(), ManagerSchema: compiled.ManagerUISchema(), Origin: domain.RevisionOriginBootstrap,
		RegisteredBy: "system:bootstrap", RegisteredAt: time.Date(2026, 7, 15, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision, source
}

func httpDataSchemaIdentities(t *testing.T, compiled *application.CompiledModule) []domain.DataSchemaIdentity {
	t.Helper()
	current, err := domain.NewDataSchemaIdentity(compiled.DataSchemaFormat(), compiled.DataSchemaFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	future, err := domain.NewDataSchemaIdentity(2, "sha256:"+strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	return []domain.DataSchemaIdentity{future, current}
}

func httpHashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

var _ RevisionRegistryService = (*fakeRevisionRegistryService)(nil)
var _ RecordService = (*fakeRecordService)(nil)
var _ = record.SurfaceAdmin
