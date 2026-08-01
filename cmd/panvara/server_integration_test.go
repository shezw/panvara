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
	"crypto/sha256"
	"encoding/base64"
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

type integrationRelease struct {
	ReleaseID          string  `json:"release_id"`
	Module             string  `json:"module"`
	DraftID            string  `json:"draft_id"`
	DraftGeneration    uint64  `json:"draft_generation"`
	ValidationID       string  `json:"validation_id"`
	PlanID             string  `json:"plan_id"`
	PlanHash           string  `json:"plan_hash"`
	BaselineRevision   *string `json:"baseline_revision"`
	CandidateRevision  string  `json:"candidate_revision"`
	DataSchemaIdentity struct {
		Format      int    `json:"format"`
		Fingerprint string `json:"fingerprint"`
	} `json:"data_schema_identity"`
	SourceHash            string    `json:"source_hash"`
	Outcome               string    `json:"outcome"`
	Risk                  string    `json:"risk"`
	PublishedBy           string    `json:"published_by"`
	PublishedCredentialID string    `json:"published_credential_id"`
	RequestID             string    `json:"request_id"`
	PublishedAt           time.Time `json:"published_at"`
	Effects               struct {
		RevisionRegistered  bool `json:"revision_registered"`
		Published           bool `json:"published"`
		Activated           bool `json:"activated"`
		RecordsMigrated     bool `json:"records_migrated"`
		RuntimeChanged      bool `json:"runtime_changed"`
		ActivationSupported bool `json:"activation_supported"`
	} `json:"effects"`
}

type integrationActiveSnapshot struct {
	Module                  string  `json:"module"`
	ReleaseID               *string `json:"release_id"`
	RuntimeRevision         string  `json:"runtime_revision"`
	RecordNamespaceRevision string  `json:"record_namespace_revision"`
	DataSchemaIdentity      struct {
		Format      int    `json:"format"`
		Fingerprint string `json:"fingerprint"`
	} `json:"data_schema_identity"`
	Epoch                 uint64    `json:"epoch"`
	Origin                string    `json:"origin"`
	ActivatedBy           string    `json:"activated_by"`
	ActivatedCredentialID *string   `json:"activated_credential_id"`
	RequestID             string    `json:"request_id"`
	ActivatedAt           time.Time `json:"activated_at"`
}

type integrationAccessPrincipal struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type integrationAccessCredential struct {
	ID          string `json:"id"`
	PrincipalID string `json:"principal_id"`
	Hint        string `json:"hint"`
	Status      string `json:"status"`
}

type integrationIssuedCredential struct {
	Credential integrationAccessCredential `json:"credential"`
	Token      string                      `json:"token"`
}

type integrationCredentialList struct {
	Data []integrationAccessCredential `json:"data"`
}

