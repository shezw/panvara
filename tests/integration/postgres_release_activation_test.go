//go:build integration

/*
   Panvara
   tests/integration/postgres_release_activation_test.go    2026-08-02
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	accessapp "github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

func TestPostgresCompatibleReleaseActivationPersistsMonotonicSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture, value, candidate, bootstrap, mutation := newPostgresActivationFixture(t, ctx, pool)

	activatedAt := bootstrap.ActivatedAt().Add(time.Hour)
	active, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: value, Candidate: candidate, ActivatedAt: activatedAt,
		},
	)
	if err != nil || !advanced {
		t.Fatalf("ActivateCompatible() = %#v, %t, %v", active, advanced, err)
	}
	activeRelease, found := active.ReleaseID()
	credential, hasCredential := active.ActivatedCredentialID()
	if active.Epoch() != 2 || active.Origin() != domainrelease.ActiveSnapshotOriginRelease ||
		!found || activeRelease != value.ID() || active.RuntimeRevision() != value.CandidateRevision() ||
		active.RecordNamespaceRevision() != fixture.baseline.RevisionHash() ||
		active.DataSchemaFormat() != value.DataSchemaFormat() ||
		active.DataSchemaFingerprint() != value.DataSchemaFingerprint() ||
		active.ActivatedBy() != mutation.ActorID() || !hasCredential || credential != mutation.CredentialID() ||
		active.RequestID() != mutation.RequestID() || !active.ActivatedAt().Equal(activatedAt) {
		t.Fatalf("activated snapshot = %#v", active)
	}
	persisted, err := fixture.releaseStore.GetActive(ctx, fixture.scope, fixture.module)
	if err != nil || !sameIntegrationActiveSnapshot(persisted, active) {
		t.Fatalf("GetActive() = %#v, %v; want %#v", persisted, err, active)
	}

	replayed, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: value, Candidate: candidate,
			ActivatedAt: activatedAt.Add(time.Hour),
		},
	)
	if err != nil || advanced || !sameIntegrationActiveSnapshot(replayed, active) {
		t.Fatalf("ActivateCompatible(replay) = %#v, %t, %v", replayed, advanced, err)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 2, 2, 1)
}

func TestPostgresCompatibleReleaseActivationRejectsChangedDataIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baselineSource := releaseExampleSource(t, "modules/crm-leads.yaml")
	candidateSource := optionalFieldActivationSource(t, baselineSource, "1.1.0")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baselineSource, candidateSource)
	if fixture.plan.Outcome != string(domainrelease.OutcomeCompatible) ||
		fixture.plan.Candidate.DataSchemaFingerprint == fixture.plan.BaselineDataSchemaFingerprint {
		t.Fatalf("changed-data plan = %#v, want compatible with changed identity", fixture.plan)
	}
	bootstrap := ensureActivationBootstrap(t, ctx, fixture)
	value, candidate := publishActivationCandidate(t, ctx, fixture, fixture.plan, "activate-data-change")
	mutation := activationMutation(t, fixture)

	if _, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: value, Candidate: candidate,
			ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
		},
	); !errors.Is(err, releaseapp.ErrNotActivatable) || advanced {
		t.Fatalf("ActivateCompatible(changed identity) = advanced %t, error %v", advanced, err)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 1, 1, 0)
}

func TestPostgresCompatibleReleaseActivationSerializesSameBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baselineSource := releaseExampleSource(t, "modules/crm-leads.yaml")
	firstSource := runtimeOnlyActivationSource(t, baselineSource, "1.1.0")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baselineSource, firstSource)
	bootstrap := ensureActivationBootstrap(t, ctx, fixture)
	first, firstCandidate := publishActivationCandidate(t, ctx, fixture, fixture.plan, "activate-race-first")
	secondPlan := createActivationPlan(
		t, ctx, fixture, fixture.baseline.RevisionHash(),
		runtimeOnlyActivationSource(t, baselineSource, "1.2.0"), "activate-race-second-draft",
	)
	second, secondCandidate := publishActivationCandidate(t, ctx, fixture, secondPlan, "activate-race-second")
	mutation := activationMutation(t, fixture)

	type activationResult struct {
		value    domainrelease.ActiveSnapshot
		advanced bool
		err      error
	}
	commands := []releaseapp.ActivateCompatibleCommand{
		{Expected: bootstrap, Release: first, Candidate: firstCandidate, ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour)},
		{Expected: bootstrap, Release: second, Candidate: secondCandidate, ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour)},
	}
	start := make(chan struct{})
	results := make(chan activationResult, len(commands))
	var group sync.WaitGroup
	for _, command := range commands {
		group.Add(1)
		go func(command releaseapp.ActivateCompatibleCommand) {
			defer group.Done()
			<-start
			value, advanced, err := fixture.releaseStore.ActivateCompatible(ctx, mutation, command)
			results <- activationResult{value: value, advanced: advanced, err: err}
		}(command)
	}
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result.err == nil && result.advanced && result.value.Epoch() == 2:
			successes++
		case errors.Is(result.err, releaseapp.ErrActivationConflict) && !result.advanced:
			conflicts++
		default:
			t.Fatalf("concurrent ActivateCompatible() = %#v, %t, %v", result.value, result.advanced, result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent activation successes/conflicts = %d/%d, want 1/1", successes, conflicts)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 2, 2, 1)
}

func TestPostgresCompatibleReleaseActivationCoalescesConcurrentSameRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture, value, candidate, bootstrap, mutation := newPostgresActivationFixture(t, ctx, pool)
	command := releaseapp.ActivateCompatibleCommand{
		Expected: bootstrap, Release: value, Candidate: candidate,
		ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
	}
	const writers = 8
	type activationResult struct {
		value    domainrelease.ActiveSnapshot
		advanced bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan activationResult, writers)
	var group sync.WaitGroup
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			active, advanced, err := fixture.releaseStore.ActivateCompatible(ctx, mutation, command)
			results <- activationResult{value: active, advanced: advanced, err: err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	advancedCount := 0
	for result := range results {
		if result.err != nil || result.value.Epoch() != 2 {
			t.Fatalf("concurrent same Release = %#v, %t, %v", result.value, result.advanced, result.err)
		}
		if result.advanced {
			advancedCount++
		}
	}
	if advancedCount != 1 {
		t.Fatalf("concurrent same Release advanced count = %d, want 1", advancedCount)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 2, 2, 1)
}

func TestPostgresCompatibleReleaseActivationRejectsHistoricalRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	baselineSource := releaseExampleSource(t, "modules/crm-leads.yaml")
	firstSource := runtimeOnlyActivationSource(t, baselineSource, "1.1.0")
	fixture := newPostgresReleaseFixture(t, ctx, pool, baselineSource, firstSource)
	bootstrap := ensureActivationBootstrap(t, ctx, fixture)
	first, firstCandidate := publishActivationCandidate(t, ctx, fixture, fixture.plan, "activate-history-first")
	mutation := activationMutation(t, fixture)
	firstActive, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: first, Candidate: firstCandidate,
			ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
		},
	)
	if err != nil || !advanced {
		t.Fatalf("ActivateCompatible(first) = %#v, %t, %v", firstActive, advanced, err)
	}
	secondSource := runtimeOnlyActivationSource(t, baselineSource, "1.2.0")
	secondPlan := createActivationPlan(
		t, ctx, fixture, first.CandidateRevision(), secondSource, "activate-history-second-draft",
	)
	second, secondCandidate := publishActivationCandidate(t, ctx, fixture, secondPlan, "activate-history-second")
	secondActive, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: firstActive, Release: second, Candidate: secondCandidate,
			ActivatedAt: firstActive.ActivatedAt().Add(time.Hour),
		},
	)
	if err != nil || !advanced {
		t.Fatalf("ActivateCompatible(second) = %#v, %t, %v", secondActive, advanced, err)
	}
	if _, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: secondActive, Release: first, Candidate: firstCandidate,
			ActivatedAt: secondActive.ActivatedAt().Add(time.Hour),
		},
	); !errors.Is(err, releaseapp.ErrActivationConflict) || advanced {
		t.Fatalf("ActivateCompatible(historical) = advanced %t, error %v", advanced, err)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 3, 3, 2)
}

func TestPostgresReleaseActivationReauthorizesAndRollsBack(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture, value, candidate, bootstrap, mutation := newPostgresActivationFixture(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_access_grant
		SET revoked_by_principal_id = $3, revoked_at = clock_timestamp(), updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = $3 AND role = 'project.owner' AND revoked_at IS NULL
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String(),
		mutation.ActorID()); err != nil {
		t.Fatal(err)
	}
	if _, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, mutation, releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: value, Candidate: candidate,
			ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
		},
	); !errors.Is(err, accessapp.ErrForbidden) || advanced {
		t.Fatalf("ActivateCompatible(revoked) = advanced %t, error %v", advanced, err)
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 1, 1, 0)
}

func TestPostgresReleaseActivationRollsBackEveryWritePoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture, value, candidate, bootstrap, mutation := newPostgresActivationFixture(t, ctx, pool)
	command := releaseapp.ActivateCompatibleCommand{
		Expected: bootstrap, Release: value, Candidate: candidate,
		ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
	}
	for _, failure := range []struct {
		table string
		event string
	}{
		{table: "panvara_project_release_snapshot", event: "INSERT"},
		{table: "panvara_project_release_snapshot_module", event: "INSERT"},
		{table: "panvara_environment_release_pointer", event: "UPDATE"},
		{table: "panvara_security_audit_event", event: "INSERT"},
	} {
		t.Run(failure.table, func(t *testing.T) {
			installActivationWriteFailure(t, ctx, pool, failure.table, failure.event)
			if _, advanced, err := fixture.releaseStore.ActivateCompatible(
				ctx, mutation, command,
			); err == nil || advanced {
				t.Fatalf("ActivateCompatible(forced %s failure) = advanced %t, error %v", failure.table, advanced, err)
			}
			removeActivationWriteFailure(t, ctx, pool, failure.table)
			assertActivationFactCounts(t, ctx, pool, fixture, 1, 1, 0)
		})
	}
}

func TestPostgresReleaseActivationTablesEnforceAppendOnlyAndMonotonicity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	fixture := newPostgresReleaseFixture(
		t, ctx, pool, releaseExampleSource(t, "modules/crm-leads.yaml"),
		runtimeOnlyActivationSource(t, releaseExampleSource(t, "modules/crm-leads.yaml"), "1.1.0"),
	)
	_ = ensureActivationBootstrap(t, ctx, fixture)
	for name, statement := range map[string]string{
		"snapshot update":   `UPDATE panvara_project_release_snapshot SET request_id = request_id`,
		"snapshot delete":   `DELETE FROM panvara_project_release_snapshot`,
		"snapshot truncate": `TRUNCATE panvara_project_release_snapshot`,
		"binding update":    `UPDATE panvara_project_release_snapshot_module SET module_name = module_name`,
		"binding delete":    `DELETE FROM panvara_project_release_snapshot_module`,
		"binding truncate":  `TRUNCATE panvara_project_release_snapshot_module`,
		"pointer skip":      `UPDATE panvara_environment_release_pointer SET active_epoch = active_epoch + 2`,
		"pointer delete":    `DELETE FROM panvara_environment_release_pointer`,
		"pointer truncate":  `TRUNCATE panvara_environment_release_pointer`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx, statement)
			var databaseError *pgconn.PgError
			if !errors.As(err, &databaseError) ||
				(databaseError.Code != "55000" && databaseError.Code != "0A000") {
				t.Fatalf("%s error = %v, want PostgreSQL 55000 or FK truncate rejection", statement, err)
			}
		})
	}
	assertActivationFactCounts(t, ctx, pool, fixture, 1, 1, 0)
}

func installActivationWriteFailure(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
	event string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION panvara_test_reject_activation_write()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced activation write failure';
		END;
		$$
	`); err != nil {
		t.Fatal(err)
	}
	statement := fmt.Sprintf(`
		CREATE TRIGGER panvara_test_reject_activation_write
		BEFORE %s ON %s
		FOR EACH ROW EXECUTE FUNCTION panvara_test_reject_activation_write()
	`, event, table)
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatal(err)
	}
}

func removeActivationWriteFailure(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	table string,
) {
	t.Helper()
	statement := fmt.Sprintf(
		"DROP TRIGGER panvara_test_reject_activation_write ON %s", table,
	)
	if _, err := pool.Exec(ctx, statement); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION panvara_test_reject_activation_write()`); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresMigrateUpgrades0006ReleaseForActivation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	checksums := applyMigrationsThrough0006(t, ctx, pool)
	baselineSource := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(
		t, ctx, pool, baselineSource, runtimeOnlyActivationSource(t, baselineSource, "1.1.0"),
	)
	value, candidate := publishActivationCandidate(t, ctx, fixture, fixture.plan, "activate-after-0006-upgrade")
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate(0006 -> current) error = %v", err)
	}
	checksums["0007_compatible_release_activation.sql"] = embeddedMigrationChecksum(
		t, "0007_compatible_release_activation.sql",
	)
	assertMigrationLedger(t, ctx, pool, checksums)
	bootstrap := ensureActivationBootstrap(t, ctx, fixture)
	active, advanced, err := fixture.releaseStore.ActivateCompatible(
		ctx, activationMutation(t, fixture), releaseapp.ActivateCompatibleCommand{
			Expected: bootstrap, Release: value, Candidate: candidate,
			ActivatedAt: bootstrap.ActivatedAt().Add(time.Hour),
		},
	)
	if err != nil || !advanced || active.Epoch() != 2 || active.RuntimeRevision() != value.CandidateRevision() {
		t.Fatalf("ActivateCompatible(after 0006 upgrade) = %#v, %t, %v", active, advanced, err)
	}
}

func newPostgresActivationFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) (*postgresReleaseFixture, domainrelease.ModuleRelease, domainappmodule.Revision, domainrelease.ActiveSnapshot, accessapp.MutationContext) {
	t.Helper()
	baselineSource := releaseExampleSource(t, "modules/crm-leads.yaml")
	fixture := newPostgresReleaseFixture(
		t, ctx, pool, baselineSource, runtimeOnlyActivationSource(t, baselineSource, "1.1.0"),
	)
	bootstrap := ensureActivationBootstrap(t, ctx, fixture)
	value, candidate := publishActivationCandidate(t, ctx, fixture, fixture.plan, "activate-compatible")
	return fixture, value, candidate, bootstrap, activationMutation(t, fixture)
}

func ensureActivationBootstrap(
	t *testing.T,
	ctx context.Context,
	fixture *postgresReleaseFixture,
) domainrelease.ActiveSnapshot {
	t.Helper()
	at := time.Date(2026, time.July, 19, 11, 0, 0, 0, time.UTC)
	value, created, err := fixture.releaseStore.EnsureBootstrap(
		ctx, fixture.scope, fixture.module, fixture.baseline.RevisionHash(), at,
	)
	if err != nil || !created || value.Epoch() != 1 ||
		value.RuntimeRevision() != fixture.baseline.RevisionHash() ||
		value.RecordNamespaceRevision() != fixture.baseline.RevisionHash() {
		t.Fatalf("EnsureBootstrap() = %#v, %t, %v", value, created, err)
	}
	replayed, created, err := fixture.releaseStore.EnsureBootstrap(
		ctx, fixture.scope, fixture.module, fixture.plan.Candidate.RevisionHash, at.Add(time.Hour),
	)
	if err != nil || created || !sameIntegrationActiveSnapshot(replayed, value) {
		t.Fatalf("EnsureBootstrap(restart) = %#v, %t, %v", replayed, created, err)
	}
	return value
}

func publishActivationCandidate(
	t *testing.T,
	ctx context.Context,
	fixture *postgresReleaseFixture,
	plan moduleapp.DraftPlan,
	key string,
) (domainrelease.ModuleRelease, domainappmodule.Revision) {
	t.Helper()
	if plan.Outcome != string(domainrelease.OutcomeCompatible) {
		t.Fatalf("activation plan outcome = %q, want compatible", plan.Outcome)
	}
	value, created, err := fixture.publisher.Publish(
		ctx, fixture.invocation, fixture.module,
		releaseapp.PublishInput{PlanID: plan.ID, IdempotencyKey: key},
	)
	if err != nil || !created {
		t.Fatalf("Publish(%s) = %#v, %t, %v", key, value, created, err)
	}
	store, err := panvarapg.NewRevisionStore(fixture.pool)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.Get(
		ctx, fixture.scope.ProjectID(), fixture.module, value.CandidateRevision(),
	)
	if err != nil {
		t.Fatalf("Get(activation Candidate) error = %v", err)
	}
	return value, candidate
}

func createActivationPlan(
	t *testing.T,
	ctx context.Context,
	fixture *postgresReleaseFixture,
	baseline string,
	source []byte,
	key string,
) moduleapp.DraftPlan {
	t.Helper()
	draft, created, err := fixture.workflow.Create(
		ctx, fixture.invocation.Execution(), fixture.module, moduleapp.CreateDraftInput{
			BaselineRevision: baseline, Format: domainappmodule.SourceFormatYAML,
			Source: source, IdempotencyKey: key,
		},
	)
	if err != nil || !created {
		t.Fatalf("Create(%s) = %#v, %t, %v", key, draft, created, err)
	}
	validation, created, err := fixture.workflow.Validate(
		ctx, fixture.invocation.Execution(), fixture.module, draft.ID().String(), draft.Generation(),
	)
	if err != nil || !created || !validation.Valid {
		t.Fatalf("Validate(%s) = %#v, %t, %v", key, validation, created, err)
	}
	plan, created, err := fixture.workflow.Plan(
		ctx, fixture.invocation.Execution(), fixture.module, draft.ID().String(),
		validation.ID, draft.Generation(),
	)
	if err != nil || !created || plan.Outcome != string(domainrelease.OutcomeCompatible) {
		t.Fatalf("Plan(%s) = %#v, %t, %v", key, plan, created, err)
	}
	return plan
}

func activationMutation(t *testing.T, fixture *postgresReleaseFixture) accessapp.MutationContext {
	t.Helper()
	value, err := accessapp.NewMutationContext(fixture.invocation, accessapp.OperationReleaseActivate)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func runtimeOnlyActivationSource(t *testing.T, source []byte, version string) []byte {
	t.Helper()
	value := strings.Replace(string(source), "version: 1.0.0", "version: "+version, 1)
	if value == string(source) {
		t.Fatalf("source has no replaceable module version")
	}
	return []byte(value)
}

func optionalFieldActivationSource(t *testing.T, source []byte, version string) []byte {
	t.Helper()
	value := string(runtimeOnlyActivationSource(t, source, version))
	needle := "      api:\n        public:\n"
	replacement := "        - name: nickname\n          type: string\n" + needle
	value = strings.Replace(value, needle, replacement, 1)
	if !strings.Contains(value, "name: nickname") {
		t.Fatal("failed to add optional field to activation source")
	}
	return []byte(value)
}

func sameIntegrationActiveSnapshot(left, right domainrelease.ActiveSnapshot) bool {
	leftRelease, leftHasRelease := left.ReleaseID()
	rightRelease, rightHasRelease := right.ReleaseID()
	leftCredential, leftHasCredential := left.ActivatedCredentialID()
	rightCredential, rightHasCredential := right.ActivatedCredentialID()
	return left.Scope() == right.Scope() && left.Epoch() == right.Epoch() && left.Origin() == right.Origin() &&
		left.ModuleName() == right.ModuleName() && leftHasRelease == rightHasRelease &&
		(!leftHasRelease || leftRelease == rightRelease) &&
		left.RuntimeRevision() == right.RuntimeRevision() &&
		left.RecordNamespaceRevision() == right.RecordNamespaceRevision() &&
		left.DataSchemaFormat() == right.DataSchemaFormat() &&
		left.DataSchemaFingerprint() == right.DataSchemaFingerprint() &&
		left.ActivatedBy() == right.ActivatedBy() && leftHasCredential == rightHasCredential &&
		(!leftHasCredential || leftCredential == rightCredential) &&
		left.RequestID() == right.RequestID() && left.ActivatedAt().Equal(right.ActivatedAt())
}

func assertActivationFactCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	fixture *postgresReleaseFixture,
	wantSnapshots int,
	wantBindings int,
	wantAudits int,
) {
	t.Helper()
	for table, want := range map[string]int{
		"panvara_project_release_snapshot":        wantSnapshots,
		"panvara_project_release_snapshot_module": wantBindings,
		"panvara_environment_release_pointer":     1,
	} {
		var got int
		if err := pool.QueryRow(ctx, fmt.Sprintf(
			"SELECT count(*) FROM %s WHERE project_id = $1 AND environment_id = $2", table,
		), fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String()).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
	var epoch int
	if err := pool.QueryRow(ctx, `
		SELECT active_epoch FROM panvara_environment_release_pointer
		WHERE project_id = $1 AND environment_id = $2
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String()).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch != wantSnapshots {
		t.Fatalf("active epoch = %d, want %d", epoch, wantSnapshots)
	}
	var audits int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE project_id = $1 AND environment_id = $2
		  AND action = 'release.activate' AND outcome = 'success'
	`, fixture.scope.ProjectID().String(), fixture.scope.EnvironmentID().String()).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != wantAudits {
		t.Fatalf("successful activation audit count = %d, want %d", audits, wantAudits)
	}
}
