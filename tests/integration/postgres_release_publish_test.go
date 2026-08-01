//go:build integration

/*
   Panvara
   tests/integration/postgres_release_publish_test.go    2026-07-19
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
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accessapp "github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestPostgresReleasePublishFactsIdempotencyAndIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, baseline)

	first, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-main",
	})
	if err != nil || !created {
		t.Fatalf("Publish(first) = %#v, %t, %v", first, created, err)
	}
	replayed, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-main",
	})
	if err != nil || created || replayed.ID() != first.ID() {
		t.Fatalf("Publish(same key replay) = %#v, %t, %v", replayed, created, err)
	}
	alias, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-alias",
	})
	if err != nil || created || alias.ID() != first.ID() {
		t.Fatalf("Publish(same plan alias) = %#v, %t, %v", alias, created, err)
	}
	assertReleaseAuditFactsForKeys(
		t, ctx, pool, fixture, first, []string{"publish-main", "publish-alias"},
	)

	const writers = 8
	type publishResult struct {
		value   domainrelease.ModuleRelease
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan publishResult, writers)
	var group sync.WaitGroup
	for index := range writers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			value, created, err := fixture.publisher.Publish(
				ctx, fixture.invocation, fixture.module,
				releaseapp.PublishInput{
					PlanID: fixture.plan.ID, IdempotencyKey: fmt.Sprintf("publish-concurrent-%d", index),
				},
			)
			results <- publishResult{value: value, created: created, err: err}
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	for result := range results {
		if result.err != nil || result.created || result.value.ID() != first.ID() {
			t.Fatalf("concurrent Publish() = %#v, %t, %v", result.value, result.created, result.err)
		}
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 1, 10)

	var origin, registeredBy string
	if err := pool.QueryRow(ctx, `
		SELECT origin, registered_by FROM panvara_module_revision
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, fixture.scope.ProjectID().String(), fixture.module, first.CandidateRevision()).Scan(&origin, &registeredBy); err != nil {
		t.Fatal(err)
	}
	if origin != "bootstrap" || registeredBy != "system:bootstrap" {
		t.Fatalf("existing revision provenance = %s/%s, want bootstrap/system:bootstrap", origin, registeredBy)
	}

	oldPlan := fixture.plan
	newSource := releaseExampleSource(t, "drafts/crm-leads-valid.yaml")
	fixture.replaceAndPlan(t, ctx, newSource)
	replayed, created, err = fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: oldPlan.ID, IdempotencyKey: "publish-main",
	})
	if err != nil || created || replayed.ID() != first.ID() {
		t.Fatalf("Publish(stale plan original key) = %#v, %t, %v", replayed, created, err)
	}
	staleAlias, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: oldPlan.ID, IdempotencyKey: "publish-stale-plan",
	})
	if err != nil || created || staleAlias.ID() != first.ID() {
		t.Fatalf("Publish(stale plan alias) = %#v, %t, %v", staleAlias, created, err)
	}
	assertReleaseKeyBinding(t, ctx, pool, fixture, "publish-stale-plan", first)
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 1, 11)
	if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-main",
	}); !errors.Is(err, releaseapp.ErrIdempotencyConflict) {
		t.Fatalf("Publish(key with another plan) error = %v, want ErrIdempotencyConflict", err)
	}
	missingPlan := "sha256:" + strings.Repeat("f", 64)
	if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: missingPlan, IdempotencyKey: "publish-main",
	}); !errors.Is(err, releaseapp.ErrIdempotencyConflict) {
		t.Fatalf("Publish(bound key with missing plan) error = %v, want ErrIdempotencyConflict", err)
	}
	assertRevisionCount(t, ctx, pool, fixture.scope.ProjectID(), fixture.module, fixture.plan.Candidate.RevisionHash, 0)

	second, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-second",
	})
	if err != nil || !created || second.ID() == first.ID() {
		t.Fatalf("Publish(second plan) = %#v, %t, %v", second, created, err)
	}
	got, err := fixture.publisher.Get(ctx, fixture.invocation, fixture.module, second.ID().String())
	if err != nil || got.ID() != second.ID() || !got.SamePublishIntent(second) {
		t.Fatalf("Get(second release) = %#v, %v", got, err)
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 2, 12)

	unsupportedSource := []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: crm.leads\n  version: 3.0.0\nspec:\n  resources: []\n")
	fixture.replaceAndPlan(t, ctx, unsupportedSource)
	if fixture.plan.Outcome != "unsupported" {
		t.Fatalf("replacement plan outcome = %q, want unsupported", fixture.plan.Outcome)
	}
	unsupportedPlanID := fixture.plan.ID
	if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: unsupportedPlanID, IdempotencyKey: "publish-main",
	}); !errors.Is(err, releaseapp.ErrIdempotencyConflict) {
		t.Fatalf("Publish(bound key with unsupported plan) error = %v, want ErrIdempotencyConflict", err)
	}
	fixture.replaceAndPlan(t, ctx, newSource)
	if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: unsupportedPlanID, IdempotencyKey: "publish-main",
	}); !errors.Is(err, releaseapp.ErrIdempotencyConflict) {
		t.Fatalf("Publish(bound key with stale unsupported plan) error = %v, want ErrIdempotencyConflict", err)
	}

	otherEnvironment := mustEnvironmentID(t, integrationEnvironmentB)
	wrongEnvironment, err := project.NewScope(fixture.scope.ProjectID(), otherEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.releaseStore.Get(ctx, wrongEnvironment, fixture.module, second.ID()); !errors.Is(err, releaseapp.ErrNotFound) {
		t.Fatalf("Get(cross environment) error = %v, want ErrNotFound", err)
	}
	otherProject := mustProjectID(t, "019f5c36-b322-7c52-9325-ec59f95c8ff0")
	wrongProject, err := project.NewScope(otherProject, otherEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.releaseStore.Get(ctx, wrongProject, fixture.module, second.ID()); !errors.Is(err, releaseapp.ErrNotFound) {
		t.Fatalf("Get(cross project) error = %v, want ErrNotFound", err)
	}
	assertMismatchedPlanRejected(t, ctx, fixture, second)
	assertCrossScopePublishRejected(t, ctx, fixture, second, wrongEnvironment, wrongProject)

	assertReleaseFactsAppendOnly(t, ctx, pool, fixture, second)
}

func TestPostgresReleaseConcurrentFirstPublishConvergesAfterReplayMiss(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, baseline)
	const writers = 8
	barrier := newReleaseStoreReplayMissBarrier(fixture.releaseStore, writers)
	publisher, err := releaseapp.NewDefaultPublisher(
		fixture.releaseStore, barrier, fixture.policy, fixture.accessStore,
	)
	if err != nil {
		t.Fatal(err)
	}

	type publishResult struct {
		value   domainrelease.ModuleRelease
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan publishResult, writers)
	var group sync.WaitGroup
	for index := range writers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			value, created, err := publisher.Publish(
				ctx, fixture.invocation, fixture.module,
				releaseapp.PublishInput{
					PlanID: fixture.plan.ID, IdempotencyKey: fmt.Sprintf("publish-first-race-%d", index),
				},
			)
			results <- publishResult{value: value, created: created, err: err}
		}(index)
	}
	close(start)
	group.Wait()
	close(results)
	var releaseID domainrelease.ID
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent first Publish() error = %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if !releaseID.Valid() {
			releaseID = result.value.ID()
		}
		if result.value.ID() != releaseID {
			t.Fatalf("concurrent release id = %s, want %s", result.value.ID(), releaseID)
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent created count = %d, want 1", createdCount)
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 1, writers)
	assertReleaseSuccessAuditCount(t, ctx, pool, fixture.scope, writers)
}

func TestPostgresReleasePublishReauthorizesAndRechecksDraftInTransaction(t *testing.T) {
	tests := []struct {
		name      string
		before    func(context.Context, *postgresReleaseFixture) error
		wantError error
	}{
		{
			name: "owner grant revoked after application authorization",
			before: func(ctx context.Context, fixture *postgresReleaseFixture) error {
				_, err := fixture.pool.Exec(ctx, `
					UPDATE panvara_access_grant
					SET revoked_by_principal_id = $3, revoked_at = clock_timestamp(), updated_at = clock_timestamp()
					WHERE project_id = $1 AND environment_id = $2
					  AND principal_id = $3 AND role = 'project.owner' AND revoked_at IS NULL
				`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
					fixture.invocation.Execution().Actor().ActorID())
				return err
			},
			wantError: accessapp.ErrForbidden,
		},
		{
			name: "draft changes after application snapshot read",
			before: func(ctx context.Context, fixture *postgresReleaseFixture) error {
				source := strings.ReplaceAll(string(fixture.candidateSource), "version: 1.1.0", "version: 1.2.0")
				_, _, err := fixture.workflow.Replace(
					ctx, fixture.invocation.Execution(), fixture.module, fixture.draft.ID().String(),
					fixture.draft.Generation(), moduleapp.ReplaceDraftInput{
						Format: domainappmodule.SourceFormatYAML, Source: []byte(source),
					},
				)
				return err
			},
			wantError: releaseapp.ErrStale,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
			if err := panvarapg.Migrate(ctx, pool); err != nil {
				t.Fatal(err)
			}
			baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
			candidate := releaseExampleSource(t, "drafts/crm-leads-valid.yaml")
			fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, candidate)
			hooked := &releaseStoreHook{
				inner: fixture.releaseStore,
				publishBefore: func(ctx context.Context) error {
					return test.before(ctx, fixture)
				},
			}
			publisher, err := releaseapp.NewDefaultPublisher(
				fixture.releaseStore, hooked, fixture.policy, fixture.accessStore,
			)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
				PlanID: fixture.plan.ID, IdempotencyKey: "publish-transaction-recheck",
			})
			if !errors.Is(err, test.wantError) {
				t.Fatalf("Publish() error = %v, want %v", err, test.wantError)
			}
			assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 0, 0)
			assertRevisionCount(t, ctx, pool, fixture.scope.ProjectID(), fixture.module, fixture.plan.Candidate.RevisionHash, 0)
			assertReleaseSuccessAuditCount(t, ctx, pool, fixture.scope, 0)
		})
	}
}

func TestPostgresReleaseReplayReauthorizesBeforeReturningPublishedFact(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, baseline)
	first, created, err := fixture.publisher.Publish(
		ctx, fixture.invocation, fixture.module,
		releaseapp.PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "publish-before-revoke"},
	)
	if err != nil || !created {
		t.Fatalf("Publish(first) = %#v, %t, %v", first, created, err)
	}

	hooked := &releaseStoreHook{
		inner: fixture.releaseStore,
		resolveBefore: func(ctx context.Context) error {
			_, err := fixture.pool.Exec(ctx, `
				UPDATE panvara_access_grant
				SET revoked_by_principal_id = $3, revoked_at = clock_timestamp(), updated_at = clock_timestamp()
				WHERE project_id = $1 AND environment_id = $2
				  AND principal_id = $3 AND role = 'project.owner' AND revoked_at IS NULL
			`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
				fixture.invocation.Execution().Actor().ActorID())
			return err
		},
	}
	publisher, err := releaseapp.NewDefaultPublisher(
		fixture.releaseStore, hooked, fixture.policy, fixture.accessStore,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := publisher.Publish(
		ctx, fixture.invocation, fixture.module,
		releaseapp.PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "publish-replay-revoked"},
	); !errors.Is(err, accessapp.ErrForbidden) {
		t.Fatalf("Publish(replay after revoke) error = %v, want ErrForbidden", err)
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 1, 1)
	assertReleaseSuccessAuditCount(t, ctx, pool, fixture.scope, 1)
	var aliasCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_module_release_idempotency
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND idempotency_key = 'publish-replay-revoked'
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
		fixture.module).Scan(&aliasCount); err != nil {
		t.Fatal(err)
	}
	if aliasCount != 0 {
		t.Fatalf("revoked replay alias count = %d, want 0", aliasCount)
	}
	var actor, credential, requestID, action, outcome, targetKind, targetID, reason string
	var occurredAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT actor_principal_id, actor_credential_id::text, request_id,
		       action, outcome, target_kind, target_id, reason_code, occurred_at
		FROM panvara_security_audit_event
		WHERE project_id = $1 AND environment_id = $2
		  AND action = 'release.publish' AND outcome = 'denied'
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String()).Scan(
		&actor, &credential, &requestID, &action, &outcome, &targetKind, &targetID, &reason, &occurredAt,
	); err != nil {
		t.Fatal(err)
	}
	if actor != first.PublishedBy() || credential != first.PublishedCredentialID().String() ||
		requestID != fixture.invocation.RequestID() || action != string(accessapp.OperationReleasePublish) ||
		outcome != "denied" || targetKind != "authorization" || targetID != action ||
		reason != "forbidden" || occurredAt.IsZero() {
		t.Fatalf(
			"revoked replay audit = %q/%q/%q/%q/%q/%q/%q/%q/%s",
			actor, credential, requestID, action, outcome, targetKind, targetID, reason, occurredAt,
		)
	}
}

func TestPostgresReleasePublishRollsBackEveryWritePoint(t *testing.T) {
	for _, table := range []string{
		"panvara_module_revision",
		"panvara_module_release",
		"panvara_module_release_idempotency",
		"panvara_security_audit_event",
	} {
		t.Run(table, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
			if err := panvarapg.Migrate(ctx, pool); err != nil {
				t.Fatal(err)
			}
			fixture := newPostgresReleaseFixture(
				t, ctx, pool,
				releaseExampleSource(t, "modules/crm-leads.yaml"),
				releaseExampleSource(t, "drafts/crm-leads-valid.yaml"),
			)
			installReleaseInsertFailure(t, ctx, pool, table)
			if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
				PlanID: fixture.plan.ID, IdempotencyKey: "publish-forced-failure",
			}); err == nil {
				t.Fatal("Publish(forced failure) error = nil")
			}
			assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 0, 0)
			assertRevisionCount(t, ctx, pool, fixture.scope.ProjectID(), fixture.module, fixture.plan.Candidate.RevisionHash, 0)
			assertReleaseSuccessAuditCount(t, ctx, pool, fixture.scope, 0)
		})
	}
}

func TestPostgresReleaseReplayAliasRollsBackIdempotencyAndAuditTogether(t *testing.T) {
	for _, table := range []string{
		"panvara_module_release_idempotency",
		"panvara_security_audit_event",
	} {
		t.Run(table, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
			if err := panvarapg.Migrate(ctx, pool); err != nil {
				t.Fatal(err)
			}
			baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
			fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, baseline)
			first, created, err := fixture.publisher.Publish(
				ctx, fixture.invocation, fixture.module,
				releaseapp.PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "publish-before-alias-failure"},
			)
			if err != nil || !created {
				t.Fatalf("Publish(first) = %#v, %t, %v", first, created, err)
			}
			installReleaseInsertFailure(t, ctx, pool, table)
			if _, _, err := fixture.publisher.Publish(
				ctx, fixture.invocation, fixture.module,
				releaseapp.PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "publish-alias-failure"},
			); err == nil {
				t.Fatal("Publish(alias forced failure) error = nil")
			}
			assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 1, 1)
			assertReleaseSuccessAuditCount(t, ctx, pool, fixture.scope, 1)
		})
	}
}

func TestPostgresReleasePublishRejectsUnsupportedPlan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture := newPostgresReleaseFixture(
		t, ctx, pool,
		releaseExampleSource(t, "modules/crm-leads.yaml"),
		[]byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: crm.leads\n  version: 2.0.0\nspec:\n  resources: []\n"),
	)
	if fixture.plan.Outcome != "unsupported" {
		t.Fatalf("destructive plan outcome = %q, want unsupported", fixture.plan.Outcome)
	}
	if _, _, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-unsupported",
	}); !errors.Is(err, releaseapp.ErrNotPublishable) {
		t.Fatalf("Publish(unsupported) error = %v, want ErrNotPublishable", err)
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 0, 0)
	assertRevisionCount(t, ctx, pool, fixture.scope.ProjectID(), fixture.module, fixture.plan.Candidate.RevisionHash, 0)
}

func TestPostgresMigrateUpgrades0005FactsForModulePublish(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	checksums := applyMigrationsThrough0005(t, ctx, pool)
	baseline := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baseline, baseline)

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(0005 -> current) error = %v", err)
	}
	checksums["0006_module_publish_facts.sql"] = embeddedMigrationChecksum(t, "0006_module_publish_facts.sql")
	checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
		t, "0007_compatible_release_activation.sql",
	)
	assertMigrationLedger(t, ctx, pool, checksums)
	for table, want := range map[string]int{
		"panvara_module_revision":         1,
		"panvara_module_draft":            1,
		"panvara_module_draft_validation": 1,
		"panvara_module_draft_plan":       1,
	} {
		var got int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("upgraded %s count = %d, want %d", table, got, want)
		}
	}
	var baselineIdentity string
	if err := pool.QueryRow(ctx, `
		SELECT publish_baseline_revision_identity FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = $2 AND plan_id = $3
	`, fixture.scope.ProjectID().String(), fixture.module, fixture.plan.ID).Scan(&baselineIdentity); err != nil {
		t.Fatal(err)
	}
	if baselineIdentity != fixture.baseline.RevisionHash() {
		t.Fatalf("upgraded plan baseline identity = %q, want %q", baselineIdentity, fixture.baseline.RevisionHash())
	}
	value, created, err := fixture.publisher.Publish(ctx, fixture.invocation, fixture.module, releaseapp.PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-after-0005-upgrade",
	})
	if err != nil || !created {
		t.Fatalf("Publish(after 0005 upgrade) = %#v, %t, %v", value, created, err)
	}
	var origin, registeredBy string
	if err := pool.QueryRow(ctx, `
		SELECT origin, registered_by FROM panvara_module_revision
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, fixture.scope.ProjectID().String(), fixture.module, value.CandidateRevision()).Scan(&origin, &registeredBy); err != nil {
		t.Fatal(err)
	}
	if origin != "bootstrap" || registeredBy != "system:bootstrap" {
		t.Fatalf("upgraded bootstrap revision provenance = %s/%s", origin, registeredBy)
	}
}

type postgresReleaseFixture struct {
	pool            *pgxpool.Pool
	module          string
	scope           project.Scope
	invocation      accessapp.Invocation
	policy          *accessapp.Policy
	accessStore     *panvarapg.AccessAdminStore
	releaseStore    *panvarapg.ReleaseStore
	draftStore      *panvarapg.DraftStore
	workflow        *moduleapp.DraftWorkflow
	publisher       *releaseapp.Publisher
	baseline        domainappmodule.Revision
	draft           domainappmodule.Draft
	validation      moduleapp.DraftValidation
	plan            moduleapp.DraftPlan
	candidateSource []byte
}

func newPostgresReleaseFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	baselineSource []byte,
	candidateSource []byte,
) *postgresReleaseFixture {
	t.Helper()
	const module = "crm.leads"
	definition := mustProjectAccessDefinition(
		t, "019f5c36-b322-7c52-9325-ec59f95c8fe0", "release-test", "en-US", "UTC", "USD",
	)
	projectAccess, err := panvarapg.NewProjectAccessStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := projectAccess.EnsureBootstrapScope(ctx, definition, "default", "release-owner")
	if err != nil {
		t.Fatal(err)
	}
	accessStore, err := panvarapg.NewAccessAdminStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := accessapp.NewDefaultBootstrapCredentialRegistrar(accessStore)
	if err != nil {
		t.Fatal(err)
	}
	const token = "postgres-release-bootstrap-token-canary-0123456789"
	if _, err := registrar.Register(ctx, scope, "release-owner", token); err != nil {
		t.Fatal(err)
	}
	authenticator, err := accessapp.NewCredentialAuthenticator(accessStore)
	if err != nil {
		t.Fatal(err)
	}
	invocation := mustAccessAdminInvocation(t, ctx, authenticator, scope, token, "request:release-publish")
	policy, err := accessapp.NewPolicy(projectAccess)
	if err != nil {
		t.Fatal(err)
	}
	clock := integrationDraftClock{at: time.Date(2026, time.July, 19, 10, 0, 0, 0, time.UTC)}
	revisionStore, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := moduleapp.NewRevisionRegistry(revisionStore, policy, clock)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := moduleapp.NewCompiler().Compile(baselineSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _, err := registry.RegisterBootstrap(
		ctx, definition.ID(), compiled, baselineSource, spec.FormatYAML,
	)
	if err != nil {
		t.Fatal(err)
	}
	draftStore, err := panvarapg.NewDraftStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := moduleapp.NewDraftWorkflow(
		draftStore, registry, policy, clock, &integrationDraftIDGenerator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	draft, created, err := workflow.Create(ctx, invocation.Execution(), module, moduleapp.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domainappmodule.SourceFormatYAML,
		Source: candidateSource, IdempotencyKey: "release-fixture-draft",
	})
	if err != nil || !created {
		t.Fatalf("Create(release fixture) = %#v, %t, %v", draft, created, err)
	}
	validation, created, err := workflow.Validate(
		ctx, invocation.Execution(), module, draft.ID().String(), draft.Generation(),
	)
	if err != nil || !created || !validation.Valid {
		t.Fatalf("Validate(release fixture) = %#v, %t, %v", validation, created, err)
	}
	plan, created, err := workflow.Plan(
		ctx, invocation.Execution(), module, draft.ID().String(), validation.ID, draft.Generation(),
	)
	if err != nil || !created {
		t.Fatalf("Plan(release fixture) = %#v, %t, %v", plan, created, err)
	}
	releaseStore, err := panvarapg.NewReleaseStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := releaseapp.NewDefaultPublisher(releaseStore, releaseStore, policy, accessStore)
	if err != nil {
		t.Fatal(err)
	}
	return &postgresReleaseFixture{
		pool: pool, module: module, scope: scope, invocation: invocation, policy: policy,
		accessStore: accessStore, releaseStore: releaseStore, draftStore: draftStore,
		workflow: workflow, publisher: publisher, baseline: baseline, draft: draft,
		validation: validation, plan: plan, candidateSource: append([]byte(nil), candidateSource...),
	}
}

func (fixture *postgresReleaseFixture) replaceAndPlan(t *testing.T, ctx context.Context, source []byte) {
	t.Helper()
	draft, changed, err := fixture.workflow.Replace(
		ctx, fixture.invocation.Execution(), fixture.module, fixture.draft.ID().String(),
		fixture.draft.Generation(), moduleapp.ReplaceDraftInput{
			Format: domainappmodule.SourceFormatYAML, Source: source,
		},
	)
	if err != nil || !changed {
		t.Fatalf("Replace(release fixture) = %#v, %t, %v", draft, changed, err)
	}
	validation, created, err := fixture.workflow.Validate(
		ctx, fixture.invocation.Execution(), fixture.module, draft.ID().String(), draft.Generation(),
	)
	if err != nil || !created || !validation.Valid {
		t.Fatalf("Validate(replaced release fixture) = %#v, %t, %v", validation, created, err)
	}
	plan, created, err := fixture.workflow.Plan(
		ctx, fixture.invocation.Execution(), fixture.module, draft.ID().String(), validation.ID, draft.Generation(),
	)
	if err != nil || !created {
		t.Fatalf("Plan(replaced release fixture) = %#v, %t, %v", plan, created, err)
	}
	fixture.draft, fixture.validation, fixture.plan = draft, validation, plan
	fixture.candidateSource = append([]byte(nil), source...)
}

type releaseStoreHook struct {
	inner            releaseapp.ReleaseStore
	resolveBefore    func(context.Context) error
	publishBefore    func(context.Context) error
	resolveOnce      sync.Once
	publishOnce      sync.Once
	resolveBeforeErr error
	publishBeforeErr error
}

type releaseStoreReplayMissBarrier struct {
	inner       releaseapp.ReleaseStore
	want        int
	mu          sync.Mutex
	misses      int
	allResolved chan struct{}
}

func newReleaseStoreReplayMissBarrier(
	inner releaseapp.ReleaseStore,
	want int,
) *releaseStoreReplayMissBarrier {
	return &releaseStoreReplayMissBarrier{
		inner: inner, want: want, allResolved: make(chan struct{}),
	}
}

func (store *releaseStoreReplayMissBarrier) ResolveReplay(
	ctx context.Context,
	mutation accessapp.MutationContext,
	module string,
	planID string,
	key string,
	intentHash string,
	at time.Time,
) (domainrelease.ModuleRelease, bool, error) {
	value, found, err := store.inner.ResolveReplay(ctx, mutation, module, planID, key, intentHash, at)
	if err != nil || found {
		return value, found, err
	}
	store.mu.Lock()
	store.misses++
	if store.misses == store.want {
		close(store.allResolved)
	}
	store.mu.Unlock()
	return value, false, nil
}

func (store *releaseStoreReplayMissBarrier) Publish(
	ctx context.Context,
	mutation accessapp.MutationContext,
	revision domainappmodule.Revision,
	value domainrelease.ModuleRelease,
	key string,
	intentHash string,
) (domainrelease.ModuleRelease, bool, error) {
	select {
	case <-ctx.Done():
		return domainrelease.ModuleRelease{}, false, ctx.Err()
	case <-store.allResolved:
	}
	return store.inner.Publish(ctx, mutation, revision, value, key, intentHash)
}

func (store *releaseStoreReplayMissBarrier) Get(
	ctx context.Context,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	return store.inner.Get(ctx, scope, module, id)
}

func (store *releaseStoreHook) ResolveReplay(
	ctx context.Context,
	mutation accessapp.MutationContext,
	module string,
	planID string,
	key string,
	intentHash string,
	at time.Time,
) (domainrelease.ModuleRelease, bool, error) {
	if err := store.runResolveBefore(ctx); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	return store.inner.ResolveReplay(ctx, mutation, module, planID, key, intentHash, at)
}

func (store *releaseStoreHook) Publish(
	ctx context.Context,
	mutation accessapp.MutationContext,
	revision domainappmodule.Revision,
	value domainrelease.ModuleRelease,
	key string,
	intentHash string,
) (domainrelease.ModuleRelease, bool, error) {
	if err := store.runPublishBefore(ctx); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	return store.inner.Publish(ctx, mutation, revision, value, key, intentHash)
}

func (store *releaseStoreHook) runResolveBefore(ctx context.Context) error {
	if store.resolveBefore == nil {
		return nil
	}
	store.resolveOnce.Do(func() { store.resolveBeforeErr = store.resolveBefore(ctx) })
	return store.resolveBeforeErr
}

func (store *releaseStoreHook) runPublishBefore(ctx context.Context) error {
	if store.publishBefore == nil {
		return nil
	}
	store.publishOnce.Do(func() { store.publishBeforeErr = store.publishBefore(ctx) })
	return store.publishBeforeErr
}

func (store *releaseStoreHook) Get(
	ctx context.Context,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	return store.inner.Get(ctx, scope, module, id)
}

func releaseExampleSource(t *testing.T, relative string) []byte {
	t.Helper()
	value, err := os.ReadFile("../../examples/" + relative)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustEnvironmentID(t *testing.T, value string) project.EnvironmentID {
	t.Helper()
	id, err := project.ParseEnvironmentID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertReleaseCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	scope project.Scope,
	module string,
	wantReleases int,
	wantIdempotency int,
) {
	t.Helper()
	for table, want := range map[string]int{
		"panvara_module_release":             wantReleases,
		"panvara_module_release_idempotency": wantIdempotency,
	} {
		var got int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+table+`
			WHERE project_id = $1 AND environment_id = $2 AND module_name = $3`,
			scope.ProjectID().String(), scope.EnvironmentID().String(), module).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}

func assertRevisionCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID project.ID,
	module string,
	revisionHash string,
	want int,
) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_module_revision
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, projectID.String(), module, revisionHash).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("revision %s count = %d, want %d", revisionHash, got, want)
	}
}

func assertReleaseSuccessAuditCount(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	scope project.Scope,
	want int,
) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE project_id = $1 AND environment_id = $2
		  AND action = 'release.publish' AND outcome = 'success'
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("successful release audit count = %d, want %d", got, want)
	}
}

func assertReleaseAuditFactsForKeys(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture *postgresReleaseFixture,
	value domainrelease.ModuleRelease,
	keys []string,
) {
	t.Helper()
	expectedTimes := make([]time.Time, 0, len(keys))
	for _, key := range keys {
		var intentHash, releaseText string
		var createdAt time.Time
		if err := pool.QueryRow(ctx, `
			SELECT intent_hash, release_id::text, created_at
			FROM panvara_module_release_idempotency
			WHERE project_id = $1 AND environment_id = $2
			  AND module_name = $3 AND idempotency_key = $4
		`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
			fixture.module, key).Scan(&intentHash, &releaseText, &createdAt); err != nil {
			t.Fatal(err)
		}
		if !domainappmodule.ValidContentHash(intentHash) || releaseText != value.ID().String() || createdAt.IsZero() {
			t.Fatalf("idempotency facts for %q = %q/%q/%s", key, intentHash, releaseText, createdAt)
		}
		expectedTimes = append(expectedTimes, createdAt)
	}

	rows, err := pool.Query(ctx, `
		SELECT actor_principal_id, actor_credential_id::text, request_id,
		       action, outcome, target_kind, target_id, reason_code, occurred_at
		FROM panvara_security_audit_event
		WHERE project_id = $1 AND environment_id = $2
		  AND action = 'release.publish' AND outcome = 'success'
		ORDER BY occurred_at, event_id
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	matched := make([]bool, len(expectedTimes))
	count := 0
	for rows.Next() {
		var actor, credential, requestID, action, outcome, targetKind, targetID string
		var reason *string
		var occurredAt time.Time
		if err := rows.Scan(
			&actor, &credential, &requestID, &action, &outcome, &targetKind, &targetID, &reason, &occurredAt,
		); err != nil {
			t.Fatal(err)
		}
		if actor != value.PublishedBy() || credential != fixture.invocation.Execution().CredentialID().String() ||
			requestID != fixture.invocation.RequestID() || action != string(accessapp.OperationReleasePublish) ||
			outcome != "success" || targetKind != "module_release" || targetID != value.ID().String() || reason != nil {
			t.Fatalf(
				"release audit facts = %q/%q/%q/%q/%q/%q/%q/%v/%s",
				actor, credential, requestID, action, outcome, targetKind, targetID, reason, occurredAt,
			)
		}
		found := false
		for index, expected := range expectedTimes {
			if !matched[index] && occurredAt.Equal(expected) {
				matched[index], found = true, true
				break
			}
		}
		if !found {
			t.Fatalf("release audit timestamp %s does not match idempotency facts %#v", occurredAt, expectedTimes)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(keys) {
		t.Fatalf("release audit rows = %d, want %d", count, len(keys))
	}
}

func assertReleaseKeyBinding(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture *postgresReleaseFixture,
	key string,
	value domainrelease.ModuleRelease,
) {
	t.Helper()
	var releaseText string
	if err := pool.QueryRow(ctx, `
		SELECT release_id::text
		FROM panvara_module_release_idempotency
		WHERE project_id = $1 AND environment_id = $2
		  AND module_name = $3 AND idempotency_key = $4
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
		fixture.module, key).Scan(&releaseText); err != nil {
		t.Fatal(err)
	}
	if releaseText != value.ID().String() {
		t.Fatalf("idempotency key %q release = %q, want %q", key, releaseText, value.ID().String())
	}
}

func installReleaseInsertFailure(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION panvara_test_reject_publish_insert()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced publish insert failure';
		END;
		$$
	`); err != nil {
		t.Fatal(err)
	}
	statement := fmt.Sprintf(`
		CREATE TRIGGER panvara_test_reject_publish_insert
		BEFORE INSERT ON %s
		FOR EACH ROW EXECUTE FUNCTION panvara_test_reject_publish_insert()
	`, table)
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatal(err)
	}
}

func assertReleaseFactsAppendOnly(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture *postgresReleaseFixture,
	value domainrelease.ModuleRelease,
) {
	t.Helper()
	statements := []string{
		`UPDATE panvara_module_release SET risk = 'low'
		 WHERE project_id = $1 AND environment_id = $2 AND module_name = $3 AND release_id = $4`,
		`DELETE FROM panvara_module_release
		 WHERE project_id = $1 AND environment_id = $2 AND module_name = $3 AND release_id = $4`,
		`UPDATE panvara_module_release_idempotency SET intent_hash = repeat('a', 71)
		 WHERE project_id = $1 AND environment_id = $2 AND module_name = $3 AND release_id = $4`,
		`DELETE FROM panvara_module_release_idempotency
		 WHERE project_id = $1 AND environment_id = $2 AND module_name = $3 AND release_id = $4`,
	}
	arguments := []any{
		fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(), fixture.module, value.ID().String(),
	}
	for _, statement := range statements {
		_, err := pool.Exec(ctx, statement, arguments...)
		assertPostgresSQLState(t, strings.TrimSpace(statement), err, "55000")
	}
	for _, statement := range []string{
		`TRUNCATE panvara_module_release_idempotency`,
		`TRUNCATE panvara_module_release CASCADE`,
	} {
		_, err := pool.Exec(ctx, statement)
		assertPostgresSQLState(t, statement, err, "55000")
	}
	assertReleaseCounts(t, ctx, pool, fixture.scope, fixture.module, 2, 12)
}

func assertCrossScopePublishRejected(
	t *testing.T,
	ctx context.Context,
	fixture *postgresReleaseFixture,
	value domainrelease.ModuleRelease,
	wrongEnvironment project.Scope,
	wrongProject project.Scope,
) {
	t.Helper()
	revisions, err := panvarapg.NewRevisionStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := revisions.Get(
		ctx, fixture.scope.ProjectID(), fixture.module, value.CandidateRevision(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for index, scope := range []project.Scope{wrongEnvironment, wrongProject} {
		execution := integrationAdminExecution(
			t, scope.ProjectID(), scope.EnvironmentID().String(), "release-owner",
		)
		invocation, err := accessapp.NewInvocation(execution, fmt.Sprintf("request:cross-scope-%d", index))
		if err != nil {
			t.Fatal(err)
		}
		mutation, err := accessapp.NewMutationContext(invocation, accessapp.OperationReleasePublish)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = fixture.releaseStore.Publish(
			ctx, mutation, revision, value, fmt.Sprintf("cross-scope-%d", index),
			"sha256:"+strings.Repeat("a", 64),
		)
		if !errors.Is(err, releaseapp.ErrInvalid) {
			t.Fatalf("Publish(cross scope %d) error = %v, want ErrInvalid", index, err)
		}
	}
	assertReleaseCounts(t, ctx, fixture.pool, fixture.scope, fixture.module, 2, 12)
}

func assertMismatchedPlanRejected(
	t *testing.T,
	ctx context.Context,
	fixture *postgresReleaseFixture,
	value domainrelease.ModuleRelease,
) {
	t.Helper()
	revisions, err := panvarapg.NewRevisionStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := revisions.Get(
		ctx, fixture.scope.ProjectID(), fixture.module, value.CandidateRevision(),
	)
	if err != nil {
		t.Fatal(err)
	}
	mismatched, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: value.ID(), Scope: value.Scope(), ModuleName: value.ModuleName(), DraftID: value.DraftID(),
		DraftGeneration: value.DraftGeneration(), ValidationID: value.ValidationID(), PlanID: value.PlanID(),
		PlanHash: "sha256:" + strings.Repeat("b", 64), BaselineRevision: value.BaselineRevision(),
		CandidateRevision: value.CandidateRevision(), DataSchemaFormat: value.DataSchemaFormat(),
		DataSchemaFingerprint: value.DataSchemaFingerprint(), SourceHash: value.SourceHash(),
		Outcome: value.Outcome(), Risk: value.Risk(), PublishedBy: value.PublishedBy(),
		PublishedCredentialID: value.PublishedCredentialID(), RequestID: value.RequestID(),
		PublishedAt: value.PublishedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := accessapp.NewMutationContext(fixture.invocation, accessapp.OperationReleasePublish)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = fixture.releaseStore.Publish(
		ctx, mutation, revision, mismatched, "publish-mismatched-plan",
		"sha256:"+strings.Repeat("c", 64),
	)
	if !errors.Is(err, releaseapp.ErrCorrupt) {
		t.Fatalf("Publish(mismatched plan facts) error = %v, want ErrCorrupt", err)
	}
	assertReleaseCounts(t, ctx, fixture.pool, fixture.scope, fixture.module, 2, 12)
}