type integrationOwnerGrant struct {
	PrincipalID string `json:"principal_id"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
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
	for index, invalidToken := range []string{
		strings.Repeat("a", 31) + ",",
		strings.Repeat("a", 31) + "\x7f",
	} {
		invalidBootstrapConfig := config
		invalidBootstrapConfig.projectID = fmt.Sprintf(
			"01981234-5678-7abc-8def-0123456789a%c", 'c'+rune(index),
		)
		invalidBootstrapConfig.projectKey = fmt.Sprintf("invalid-bootstrap-%d", index)
		invalidBootstrapConfig.adminToken = invalidToken
		invalidApplication, invalidError := buildServerApplication(ctx, invalidBootstrapConfig)
		if invalidApplication != nil {
			invalidApplication.Close()
		}
		if invalidError == nil || !strings.Contains(invalidError.Error(), "invalid bootstrap token") ||
			strings.Contains(invalidError.Error(), invalidToken) {
			t.Fatalf("buildServerApplication(non-Bearer bootstrap token %d) error = %v", index, invalidError)
		}
	}

	application, server, baseURL := startIntegrationServer(t, ctx, config)
	client := &http.Client{Timeout: 5 * time.Second}
	assertIntegrationStatus(t, client, http.MethodGet, baseURL+"/readyz", "", "", "", http.StatusOK, nil)
	assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/core/v1alpha1/access/principals",
		testIntegrationAdminToken,
		`{"display_name":"first","diſplay_name":"must-not-override"}`,
		"", http.StatusBadRequest, nil,
	)
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
	prePublishRecord := assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	prePublishArtifact := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
		"", "", "", http.StatusOK, nil,
	)
	releaseCollectionPath := "/api/admin/core/v1alpha1/modules/crm.leads/releases"
	releaseBody := fmt.Sprintf(`{"plan_id":%q}`, equivalentPlan.PlanID)
	createdReleaseResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+releaseCollectionPath,
		testIntegrationAdminToken, releaseBody, "", http.StatusCreated,
		http.Header{"Idempotency-Key": []string{"server-e2e-publish"}},
	)
	if createdReleaseResponse.Header.Get("Cache-Control") != "private, no-store" ||
		createdReleaseResponse.Header.Get("Location") == "" {
		t.Fatalf("created Release headers = %#v", createdReleaseResponse.Header)
	}
	var publishedRelease integrationRelease
	if err := json.Unmarshal(createdReleaseResponse.Body, &publishedRelease); err != nil {
		t.Fatal(err)
	}
	if publishedRelease.ReleaseID == "" || publishedRelease.Module != "crm.leads" ||
		publishedRelease.DraftID != equivalentDraft.DraftID ||
		publishedRelease.DraftGeneration != equivalentDraft.DraftVersion ||
		publishedRelease.ValidationID != equivalentValidation.ValidationID ||
		publishedRelease.PlanID != equivalentPlan.PlanID || publishedRelease.PlanHash != equivalentPlan.PlanHash ||
		publishedRelease.BaselineRevision == nil || *publishedRelease.BaselineRevision != runtimeRevision ||
		equivalentValidation.CandidateRevision == nil ||
		publishedRelease.CandidateRevision != *equivalentValidation.CandidateRevision ||
		publishedRelease.DataSchemaIdentity.Format <= 0 || publishedRelease.DataSchemaIdentity.Fingerprint == "" ||
		publishedRelease.SourceHash != equivalentDraft.SourceHash || publishedRelease.Outcome == "" ||
		publishedRelease.Risk == "" || publishedRelease.PublishedBy != "bootstrap-admin" ||
		publishedRelease.PublishedCredentialID == "" || publishedRelease.RequestID == "" ||
		publishedRelease.PublishedAt.IsZero() || !publishedRelease.Effects.RevisionRegistered ||
		!publishedRelease.Effects.Published || publishedRelease.Effects.Activated ||
		publishedRelease.Effects.RecordsMigrated || publishedRelease.Effects.RuntimeChanged ||
		publishedRelease.Effects.ActivationSupported {
		t.Fatalf("published Release = %#v", publishedRelease)
	}
	releasePath := createdReleaseResponse.Header.Get("Location")
	for _, key := range []string{"server-e2e-publish", "server-e2e-publish-new-key"} {
		replayed := assertIntegrationStatus(
			t, client, http.MethodPost, baseURL+releaseCollectionPath,
			testIntegrationAdminToken, releaseBody, "", http.StatusOK,
			http.Header{"Idempotency-Key": []string{key}},
		)
		var replayedRelease integrationRelease
		if err := json.Unmarshal(replayed.Body, &replayedRelease); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(replayedRelease, publishedRelease) || replayed.Header.Get("Location") != releasePath {
			t.Fatalf("Release replay with key %q = %#v, want %#v", key, replayedRelease, publishedRelease)
		}
	}
	staleDraftSource := append([]byte("# post-publish Draft change\n"), equivalentSource...)
	staleDraftResponse := assertIntegrationStatus(
		t, client, http.MethodPut, baseURL+draftItemPath+"/source",
		testIntegrationAdminToken, string(staleDraftSource), `"3"`, http.StatusOK,
		http.Header{"Content-Type": []string{"application/yaml"}},
	)
	var staleDraft integrationDraft
	if err := json.Unmarshal(staleDraftResponse.Body, &staleDraft); err != nil {
		t.Fatal(err)
	}
	if staleDraft.DraftVersion != 4 || staleDraft.SourceHash == equivalentDraft.SourceHash ||
		staleDraftResponse.Header.Get("ETag") != `"4"` {
		t.Fatalf("post-publish Draft replacement = %#v headers=%#v", staleDraft, staleDraftResponse.Header)
	}
	for _, key := range []string{"server-e2e-publish", "server-e2e-publish-stale-alias"} {
		replayed := assertIntegrationStatus(
			t, client, http.MethodPost, baseURL+releaseCollectionPath,
			testIntegrationAdminToken, releaseBody, "", http.StatusOK,
			http.Header{"Idempotency-Key": []string{key}},
		)
		var replayedRelease integrationRelease
		if err := json.Unmarshal(replayed.Body, &replayedRelease); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(replayedRelease, publishedRelease) || replayed.Header.Get("Location") != releasePath {
			t.Fatalf("stale Plan replay with key %q = %#v, want %#v", key, replayedRelease, publishedRelease)
		}
	}
	conflictingReleaseBody := fmt.Sprintf(
		`{"plan_id":%q}`,
		"sha256:"+strings.Repeat("0", 64),
	)
	conflictResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+releaseCollectionPath,
		testIntegrationAdminToken, conflictingReleaseBody, "", http.StatusConflict,
		http.Header{"Idempotency-Key": []string{"server-e2e-publish"}},
	)
	if !bytes.Contains(conflictResponse.Body, []byte(`"code":"idempotency_key_conflict"`)) {
		t.Fatalf("bound-key conflict body = %s", conflictResponse.Body)
	}
	assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+releaseCollectionPath,
		testIntegrationAdminToken,
		`{"\u0070lan_id":"`+equivalentPlan.PlanID+`"}`,
		"", http.StatusBadRequest,
		http.Header{"Idempotency-Key": []string{"server-e2e-escaped-release-key"}},
	)
	gotReleaseResponse := assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+releasePath,
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var gotRelease integrationRelease
	if err := json.Unmarshal(gotReleaseResponse.Body, &gotRelease); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRelease, publishedRelease) {
		t.Fatalf("GET Release = %#v, want %#v", gotRelease, publishedRelease)
	}
	publishedRegistry := assertIntegrationRevisionRegistry(
		t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 2,
	)
	if publishedRegistry.Metadata.RegisteredAt != initialRegistry.Metadata.RegisteredAt ||
		publishedRegistry.Metadata.SourceHash != initialRegistry.Metadata.SourceHash {
		t.Fatal("Publish replaced the bootstrap Revision fact")
	}
	publishedRevisionResponse := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/modules/crm.leads/revisions/"+
			url.PathEscape(publishedRelease.CandidateRevision),
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var publishedRevision integrationRevision
	if err := json.Unmarshal(publishedRevisionResponse.Body, &publishedRevision); err != nil {
		t.Fatal(err)
	}
	if publishedRevision.Origin != "publish" || publishedRevision.RegisteredBy != "bootstrap-admin" ||
		publishedRevision.SourceHash != equivalentDraft.SourceHash {
		t.Fatalf("published Revision metadata = %#v", publishedRevision)
	}
	postPublishArtifact := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
		"", "", "", http.StatusOK, nil,
	)
	postPublishRecord := assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if !bytes.Equal(postPublishArtifact.Body, prePublishArtifact.Body) ||
		postPublishArtifact.Header.Get("ETag") != prePublishArtifact.Header.Get("ETag") ||
		!bytes.Equal(postPublishRecord.Body, prePublishRecord.Body) ||
		postPublishRecord.Header.Get("ETag") != prePublishRecord.Header.Get("ETag") {
		t.Fatal("Publish changed active OpenAPI or the existing Record")
	}

	activatePath := releasePath + "/activate"
	activatedResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+activatePath,
		testIntegrationAdminToken, "", "", http.StatusCreated, nil,
	)
	if activatedResponse.Header.Get("Location") !=
		"/api/admin/core/v1alpha1/modules/crm.leads/active" ||
		activatedResponse.Header.Get("ETag") != `"release-epoch-2"` ||
		activatedResponse.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Activate headers = %#v", activatedResponse.Header)
	}
	var activated integrationActiveSnapshot
	if err := json.Unmarshal(activatedResponse.Body, &activated); err != nil {
		t.Fatal(err)
	}
	if activated.Module != "crm.leads" || activated.ReleaseID == nil ||
		*activated.ReleaseID != publishedRelease.ReleaseID ||
		activated.RuntimeRevision != publishedRelease.CandidateRevision ||
		activated.RecordNamespaceRevision != runtimeRevision || activated.Epoch != 2 ||
		activated.Origin != "release" || activated.ActivatedBy != "bootstrap-admin" ||
		activated.ActivatedCredentialID == nil || *activated.ActivatedCredentialID == "" ||
		activated.RequestID == "" || activated.ActivatedAt.IsZero() ||
		activated.DataSchemaIdentity != publishedRelease.DataSchemaIdentity {
		t.Fatalf("activated Snapshot = %#v, release = %#v", activated, publishedRelease)
	}
	activePath := "/api/admin/core/v1alpha1/modules/crm.leads/active"
	activeResponse := assertIntegrationStatus(
		t, client, http.MethodGet, baseURL+activePath,
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var active integrationActiveSnapshot
	if err := json.Unmarshal(activeResponse.Body, &active); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(active, activated) {
		t.Fatalf("active Snapshot = %#v, want %#v", active, activated)
	}
	replayedActivationResponse := assertIntegrationStatus(
		t, client, http.MethodPost, baseURL+activatePath,
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var replayedActivation integrationActiveSnapshot
	if err := json.Unmarshal(replayedActivationResponse.Body, &replayedActivation); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayedActivation, activated) ||
		replayedActivationResponse.Header.Get("ETag") != `"release-epoch-2"` {
		t.Fatalf("replayed activation = %#v, want %#v", replayedActivation, activated)
	}
	activeArtifact := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
		"", "", "", http.StatusOK, nil,
	)
	var activeOpenAPI map[string]any
	if err := json.Unmarshal(activeArtifact.Body, &activeOpenAPI); err != nil {
		t.Fatal(err)
	}
	if activeOpenAPI["x-panvara-revision"] != publishedRelease.CandidateRevision ||
		bytes.Equal(activeArtifact.Body, prePublishArtifact.Body) {
		t.Fatalf("active OpenAPI did not switch to Candidate: %#v", activeOpenAPI)
	}
	postActivationRecord := assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if !bytes.Equal(postActivationRecord.Body, prePublishRecord.Body) ||
		postActivationRecord.Header.Get("ETag") != prePublishRecord.Header.Get("ETag") {
		t.Fatal("compatible Activate changed or hid the existing Record")
	}

	servicePrincipal := createIntegrationServicePrincipal(
		t, client, baseURL, testIntegrationAdminToken, "Server smoke worker",
	)
	serviceCredential := issueIntegrationCredential(
		t, client, baseURL, testIntegrationAdminToken, servicePrincipal.ID, "server smoke key",
	)
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/access/principals",
		serviceCredential.Token, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusForbidden, nil,
	)
	grant := mutateIntegrationOwnerGrant(
		t, client, http.MethodPut, baseURL, testIntegrationAdminToken, servicePrincipal.ID,
	)
	if !grant.Active || grant.Role != "project.owner" {
		t.Fatalf("granted service owner = %#v", grant)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusOK, nil,
	)
	grant = mutateIntegrationOwnerGrant(
		t, client, http.MethodDelete, baseURL, testIntegrationAdminToken, servicePrincipal.ID,
	)
	if grant.Active {
		t.Fatalf("revoked service owner = %#v", grant)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusForbidden, nil,
	)
	grant = mutateIntegrationOwnerGrant(
		t, client, http.MethodPut, baseURL, testIntegrationAdminToken, servicePrincipal.ID,
	)
	if !grant.Active {
		t.Fatalf("explicitly re-granted service owner = %#v", grant)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusOK, nil,
	)
	grant = mutateIntegrationOwnerGrant(
		t, client, http.MethodDelete, baseURL, testIntegrationAdminToken, servicePrincipal.ID,
	)
	if grant.Active {
		t.Fatalf("second revoked service owner = %#v", grant)
	}
	stopIntegrationServer(t, server, application)
	application, server, baseURL = startIntegrationServer(t, ctx, config)
	itemPath = baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusForbidden, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	revokedCredentialResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/core/v1alpha1/access/credentials/"+serviceCredential.Credential.ID+"/revoke",
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	var revokedCredential integrationAccessCredential
	if err := json.Unmarshal(revokedCredentialResponse.Body, &revokedCredential); err != nil {
		t.Fatal(err)
	}
	if revokedCredential.Status != "revoked" {
		t.Fatalf("revoked service credential = %#v", revokedCredential)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, serviceCredential.Token, "", "", http.StatusUnauthorized, nil,
	)
	disabledPrincipal := createIntegrationServicePrincipal(
		t, client, baseURL, testIntegrationAdminToken, "Disabled smoke worker",
	)
	disabledCredential := issueIntegrationCredential(
		t, client, baseURL, testIntegrationAdminToken, disabledPrincipal.ID, "disable test key",
	)
	mutateIntegrationOwnerGrant(
		t, client, http.MethodPut, baseURL, testIntegrationAdminToken, disabledPrincipal.ID,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, disabledCredential.Token, "", "", http.StatusOK, nil,
	)
	disabledResponse := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/core/v1alpha1/access/principals/"+url.PathEscape(disabledPrincipal.ID)+"/disable",
		testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if err := json.Unmarshal(disabledResponse.Body, &disabledPrincipal); err != nil {
		t.Fatal(err)
	}
	if disabledPrincipal.Status != "disabled" {
		t.Fatalf("disabled service principal = %#v", disabledPrincipal)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, disabledCredential.Token, "", "", http.StatusUnauthorized, nil,
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
		if !bytes.Equal(restarted.Body, prePublishRecord.Body) ||
			restarted.Header.Get("ETag") != prePublishRecord.Header.Get("ETag") {
			t.Fatalf("Record changed after release restart %d", restart)
		}
		restartedArtifact := assertIntegrationStatus(
			t, client, http.MethodGet,
			baseURL+"/api/core/v1alpha1/modules/crm.leads/openapi.json",
			"", "", "", http.StatusOK, nil,
		)
		if !bytes.Equal(restartedArtifact.Body, activeArtifact.Body) ||
			restartedArtifact.Header.Get("ETag") != activeArtifact.Header.Get("ETag") {
			t.Fatalf("active OpenAPI changed after release restart %d", restart)
		}
		restartedActiveResponse := assertIntegrationStatus(
			t, client, http.MethodGet, baseURL+activePath,
			testIntegrationAdminToken, "", "", http.StatusOK, nil,
		)
		var restartedActive integrationActiveSnapshot
		if err := json.Unmarshal(restartedActiveResponse.Body, &restartedActive); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restartedActive, activated) {
			t.Fatalf("active Snapshot changed after restart %d: got %#v want %#v", restart, restartedActive, activated)
		}
		restartedReleaseResponse := assertIntegrationStatus(
			t, client, http.MethodGet, baseURL+releasePath,
			testIntegrationAdminToken, "", "", http.StatusOK, nil,
		)
		var restartedRelease integrationRelease
		if err := json.Unmarshal(restartedReleaseResponse.Body, &restartedRelease); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restartedRelease, publishedRelease) {
			t.Fatalf("Release changed after restart %d: got %#v want %#v", restart, restartedRelease, publishedRelease)
		}
		restartedReplayResponse := assertIntegrationStatus(
			t, client, http.MethodPost, baseURL+releaseCollectionPath,
			testIntegrationAdminToken, releaseBody, "", http.StatusOK,
			http.Header{"Idempotency-Key": []string{"server-e2e-publish"}},
		)
		var restartedReplay integrationRelease
		if err := json.Unmarshal(restartedReplayResponse.Body, &restartedReplay); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restartedReplay, publishedRelease) ||
			restartedReplayResponse.Header.Get("Location") != releasePath {
			t.Fatalf("Release replay changed after restart %d: got %#v want %#v", restart, restartedReplay, publishedRelease)
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
			if draftAfterRestart.DraftVersion != 4 || draftAfterRestart.SourceHash != staleDraft.SourceHash {
				t.Fatalf("Draft after restart = %#v, want %#v", draftAfterRestart, staleDraft)
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
			t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 2,
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

	// Once an active pointer exists, a changed local Source may register another
	// immutable bootstrap Revision but cannot replace the authoritative active
	// Release, Runtime Revision, epoch, or retained Record namespace.
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
		t, client, changedBaseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 3,
	)
	if integrationDataSchemaFingerprint(changedRegistry.Metadata, 1) != integrationDataSchemaFingerprint(initialRegistry.Metadata, 1) {
		t.Fatal("semantic version-only revision changed DataSchemaFingerprint")
	}
	if changedApplication.revision != publishedRelease.CandidateRevision || changedApplication.epoch != 2 {
		t.Fatalf("changed local Source replaced active Runtime: revision=%q epoch=%d", changedApplication.revision, changedApplication.epoch)
	}
	changedItemPath := changedBaseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	assertIntegrationStatus(
		t, client, http.MethodGet, changedItemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodPost,
		changedBaseURL+"/api/admin/v1alpha1/crm.leads/organization",
		testIntegrationAdminToken, `{"name":"Analytical Engines"}`, "", http.StatusConflict, nil,
	)
	changedLeadBody := fmt.Sprintf(
		`{"organization":%q,"email":"ada@example.com","stage":"new"}`,
		organization.ID,
	)
	assertIntegrationStatus(
		t, client, http.MethodPost,
		changedBaseURL+"/api/public/v1alpha1/crm.leads/lead",
		"", changedLeadBody, "", http.StatusConflict, nil,
	)
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
		t, client, baseURL, testIntegrationAdminToken, "crm.leads", runtimeRevision, 3,
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

	rotationPrincipal := createIntegrationServicePrincipal(
		t, client, baseURL, testIntegrationAdminToken, "Bootstrap replacement worker",
	)
	rotationCredential := issueIntegrationCredential(
		t, client, baseURL, testIntegrationAdminToken, rotationPrincipal.ID, "replacement owner key",
	)
	mutateIntegrationOwnerGrant(
		t, client, http.MethodPut, baseURL, testIntegrationAdminToken, rotationPrincipal.ID,
	)
	accessPrincipalsPath := baseURL + "/api/admin/core/v1alpha1/access/principals"
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	bootstrapCredentialsResponse := assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/access/principals/bootstrap-admin/credentials",
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	var bootstrapCredentials integrationCredentialList
	if err := json.Unmarshal(bootstrapCredentialsResponse.Body, &bootstrapCredentials); err != nil {
		t.Fatal(err)
	}
	if len(bootstrapCredentials.Data) != 1 || bootstrapCredentials.Data[0].Status != "active" {
		t.Fatalf("bootstrap credentials before rotation = %#v", bootstrapCredentials)
	}
	bootstrapCredentialID := bootstrapCredentials.Data[0].ID
	bootstrapRevoke := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/core/v1alpha1/access/credentials/"+bootstrapCredentialID+"/revoke",
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	var revokedBootstrap integrationAccessCredential
	if err := json.Unmarshal(bootstrapRevoke.Body, &revokedBootstrap); err != nil {
		t.Fatal(err)
	}
	if revokedBootstrap.Status != "revoked" {
		t.Fatalf("revoked bootstrap credential = %#v", revokedBootstrap)
	}
	bootstrapUnauthorized := assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		testIntegrationAdminToken, "", "", http.StatusUnauthorized, nil,
	)
	if bytes.Contains(bootstrapUnauthorized.Body, []byte(testIntegrationAdminToken)) {
		t.Fatalf("bootstrap authentication error leaked token: %s", bootstrapUnauthorized.Body)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	lastPathResponses := []integrationResponse{
		assertIntegrationStatus(
			t, client, http.MethodPost,
			baseURL+"/api/admin/core/v1alpha1/access/credentials/"+
				rotationCredential.Credential.ID+"/revoke",
			rotationCredential.Token, "", "", http.StatusConflict, nil,
		),
		assertIntegrationStatus(
			t, client, http.MethodDelete,
			baseURL+"/api/admin/core/v1alpha1/access/principals/"+
				url.PathEscape(rotationPrincipal.ID)+"/grants/project.owner",
			rotationCredential.Token, "", "", http.StatusConflict, nil,
		),
		assertIntegrationStatus(
			t, client, http.MethodPost,
			baseURL+"/api/admin/core/v1alpha1/access/principals/"+
				url.PathEscape(rotationPrincipal.ID)+"/disable",
			rotationCredential.Token, "", "", http.StatusConflict, nil,
		),
	}
	for _, response := range lastPathResponses {
		if bytes.Contains(response.Body, []byte(rotationCredential.Token)) ||
			bytes.Contains(response.Body, []byte(testIntegrationAdminToken)) {
			t.Fatalf("last-owner-path response leaked token: %s", response.Body)
		}
	}

	securityPool, err := panvarapg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL for access security assertions: %v", err)
	}
	var storedDigest []byte
	if err := securityPool.QueryRow(ctx, `
		SELECT secret_digest
		FROM panvara_api_credential
		WHERE project_id = $1 AND credential_id = $2
	`, config.projectID, rotationCredential.Credential.ID).Scan(&storedDigest); err != nil {
		securityPool.Close()
		t.Fatalf("read persisted credential digest: %v", err)
	}
	wantDigest := sha256.Sum256([]byte(rotationCredential.Token))
	if !bytes.Equal(storedDigest, wantDigest[:]) || bytes.Contains(storedDigest, []byte(rotationCredential.Token)) {
		securityPool.Close()
		t.Fatal("persisted credential is not the expected irreversible SHA-256 digest")
	}
	digestBearer := "pvk1." + rotationCredential.Credential.ID + "." +
		base64.RawURLEncoding.EncodeToString(storedDigest)
	digestRejected := assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		digestBearer, "", "", http.StatusUnauthorized, nil,
	)
	if bytes.Contains(digestRejected.Body, storedDigest) || bytes.Contains(digestRejected.Body, []byte(digestBearer)) {
		securityPool.Close()
		t.Fatalf("digest authentication error leaked credential material: %s", digestRejected.Body)
	}
	var leakedFacts int
	if err := securityPool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM panvara_api_credential
		   WHERE label = $1 OR secret_hint = $1 OR principal_id = $1) +
		  (SELECT count(*) FROM panvara_access_bootstrap_marker
		   WHERE secret_hint = $1 OR principal_id = $1) +
		  (SELECT count(*) FROM panvara_security_audit_event
		   WHERE request_id = $1 OR action = $1 OR target_kind = $1
		      OR target_id = $1 OR reason_code = $1)
	`, rotationCredential.Token).Scan(&leakedFacts); err != nil {
		securityPool.Close()
		t.Fatalf("scan access facts for raw token: %v", err)
	}
	if leakedFacts != 0 {
		securityPool.Close()
		t.Fatalf("raw issued token leaked into %d persisted access facts", leakedFacts)
	}
	var deniedAuditCount int
	if err := securityPool.QueryRow(ctx, `
		SELECT count(*)
		FROM panvara_security_audit_event
		WHERE project_id = $1 AND actor_principal_id = $2
		  AND action = 'access.principal.list'
		  AND outcome = 'denied' AND reason_code = 'forbidden'
	`, config.projectID, servicePrincipal.ID).Scan(&deniedAuditCount); err != nil {
		securityPool.Close()
		t.Fatalf("read denied access audit: %v", err)
	}
	if deniedAuditCount < 1 {
		securityPool.Close()
		t.Fatal("authenticated denied access administration attempt was not audited")
	}
	if _, err := securityPool.Exec(ctx, `
		UPDATE panvara_project
		SET status = 'disabled', updated_at = clock_timestamp()
		WHERE project_id = $1
	`, config.projectID); err != nil {
		securityPool.Close()
		t.Fatalf("disable project for inactive-scope HTTP assertion: %v", err)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusForbidden, nil,
	)
	var inactiveScopeAuditCount int
	if err := securityPool.QueryRow(ctx, `
		SELECT count(*)
		FROM panvara_security_audit_event
		WHERE project_id = $1 AND actor_principal_id = $2
		  AND action = 'access.principal.list'
		  AND outcome = 'denied' AND reason_code = 'scope_inactive'
	`, config.projectID, rotationPrincipal.ID).Scan(&inactiveScopeAuditCount); err != nil {
		securityPool.Close()
		t.Fatalf("read inactive-scope access audit: %v", err)
	}
	if inactiveScopeAuditCount < 1 {
		securityPool.Close()
		t.Fatal("inactive-scope access denial was not audited")
	}
	if _, err := securityPool.Exec(ctx, `
		UPDATE panvara_project
		SET status = 'active', updated_at = clock_timestamp()
		WHERE project_id = $1
	`, config.projectID); err != nil {
		securityPool.Close()
		t.Fatalf("restore project after inactive-scope HTTP assertion: %v", err)
	}
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	securityPool.Close()
	stopIntegrationServer(t, server, application)

	restartWithoutToken := config
	restartWithoutToken.adminToken = ""
	application, server, baseURL = startIntegrationServer(t, ctx, restartWithoutToken)
	accessPrincipalsPath = baseURL + "/api/admin/core/v1alpha1/access/principals"
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		testIntegrationAdminToken, "", "", http.StatusUnauthorized, nil,
	)
	bootstrapCredentialsResponse = assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/admin/core/v1alpha1/access/principals/bootstrap-admin/credentials",
		rotationCredential.Token, "", "", http.StatusOK, nil,
	)
	bootstrapCredentials = integrationCredentialList{}
	if err := json.Unmarshal(bootstrapCredentialsResponse.Body, &bootstrapCredentials); err != nil {
		t.Fatal(err)
	}
	if len(bootstrapCredentials.Data) != 1 || bootstrapCredentials.Data[0].Status != "revoked" ||
		bootstrapCredentials.Data[0].ID != bootstrapCredentialID {
		t.Fatalf("bootstrap credential resurrected after restart = %#v", bootstrapCredentials)
	}
	application.Close()
	assertIntegrationStatus(
		t, client, http.MethodGet, accessPrincipalsPath,
		rotationCredential.Token, "", "", http.StatusServiceUnavailable, nil,
	)
	stopIntegrationServer(t, server, application)

	driftedCredentialConfig := config
	driftedCredentialConfig.adminToken = "changed-bootstrap-token-must-not-replace-existing"
	driftedApplication, driftedError := buildServerApplication(ctx, driftedCredentialConfig)
	if driftedApplication != nil {
		driftedApplication.Close()
	}
	if driftedError == nil || !strings.Contains(driftedError.Error(), "conflict") ||
		strings.Contains(driftedError.Error(), driftedCredentialConfig.adminToken) ||
		strings.Contains(driftedError.Error(), testIntegrationAdminToken) {
		t.Fatalf("changed bootstrap credential error = %v", driftedError)
	}

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

