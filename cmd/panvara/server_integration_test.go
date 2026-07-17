//go:build integration

/*
   Panvara
   cmd/panvara/server_integration_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shezw/panvara/internal/buildinfo"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
	"github.com/shezw/panvara/internal/interfaces/httpserver"
)

const integrationPostgresImage = "postgres:18.4-alpine"

type integrationRecord struct {
	ID      string         `json:"id"`
	Version uint64         `json:"version"`
	Data    map[string]any `json:"data"`
}

type integrationList struct {
	Data       []integrationRecord `json:"data"`
	NextCursor string              `json:"next_cursor"`
}

type integrationRevision struct {
	Module               string                          `json:"module"`
	Revision             string                          `json:"revision"`
	DataSchemaIdentities []integrationDataSchemaIdentity `json:"data_schema_identities"`
	SourceFormat         string                          `json:"source_format"`
	SourceHash           string                          `json:"source_hash"`
	Origin               string                          `json:"origin"`
	RegisteredBy         string                          `json:"registered_by"`
	RegisteredAt         time.Time                       `json:"registered_at"`
}

type integrationDataSchemaIdentity struct {
	Format      int    `json:"format"`
	Fingerprint string `json:"fingerprint"`
}

type integrationRevisionList struct {
	Data []integrationRevision `json:"data"`
}

type integrationRevisionSnapshot struct {
	Metadata integrationRevision
	Source   []byte
}

type integrationDraft struct {
	DraftID      string `json:"draft_id"`
	DraftVersion uint64 `json:"draft_version"`
	SourceHash   string `json:"source_hash"`
}

type integrationValidation struct {
	ValidationID      string  `json:"validation_id"`
	ValidationHash    string  `json:"validation_hash"`
	DraftVersion      uint64  `json:"draft_version"`
	Valid             bool    `json:"valid"`
	CandidateRevision *string `json:"candidate_revision"`
	Violations        []struct {
		Code string `json:"code"`
		Path string `json:"path"`
	} `json:"violations"`
}

type integrationPlan struct {
	PlanID       string `json:"plan_id"`
	PlanHash     string `json:"plan_hash"`
	ValidationID string `json:"validation_id"`
	Effects      struct {
		PlanRecorded       bool `json:"plan_recorded"`
		RevisionRegistered bool `json:"revision_registered"`
		Published          bool `json:"published"`
		Activated          bool `json:"activated"`
		RecordsMigrated    bool `json:"records_migrated"`
		RuntimeChanged     bool `json:"runtime_changed"`
	} `json:"effects"`
}

func TestServerProfileHTTPPersistenceLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	databaseURL := isolatedHTTPIntegrationDatabaseURL(t, ctx)
	config := serverConfig{
		databaseURL: databaseURL, moduleSource: "testdata/crm-leads.yaml", moduleFormat: "yaml",
		projectID: "01981234-5678-7abc-8def-0123456789ab", projectKey: "crm",
		projectLocale: "en-US", projectZone: "UTC", projectMoney: "USD",
		environmentKey: "default",
		adminToken:     testIntegrationAdminToken,
	}

	application, server, baseURL := startIntegrationServer(t, ctx, config)
	client := &http.Client{Timeout: 5 * time.Second}
	assertIntegrationStatus(t, client, http.MethodGet, baseURL+"/readyz", "", "", "", http.StatusOK, nil)
	artifact := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
		"", "", "", http.StatusOK, nil,
	)
	if artifact.Header.Get("ETag") == "" || !bytes.Contains(artifact.Body, []byte(`"openapi":"3.1.0"`)) {
		t.Fatalf("OpenAPI response is incomplete: etag=%q body=%s", artifact.Header.Get("ETag"), artifact.Body)
	}
	var openAPI map[string]any
	if err := json.Unmarshal(artifact.Body, &openAPI); err != nil {
		t.Fatal(err)
	}
	runtimeRevision, _ := openAPI["x-panvara-revision"].(string)
	if runtimeRevision == "" {
		t.Fatalf("OpenAPI x-panvara-revision = %#v", openAPI["x-panvara-revision"])
	}
	initialRegistry := assertIntegrationRevisionRegistry(
		t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 1,
	)
	initialSource, err := os.ReadFile(config.moduleSource)
	if err != nil {
		t.Fatalf("read initial integration module source: %v", err)
	}
	if !bytes.Equal(initialRegistry.Source, initialSource) {
		t.Fatal("bootstrap Registry source differs from the first module source bytes")
	}
	draftBasePath := "/api/admin/core/v1alpha1/modules/crm.leads/drafts"
	draftHeaders := http.Header{
		"Content-Type":    []string{"application/yaml"},
		"Idempotency-Key": []string{"server-e2e-invalid-draft"},
	}
	createdDraftResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+draftBasePath+"?baseline_revision="+url.QueryEscape(runtimeRevision),
		testIntegrationAdminToken, "spec: [", "", http.StatusCreated, draftHeaders,
	)
	var createdDraft integrationDraft
	if err := json.Unmarshal(createdDraftResponse.Body, &createdDraft); err != nil {
		t.Fatal(err)
	}
	if createdDraft.DraftID == "" || createdDraft.DraftVersion != 1 || createdDraftResponse.Header.Get("ETag") != `"1"` {
		t.Fatalf("created Draft = %#v headers=%#v", createdDraft, createdDraftResponse.Header)
	}
	replayedDraftResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+draftBasePath+"?baseline_revision="+url.QueryEscape(runtimeRevision),
		testIntegrationAdminToken, "spec: [", "", http.StatusOK, draftHeaders,
	)
	var replayedDraft integrationDraft
	if err := json.Unmarshal(replayedDraftResponse.Body, &replayedDraft); err != nil {
		t.Fatal(err)
	}
	if replayedDraft.DraftID != createdDraft.DraftID || replayedDraft.SourceHash != createdDraft.SourceHash {
		t.Fatalf("idempotent Draft replay = %#v, want %#v", replayedDraft, createdDraft)
	}
	draftItemPath := draftBasePath + "/" + createdDraft.DraftID
	createdSourceResponse := assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+draftItemPath+"/source",
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if createdSourceResponse.Header.Get("ETag") != `"1"` ||
		createdSourceResponse.Header.Get("X-Panvara-Source-Hash") != createdDraft.SourceHash ||
		!bytes.Equal(createdSourceResponse.Body, []byte("spec: [")) {
		t.Fatalf("created Draft Source = headers %#v body %q", createdSourceResponse.Header, createdSourceResponse.Body)
	}
	validationCollectionPath := draftItemPath + "/validations"
	invalidValidationResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+validationCollectionPath,
		testIntegrationAdminToken, "", `"1"`, http.StatusCreated, nil,
	)
	var invalidValidation integrationValidation
	if err := json.Unmarshal(invalidValidationResponse.Body, &invalidValidation); err != nil {
		t.Fatal(err)
	}
	if invalidValidation.Valid || invalidValidation.ValidationID == "" ||
		invalidValidation.ValidationID != invalidValidation.ValidationHash || len(invalidValidation.Violations) == 0 {
		t.Fatalf("invalid Draft validation = %#v", invalidValidation)
	}
	replayedValidationResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+validationCollectionPath,
		testIntegrationAdminToken, "", `"1"`, http.StatusOK, nil,
	)
	var replayedValidation integrationValidation
	if err := json.Unmarshal(replayedValidationResponse.Body, &replayedValidation); err != nil {
		t.Fatal(err)
	}
	if replayedValidation.ValidationID != invalidValidation.ValidationID {
		t.Fatalf("validation replay ID = %q, want %q", replayedValidation.ValidationID, invalidValidation.ValidationID)
	}
	validDraftSource := bytes.Replace(initialSource, []byte("version: 1.0.0"), []byte("version: 1.1.0"), 1)
	if bytes.Equal(validDraftSource, initialSource) {
		t.Fatal("Draft integration candidate version marker was not replaced")
	}
	replacedDraftResponse := assertIntegrationStatus(
		t, client, http.MethodPut, baseURL+draftItemPath+"/source",
		testIntegrationAdminToken, string(validDraftSource), `"1"`, http.StatusOK,
		http.Header{"Content-Type": []string{"application/yaml"}},
	)
	var replacedDraft integrationDraft
	if err := json.Unmarshal(replacedDraftResponse.Body, &replacedDraft); err != nil {
		t.Fatal(err)
	}
	if replacedDraft.DraftVersion != 2 || replacedDraftResponse.Header.Get("ETag") != `"2"` {
		t.Fatalf("replaced Draft = %#v headers=%#v", replacedDraft, replacedDraftResponse.Header)
	}
	assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+validationCollectionPath,
		testIntegrationAdminToken, "", `"1"`, http.StatusPreconditionFailed, nil,
	)
	validValidationResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+validationCollectionPath,
		testIntegrationAdminToken, "", `"2"`, http.StatusCreated, nil,
	)
	var validValidation integrationValidation
	if err := json.Unmarshal(validValidationResponse.Body, &validValidation); err != nil {
		t.Fatal(err)
	}
	if !validValidation.Valid || validValidation.CandidateRevision == nil || *validValidation.CandidateRevision == "" ||
		len(validValidation.Violations) != 0 {
		t.Fatalf("valid Draft validation = %#v", validValidation)
	}
	planCollectionPath := draftItemPath + "/plans"
	planBody := fmt.Sprintf(`{"validation_id":%q}`, validValidation.ValidationID)
	createdPlanResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+planCollectionPath,
		testIntegrationAdminToken, planBody, `"2"`, http.StatusCreated, nil,
	)
	var createdPlan integrationPlan
	if err := json.Unmarshal(createdPlanResponse.Body, &createdPlan); err != nil {
		t.Fatal(err)
	}
	if createdPlan.PlanID == "" || createdPlan.PlanHash == "" || createdPlan.PlanID == createdPlan.PlanHash ||
		createdPlan.ValidationID != validValidation.ValidationID || !createdPlan.Effects.PlanRecorded ||
		createdPlan.Effects.RevisionRegistered || createdPlan.Effects.Published || createdPlan.Effects.Activated ||
		createdPlan.Effects.RecordsMigrated || createdPlan.Effects.RuntimeChanged {
		t.Fatalf("created change Plan = %#v", createdPlan)
	}
	replayedPlanResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+planCollectionPath,
		testIntegrationAdminToken, planBody, `"2"`, http.StatusOK, nil,
	)
	var replayedPlan integrationPlan
	if err := json.Unmarshal(replayedPlanResponse.Body, &replayedPlan); err != nil {
		t.Fatal(err)
	}
	if replayedPlan.PlanID != createdPlan.PlanID || replayedPlan.PlanHash != createdPlan.PlanHash {
		t.Fatalf("Plan replay = %#v, want %#v", replayedPlan, createdPlan)
	}
	equivalentSource := append([]byte("# semantically equivalent Draft generation\n"), validDraftSource...)
	equivalentDraftResponse := assertIntegrationStatus(
		t, client, http.MethodPut, baseURL+draftItemPath+"/source",
		testIntegrationAdminToken, string(equivalentSource), `"2"`, http.StatusOK,
		http.Header{"Content-Type": []string{"application/yaml"}},
	)
	var equivalentDraft integrationDraft
	if err := json.Unmarshal(equivalentDraftResponse.Body, &equivalentDraft); err != nil {
		t.Fatal(err)
	}
	if equivalentDraft.DraftVersion != 3 || equivalentDraft.SourceHash == replacedDraft.SourceHash ||
		equivalentDraftResponse.Header.Get("ETag") != `"3"` {
		t.Fatalf("equivalent Draft replacement = %#v headers=%#v", equivalentDraft, equivalentDraftResponse.Header)
	}
	equivalentValidationResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+validationCollectionPath,
		testIntegrationAdminToken, "", `"3"`, http.StatusCreated, nil,
	)
	var equivalentValidation integrationValidation
	if err := json.Unmarshal(equivalentValidationResponse.Body, &equivalentValidation); err != nil {
		t.Fatal(err)
	}
	if !equivalentValidation.Valid || equivalentValidation.ValidationID == validValidation.ValidationID ||
		equivalentValidation.CandidateRevision == nil || validValidation.CandidateRevision == nil ||
		*equivalentValidation.CandidateRevision != *validValidation.CandidateRevision {
		t.Fatalf("equivalent Draft validation = %#v, first=%#v", equivalentValidation, validValidation)
	}
	equivalentPlanBody := fmt.Sprintf(`{"validation_id":%q}`, equivalentValidation.ValidationID)
	equivalentPlanResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+planCollectionPath,
		testIntegrationAdminToken, equivalentPlanBody, `"3"`, http.StatusCreated, nil,
	)
	var equivalentPlan integrationPlan
	if err := json.Unmarshal(equivalentPlanResponse.Body, &equivalentPlan); err != nil {
		t.Fatal(err)
	}
	if equivalentPlan.PlanID == createdPlan.PlanID || equivalentPlan.PlanHash != createdPlan.PlanHash {
		t.Fatalf("equivalent Draft plan = %#v, first=%#v", equivalentPlan, createdPlan)
	}
	equivalentPlanReplay := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+planCollectionPath,
		testIntegrationAdminToken, equivalentPlanBody, `"3"`, http.StatusOK, nil,
	)
	var replayedEquivalentPlan integrationPlan
	if err := json.Unmarshal(equivalentPlanReplay.Body, &replayedEquivalentPlan); err != nil {
		t.Fatal(err)
	}
	if replayedEquivalentPlan.PlanID != equivalentPlan.PlanID || replayedEquivalentPlan.PlanHash != equivalentPlan.PlanHash {
		t.Fatalf("equivalent Plan replay = %#v, want %#v", replayedEquivalentPlan, equivalentPlan)
	}
	escapedNULLabelSource := `{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","metadata":{"name":"crm.leads","version":"1.0.0","labels":{"en-US":"Bad\u0000Label"}},"spec":{"resources":[]}}`
	escapedNULCreate := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+draftBasePath+"?baseline_revision=none",
		testIntegrationAdminToken, escapedNULLabelSource, "", http.StatusCreated,
		http.Header{"Content-Type": []string{"application/json"}, "Idempotency-Key": []string{"server-e2e-escaped-nul-label"}},
	)
	var escapedNULDraft integrationDraft
	if err := json.Unmarshal(escapedNULCreate.Body, &escapedNULDraft); err != nil {
		t.Fatal(err)
	}
	escapedNULValidationResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+draftBasePath+"/"+escapedNULDraft.DraftID+"/validations",
		testIntegrationAdminToken, "", `"1"`, http.StatusCreated, nil,
	)
	var escapedNULValidation integrationValidation
	if err := json.Unmarshal(escapedNULValidationResponse.Body, &escapedNULValidation); err != nil {
		t.Fatal(err)
	}
	if escapedNULValidation.Valid || len(escapedNULValidation.Violations) == 0 {
		t.Fatalf("escaped NUL label validation = %#v", escapedNULValidation)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/modules/crm.leads/revisions/"+url.PathEscape(*validValidation.CandidateRevision),
		testIntegrationAdminToken, "", "", http.StatusNotFound, nil,
	)
	unchangedArtifact := assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
		"", "", "", http.StatusOK, nil,
	)
	var unchangedOpenAPI map[string]any
	if err := json.Unmarshal(unchangedArtifact.Body, &unchangedOpenAPI); err != nil {
		t.Fatal(err)
	}
	if unchangedOpenAPI["x-panvara-revision"] != runtimeRevision {
		t.Fatalf("runtime Revision changed after Plan: %#v", unchangedOpenAPI["x-panvara-revision"])
	}
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/ui-schema.json",
		"", "", "", http.StatusOK, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/v1alpha1/crm.leads/organization",
		testIntegrationAdminToken, `{"name":"Unsafe\u0000Organization"}`, "", http.StatusUnprocessableEntity, nil,
	)

	organizationResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/v1alpha1/crm.leads/organization",
		testIntegrationAdminToken, `{"name":"Analytical Engines"}`, "", http.StatusCreated, nil,
	)
	organization := decodeIntegrationRecord(t, organizationResponse.Body)
	if organization.ID == "" || organization.Version != 1 {
		t.Fatalf("organization = %+v", organization)
	}

	leadBody := fmt.Sprintf(
		`{"organization":%q,"email":"ada@example.com","stage":"new"}`,
		organization.ID,
	)
	leadResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/public/v1alpha1/crm.leads/lead",
		"", leadBody, "", http.StatusCreated, nil,
	)
	lead := decodeIntegrationRecord(t, leadResponse.Body)
	if lead.ID == "" || lead.Version != 1 {
		t.Fatalf("lead = %+v", lead)
	}

	listResponse := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/v1alpha1/crm.leads/lead?filter[stage]=new",
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var list integrationList
	if err := json.Unmarshal(listResponse.Body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != lead.ID {
		t.Fatalf("filtered list = %+v", list)
	}

	itemPath := baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	getResponse := assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if getResponse.Header.Get("ETag") != `"1"` {
		t.Fatalf("GET ETag = %q", getResponse.Header.Get("ETag"))
	}
	patchResponse := assertIntegrationStatus(
		t, client, http.MethodPatch, itemPath, testIntegrationAdminToken,
		`{"stage":"qualified"}`, getResponse.Header.Get("ETag"), http.StatusOK, nil,
	)
	if patchResponse.Header.Get("ETag") != `"2"` {
		t.Fatalf("PATCH ETag = %q", patchResponse.Header.Get("ETag"))
	}

	setBootstrapOwnerGrantRevoked(t, ctx, databaseURL, config.projectID, true)
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/modules/crm.leads/revisions?limit=100",
		testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+draftItemPath,
		testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	stopIntegrationServer(t, server, application)
	application, server, baseURL = startIntegrationServer(t, ctx, config)
	itemPath = baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/modules/crm.leads/revisions?limit=100",
		testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+draftItemPath,
		testIntegrationAdminToken, "", "", http.StatusForbidden, nil,
	)
	setBootstrapOwnerGrantRevoked(t, ctx, databaseURL, config.projectID, false)
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	stopIntegrationServer(t, server, application)

	var restarted integrationResponse
	for restart := 1; restart <= 3; restart++ {
		application, server, baseURL = startIntegrationServer(t, ctx, config)
		itemPath = baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
		restarted = assertIntegrationStatus(
			t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
		)
		if got := decodeIntegrationRecord(t, restarted.Body).Data["stage"]; got != "qualified" {
			t.Fatalf("stage after restart %d = %#v", restart, got)
		}
		if restart == 1 {
			persistedDraft := assertIntegrationStatus(
				t, client, http.MethodGet, baseURL+draftItemPath,
				testIntegrationAdminToken, "", "", http.StatusOK, nil,
			)
			var draftAfterRestart integrationDraft
			if err := json.Unmarshal(persistedDraft.Body, &draftAfterRestart); err != nil {
				t.Fatal(err)
			}
			if draftAfterRestart.DraftVersion != 3 || draftAfterRestart.SourceHash != equivalentDraft.SourceHash {
				t.Fatalf("Draft after restart = %#v, want %#v", draftAfterRestart, equivalentDraft)
			}
			assertIntegrationStatus(
				t, client, http.MethodGet,
				baseURL+validationCollectionPath+"/"+url.PathEscape(validValidation.ValidationID),
				testIntegrationAdminToken, "", "", http.StatusOK, nil,
			)
			assertIntegrationStatus(
				t, client, http.MethodGet,
				baseURL+planCollectionPath+"/"+url.PathEscape(createdPlan.PlanID),
				testIntegrationAdminToken, "", "", http.StatusOK, nil,
			)
			assertIntegrationStatus(
				t, client, http.MethodGet,
				baseURL+planCollectionPath+"/"+url.PathEscape(equivalentPlan.PlanID),
				testIntegrationAdminToken, "", "", http.StatusOK, nil,
			)
		}
		restartedRegistry := assertIntegrationRevisionRegistry(
			t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 1,
		)
		if restartedRegistry.Metadata.RegisteredAt != initialRegistry.Metadata.RegisteredAt ||
			restartedRegistry.Metadata.SourceHash != initialRegistry.Metadata.SourceHash ||
			integrationDataSchemaFingerprint(restartedRegistry.Metadata, 1) != integrationDataSchemaFingerprint(initialRegistry.Metadata, 1) ||
			!bytes.Equal(restartedRegistry.Source, initialRegistry.Source) {
			t.Fatalf("Registry bootstrap fact changed after restart %d: got %#v want %#v", restart, restartedRegistry, initialRegistry)
		}
		if restart < 3 {
			stopIntegrationServer(t, server, application)
		}
	}

	// A changed Source compiles to a new immutable revision namespace. The old
	// record is invisible and model-declared unique values are revision-local;
	// returning to the original Source makes the original record visible again.
	source, err := os.ReadFile(config.moduleSource)
	if err != nil {
		t.Fatalf("read integration module source: %v", err)
	}
	changedSource := bytes.Replace(source, []byte("version: 1.0.0"), []byte("version: 1.0.1"), 1)
	if bytes.Equal(source, changedSource) {
		t.Fatal("integration module version marker was not replaced")
	}
	changedPath := filepath.Join(t.TempDir(), "crm-leads-v1.0.1.yaml")
	if err := os.WriteFile(changedPath, changedSource, 0o600); err != nil {
		t.Fatalf("write changed integration module source: %v", err)
	}
	stopIntegrationServer(t, server, application)
	changedConfig := config
	changedConfig.moduleSource = changedPath
	changedApplication, changedServer, changedBaseURL := startIntegrationServer(t, ctx, changedConfig)
	changedRegistry := assertIntegrationRevisionRegistry(
		t, client, changedBaseURL, testIntegrationAdminToken, "crm.leads", changedApplication.revision, 2,
	)
	if integrationDataSchemaFingerprint(changedRegistry.Metadata, 1) != integrationDataSchemaFingerprint(initialRegistry.Metadata, 1) {
		t.Fatal("semantic version-only revision changed DataSchemaFingerprint")
	}
	changedItemPath := changedBaseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	assertIntegrationStatus(
		t, client, http.MethodGet, changedItemPath, testIntegrationAdminToken, "", "", http.StatusNotFound, nil,
	)
	changedOrganizationResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		changedBaseURL+"/api/admin/v1alpha1/crm.leads/organization",
		testIntegrationAdminToken, `{"name":"Analytical Engines"}`, "", http.StatusCreated, nil,
	)
	changedOrganization := decodeIntegrationRecord(t, changedOrganizationResponse.Body)
	changedLeadBody := fmt.Sprintf(
		`{"organization":%q,"email":"ada@example.com","stage":"new"}`,
		changedOrganization.ID,
	)
	changedLeadResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		changedBaseURL+"/api/public/v1alpha1/crm.leads/lead",
		"", changedLeadBody, "", http.StatusCreated, nil,
	)
	if changedLead := decodeIntegrationRecord(t, changedLeadResponse.Body); changedLead.ID == lead.ID {
		t.Fatalf("changed revision unexpectedly reused generated lead ID %q", changedLead.ID)
	}
	stopIntegrationServer(t, changedServer, changedApplication)

	application, server, baseURL = startIntegrationServer(t, ctx, config)
	itemPath = baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	restarted = assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if got := decodeIntegrationRecord(t, restarted.Body).Data["stage"]; got != "qualified" {
		t.Fatalf("stage after returning to original revision = %#v", got)
	}
	returnedRegistry := assertIntegrationRevisionRegistry(
		t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 2,
	)
	if returnedRegistry.Metadata.RegisteredAt != initialRegistry.Metadata.RegisteredAt ||
		returnedRegistry.Metadata.SourceHash != initialRegistry.Metadata.SourceHash {
		t.Fatal("returning to original source replaced bootstrap Registry provenance")
	}
	deleted := assertIntegrationStatus(
		t, client, http.MethodDelete, itemPath, testIntegrationAdminToken,
		"", restarted.Header.Get("ETag"), http.StatusNoContent, nil,
	)
	if len(deleted.Body) != 0 {
		t.Fatalf("DELETE body = %q, want empty", deleted.Body)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusNotFound, nil,
	)
	stopIntegrationServer(t, server, application)

	pool, err := panvarapg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE panvara_module_revision DISABLE TRIGGER panvara_module_revision_immutable_rows`); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_module_revision
		SET canonical_ir_bytes = $4
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, config.projectID, "crm.leads", runtimeRevision, []byte(`{"corrupt":true}`)); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE panvara_module_revision ENABLE TRIGGER panvara_module_revision_immutable_rows`); err != nil {
		pool.Close()
		t.Fatal(err)
	}
	pool.Close()
	corruptApplication, err := buildServerApplication(ctx, config)
	if corruptApplication != nil {
		corruptApplication.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "registry is corrupt") {
		t.Fatalf("buildServerApplication(corrupt Registry) error = %v", err)
	}
}

func setBootstrapOwnerGrantRevoked(
	t *testing.T,
	ctx context.Context,
	databaseURL string,
	projectID string,
	revoked bool,
) {
	t.Helper()
	pool, err := panvarapg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL to change bootstrap owner grant: %v", err)
	}
	defer pool.Close()
	var command string
	if revoked {
		command = `
			UPDATE panvara_access_grant
			SET revoked_at = clock_timestamp()
			WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
			  AND role = 'project.owner' AND revoked_at IS NULL
		`
	} else {
		command = `
			UPDATE panvara_access_grant
			SET revoked_at = NULL
			WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
			  AND role = 'project.owner' AND revoked_at IS NOT NULL
		`
	}
	result, err := pool.Exec(ctx, command, projectID)
	if err != nil {
		t.Fatalf("change bootstrap owner grant revoked=%t: %v", revoked, err)
	}
	if result.RowsAffected() != 1 {
		t.Fatalf("change bootstrap owner grant revoked=%t affected %d rows, want 1", revoked, result.RowsAffected())
	}
}

const testIntegrationAdminToken = "panvara-integration-admin-token-32-bytes"

type integrationResponse struct {
	Header http.Header
	Body   []byte
}

func assertIntegrationStatus(
	t *testing.T,
	client *http.Client,
	method string,
	target string,
	token string,
	body string,
	ifMatch string,
	want int,
	headers http.Header,
) integrationResponse {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" && headers.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != want {
		t.Fatalf("%s %s status/body = %d %s, want %d", method, target, response.StatusCode, responseBody, want)
	}
	return integrationResponse{Header: response.Header.Clone(), Body: responseBody}
}

func decodeIntegrationRecord(t *testing.T, body []byte) integrationRecord {
	t.Helper()
	var result integrationRecord
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode record %s: %v", body, err)
	}
	return result
}

func assertIntegrationRevisionRegistry(
	t *testing.T,
	client *http.Client,
	baseURL string,
	token string,
	module string,
	revision string,
	wantCount int,
) integrationRevisionSnapshot {
	t.Helper()
	listResponse := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/modules/"+module+"/revisions?limit=100",
		token, "", "", http.StatusOK, nil,
	)
	var list integrationRevisionList
	if err := json.Unmarshal(listResponse.Body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != wantCount {
		t.Fatalf("Revision Registry count = %d, want %d: %s", len(list.Data), wantCount, listResponse.Body)
	}
	var metadata integrationRevision
	for _, candidate := range list.Data {
		if candidate.Revision == revision {
			metadata = candidate
			break
		}
	}
	if metadata.Revision == "" || metadata.Module != module ||
		integrationDataSchemaFingerprint(metadata, 1) == "" || metadata.SourceHash == "" ||
		metadata.Origin != "bootstrap" || metadata.RegisteredBy != "system:bootstrap" ||
		metadata.RegisteredAt.IsZero() {
		t.Fatalf("Revision Registry metadata = %#v", metadata)
	}
	detailPath := baseURL + "/api/admin/core/v1alpha1/modules/" + module + "/revisions/" + revision
	detailResponse := assertIntegrationStatus(
		t, client, http.MethodGet, detailPath, token, "", "", http.StatusOK, nil,
	)
	var detail integrationRevision
	if err := json.Unmarshal(detailResponse.Body, &detail); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detail, metadata) {
		t.Fatalf("Revision detail = %#v, want %#v", detail, metadata)
	}
	sourceResponse := assertIntegrationStatus(
		t, client, http.MethodGet, detailPath+"/source", token, "", "", http.StatusOK, nil,
	)
	if sourceResponse.Header.Get("ETag") != `"`+metadata.SourceHash+`"` {
		t.Fatalf("Revision source ETag = %q, want %q", sourceResponse.Header.Get("ETag"), `"`+metadata.SourceHash+`"`)
	}
	if sourceResponse.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("Revision source Cache-Control = %q", sourceResponse.Header.Get("Cache-Control"))
	}
	wantContentType := "application/yaml; charset=utf-8"
	if metadata.SourceFormat == "json" {
		wantContentType = "application/json; charset=utf-8"
	}
	if sourceResponse.Header.Get("Content-Type") != wantContentType {
		t.Fatalf("Revision source Content-Type = %q, want %q", sourceResponse.Header.Get("Content-Type"), wantContentType)
	}
	return integrationRevisionSnapshot{Metadata: metadata, Source: sourceResponse.Body}
}

func integrationDataSchemaFingerprint(metadata integrationRevision, format int) string {
	previous := 0
	for _, identity := range metadata.DataSchemaIdentities {
		if identity.Format <= previous || identity.Fingerprint == "" {
			return ""
		}
		previous = identity.Format
		if identity.Format == format {
			return identity.Fingerprint
		}
	}
	return ""
}

func startIntegrationServer(
	t *testing.T,
	ctx context.Context,
	config serverConfig,
) (*applicationRuntime, *httpserver.Server, string) {
	t.Helper()
	application, err := buildServerApplication(ctx, config)
	if err != nil {
		t.Fatalf("buildServerApplication() error = %v", err)
	}
	server := httpserver.NewWithHandler(
		"127.0.0.1:0", application.ready, buildinfo.Current(), application.handler,
	)
	if err := server.Start(ctx); err != nil {
		application.Close()
		t.Fatalf("start HTTP server: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(stopCtx)
		application.Close()
	})
	return application, server, "http://" + server.Addr()
}

func stopIntegrationServer(t *testing.T, server *httpserver.Server, application *applicationRuntime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Stop(ctx); err != nil {
		t.Fatalf("stop HTTP server: %v", err)
	}
	application.Close()
}

func isolatedHTTPIntegrationDatabaseURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	baseURL := httpIntegrationDatabaseURL(t, ctx)
	admin, err := panvarapg.Open(ctx, baseURL)
	if err != nil {
		t.Fatalf("open integration PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("panvara_http_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+identifier+" CASCADE")
		admin.Close()
	})
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func httpIntegrationDatabaseURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	if databaseURL := strings.TrimSpace(os.Getenv("PANVARA_TEST_DATABASE_URL")); databaseURL != "" {
		assertPostgres184(t, ctx, databaseURL)
		return databaseURL
	}
	requireDocker := os.Getenv("PANVARA_REQUIRE_DOCKER") == "1"
	if _, err := exec.LookPath("docker"); err != nil {
		httpDockerUnavailable(t, requireDocker, "docker executable is unavailable", err)
	}
	infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(infoCtx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		httpDockerUnavailable(t, requireDocker, "Docker daemon is unavailable: "+strings.TrimSpace(string(output)), err)
	}

	containerName := fmt.Sprintf("panvara-http-pg18-%d-%d", os.Getpid(), time.Now().UnixNano())
	runCtx, cancelRun := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelRun()
	output, err := exec.CommandContext(
		runCtx,
		"docker", "run", "--detach", "--rm", "--name", containerName,
		"--env", "POSTGRES_PASSWORD=panvara_test",
		"--env", "POSTGRES_DB=panvara_test",
		"--publish", "127.0.0.1::5432",
		integrationPostgresImage,
	).CombinedOutput()
	if err != nil {
		httpDockerUnavailable(t, requireDocker, "start PostgreSQL 18.4 container: "+strings.TrimSpace(string(output)), err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerName).Run()
	})

	address := waitDockerPort(t, ctx, containerName)
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("parse PostgreSQL address %q: %v", address, err)
	}
	databaseURL := fmt.Sprintf(
		"postgres://postgres:panvara_test@127.0.0.1:%s/panvara_test?sslmode=disable",
		port,
	)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		pool, err := panvarapg.Open(ctx, databaseURL)
		if err == nil {
			pool.Close()
			assertPostgres184(t, ctx, databaseURL)
			return databaseURL
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("PostgreSQL 18.4 container did not become ready within 60 seconds")
	return ""
}

func waitDockerPort(t *testing.T, ctx context.Context, containerName string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		portCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		output, err := exec.CommandContext(portCtx, "docker", "port", containerName, "5432/tcp").Output()
		cancel()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return strings.Split(strings.TrimSpace(string(output)), "\n")[0]
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Docker did not publish the PostgreSQL test port")
	return ""
}

func assertPostgres184(t *testing.T, ctx context.Context, databaseURL string) {
	t.Helper()
	pool, err := panvarapg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL for version check: %v", err)
	}
	defer pool.Close()
	var version string
	if err := pool.QueryRow(ctx, `SHOW server_version`).Scan(&version); err != nil {
		t.Fatalf("SHOW server_version: %v", err)
	}
	if !strings.HasPrefix(version, "18.4") {
		t.Fatalf("PostgreSQL version = %q, want 18.4.x", version)
	}
}

func httpDockerUnavailable(t *testing.T, required bool, message string, err error) {
	t.Helper()
	if required {
		t.Fatalf("%s: %v", message, err)
	}
	t.Skipf("%s; set PANVARA_REQUIRE_DOCKER=1 to make this fatal: %v", message, err)
}
