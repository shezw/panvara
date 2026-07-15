//go:build integration

/*
   Panvara
   tests/integration/postgres_migration_upgrade_test.go    2026-07-15
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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shezw/panvara/db/migrations"
	"github.com/shezw/panvara/internal/application/record"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

func TestPostgresMigrateUpgrades0001OnlyDatabaseWithoutChangingFlexData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	checksums := applyOnlyMigration0001(t, ctx, pool)

	store, err := panvarapg.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	organizationScope := mustScope(t, projectID, "organization")
	leadScope := mustScope(t, projectID, "lead")
	organizationID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	leadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ad")
	secondLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ae")
	at := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)

	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: organizationScope, ID: organizationID, Data: json.RawMessage(`{"name":"Analytical Engines"}`),
		Uniques: []record.UniqueValue{{Field: "name", CanonicalValue: "string:Analytical Engines"}}, At: at,
	}); err != nil {
		t.Fatalf("create 0001 organization: %v", err)
	}
	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: leadID,
		Data:    json.RawMessage(`{"email":"ada@example.com","stage":"new"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		References: []record.Reference{{
			Field: "organization", TargetResource: "organization", TargetID: organizationID,
		}},
		At: at.Add(time.Second),
	}); err != nil {
		t.Fatalf("create 0001 lead: %v", err)
	}
	assertFlexRowCounts(t, ctx, pool, 2, 2, 1)

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(0001-only -> current) error = %v", err)
	}
	checksums["0002_module_revision_registry.sql"] = embeddedMigrationChecksum(t, "0002_module_revision_registry.sql")
	assertMigrationLedger(t, ctx, pool, checksums)
	assertFlexRowCounts(t, ctx, pool, 2, 2, 1)

	organization, err := store.Get(ctx, organizationScope, organizationID)
	var organizationData map[string]string
	if err == nil {
		err = json.Unmarshal(organization.Data, &organizationData)
	}
	if err != nil || organizationData["name"] != "Analytical Engines" {
		t.Fatalf("get upgraded organization = %s, %v", organization.Data, err)
	}
	lead, err := store.Get(ctx, leadScope, leadID)
	if err != nil || lead.Version != 1 {
		t.Fatalf("get upgraded lead = %#v, %v", lead, err)
	}
	updated, err := store.Update(ctx, record.UpdateCommand{
		Scope: leadScope, ID: leadID, ExpectedVersion: 1,
		Data:    json.RawMessage(`{"email":"ada@example.com","stage":"qualified"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		References: []record.Reference{{
			Field: "organization", TargetResource: "organization", TargetID: organizationID,
		}},
		At: at.Add(2 * time.Second),
	})
	if err != nil || updated.Version != 2 {
		t.Fatalf("update upgraded lead = %#v, %v", updated, err)
	}
	if _, err := store.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: secondLeadID, Data: json.RawMessage(`{"email":"ada@example.com"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		At:      at.Add(3 * time.Second),
	}); !errors.Is(err, record.ErrUniqueConflict) {
		t.Fatalf("duplicate unique after upgrade error = %v, want ErrUniqueConflict", err)
	}
	if _, err := store.Delete(ctx, record.DeleteCommand{
		Scope: organizationScope, ID: organizationID, ExpectedVersion: 1, At: at.Add(4 * time.Second),
	}); !errors.Is(err, record.ErrReferenced) {
		t.Fatalf("delete referenced organization after upgrade error = %v, want ErrReferenced", err)
	}
	page, err := store.List(ctx, leadScope, record.ListOptions{Limit: 10})
	if err != nil || len(page.Records) != 1 || page.Records[0].Version != 2 {
		t.Fatalf("list upgraded leads = %#v, %v", page, err)
	}

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(repeated current) error = %v", err)
	}
	assertMigrationLedger(t, ctx, pool, checksums)
	assertFlexRowCounts(t, ctx, pool, 2, 2, 1)
}

func applyOnlyMigration0001(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE panvara_schema_migration (
			version text PRIMARY KEY,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		t.Fatal(err)
	}
	script, err := fs.ReadFile(migrations.Files(), "0001_flex_record.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0001_flex_record.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"0001_flex_record.sql": checksum}
}

func embeddedMigrationChecksum(t *testing.T, name string) string {
	t.Helper()
	script, err := fs.ReadFile(migrations.Files(), name)
	if err != nil {
		t.Fatal(err)
	}
	return migrationChecksum(script)
}

func migrationChecksum(script []byte) string {
	digest := sha256.Sum256(script)
	return hex.EncodeToString(digest[:])
}

func assertMigrationLedger(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	want map[string]string,
) {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT version, checksum FROM panvara_schema_migration ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := make(map[string]string)
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			t.Fatal(err)
		}
		got[version] = checksum
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("migration ledger = %#v, want %#v", got, want)
	}
	for version, checksum := range want {
		if got[version] != checksum {
			t.Fatalf("migration ledger checksum %s = %q, want %q", version, got[version], checksum)
		}
	}
}

func assertFlexRowCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	wantRecords int,
	wantUniques int,
	wantReferences int,
) {
	t.Helper()
	for table, want := range map[string]int{
		"flex_record": wantRecords, "flex_unique": wantUniques, "flex_reference": wantReferences,
	} {
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s row count = %d, want %d", table, got, want)
		}
	}
}