const testIntegrationAdminToken = "panvara-integration-admin-token-32-bytes"

func createIntegrationServicePrincipal(
	t *testing.T,
	client *http.Client,
	baseURL string,
	ownerToken string,
	displayName string,
) integrationAccessPrincipal {
	t.Helper()
	response := assertIntegrationStatus(
		t, client, http.MethodPost,
		baseURL+"/api/admin/core/v1alpha1/access/principals",
		ownerToken, fmt.Sprintf(`{"display_name":%q}`, displayName), "",
		http.StatusCreated, nil,
	)
	if response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("create Principal Cache-Control = %q", response.Header.Get("Cache-Control"))
	}
	var principal integrationAccessPrincipal
	if err := json.Unmarshal(response.Body, &principal); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(principal.ID, "svc:") || principal.Status != "active" {
		t.Fatalf("created service principal = %#v", principal)
	}
	return principal
}

func issueIntegrationCredential(
	t *testing.T,
	client *http.Client,
	baseURL string,
	ownerToken string,
	principalID string,
	label string,
) integrationIssuedCredential {
	t.Helper()
	path := baseURL + "/api/admin/core/v1alpha1/access/principals/" +
		url.PathEscape(principalID) + "/credentials"
	response := assertIntegrationStatus(
		t, client, http.MethodPost, path, ownerToken,
		fmt.Sprintf(`{"label":%q}`, label), "", http.StatusCreated, nil,
	)
	if response.Header.Get("Cache-Control") != "private, no-store" ||
		response.Header.Get("Pragma") != "no-cache" {
		t.Fatalf("issue Credential cache headers = %#v", response.Header)
	}
	var issued integrationIssuedCredential
	if err := json.Unmarshal(response.Body, &issued); err != nil {
		t.Fatal(err)
	}
	if issued.Credential.PrincipalID != principalID || issued.Credential.Status != "active" ||
		issued.Credential.Hint == "" || !strings.HasPrefix(issued.Token, "pvk1.") ||
		bytes.Count(response.Body, []byte(issued.Token)) != 1 {
		t.Fatalf("issued service credential = %#v", issued)
	}
	listed := assertIntegrationStatus(
		t, client, http.MethodGet, path, ownerToken, "", "", http.StatusOK, nil,
	)
	if bytes.Contains(listed.Body, []byte(issued.Token)) || bytes.Contains(listed.Body, []byte(`"token"`)) {
		t.Fatalf("credential list leaked one-time token: %s", listed.Body)
	}
	return issued
}

func mutateIntegrationOwnerGrant(
	t *testing.T,
	client *http.Client,
	method string,
	baseURL string,
	ownerToken string,
	principalID string,
) integrationOwnerGrant {
	t.Helper()
	response := assertIntegrationStatus(
		t, client, method,
		baseURL+"/api/admin/core/v1alpha1/access/principals/"+
			url.PathEscape(principalID)+"/grants/project.owner",
		ownerToken, "", "", http.StatusOK, nil,
	)
	var grant integrationOwnerGrant
	if err := json.Unmarshal(response.Body, &grant); err != nil {
		t.Fatal(err)
	}
	if grant.PrincipalID != principalID || grant.Role != "project.owner" {
		t.Fatalf("owner grant response = %#v", grant)
	}
	return grant
}

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
