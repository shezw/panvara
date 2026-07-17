//go:build integration

/*
   Panvara
   tests/integration/postgres_draft_workflow_test.go    2026-07-16
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestPostgresDraftWorkflowCASIdempotencySnapshotsAndIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	otherProject := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ac")
	owner, err := actor.New(projectID.String(), "integration-owner", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := actor.New(otherProject.String(), "integration-owner-b", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	clock := integrationDraftClock{at: time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)}
	revisionStore, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := application.NewRevisionRegistry(revisionStore, clock)
	if err != nil {
		t.Fatal(err)
	}
	baselineSource := integrationDraftSource("1.0.0")
	baselineModule, err := application.NewCompiler().Compile(baselineSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _, err := registry.RegisterBootstrap(ctx, projectID, baselineModule, baselineSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	draftStore, err := panvarapg.NewDraftStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.NewDraftWorkflow(draftStore, registry, clock, &integrationDraftIDGenerator{})
	if err != nil {
		t.Fatal(err)
	}

	draft, created, err := workflow.Create(ctx, projectID, owner, "notes", application.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domain.SourceFormatYAML,
		Source: baselineSource, IdempotencyKey: "integration-create",
	})
	if err != nil || !created {
		t.Fatalf("Create() = %#v, %v, %v", draft, created, err)
	}
	replayed, created, err := workflow.Create(ctx, projectID, owner, "notes", application.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domain.SourceFormatYAML,
		Source: baselineSource, IdempotencyKey: "integration-create",
	})
	if err != nil || created || replayed.ID() != draft.ID() {
		t.Fatalf("Create(replay) = %#v, %v, %v", replayed, created, err)
	}
	if _, _, err := workflow.Create(ctx, projectID, owner, "notes", application.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domain.SourceFormatYAML,
		Source: []byte("different"), IdempotencyKey: "integration-create",
	}); !errors.Is(err, application.ErrDraftIdempotencyConflict) {
		t.Fatalf("Create(conflicting idempotency) error = %v", err)
	}
	var draftCountBeforeNUL int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM panvara_module_draft WHERE project_id = $1`, projectID.String()).Scan(&draftCountBeforeNUL); err != nil {
		t.Fatal(err)
	}
	if _, _, err := workflow.Create(ctx, projectID, owner, "notes", application.CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML,
		Source: []byte("bad\x00source"), IdempotencyKey: "integration-raw-nul",
	}); !errors.Is(err, application.ErrDraftInvalid) {
		t.Fatalf("Create(raw NUL) error = %v", err)
	}
	var draftCountAfterNUL int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM panvara_module_draft WHERE project_id = $1`, projectID.String()).Scan(&draftCountAfterNUL); err != nil {
		t.Fatal(err)
	}
	if draftCountAfterNUL != draftCountBeforeNUL {
		t.Fatalf("raw NUL changed persisted Draft count: %d -> %d", draftCountBeforeNUL, draftCountAfterNUL)
	}
	noOp, changed, err := workflow.Replace(ctx, projectID, owner, "notes", draft.ID().String(), 1,
		application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: baselineSource})
	if err != nil || changed || noOp.Generation() != 1 || noOp.UpdatedAt() != draft.UpdatedAt() {
		t.Fatalf("Replace(no-op) = %#v, %v, %v", noOp, changed, err)
	}
	validation, created, err := workflow.Validate(ctx, projectID, owner, "notes", draft.ID().String(), 1)
	if err != nil || !created || !validation.Valid {
		t.Fatalf("Validate() = %#v, %v, %v", validation, created, err)
	}
	if _, created, err := workflow.Validate(ctx, projectID, owner, "notes", draft.ID().String(), 1); err != nil || created {
		t.Fatalf("Validate(replay) created = %v error = %v", created, err)
	}
	plan, created, err := workflow.Plan(ctx, projectID, owner, "notes", draft.ID().String(), validation.ID, 1)
	if err != nil || !created || len(plan.Changes) != 0 || plan.Risk != "none" {
		t.Fatalf("Plan() = %#v, %v, %v", plan, created, err)
	}
	if _, created, err := workflow.Plan(ctx, projectID, owner, "notes", draft.ID().String(), validation.ID, 1); err != nil || created {
		t.Fatalf("Plan(replay) created = %v error = %v", created, err)
	}

	restartedStore, err := panvarapg.NewDraftStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	restartedWorkflow, err := application.NewDraftWorkflow(restartedStore, registry, clock, &integrationDraftIDGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := restartedWorkflow.GetValidation(ctx, projectID, owner, "notes", draft.ID().String(), validation.ID); err != nil || got.ID != validation.ID {
		t.Fatalf("restart GetValidation() = %#v, %v", got, err)
	}
	if got, err := restartedWorkflow.GetPlan(ctx, projectID, owner, "notes", draft.ID().String(), plan.ID); err != nil || got.ID != plan.ID {
		t.Fatalf("restart GetPlan() = %#v, %v", got, err)
	}
	if _, err := restartedStore.Get(ctx, otherProject, "notes", draft.ID()); !errors.Is(err, application.ErrDraftNotFound) {
		t.Fatalf("cross-project Get() error = %v", err)
	}
	otherWorkflow, err := application.NewDraftWorkflow(restartedStore, registry, clock, &integrationDraftIDGenerator{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherWorkflow.GetValidation(ctx, otherProject, otherOwner, "notes", draft.ID().String(), validation.ID); !errors.Is(err, application.ErrValidationNotFound) {
		t.Fatalf("cross-project GetValidation() error = %v", err)
	}
	if _, err := otherWorkflow.GetPlan(ctx, otherProject, otherOwner, "notes", draft.ID().String(), plan.ID); !errors.Is(err, application.ErrPlanNotFound) {
		t.Fatalf("cross-project GetPlan() error = %v", err)
	}
	if _, _, err := otherWorkflow.Replace(ctx, otherProject, otherOwner, "notes", draft.ID().String(), 1,
		application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: baselineSource}); !errors.Is(err, application.ErrDraftNotFound) {
		t.Fatalf("cross-project Replace() error = %v", err)
	}
	if _, _, err := otherWorkflow.Create(ctx, otherProject, otherOwner, "notes", application.CreateDraftInput{
		BaselineRevision: baseline.RevisionHash(), Format: domain.SourceFormatYAML,
		Source: baselineSource, IdempotencyKey: "integration-cross-baseline",
	}); !errors.Is(err, application.ErrRevisionNotFound) {
		t.Fatalf("cross-project baseline Create() error = %v", err)
	}
	otherDraft, otherCreated, err := otherWorkflow.Create(ctx, otherProject, otherOwner, "notes", application.CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML,
		Source: baselineSource, IdempotencyKey: "integration-create",
	})
	if err != nil || !otherCreated || otherDraft.ID() != draft.ID() {
		t.Fatalf("same identity in other project Create() = %#v, %v, %v; first ID=%s", otherDraft, otherCreated, err, draft.ID())
	}
	if replay, created, err := otherWorkflow.Create(ctx, otherProject, otherOwner, "notes", application.CreateDraftInput{
		BaselineRevision: "none", Format: domain.SourceFormatYAML,
		Source: baselineSource, IdempotencyKey: "integration-create",
	}); err != nil || created || replay.ID() != otherDraft.ID() {
		t.Fatalf("other-project idempotency replay = %#v, %v, %v", replay, created, err)
	}

	const writers = 8
	equivalentSource := append([]byte("# same compiled semantics\n"), baselineSource...)
	start := make(chan struct{})
	results := make(chan error, writers)
	var group sync.WaitGroup
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, changed, err := workflow.Replace(ctx, projectID, owner, "notes", draft.ID().String(), 1,
				application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: equivalentSource})
			if err == nil && !changed {
				err = errors.New("concurrent winning replacement was a no-op")
			}
			results <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, application.ErrDraftConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Replace() error = %v", result)
		}
	}
	if successes != 1 || conflicts != writers-1 {
		t.Fatalf("concurrent Replace() = %d successes, %d conflicts", successes, conflicts)
	}
	current, err := workflow.Get(ctx, projectID, owner, "notes", draft.ID().String())
	if err != nil || current.Generation() != 2 {
		t.Fatalf("Get(after CAS) = %#v, %v", current, err)
	}

	noOpStart := make(chan struct{})
	noOpResults := make(chan error, writers)
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-noOpStart
			value, changed, err := workflow.Replace(ctx, projectID, owner, "notes", draft.ID().String(), 2,
				application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: equivalentSource})
			if err == nil && (changed || value.Generation() != 2) {
				err = errors.New("concurrent identical replacement changed generation")
			}
			noOpResults <- err
		}()
	}
	close(noOpStart)
	group.Wait()
	close(noOpResults)
	for result := range noOpResults {
		if result != nil {
			t.Fatalf("concurrent no-op Replace() error = %v", result)
		}
	}

	type validationResult struct {
		value   application.DraftValidation
		created bool
		err     error
	}
	validationStart := make(chan struct{})
	validationResults := make(chan validationResult, writers)
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-validationStart
			value, created, err := workflow.Validate(ctx, projectID, owner, "notes", draft.ID().String(), 2)
			validationResults <- validationResult{value: value, created: created, err: err}
		}()
	}
	close(validationStart)
	group.Wait()
	close(validationResults)
	validationCreated := 0
	var equivalentValidation application.DraftValidation
	for result := range validationResults {
		if result.err != nil {
			t.Fatalf("concurrent Validate() error = %v", result.err)
		}
		if result.created {
			validationCreated++
		}
		if equivalentValidation.ID == "" {
			equivalentValidation = result.value
		} else if result.value.ID != equivalentValidation.ID {
			t.Fatalf("concurrent Validate IDs = %q and %q", equivalentValidation.ID, result.value.ID)
		}
	}
	if validationCreated != 1 || !equivalentValidation.Valid || equivalentValidation.ID == validation.ID {
		t.Fatalf("concurrent Validate created=%d value=%#v", validationCreated, equivalentValidation)
	}

	type planResult struct {
		value   application.DraftPlan
		created bool
		err     error
	}
	planStart := make(chan struct{})
	planResults := make(chan planResult, writers)
	for range writers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-planStart
			value, created, err := workflow.Plan(
				ctx, projectID, owner, "notes", draft.ID().String(), equivalentValidation.ID, 2,
			)
			planResults <- planResult{value: value, created: created, err: err}
		}()
	}
	close(planStart)
	group.Wait()
	close(planResults)
	planCreated := 0
	var equivalentPlan application.DraftPlan
	for result := range planResults {
		if result.err != nil {
			t.Fatalf("concurrent Plan() error = %v", result.err)
		}
		if result.created {
			planCreated++
		}
		if equivalentPlan.ID == "" {
			equivalentPlan = result.value
		} else if result.value.ID != equivalentPlan.ID || result.value.PlanHash != equivalentPlan.PlanHash {
			t.Fatalf("concurrent Plan identities differ: %#v / %#v", equivalentPlan, result.value)
		}
	}
	if planCreated != 1 || equivalentPlan.ID == plan.ID || equivalentPlan.PlanHash != plan.PlanHash {
		t.Fatalf("concurrent Plan created=%d value=%#v first=%#v", planCreated, equivalentPlan, plan)
	}
	if _, _, err := workflow.Plan(ctx, projectID, owner, "notes", draft.ID().String(), validation.ID, 1); !errors.Is(err, application.ErrDraftConflict) {
		t.Fatalf("Plan(stale validation) error = %v", err)
	}

	assertDraftMutationRejected(t, ctx, pool, `
		UPDATE panvara_module_draft_validation SET created_by = 'changed'
		WHERE project_id = $1 AND module_name = 'notes' AND draft_id = $2
	`, projectID.String(), draft.ID().String())
	assertDraftMutationRejected(t, ctx, pool, `
		DELETE FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = 'notes' AND draft_id = $2
	`, projectID.String(), draft.ID().String())
	assertDraftMutationRejected(t, ctx, pool, `
		UPDATE panvara_module_draft SET baseline_revision_hash = NULL
		WHERE project_id = $1 AND module_name = 'notes' AND draft_id = $2
	`, projectID.String(), draft.ID().String())
	for _, statement := range []string{
		`TRUNCATE panvara_module_draft_validation CASCADE`,
		`TRUNCATE panvara_module_draft_plan`,
	} {
		assertDraftMutationRejected(t, ctx, pool, statement)
	}
}

func assertDraftMutationRejected(t *testing.T, ctx context.Context, executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, statement string, arguments ...any) {
	t.Helper()
	_, err := executor.Exec(ctx, statement, arguments...)
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "55000" {
		t.Fatalf("mutation %q error = %v, want SQLSTATE 55000", strings.TrimSpace(statement), err)
	}
}

type integrationDraftClock struct{ at time.Time }

func (clock integrationDraftClock) Now() time.Time { return clock.at }

type integrationDraftIDGenerator struct{ calls int }

func (generator *integrationDraftIDGenerator) New(time.Time) (domain.DraftID, error) {
	values := []string{
		"01981234-5678-7abc-8def-0123456789b0", "01981234-5678-7abc-8def-0123456789b1",
		"01981234-5678-7abc-8def-0123456789b2", "01981234-5678-7abc-8def-0123456789b3",
	}
	value := values[generator.calls%len(values)]
	generator.calls++
	return domain.ParseDraftID(value)
}

func integrationDraftSource(version string) []byte {
	return []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: notes\n  version: " + version + "\nspec:\n  resources: []\n")
}
