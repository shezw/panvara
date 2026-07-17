//go:build integration

/*
   Panvara
   tests/integration/postgres_draft_generation_boundary_test.go    2026-07-17
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
	"testing"
	"time"

	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

func TestPostgresDraftGenerationBigintBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	owner, err := actor.New(projectID.String(), "generation-owner", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	draftID, err := domain.ParseDraftID("01981234-5678-7abc-8def-0123456789bf")
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := domain.NewDraftBaseline("")
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)
	initialSource := []byte("draft: before-bigint-boundary\n")
	draft, err := domain.NewDraft(domain.DraftMaterial{
		ID: draftID, ProjectID: projectID, ModuleName: "notes", Baseline: baseline,
		SourceFormat: domain.SourceFormatYAML, SourceHash: domain.DraftSourceHash(initialSource), Source: initialSource,
		Generation: domain.MaxDraftGeneration - 1, CreatedBy: owner.ActorID(), CreatedAt: createdAt,
		UpdatedBy: owner.ActorID(), UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	draftStore, err := panvarapg.NewDraftStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := draftStore.Create(
		ctx, draft, "generation-boundary", "sha256:"+strings.Repeat("f", 64),
	); err != nil || !created {
		t.Fatalf("Create(max-1 generation) = created %v, error %v", created, err)
	}
	revisionStore, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	clock := integrationDraftClock{at: createdAt.Add(time.Minute)}
	registry, err := application.NewRevisionRegistry(revisionStore, clock)
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := application.NewDraftWorkflow(draftStore, registry, clock, &integrationDraftIDGenerator{})
	if err != nil {
		t.Fatal(err)
	}

	maximumSource := []byte("draft: at-bigint-boundary\n")
	maximum, changed, err := workflow.Replace(
		ctx, projectID, owner, "notes", draftID.String(), domain.MaxDraftGeneration-1,
		application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: maximumSource},
	)
	if err != nil || !changed || maximum.Generation() != domain.MaxDraftGeneration {
		t.Fatalf("Replace(max-1 -> max) = generation %d, changed %v, error %v", maximum.Generation(), changed, err)
	}

	noOp, err := domain.NewDraftReplacement(domain.SourceFormatYAML, maximumSource, owner.ActorID(), createdAt.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	stored, changed, err := draftStore.Replace(ctx, projectID, "notes", draftID, domain.MaxDraftGeneration, noOp)
	if err != nil || changed || stored.Generation() != domain.MaxDraftGeneration {
		t.Fatalf("PostgreSQL Replace(max no-op) = generation %d, changed %v, error %v", stored.Generation(), changed, err)
	}

	overflowing, err := domain.NewDraftReplacement(
		domain.SourceFormatYAML, []byte("draft: would-overflow\n"), owner.ActorID(), createdAt.Add(3*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := draftStore.Replace(
		ctx, projectID, "notes", draftID, domain.MaxDraftGeneration, overflowing,
	); !errors.Is(err, application.ErrDraftConflict) {
		t.Fatalf("PostgreSQL Replace(max changed) error = %v, want ErrDraftConflict", err)
	}
	if _, _, err := workflow.Replace(
		ctx, projectID, owner, "notes", draftID.String(), domain.MaxDraftGeneration,
		application.ReplaceDraftInput{Format: domain.SourceFormatYAML, Source: overflowing.Source()},
	); !errors.Is(err, application.ErrDraftConflict) {
		t.Fatalf("Workflow Replace(max changed) error = %v, want ErrDraftConflict", err)
	}
}
