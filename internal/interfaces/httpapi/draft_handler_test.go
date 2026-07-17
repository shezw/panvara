/*
   Panvara
   internal/interfaces/httpapi/draft_handler_test.go    2026-07-16
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
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	testDraftID      = "018f7e93-7b2c-7abc-8def-1234567890ad"
	testValidationID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	testPlanID       = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	testPlanHash     = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testCandidate    = "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	testDataSchema   = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

func TestDraftCreateAndSourceUseRawOwnerContract(t *testing.T) {
	t.Parallel()
	source := []byte("spec: [")
	draft := makeHTTPDraft(t, 1, source, testRevision)
	service := &fakeDraftWorkflowService{
		create: func(_ context.Context, projectID project.ID, owner actor.Context, module string, input application.CreateDraftInput) (domain.Draft, bool, error) {
			if projectID.String() != testProjectID || owner.ActorID() != "bootstrap-admin" || module != "crm" {
				t.Fatalf("scope = %s/%s/%s", projectID.String(), owner.ActorID(), module)
			}
			if input.BaselineRevision != testRevision || input.Format != domain.SourceFormatYAML ||
				string(input.Source) != string(source) || input.IdempotencyKey != "draft-acceptance-1" {
				t.Fatalf("create input = %+v source=%q", input, input.Source)
			}
			return draft, true, nil
		},
		getSource: func(context.Context, project.ID, actor.Context, string, string) (application.DraftSource, error) {
			return application.DraftSource{
				Format: draft.SourceFormat(), Hash: draft.SourceHash(), Bytes: draft.Source(), Generation: draft.Generation(),
			}, nil
		},
	}
	handler := newTestHandlerWithDrafts(t, service)

	unauthorized := httptest.NewRequest(
		http.MethodPost, "/api/admin/core/v1alpha1/modules/crm/drafts?baseline_revision="+testRevision,
		strings.NewReader(string(source)),
	)
	unauthorized.Header.Set("Content-Type", "application/yaml")
	unauthorized.Header.Set("Idempotency-Key", "draft-acceptance-1")
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status/body = %d %q", unauthorizedResponse.Code, unauthorizedResponse.Body.String())
	}

	request := httptest.NewRequest(
		http.MethodPost, "/api/admin/core/v1alpha1/modules/crm/drafts?baseline_revision="+testRevision,
		strings.NewReader(string(source)),
	)
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	request.Header.Set("Content-Type", "application/yaml; charset=utf-8")
	request.Header.Set("Idempotency-Key", "draft-acceptance-1")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("ETag") != `"1"` ||
		response.Header().Get("Location") != draftLocation("crm", testDraftID) ||
		response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create status/headers = %d %#v body=%q", response.Code, response.Header(), response.Body.String())
	}
	var metadata draftResponse
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.DraftID != testDraftID || metadata.BaselineRevision == nil || *metadata.BaselineRevision != testRevision ||
		metadata.DraftVersion != 1 || metadata.SourceHash != draft.SourceHash() {
		t.Fatalf("draft response = %+v", metadata)
	}

	sourceRequest := httptest.NewRequest(
		http.MethodGet, draftLocation("crm", testDraftID)+"/source", nil,
	)
	sourceRequest.Header.Set("Authorization", "Bearer "+testAdminToken)
	sourceResponse := httptest.NewRecorder()
	handler.ServeHTTP(sourceResponse, sourceRequest)
	if sourceResponse.Code != http.StatusOK || sourceResponse.Body.String() != string(source) ||
		sourceResponse.Header().Get("Content-Type") != "application/yaml; charset=utf-8" ||
		sourceResponse.Header().Get("ETag") != `"1"` ||
		sourceResponse.Header().Get("X-Panvara-Source-Hash") != draft.SourceHash() ||
		sourceResponse.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("source status/headers/body = %d %#v %q", sourceResponse.Code, sourceResponse.Header(), sourceResponse.Body.String())
	}
}

func TestDraftValidationPersistsInvalidResultAndReplays(t *testing.T) {
	t.Parallel()
	draft := makeHTTPDraft(t, 1, []byte("spec: ["), testRevision)
	validation := makeHTTPValidation(t, draft, false)
	calls := 0
	service := &fakeDraftWorkflowService{validate: func(
		_ context.Context, _ project.ID, _ actor.Context, module, draftID string, expected uint64,
	) (application.DraftValidation, bool, error) {
		if module != "crm" || draftID != testDraftID || expected != 1 {
			t.Fatalf("validate input = %s/%s/%d", module, draftID, expected)
		}
		calls++
		return validation, calls == 1, nil
	}}
	handler := newTestHandlerWithDrafts(t, service)
	path := draftLocation("crm", testDraftID) + "/validations"

	missing := performDraftAdminRequest(handler, http.MethodPost, path, "", "", "")
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match status/body = %d %q", missing.Code, missing.Body.String())
	}
	for index, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		response := performDraftAdminRequest(handler, http.MethodPost, path, "", "", `"1"`)
		if response.Code != wantStatus || response.Header().Get("ETag") != `"1"` {
			t.Fatalf("call %d status/headers/body = %d %#v %q", index, response.Code, response.Header(), response.Body.String())
		}
		var body validationResponse
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Valid || body.CandidateRevision != nil || body.ValidationID != testValidationID ||
			body.ValidationHash != testValidationID || len(body.Violations) != 1 ||
			body.Effects.RevisionRegistered || body.Effects.Published || body.Effects.Activated ||
			body.Effects.RecordsMigrated || body.Effects.RuntimeChanged {
			t.Fatalf("validation response = %+v", body)
		}
	}
}

func TestDraftReplaceAndPlanUseCurrentETagWithoutExecutionEffects(t *testing.T) {
	t.Parallel()
	source := []byte("apiVersion: panvara.dev/v1alpha1")
	draft := makeHTTPDraft(t, 2, source, testRevision)
	plan := makeHTTPPlan(t, draft)
	planCalls := 0
	service := &fakeDraftWorkflowService{
		replace: func(_ context.Context, _ project.ID, _ actor.Context, module, draftID string, expected uint64, input application.ReplaceDraftInput) (domain.Draft, bool, error) {
			if module != "crm" || draftID != testDraftID || expected != 1 || input.Format != domain.SourceFormatYAML || string(input.Source) != string(source) {
				t.Fatalf("replace input = %s/%s/%d/%+v", module, draftID, expected, input)
			}
			return draft, true, nil
		},
		plan: func(_ context.Context, _ project.ID, _ actor.Context, module, draftID, validationID string, expected uint64) (application.DraftPlan, bool, error) {
			if module != "crm" || draftID != testDraftID || validationID != testValidationID || expected != 2 {
				t.Fatalf("plan input = %s/%s/%s/%d", module, draftID, validationID, expected)
			}
			planCalls++
			return plan, planCalls == 1, nil
		},
	}
	handler := newTestHandlerWithDrafts(t, service)
	putPath := draftLocation("crm", testDraftID) + "/source"
	missing := performDraftAdminRequest(handler, http.MethodPut, putPath, string(source), "application/yaml", "")
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing PUT If-Match status/body = %d %q", missing.Code, missing.Body.String())
	}
	put := performDraftAdminRequest(handler, http.MethodPut, putPath, string(source), "application/yaml", `"1"`)
	if put.Code != http.StatusOK || put.Header().Get("ETag") != `"2"` {
		t.Fatalf("PUT status/headers/body = %d %#v %q", put.Code, put.Header(), put.Body.String())
	}

	planPath := draftLocation("crm", testDraftID) + "/plans"
	for index, wantStatus := range []int{http.StatusCreated, http.StatusOK} {
		response := performDraftAdminRequest(
			handler, http.MethodPost, planPath, `{"validation_id":"`+testValidationID+`"}`, "application/json", `"2"`,
		)
		if response.Code != wantStatus || response.Header().Get("ETag") != `"2"` {
			t.Fatalf("plan call %d status/headers/body = %d %#v %q", index, response.Code, response.Header(), response.Body.String())
		}
		var body planResponse
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.PlanID != testPlanID || body.PlanHash != testPlanHash || body.Summary.Classification != "compatible" ||
			len(body.Changes) != 1 || !body.Effects.PlanRecorded || body.Effects.RevisionRegistered ||
			body.Effects.Published || body.Effects.Activated || body.Effects.RecordsMigrated || body.Effects.RuntimeChanged ||
			body.MigrationExecutionSupported {
			t.Fatalf("plan response = %+v", body)
		}
	}
}

func TestDraftApplicationErrorsMapWithoutLeakingSource(t *testing.T) {
	t.Parallel()
	service := &fakeDraftWorkflowService{replace: func(
		context.Context, project.ID, actor.Context, string, string, uint64, application.ReplaceDraftInput,
	) (domain.Draft, bool, error) {
		return domain.Draft{}, false, application.ErrDraftConflict
	}}
	handler := newTestHandlerWithDrafts(t, service)
	response := performDraftAdminRequest(
		handler, http.MethodPut, draftLocation("crm", testDraftID)+"/source", "secret-source", "application/yaml", `"1"`,
	)
	if response.Code != http.StatusPreconditionFailed || strings.Contains(response.Body.String(), "secret-source") {
		t.Fatalf("status/body = %d %q", response.Code, response.Body.String())
	}
	assertErrorEnvelope(t, response, "precondition_failed")
}

func TestDraftIfMatchRejectsNonCanonicalAndPostgresOverflowValues(t *testing.T) {
	t.Parallel()
	service := &fakeDraftWorkflowService{replace: func(
		context.Context, project.ID, actor.Context, string, string, uint64, application.ReplaceDraftInput,
	) (domain.Draft, bool, error) {
		t.Fatal("Replace must not run for an invalid Draft ETag")
		return domain.Draft{}, false, nil
	}}
	handler := newTestHandlerWithDrafts(t, service)
	for _, etag := range []string{`"01"`, `"9223372036854775808"`} {
		response := performDraftAdminRequest(
			handler, http.MethodPut, draftLocation("crm", testDraftID)+"/source",
			"source", "application/yaml", etag,
		)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("If-Match %s status/body = %d %q", etag, response.Code, response.Body.String())
		}
		assertErrorEnvelope(t, response, "invalid_if_match")
	}
}

func newTestHandlerWithDrafts(t *testing.T, drafts DraftWorkflowService) *Handler {
	t.Helper()
	projectContext, err := project.NewContext(testProjectID, "crm", "en-US", "UTC", "USD")
	if err != nil {
		t.Fatal(err)
	}
	publicActor, err := actor.NewAnonymous(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	adminActor, err := actor.New(testProjectID, "bootstrap-admin", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err := NewBootstrapAdminAuth(testAdminToken, adminActor)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(Config{
		Project: projectContext, Scope: testProjectScope(t),
		PublicActor: publicActor, Module: fakeModule{}, Records: &fakeRecordService{},
		Drafts: drafts, AdminAuth: auth,
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func makeHTTPDraft(t *testing.T, generation uint64, source []byte, baselineRevision string) domain.Draft {
	t.Helper()
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	draftID, err := domain.ParseDraftID(testDraftID)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := domain.NewDraftBaseline(baselineRevision)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	value, err := domain.NewDraft(domain.DraftMaterial{
		ID: draftID, ProjectID: projectID, ModuleName: "crm", Baseline: baseline,
		SourceFormat: domain.SourceFormatYAML, SourceHash: domain.DraftSourceHash(source), Source: source,
		Generation: generation, CreatedBy: "bootstrap-admin", CreatedAt: now,
		UpdatedBy: "bootstrap-admin", UpdatedAt: now.Add(time.Duration(generation-1) * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func makeHTTPValidation(t *testing.T, draft domain.Draft, valid bool) application.DraftValidation {
	t.Helper()
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	result := application.DraftValidation{
		ID: testValidationID, ValidationHash: testValidationID, FormatVersion: application.DraftValidationFormatVersion,
		ProjectID: projectID, ModuleName: "crm", DraftID: draft.ID(), Generation: draft.Generation(),
		BaselineRevision: testRevision, SourceFormat: draft.SourceFormat(), SourceHash: draft.SourceHash(),
		Valid: valid, CreatedBy: "bootstrap-admin", CreatedAt: time.Date(2026, 7, 16, 8, 1, 0, 0, time.UTC),
	}
	if valid {
		result.Candidate = &application.CandidateIdentity{
			RevisionHash: testCandidate, ModuleVersion: "1.1.0", DataSchemaFormat: 1, DataSchemaFingerprint: testDataSchema,
		}
	} else {
		result.Issues = []application.ValidationIssue{{
			Code: "source_invalid", Stage: "decode", Path: "/source", Message: "source is incomplete", Severity: "error",
		}}
	}
	return result
}

func makeHTTPPlan(t *testing.T, draft domain.Draft) application.DraftPlan {
	t.Helper()
	projectID, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	return application.DraftPlan{
		ID: testPlanID, PlanHash: testPlanHash, FormatVersion: application.DraftPlanFormatVersion,
		ProjectID: projectID, ModuleName: "crm", DraftID: draft.ID(), DraftGeneration: draft.Generation(),
		ValidationID: testValidationID, BaselineRevision: testRevision,
		BaselineDataSchemaFormat: 1, BaselineDataSchemaFingerprint: testDataSchema,
		SourceFormat: draft.SourceFormat(), SourceHash: draft.SourceHash(),
		Candidate: application.CandidateIdentity{
			RevisionHash: testCandidate, ModuleVersion: "1.1.0", DataSchemaFormat: 1, DataSchemaFingerprint: testDataSchema,
		},
		Changes: []application.PlanChange{{
			Code: "field.options_added", Path: "/resources/lead/fields/stage/options/contacted",
			Kind: "add", Risk: "low", Impact: "accepted values expand", RequiresMigration: false,
		}},
		RiskSummary: application.PlanRiskSummary{Low: 1}, Outcome: "compatible", Risk: "low",
		RecordNamespaceChanged: true, MigrationExecutionSupported: false,
		CreatedBy: "bootstrap-admin", CreatedAt: time.Date(2026, 7, 16, 8, 2, 0, 0, time.UTC),
	}
}

func performDraftAdminRequest(
	handler http.Handler,
	method, path, body, contentType, ifMatch string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type fakeDraftWorkflowService struct {
	create        func(context.Context, project.ID, actor.Context, string, application.CreateDraftInput) (domain.Draft, bool, error)
	get           func(context.Context, project.ID, actor.Context, string, string) (domain.Draft, error)
	getSource     func(context.Context, project.ID, actor.Context, string, string) (application.DraftSource, error)
	replace       func(context.Context, project.ID, actor.Context, string, string, uint64, application.ReplaceDraftInput) (domain.Draft, bool, error)
	validate      func(context.Context, project.ID, actor.Context, string, string, uint64) (application.DraftValidation, bool, error)
	getValidation func(context.Context, project.ID, actor.Context, string, string, string) (application.DraftValidation, error)
	plan          func(context.Context, project.ID, actor.Context, string, string, string, uint64) (application.DraftPlan, bool, error)
	getPlan       func(context.Context, project.ID, actor.Context, string, string, string) (application.DraftPlan, error)
}

func (service *fakeDraftWorkflowService) Create(ctx context.Context, execution access.Execution, module string, input application.CreateDraftInput) (domain.Draft, bool, error) {
	if service.create == nil {
		return domain.Draft{}, false, errors.New("unexpected draft Create")
	}
	return service.create(ctx, execution.Scope().ProjectID(), execution.Actor(), module, input)
}

func (service *fakeDraftWorkflowService) Get(ctx context.Context, execution access.Execution, module, id string) (domain.Draft, error) {
	if service.get == nil {
		return domain.Draft{}, errors.New("unexpected draft Get")
	}
	return service.get(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id)
}

func (service *fakeDraftWorkflowService) GetSource(ctx context.Context, execution access.Execution, module, id string) (application.DraftSource, error) {
	if service.getSource == nil {
		return application.DraftSource{}, errors.New("unexpected draft GetSource")
	}
	return service.getSource(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id)
}

func (service *fakeDraftWorkflowService) Replace(ctx context.Context, execution access.Execution, module, id string, generation uint64, input application.ReplaceDraftInput) (domain.Draft, bool, error) {
	if service.replace == nil {
		return domain.Draft{}, false, errors.New("unexpected draft Replace")
	}
	return service.replace(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id, generation, input)
}

func (service *fakeDraftWorkflowService) Validate(ctx context.Context, execution access.Execution, module, id string, generation uint64) (application.DraftValidation, bool, error) {
	if service.validate == nil {
		return application.DraftValidation{}, false, errors.New("unexpected draft Validate")
	}
	return service.validate(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id, generation)
}

func (service *fakeDraftWorkflowService) GetValidation(ctx context.Context, execution access.Execution, module, id, validationID string) (application.DraftValidation, error) {
	if service.getValidation == nil {
		return application.DraftValidation{}, errors.New("unexpected draft GetValidation")
	}
	return service.getValidation(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id, validationID)
}

func (service *fakeDraftWorkflowService) Plan(ctx context.Context, execution access.Execution, module, id, validationID string, generation uint64) (application.DraftPlan, bool, error) {
	if service.plan == nil {
		return application.DraftPlan{}, false, errors.New("unexpected draft Plan")
	}
	return service.plan(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id, validationID, generation)
}

func (service *fakeDraftWorkflowService) GetPlan(ctx context.Context, execution access.Execution, module, id, planID string) (application.DraftPlan, error) {
	if service.getPlan == nil {
		return application.DraftPlan{}, errors.New("unexpected draft GetPlan")
	}
	return service.getPlan(ctx, execution.Scope().ProjectID(), execution.Actor(), module, id, planID)
}

var _ DraftWorkflowService = (*fakeDraftWorkflowService)(nil)
