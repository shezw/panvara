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
	"bytes"
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
	"github.com/shezw/panvara/internal/application/access"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
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
	checksums["0003_module_draft_workflow.sql"] = embeddedMigrationChecksum(t, "0003_module_draft_workflow.sql")
	checksums["0004_project_environment_access.sql"] = embeddedMigrationChecksum(t, "0004_project_environment_access.sql")
	checksums["0005_project_access_administration.sql"] = embeddedMigrationChecksum(t, "0005_project_access_administration.sql")
	checksums["0006_module_publish_facts.sql"] = embeddedMigrationChecksum(t, "0006_module_publish_facts.sql")
	checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
		t, "0007_compatible_release_activation.sql",
	)
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

func TestPostgresMigrateUpgrades0002RegistryWithoutChangingFacts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	checksums := applyMigrationsThrough0002(t, ctx, pool)

	store, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registeredAt := time.Date(2026, 7, 16, 6, 0, 0, 0, time.UTC)
	registry, err := application.NewRevisionRegistry(
		store, allowAuthorizer{}, integrationDraftClock{at: registeredAt},
	)
	if err != nil {
		t.Fatal(err)
	}
	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	execution := integrationAdminExecution(
		t, projectID, integrationEnvironmentA, "migration-owner",
	)
	source := integrationDraftSource("1.0.0")
	compiled, err := application.NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	before, created, err := registry.RegisterBootstrap(ctx, projectID, compiled, source, spec.FormatYAML)
	if err != nil || !created {
		t.Fatalf("RegisterBootstrap(0002) = created %v error %v", created, err)
	}
	if _, err := registry.Get(ctx, execution, "notes", before.RevisionHash()); err != nil {
		t.Fatalf("Get(0002 Registry) error = %v", err)
	}
	recordStore, err := panvarapg.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	organizationScope := mustScope(t, projectID, "organization")
	leadScope := mustScope(t, projectID, "lead")
	organizationID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ac")
	leadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ad")
	secondLeadID := mustRecordID(t, "01981234-5678-7abc-8def-0123456789ae")
	if _, err := recordStore.Create(ctx, record.CreateCommand{
		Scope: organizationScope, ID: organizationID, Data: json.RawMessage(`{"name":"Analytical Engines"}`),
		Uniques: []record.UniqueValue{{Field: "name", CanonicalValue: "string:Analytical Engines"}}, At: registeredAt,
	}); err != nil {
		t.Fatalf("create 0002 organization: %v", err)
	}
	if _, err := recordStore.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: leadID, Data: json.RawMessage(`{"email":"ada@example.com","stage":"new"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		References: []record.Reference{{
			Field: "organization", TargetResource: "organization", TargetID: organizationID,
		}},
		At: registeredAt.Add(time.Second),
	}); err != nil {
		t.Fatalf("create 0002 lead: %v", err)
	}
	assertFlexRowCounts(t, ctx, pool, 2, 2, 1)

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(0002 -> current) error = %v", err)
	}
	checksums["0003_module_draft_workflow.sql"] = embeddedMigrationChecksum(t, "0003_module_draft_workflow.sql")
	checksums["0004_project_environment_access.sql"] = embeddedMigrationChecksum(t, "0004_project_environment_access.sql")
	checksums["0005_project_access_administration.sql"] = embeddedMigrationChecksum(t, "0005_project_access_administration.sql")
	checksums["0006_module_publish_facts.sql"] = embeddedMigrationChecksum(t, "0006_module_publish_facts.sql")
	checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
		t, "0007_compatible_release_activation.sql",
	)
	assertMigrationLedger(t, ctx, pool, checksums)
	assertFlexRowCounts(t, ctx, pool, 2, 2, 1)

	restartedStore, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	restartedRegistry, err := application.NewRevisionRegistry(
		restartedStore, allowAuthorizer{}, integrationDraftClock{at: registeredAt.Add(time.Hour)},
	)
	if err != nil {
		t.Fatal(err)
	}
	after, err := restartedRegistry.Get(ctx, execution, "notes", before.RevisionHash())
	if err != nil {
		t.Fatalf("Get(upgraded Registry) error = %v", err)
	}
	if !after.SameArtifacts(before) || after.SourceHash() != before.SourceHash() ||
		!bytes.Equal(after.Source(), before.Source()) || after.RegisteredAt() != before.RegisteredAt() ||
		after.RegisteredBy() != before.RegisteredBy() || after.Origin() != before.Origin() {
		t.Fatalf("Registry fact changed across 0002 -> current: before=%#v after=%#v", before, after)
	}
	values, err := restartedRegistry.List(ctx, execution, "notes", 100)
	if err != nil || len(values) != 1 || values[0].RevisionHash() != before.RevisionHash() {
		t.Fatalf("List(upgraded Registry) = %#v, %v", values, err)
	}
	lead, err := recordStore.Get(ctx, leadScope, leadID)
	if err != nil || lead.Version != 1 {
		t.Fatalf("Get(upgraded 0002 lead) = %#v, %v", lead, err)
	}
	updated, err := recordStore.Update(ctx, record.UpdateCommand{
		Scope: leadScope, ID: leadID, ExpectedVersion: 1,
		Data:    json.RawMessage(`{"email":"ada@example.com","stage":"qualified"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		References: []record.Reference{{
			Field: "organization", TargetResource: "organization", TargetID: organizationID,
		}},
		At: registeredAt.Add(2 * time.Second),
	})
	if err != nil || updated.Version != 2 {
		t.Fatalf("Update(upgraded 0002 lead) = %#v, %v", updated, err)
	}
	if _, err := recordStore.Create(ctx, record.CreateCommand{
		Scope: leadScope, ID: secondLeadID, Data: json.RawMessage(`{"email":"ada@example.com"}`),
		Uniques: []record.UniqueValue{{Field: "email", CanonicalValue: "email:ada@example.com"}},
		At:      registeredAt.Add(3 * time.Second),
	}); !errors.Is(err, record.ErrUniqueConflict) {
		t.Fatalf("duplicate unique after 0002 upgrade error = %v", err)
	}
	if _, err := recordStore.Delete(ctx, record.DeleteCommand{
		Scope: organizationScope, ID: organizationID, ExpectedVersion: 1, At: registeredAt.Add(4 * time.Second),
	}); !errors.Is(err, record.ErrReferenced) {
		t.Fatalf("delete referenced organization after 0002 upgrade error = %v", err)
	}
}

