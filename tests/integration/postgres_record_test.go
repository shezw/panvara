//go:build integration

/*
   Panvara
   tests/integration/postgres_record_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/project"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

const postgresImage = "postgres:18.4-alpine"

func TestPostgresFlexRecordLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	databaseURL := integrationDatabaseURL(t, ctx)
	pool := isolatedPool(t, ctx, databaseURL)

	var version string
	if err := pool.QueryRow(ctx, `SHOW server_version`).Scan(&version); err != nil {
		t.Fatalf("SHOW server_version error = %v", err)
	}
	if !strings.HasPrefix(version, "18.4") {
		t.Fatalf("PostgreSQL version = %q, want 18.4.x", version)
	}
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() first error = %v", err)
	}
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() idempotent error = %v", err)
	}
	store, err := panvarapg.NewStore(pool)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	projectOne := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	projectTwo := mustProjectID(t, "01981234-5678-7abc-8def-0123456789b0")
	userScope := mustScope(t, projectOne, "user")
	leadScope := mustScope(t, projectOne, "lead")
	userRevisionB := mustRevisionScope(t, projectOne, "user", "b")
	leadRevisionB := mustRevisionScope(t, projectOne, "lead", "b")
	otherProjectLeadScope := mustScope(t, projectTwo, "lead")
	userID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	leadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ad")
	secondLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ae")
	otherProjectLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789af")
	thirdLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789b1")
	crossRevisionUserID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789b2")
	crossRevisionLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789b3")
	at := time.Date(2026, time.July, 14, 10, 0, 0, 0, time.UTC)

	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: userScope, ID: userID, Data: json.RawMessage(`{"name":"Owner"}`), At: at,
	}); err != nil {
		t.Fatalf("Create(user) error = %v", err)
	}
	if _, err := store.Get(ctx, userScope, userID); err != nil {
		t.Fatalf("Get(user before reference) error = %v", err)
	}
	created, err := store.Create(ctx, record.CreateCommand{
		Scope: leadScope,
		ID:    leadID,
		Data: json.RawMessage(
			`{"name":"Ada","amount":12345678901234567890.123400,"owner_id":"01981234-5678-7abc-8def-0123456789ac"}`,
		),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		References: []record.Reference{{
			Field: "owner_id", TargetResource: "user", TargetID: userID,
		}},
		At: at.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Create(lead) error = %v", err)
	}
	if created.Version != 1 {
		t.Fatalf("Create(lead).Version = %d, want 1", created.Version)
	}
	assertJSONNumber(t, created.Data, "amount", "12345678901234567890.123400")

	read, err := store.Get(ctx, leadScope, leadID)
	if err != nil {
		t.Fatalf("Get(lead) error = %v", err)
	}
	assertJSONNumber(t, read.Data, "amount", "12345678901234567890.123400")

	// Revision is part of the complete persistence identity. A newly compiled
	// module may reuse both a record ID and a unique value without seeing or
	// mutating the prior revision's data.
	revisionBCreated, err := store.Create(ctx, record.CreateCommand{
		Scope: leadRevisionB, ID: leadID, Data: json.RawMessage(`{"name":"Revision B"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		At:      at.Add(1500 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("Create(same identity and unique in revision B) error = %v", err)
	}
	if revisionBCreated.Version != 1 {
		t.Fatalf("Create(revision B).Version = %d, want 1", revisionBCreated.Version)
	}
	revisionAList, err := store.List(ctx, leadScope, record.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List(revision A) error = %v", err)
	}
	revisionBList, err := store.List(ctx, leadRevisionB, record.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List(revision B) error = %v", err)
	}
	if len(revisionAList.Records) != 1 || len(revisionBList.Records) != 1 ||
		revisionAList.Records[0].Scope.RevisionHash == revisionBList.Records[0].Scope.RevisionHash {
		t.Fatalf("revision-scoped lists = A:%#v B:%#v", revisionAList, revisionBList)
	}
	revisionBUpdated, err := store.Update(ctx, record.UpdateCommand{
		Scope: leadRevisionB, ID: leadID, ExpectedVersion: 1,
		Data:    json.RawMessage(`{"name":"Revision B updated"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		At:      at.Add(1600 * time.Millisecond),
	})
	if err != nil || revisionBUpdated.Version != 2 {
		t.Fatalf("Update(revision B) = %#v, %v", revisionBUpdated, err)
	}
	unchangedRevisionA, err := store.Get(ctx, leadScope, leadID)
	if err != nil || unchangedRevisionA.Version != 1 {
		t.Fatalf("Get(revision A after revision B update) = %#v, %v", unchangedRevisionA, err)
	}
	if _, err := store.Delete(ctx, record.DeleteCommand{
		Scope: leadRevisionB, ID: leadID, ExpectedVersion: 2, At: at.Add(1700 * time.Millisecond),
	}); err != nil {
		t.Fatalf("Delete(revision B) error = %v", err)
	}
	if _, err := store.Get(ctx, leadRevisionB, leadID); !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("Get(deleted revision B) error = %v, want ErrNotFound", err)
	}
	if _, err := store.Get(ctx, leadScope, leadID); err != nil {
		t.Fatalf("Get(revision A after revision B delete) error = %v", err)
	}

	// A reference must resolve inside the same revision, even when the same
	// project/module/resource/ID tuple exists in another revision.
	_, err = store.Create(ctx, record.CreateCommand{
		Scope: leadRevisionB, ID: crossRevisionLeadID, Data: json.RawMessage(`{"name":"Invalid cross revision"}`),
		References: []record.Reference{{
			Field: "owner_id", TargetResource: "user", TargetID: userID,
		}},
		At: at.Add(1800 * time.Millisecond),
	})
	if !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("Create(cross-revision reference) error = %v, want ErrNotFound", err)
	}

	// An inbound relation in revision B must not block deletion of the same
	// target identity in revision A.
	for _, command := range []record.CreateCommand{
		{Scope: userScope, ID: crossRevisionUserID, Data: json.RawMessage(`{"name":"Revision A auxiliary"}`), At: at.Add(1810 * time.Millisecond)},
		{Scope: userRevisionB, ID: crossRevisionUserID, Data: json.RawMessage(`{"name":"Revision B auxiliary"}`), At: at.Add(1820 * time.Millisecond)},
	} {
		if _, err := store.Create(ctx, command); err != nil {
			t.Fatalf("Create(cross-revision auxiliary user) error = %v", err)
		}
	}
	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: leadRevisionB, ID: crossRevisionLeadID, Data: json.RawMessage(`{"name":"Revision B relation"}`),
		References: []record.Reference{{
			Field: "owner_id", TargetResource: "user", TargetID: crossRevisionUserID,
		}},
		At: at.Add(1830 * time.Millisecond),
	}); err != nil {
		t.Fatalf("Create(revision B reference) error = %v", err)
	}
	if _, err := store.Delete(ctx, record.DeleteCommand{
		Scope: userScope, ID: crossRevisionUserID, ExpectedVersion: 1, At: at.Add(1840 * time.Millisecond),
	}); err != nil {
		t.Fatalf("Delete(revision A target with revision B inbound relation) error = %v", err)
	}
	if _, err := store.Delete(ctx, record.DeleteCommand{
		Scope: userRevisionB, ID: crossRevisionUserID, ExpectedVersion: 1, At: at.Add(1850 * time.Millisecond),
	}); !errors.Is(err, record.ErrReferenced) {
		t.Fatalf("Delete(revision B referenced target) error = %v, want ErrReferenced", err)
	}

	_, err = store.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: secondLeadID, Data: json.RawMessage(`{"name":"Duplicate"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		At:      at.Add(2 * time.Second),
	})
	if !errors.Is(err, record.ErrUniqueConflict) {
		t.Fatalf("Create(duplicate unique) error = %v, want ErrUniqueConflict", err)
	}

	_, err = store.Create(ctx, record.CreateCommand{
		Scope: otherProjectLeadScope,
		ID:    otherProjectLeadID,
		Data:  json.RawMessage(`{"name":"Cross-boundary"}`),
		References: []record.Reference{{
			Field: "owner_id", TargetResource: "user", TargetID: userID,
		}},
		At: at.Add(2 * time.Second),
	})
	if !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("Create(cross-project reference) error = %v, want ErrNotFound", err)
	}

	_, err = store.Update(ctx, record.UpdateCommand{
		Scope: leadScope, ID: leadID, ExpectedVersion: 9,
		Data: json.RawMessage(`{"name":"Wrong version"}`), At: at.Add(3 * time.Second),
	})
	if !errors.Is(err, record.ErrVersionConflict) {
		t.Fatalf("Update(stale) error = %v, want ErrVersionConflict", err)
	}
	updated, err := store.Update(ctx, record.UpdateCommand{
		Scope: leadScope, ID: leadID, ExpectedVersion: 1,
		Data:    json.RawMessage(`{"name":"Ada Lovelace"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:lovelace@example.com"}},
		References: []record.Reference{{
			Field: "owner_id", TargetResource: "user", TargetID: userID,
		}},
		At: at.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatalf("Update(lead) error = %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("Update(lead).Version = %d, want 2", updated.Version)
	}
	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: secondLeadID, Data: json.RawMessage(`{"name":"Second"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:second@example.com"}},
		At:      at.Add(3500 * time.Millisecond),
	}); err != nil {
		t.Fatalf("Create(second lead) error = %v", err)
	}

	page, err := store.List(ctx, leadScope, record.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("List(lead) error = %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].ID != leadID || page.Next == nil {
		t.Fatalf("List(lead) = %#v", page)
	}
	nextPage, err := store.List(ctx, leadScope, record.ListOptions{Limit: 1, Cursor: page.Next})
	if err != nil {
		t.Fatalf("List(lead next page) error = %v", err)
	}
	if len(nextPage.Records) != 1 || nextPage.Records[0].ID != secondLeadID || nextPage.Next != nil {
		t.Fatalf("List(lead next page) = %#v", nextPage)
	}
	filteredPage, err := store.List(ctx, leadScope, record.ListOptions{
		Limit:   10,
		Filters: []record.ListFilter{{Field: "name", Value: "Second"}},
	})
	if err != nil {
		t.Fatalf("List(filtered lead) error = %v", err)
	}
	if len(filteredPage.Records) != 1 || filteredPage.Records[0].ID != secondLeadID {
		t.Fatalf("List(filtered lead) = %#v", filteredPage)
	}

	_, err = store.Delete(ctx, record.DeleteCommand{
		Scope: userScope, ID: userID, ExpectedVersion: 1, At: at.Add(4 * time.Second),
	})
	if !errors.Is(err, record.ErrReferenced) {
		t.Fatalf("Delete(referenced user) error = %v, want ErrReferenced", err)
	}
	deleted, err := store.Delete(ctx, record.DeleteCommand{
		Scope: leadScope, ID: leadID, ExpectedVersion: 2, At: at.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatalf("Delete(lead) error = %v", err)
	}
	if deleted.Version != 3 || deleted.DeletedAt == nil {
		t.Fatalf("Delete(lead) = %#v", deleted)
	}
	if _, err := store.Get(ctx, leadScope, leadID); !errors.Is(err, record.ErrNotFound) {
		t.Fatalf("Get(deleted lead) error = %v, want ErrNotFound", err)
	}

	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: thirdLeadID, Data: json.RawMessage(`{"name":"Replacement"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:lovelace@example.com"}},
		At:      at.Add(5 * time.Second),
	}); err != nil {
		t.Fatalf("Create(released unique) error = %v", err)
	}
	if _, err := store.Delete(ctx, record.DeleteCommand{
		Scope: userScope, ID: userID, ExpectedVersion: 1, At: at.Add(5 * time.Second),
	}); err != nil {
		t.Fatalf("Delete(unreferenced user) error = %v", err)
	}
}

func integrationDatabaseURL(t *testing.T, ctx context.Context) string {
	t.Helper()
	if databaseURL := strings.TrimSpace(os.Getenv("PANVARA_TEST_DATABASE_URL")); databaseURL != "" {
		return databaseURL
	}
	requireDocker := os.Getenv("PANVARA_REQUIRE_DOCKER") == "1"
	if _, err := exec.LookPath("docker"); err != nil {
		dockerUnavailable(t, requireDocker, "docker executable is unavailable", err)
	}
	infoCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(infoCtx, "docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput(); err != nil {
		dockerUnavailable(t, requireDocker, "Docker daemon is unavailable: "+strings.TrimSpace(string(output)), err)
	}

	containerName := fmt.Sprintf("panvara-pg18-%d-%d", os.Getpid(), time.Now().UnixNano())
	runCtx, cancelRun := context.WithTimeout(ctx, 3*time.Minute)
	defer cancelRun()
	output, err := exec.CommandContext(
		runCtx,
		"docker", "run", "--detach", "--rm", "--name", containerName,
		"--env", "POSTGRES_PASSWORD=panvara_test",
		"--env", "POSTGRES_DB=panvara_test",
		"--publish", "127.0.0.1::5432",
		postgresImage,
	).CombinedOutput()
	if err != nil {
		dockerUnavailable(t, requireDocker, "start PostgreSQL 18.4 container: "+strings.TrimSpace(string(output)), err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = exec.CommandContext(cleanupCtx, "docker", "rm", "--force", containerName).Run()
	})

	var address string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		portCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		output, err := exec.CommandContext(portCtx, "docker", "port", containerName, "5432/tcp").Output()
		cancel()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			address = strings.Split(strings.TrimSpace(string(output)), "\n")[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if address == "" {
		t.Fatal("Docker did not publish the PostgreSQL test port")
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("parse Docker PostgreSQL address %q: %v", address, err)
	}
	databaseURL := fmt.Sprintf(
		"postgres://postgres:panvara_test@127.0.0.1:%s/panvara_test?sslmode=disable",
		port,
	)

	readyDeadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(readyDeadline) {
		pool, err := panvarapg.Open(ctx, databaseURL)
		if err == nil {
			pool.Close()
			return databaseURL
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("PostgreSQL 18.4 container did not become ready within 60 seconds")
	return ""
}

func isolatedPool(t *testing.T, ctx context.Context, databaseURL string) *pgxpool.Pool {
	t.Helper()
	admin, err := panvarapg.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open(admin PostgreSQL) error = %v", err)
	}
	schema := fmt.Sprintf("panvara_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatalf("CREATE SCHEMA error = %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		admin.Close()
		t.Fatalf("ParseConfig(test PostgreSQL) error = %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("NewWithConfig(test PostgreSQL) error = %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("Ping(test PostgreSQL) error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanupCtx, "DROP SCHEMA IF EXISTS "+identifier+" CASCADE")
		admin.Close()
	})
	return pool
}

func dockerUnavailable(t *testing.T, required bool, message string, err error) {
	t.Helper()
	if required {
		t.Fatalf("%s: %v", message, err)
	}
	t.Skipf("%s; set PANVARA_REQUIRE_DOCKER=1 to make this fatal: %v", message, err)
}

func mustProjectID(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.ParseID(value)
	if err != nil {
		t.Fatalf("ParseID(%q) error = %v", value, err)
	}
	return id
}

func mustScope(t *testing.T, projectID project.ID, resource string) record.Scope {
	t.Helper()
	return mustRevisionScope(t, projectID, resource, "a")
}

func mustRevisionScope(t *testing.T, projectID project.ID, resource, revisionDigit string) record.Scope {
	t.Helper()
	if len(revisionDigit) != 1 {
		t.Fatalf("revision digit = %q, want one character", revisionDigit)
	}
	scope, err := record.NewScope(
		projectID,
		"crm.leads",
		resource,
		"sha256:"+strings.Repeat(revisionDigit, 64),
	)
	if err != nil {
		t.Fatalf("NewScope(%q) error = %v", resource, err)
	}
	return scope
}

func mustRecordID(t *testing.T, value string) record.ID {
	t.Helper()
	id, err := record.ParseID(value)
	if err != nil {
		t.Fatalf("ParseID(%q) error = %v", value, err)
	}
	return id
}

func assertJSONNumber(t *testing.T, source json.RawMessage, field, want string) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		t.Fatalf("Decode(record JSON) error = %v", err)
	}
	number, ok := object[field].(json.Number)
	if !ok {
		t.Fatalf("record JSON field %q = %#v, want json.Number", field, object[field])
	}
	if number.String() != want {
		t.Fatalf("record JSON field %q = %q, want %q", field, number.String(), want)
	}
}
