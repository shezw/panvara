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

func TestServerProfileHTTPPersistenceLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	databaseURL := isolatedHTTPIntegrationDatabaseURL(t, ctx)
	config := serverConfig{
		databaseURL: databaseURL, moduleSource: "testdata/crm-leads.yaml", moduleFormat: "yaml",
		projectID: "01981234-5678-7abc-8def-0123456789ab", projectKey: "crm",
		projectLocale: "en-US", projectZone: "UTC", projectMoney: "USD",
		adminToken: testIntegrationAdminToken,
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
	assertIntegrationStatus(
		t, client, http.MethodGet,
		baseURL+"/api/core/v1alpha1/modules/crm.leads/ui-schema.json",
		"", "", "", http.StatusOK, nil,
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

	stopIntegrationServer(t, server, application)
	application, server, baseURL = startIntegrationServer(t, ctx, config)
	itemPath = baseURL + "/api/admin/v1alpha1/crm.leads/lead/" + lead.ID
	restarted := assertIntegrationStatus(
		t, client, http.MethodGet, itemPath, testIntegrationAdminToken, "", "", http.StatusOK, nil,
	)
	if got := decodeIntegrationRecord(t, restarted.Body).Data["stage"]; got != "qualified" {
		t.Fatalf("stage after restart = %#v", got)
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
	if body != "" {
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