func TestPostgresMigrateUpgrades0003DraftFactsIntoPersistentAccessScope(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	checksums := applyMigrationsThrough0003(t, ctx, pool)

	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	legacyExecution := integrationAdminExecution(
		t, projectID, integrationEnvironmentA, "legacy-owner",
	)
	clock := integrationDraftClock{at: time.Date(2026, 7, 18, 2, 0, 0, 0, time.UTC)}
	revisionStore, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := application.NewRevisionRegistry(revisionStore, allowAuthorizer{}, clock)
	if err != nil {
		t.Fatal(err)
	}
	source := integrationDraftSource("1.0.0")
	compiled, err := application.NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	baseline, created, err := registry.RegisterBootstrap(
		ctx, projectID, compiled, source, spec.FormatYAML,
	)
	if err != nil || !created {
		t.Fatalf("RegisterBootstrap(0003 baseline) = created %t error %v", created, err)
	}
	draftStore, err := panvarapg.NewDraftStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.NewDraftWorkflow(
		draftStore, registry, allowAuthorizer{}, clock, &integrationDraftIDGenerator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	draft, created, err := workflow.Create(ctx, legacyExecution, "notes", application.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(),
		Format:           domain.SourceFormatYAML,
		Source:           source,
		IdempotencyKey:   "upgrade-0003-draft",
	})
	if err != nil || !created {
		t.Fatalf("Create(0003 Draft) = created %t error %v", created, err)
	}
	validation, created, err := workflow.Validate(
		ctx, legacyExecution, "notes", draft.ID().String(), draft.Generation(),
	)
	if err != nil || !created || !validation.Valid {
		t.Fatalf("Validate(0003 Draft) = %#v created %t error %v", validation, created, err)
	}
	plan, created, err := workflow.Plan(
		ctx, legacyExecution, "notes", draft.ID().String(), validation.ID, draft.Generation(),
	)
	if err != nil || !created {
		t.Fatalf("Plan(0003 Draft) = %#v created %t error %v", plan, created, err)
	}

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(0003 -> current) error = %v", err)
	}
	checksums["0004_project_environment_access.sql"] = embeddedMigrationChecksum(
		t, "0004_project_environment_access.sql",
	)
	checksums["0005_project_access_administration.sql"] = embeddedMigrationChecksum(
		t, "0005_project_access_administration.sql",
	)
	checksums["0006_module_publish_facts.sql"] = embeddedMigrationChecksum(
		t, "0006_module_publish_facts.sql",
	)
	checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
		t, "0007_compatible_release_activation.sql",
	)
	assertMigrationLedger(t, ctx, pool, checksums)

	definition, err := project.NewContext(
		projectID.String(), "upgrade-0003", "en-US", "UTC", "USD",
	)
	if err != nil {
		t.Fatal(err)
	}
	projectAccess, err := panvarapg.NewProjectAccessStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := projectAccess.EnsureBootstrapScope(
		ctx, definition, "default", "bootstrap-admin",
	)
	if err != nil {
		t.Fatalf("EnsureBootstrapScope(upgraded 0003) error = %v", err)
	}
	accessAdminStore, err := panvarapg.NewAccessAdminStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapRegistrar, err := access.NewDefaultBootstrapCredentialRegistrar(accessAdminStore)
	if err != nil {
		t.Fatal(err)
	}
	const upgradedBootstrapToken = "upgrade-0003-bootstrap-token-32-bytes"
	if _, err := bootstrapRegistrar.Register(
		ctx, scope, "bootstrap-admin", upgradedBootstrapToken,
	); err != nil {
		t.Fatalf("Register(upgraded 0003 bootstrap credential) error = %v", err)
	}
	authenticator, err := access.NewCredentialAuthenticator(accessAdminStore)
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := authenticator.Authenticate(ctx, scope, upgradedBootstrapToken)
	if err != nil {
		t.Fatalf("Authenticate(upgraded 0003 bootstrap credential) error = %v", err)
	}
	authorizer, err := access.NewPolicy(projectAccess)
	if err != nil {
		t.Fatal(err)
	}
	upgradedExecution, err := access.NewAdminExecution(scope, authenticated)
	if err != nil {
		t.Fatal(err)
	}
	restartedRegistry, err := application.NewRevisionRegistry(revisionStore, authorizer, clock)
	if err != nil {
		t.Fatal(err)
	}
	restartedWorkflow, err := application.NewDraftWorkflow(
		draftStore, restartedRegistry, authorizer, clock, &integrationDraftIDGenerator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	gotDraft, err := restartedWorkflow.Get(
		ctx, upgradedExecution, "notes", draft.ID().String(),
	)
	if err != nil || gotDraft.Generation() != draft.Generation() || gotDraft.SourceHash() != draft.SourceHash() {
		t.Fatalf("Get(upgraded 0003 Draft) = %#v error %v", gotDraft, err)
	}
	gotValidation, err := restartedWorkflow.GetValidation(
		ctx, upgradedExecution, "notes", draft.ID().String(), validation.ID,
	)
	if err != nil || gotValidation.ID != validation.ID || gotValidation.Candidate == nil ||
		validation.Candidate == nil || gotValidation.Candidate.RevisionHash != validation.Candidate.RevisionHash {
		t.Fatalf("GetValidation(upgraded 0003) = %#v error %v", gotValidation, err)
	}
	gotPlan, err := restartedWorkflow.GetPlan(
		ctx, upgradedExecution, "notes", draft.ID().String(), plan.ID,
	)
	if err != nil || gotPlan.ID != plan.ID || gotPlan.PlanHash != plan.PlanHash {
		t.Fatalf("GetPlan(upgraded 0003) = %#v error %v", gotPlan, err)
	}
	for table, want := range map[string]int{
		"panvara_module_draft":            1,
		"panvara_module_draft_validation": 1,
		"panvara_module_draft_plan":       1,
	} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatalf("count upgraded %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("upgraded %s rows = %d, want %d", table, count, want)
		}
	}
}

func TestPostgresMigrateUpgrades0004AccessFactsWithoutRestoringAuthority(t *testing.T) {
	tests := []struct {
		name            string
		principalStatus string
		grantState      string
		wantAuthError   error
		wantPolicyError error
	}{
		{name: "active owner", principalStatus: "active", grantState: "active"},
		{name: "revoked owner", principalStatus: "active", grantState: "revoked", wantPolicyError: access.ErrForbidden},
		{name: "missing owner", principalStatus: "active", grantState: "missing", wantPolicyError: access.ErrForbidden},
		{name: "disabled principal", principalStatus: "disabled", grantState: "active", wantAuthError: access.ErrUnauthenticated},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
			checksums := applyMigrationsThrough0004(t, ctx, pool)

			projectID := "01981234-5678-7abc-8def-0123456789ab"
			environmentID := integrationEnvironmentA
			createdAt := time.Date(2026, time.July, 18, 1, 0, 0, 0, time.UTC)
			updatedAt := createdAt
			if test.principalStatus == "disabled" {
				updatedAt = createdAt.Add(time.Hour)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO panvara_project (
					project_id, project_key, default_locale, default_time_zone,
					default_currency, created_at, updated_at
				) VALUES ($1, 'upgrade-0004', 'en-US', 'UTC', 'USD', $2, $2)
			`, projectID, createdAt); err != nil {
				t.Fatalf("insert 0004 project fact: %v", err)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO panvara_environment (
					project_id, environment_id, environment_key, is_default,
					created_at, updated_at
				) VALUES ($1, $2, 'default', true, $3, $3)
			`, projectID, environmentID, createdAt); err != nil {
				t.Fatalf("insert 0004 environment fact: %v", err)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO panvara_principal (
					project_id, principal_id, status, created_at, updated_at
				) VALUES ($1, 'bootstrap-admin', $2, $3, $4)
			`, projectID, test.principalStatus, createdAt, updatedAt); err != nil {
				t.Fatalf("insert 0004 principal fact: %v", err)
			}
			if _, err := pool.Exec(ctx, `
				INSERT INTO panvara_principal (
					project_id, principal_id, status, created_at, updated_at
				) VALUES ($1, 'svc:legacy', 'active', $2, $2)
			`, projectID, createdAt); err != nil {
				t.Fatalf("insert 0004 legacy principal fact: %v", err)
			}
			var legacyRevokedAt *time.Time
			if test.grantState == "revoked" {
				value := createdAt.Add(2 * time.Hour)
				legacyRevokedAt = &value
			}
			if test.grantState != "missing" {
				if _, err := pool.Exec(ctx, `
					INSERT INTO panvara_access_grant (
						project_id, environment_id, principal_id, role,
						granted_at, revoked_at
					) VALUES ($1, $2, 'bootstrap-admin', 'project.owner', $3, $4)
				`, projectID, environmentID, createdAt, legacyRevokedAt); err != nil {
					t.Fatalf("insert 0004 owner grant: %v", err)
				}
			}

			if err := panvarapg.Migrate(ctx, pool); err != nil {
				t.Fatalf("Migrate(0004 -> current) error = %v", err)
			}
			checksums["0005_project_access_administration.sql"] = embeddedMigrationChecksum(
				t, "0005_project_access_administration.sql",
			)
			checksums["0006_module_publish_facts.sql"] = embeddedMigrationChecksum(
				t, "0006_module_publish_facts.sql",
			)
			checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
				t, "0007_compatible_release_activation.sql",
			)
			assertMigrationLedger(t, ctx, pool, checksums)

			var kind, displayName, status string
			var disabledAt *time.Time
			if err := pool.QueryRow(ctx, `
				SELECT kind, display_name, status, disabled_at
				FROM panvara_principal
				WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
			`, projectID).Scan(&kind, &displayName, &status, &disabledAt); err != nil {
				t.Fatalf("read upgraded 0004 principal: %v", err)
			}
			if kind != "bootstrap" || displayName != "bootstrap-admin" || status != test.principalStatus {
				t.Fatalf("upgraded principal = kind %q display %q status %q", kind, displayName, status)
			}
			if (test.principalStatus == "disabled") != (disabledAt != nil) {
				t.Fatalf("upgraded principal disabled_at = %v for status %q", disabledAt, test.principalStatus)
			}
			var legacyKind string
			if err := pool.QueryRow(ctx, `
				SELECT kind FROM panvara_principal
				WHERE project_id = $1 AND principal_id = 'svc:legacy'
			`, projectID).Scan(&legacyKind); err != nil {
				t.Fatalf("read upgraded 0004 legacy principal: %v", err)
			}
			if legacyKind != "bootstrap" {
				t.Fatalf("upgraded legacy principal kind = %q, want bootstrap", legacyKind)
			}

			var grantCount int
			if err := pool.QueryRow(ctx, `
				SELECT count(*)
				FROM panvara_access_grant
				WHERE project_id = $1 AND environment_id = $2
				  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
			`, projectID, environmentID).Scan(&grantCount); err != nil {
				t.Fatal(err)
			}
			wantGrantCount := 1
			if test.grantState == "missing" {
				wantGrantCount = 0
			}
			if grantCount != wantGrantCount {
				t.Fatalf("upgraded owner grant count = %d, want %d", grantCount, wantGrantCount)
			}
			if grantCount == 1 {
				var grantedBy string
				var revokedBy *string
				var revokedAt *time.Time
				var grantUpdatedAt time.Time
				if err := pool.QueryRow(ctx, `
					SELECT granted_by_principal_id, revoked_by_principal_id,
					       revoked_at, updated_at
					FROM panvara_access_grant
					WHERE project_id = $1 AND environment_id = $2
					  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
				`, projectID, environmentID).Scan(
					&grantedBy, &revokedBy, &revokedAt, &grantUpdatedAt,
				); err != nil {
					t.Fatal(err)
				}
				if grantedBy != "bootstrap-admin" || (test.grantState == "revoked") != (revokedAt != nil) ||
					(test.grantState == "revoked") != (revokedBy != nil) || grantUpdatedAt.Before(createdAt) {
					t.Fatalf("upgraded owner grant = granted_by %q revoked_by %v revoked_at %v updated_at %v",
						grantedBy, revokedBy, revokedAt, grantUpdatedAt,
					)
				}
			}

			definition, err := project.NewContext(projectID, "upgrade-0004", "en-US", "UTC", "USD")
			if err != nil {
				t.Fatal(err)
			}
			projectAccess, err := panvarapg.NewProjectAccessStore(pool)
			if err != nil {
				t.Fatal(err)
			}
			scope, err := projectAccess.EnsureBootstrapScope(
				ctx, definition, "default", "bootstrap-admin",
			)
			if err != nil {
				t.Fatalf("EnsureBootstrapScope(upgraded 0004) error = %v", err)
			}
			if err := pool.QueryRow(ctx, `
				SELECT count(*) FROM panvara_access_grant
				WHERE project_id = $1 AND environment_id = $2
				  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
			`, projectID, environmentID).Scan(&grantCount); err != nil {
				t.Fatal(err)
			}
			if grantCount != wantGrantCount {
				t.Fatalf("EnsureBootstrapScope restored owner grant count to %d, want %d", grantCount, wantGrantCount)
			}

			accessStore, err := panvarapg.NewAccessAdminStore(pool)
			if err != nil {
				t.Fatal(err)
			}
			registrar, err := access.NewDefaultBootstrapCredentialRegistrar(accessStore)
			if err != nil {
				t.Fatal(err)
			}
			const token = "upgrade-0004-bootstrap-token-at-least-32-bytes"
			credential, err := registrar.Register(ctx, scope, "bootstrap-admin", token)
			if err != nil {
				t.Fatalf("Register(upgraded 0004 bootstrap) error = %v", err)
			}
			restartedCredential, err := registrar.Register(ctx, scope, "bootstrap-admin", "")
			if err != nil || restartedCredential.ID() != credential.ID() {
				t.Fatalf("Register(upgraded 0004 restart) = %s, %v, want %s",
					restartedCredential.ID().String(), err, credential.ID().String(),
				)
			}
			if _, err := registrar.Register(
				ctx, scope, "bootstrap-admin", "changed-upgrade-bootstrap-token-at-least-32-bytes",
			); !errors.Is(err, access.ErrConflict) {
				t.Fatalf("Register(changed upgraded 0004 token) error = %v, want conflict", err)
			}
			for table, want := range map[string]int{
				"panvara_api_credential":          1,
				"panvara_access_bootstrap_marker": 1,
				"panvara_security_audit_event":    1,
			} {
				var count int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
					t.Fatalf("count upgraded %s: %v", table, err)
				}
				if count != want {
					t.Fatalf("upgraded %s rows = %d, want %d", table, count, want)
				}
			}

			authenticator, err := access.NewCredentialAuthenticator(accessStore)
			if err != nil {
				t.Fatal(err)
			}
			authenticated, authErr := authenticator.Authenticate(ctx, scope, token)
			if test.wantAuthError != nil {
				if !errors.Is(authErr, test.wantAuthError) {
					t.Fatalf("Authenticate(upgraded 0004) error = %v, want %v", authErr, test.wantAuthError)
				}
				return
			}
			if authErr != nil {
				t.Fatalf("Authenticate(upgraded 0004) error = %v", authErr)
			}
			execution, err := access.NewAdminExecution(scope, authenticated)
			if err != nil {
				t.Fatal(err)
			}
			policy, err := access.NewPolicy(projectAccess)
			if err != nil {
				t.Fatal(err)
			}
			policyErr := policy.Authorize(ctx, execution, access.OperationPrincipalList)
			if !errors.Is(policyErr, test.wantPolicyError) ||
				(test.wantPolicyError == nil && policyErr != nil) {
				t.Fatalf("Authorize(upgraded 0004) error = %v, want %v", policyErr, test.wantPolicyError)
			}
		})
	}
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

func applyMigrationsThrough0002(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	checksums := applyOnlyMigration0001(t, ctx, pool)
	script, err := fs.ReadFile(migrations.Files(), "0002_module_revision_registry.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0002_module_revision_registry.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	checksums["0002_module_revision_registry.sql"] = checksum
	return checksums
}

func applyMigrationsThrough0003(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	checksums := applyMigrationsThrough0002(t, ctx, pool)
	script, err := fs.ReadFile(migrations.Files(), "0003_module_draft_workflow.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0003_module_draft_workflow.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	checksums["0003_module_draft_workflow.sql"] = checksum
	return checksums
}

func applyMigrationsThrough0004(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	checksums := applyMigrationsThrough0003(t, ctx, pool)
	script, err := fs.ReadFile(migrations.Files(), "0004_project_environment_access.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0004_project_environment_access.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	checksums["0004_project_environment_access.sql"] = checksum
	return checksums
}

func applyMigrationsThrough0005(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	checksums := applyMigrationsThrough0004(t, ctx, pool)
	script, err := fs.ReadFile(migrations.Files(), "0005_project_access_administration.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0005_project_access_administration.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	checksums["0005_project_access_administration.sql"] = checksum
	return checksums
}

func applyMigrationsThrough0006(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) map[string]string {
	t.Helper()
	checksums := applyMigrationsThrough0005(t, ctx, pool)
	script, err := fs.ReadFile(migrations.Files(), "0006_module_publish_facts.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	checksum := migrationChecksum(script)
	if _, err := pool.Exec(ctx,
		`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
		"0006_module_publish_facts.sql", checksum,
	); err != nil {
		t.Fatal(err)
	}
	checksums["0006_module_publish_facts.sql"] = checksum
	return checksums
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
