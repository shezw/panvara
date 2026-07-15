//go:build integration

/*
   Panvara
   tests/integration/postgres_revision_registry_test.go    2026-07-15
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
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestPostgresRevisionRegistryIsImmutableAndFailsClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	databaseURL := integrationDatabaseURL(t, ctx)
	pool := isolatedPool(t, ctx, databaseURL)
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() first error = %v", err)
	}
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() repeated error = %v", err)
	}
	store, err := panvarapg.NewRevisionStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registeredAt := time.Date(2026, 7, 15, 4, 0, 0, 0, time.UTC)
	registry, err := application.NewRevisionRegistry(store, integrationRevisionClock{at: registeredAt})
	if err != nil {
		t.Fatal(err)
	}
	projectID := mustProjectID(t, "01981234-5678-7abc-8def-0123456789ab")
	otherProject := mustProjectID(t, "01981234-5678-7abc-8def-0123456789b0")
	owner, err := actor.New(projectID.String(), "integration-owner", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	firstSource := revisionRegistrySource("")
	firstModule, err := application.NewCompiler().Compile(firstSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	first, created, err := registry.RegisterBootstrap(ctx, projectID, firstModule, firstSource, spec.FormatYAML)
	if err != nil || !created {
		t.Fatalf("first RegisterBootstrap() = created %v, error %v", created, err)
	}
	equivalentSource := revisionRegistrySource("# equivalent source must not replace provenance\n")
	equivalentModule, err := application.NewCompiler().Compile(equivalentSource, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	second, created, err := registry.RegisterBootstrap(ctx, projectID, equivalentModule, equivalentSource, spec.FormatYAML)
	if err != nil || created {
		t.Fatalf("idempotent RegisterBootstrap() = created %v, error %v", created, err)
	}
	if second.RegisteredAt() != registeredAt || !bytes.Equal(second.Source(), firstSource) ||
		second.SourceHash() != first.SourceHash() {
		t.Fatal("idempotent registration replaced first provenance")
	}

	otherRegistry, err := application.NewRevisionRegistry(store, integrationRevisionClock{at: registeredAt.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	otherRevision, created, err := otherRegistry.RegisterBootstrap(
		ctx, otherProject, firstModule, firstSource, spec.FormatYAML,
	)
	if err != nil || !created || otherRevision.RevisionHash() != first.RevisionHash() {
		t.Fatalf("other project RegisterBootstrap() = created %v revision %q error %v", created, otherRevision.RevisionHash(), err)
	}

	formatTwo, err := domain.NewDataSchemaIdentity(2, "sha256:"+strings.Repeat("2", 64))
	if err != nil {
		t.Fatal(err)
	}
	withFormatTwo := revisionWithDataSchemaIdentities(
		t, first, equivalentSource, []domain.DataSchemaIdentity{formatTwo}, registeredAt.Add(2*time.Hour),
	)
	expanded, created, err := store.Register(ctx, withFormatTwo)
	if err != nil || created {
		t.Fatalf("Register(format 2) = created %v error %v", created, err)
	}
	identities := expanded.DataSchemaIdentities()
	if len(identities) != 2 || identities[0].Format() != 1 || identities[1].Format() != 2 ||
		expanded.RegisteredAt() != registeredAt || !bytes.Equal(expanded.Source(), firstSource) {
		t.Fatalf("expanded revision identities/provenance = %#v / %s / %q", identities, expanded.RegisteredAt(), expanded.Source())
	}
	assertRevisionRowCounts(t, ctx, pool, projectID.String(), first.RevisionHash(), 1, 2)

	conflictingFormatTwo, err := domain.NewDataSchemaIdentity(2, "sha256:"+strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	conflicting := revisionWithDataSchemaIdentities(
		t, first, equivalentSource, []domain.DataSchemaIdentity{conflictingFormatTwo}, registeredAt.Add(3*time.Hour),
	)
	if _, _, err := store.Register(ctx, conflicting); !errors.Is(err, application.ErrRevisionCorrupt) {
		t.Fatalf("Register(conflicting format 2) error = %v, want ErrRevisionCorrupt", err)
	}
	assertRevisionRowCounts(t, ctx, pool, projectID.String(), first.RevisionHash(), 1, 2)

	formatFour, err := domain.NewDataSchemaIdentity(4, "sha256:"+strings.Repeat("4", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.Register(ctx, revisionWithDataSchemaIdentities(
		t, otherRevision, firstSource, []domain.DataSchemaIdentity{formatFour}, registeredAt.Add(5*time.Hour),
	)); err != nil || created {
		t.Fatalf("Register(other project format 4) = created %v error %v", created, err)
	}
	formatThree, err := domain.NewDataSchemaIdentity(3, "sha256:"+strings.Repeat("3", 64))
	if err != nil {
		t.Fatal(err)
	}
	conflictingFormatFour, err := domain.NewDataSchemaIdentity(4, "sha256:"+strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	rollbackCandidate := revisionWithDataSchemaIdentities(
		t, otherRevision, firstSource,
		[]domain.DataSchemaIdentity{formatThree, conflictingFormatFour}, registeredAt.Add(6*time.Hour),
	)
	if _, _, err := store.Register(ctx, rollbackCandidate); !errors.Is(err, application.ErrRevisionCorrupt) {
		t.Fatalf("Register(insert format 3 then conflict format 4) error = %v, want ErrRevisionCorrupt", err)
	}
	assertRevisionRowCounts(t, ctx, pool, otherProject.String(), first.RevisionHash(), 1, 2)
	var rolledBackFormatCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_module_revision_data_schema
		WHERE project_id = $1 AND module_name = 'crm.leads' AND revision_hash = $2
		  AND data_schema_format = 3
	`, otherProject.String(), first.RevisionHash()).Scan(&rolledBackFormatCount); err != nil {
		t.Fatal(err)
	}
	if rolledBackFormatCount != 0 {
		t.Fatalf("rolled-back format 3 row count = %d, want 0", rolledBackFormatCount)
	}
	verified, err := registry.Get(ctx, projectID, owner, "crm.leads", first.RevisionHash())
	if err != nil || len(verified.DataSchemaIdentities()) != 2 {
		t.Fatalf("Get(format 1 with unknown format 2) = %#v, %v", verified.DataSchemaIdentities(), err)
	}
	values, err := registry.List(ctx, projectID, owner, "crm.leads", 100)
	if err != nil || len(values) != 1 || len(values[0].DataSchemaIdentities()) != 2 ||
		values[0].DataSchemaIdentities()[0].Format() != 1 || values[0].DataSchemaIdentities()[1].Format() != 2 {
		t.Fatalf("List() = %d revisions, %v", len(values), err)
	}
	otherOwner, err := actor.New(otherProject.String(), "other-owner", []string{"project.owner"})
	if err != nil {
		t.Fatal(err)
	}
	otherValues, err := registry.List(ctx, otherProject, otherOwner, "crm.leads", 100)
	if err != nil || len(otherValues) != 1 || otherValues[0].RevisionHash() != first.RevisionHash() {
		t.Fatalf("other project List() = %#v, %v", otherValues, err)
	}
	if got, err := store.Get(ctx, otherProject, "crm.leads", first.RevisionHash()); err != nil ||
		got.ProjectID().String() != otherProject.String() {
		t.Fatalf("other project Get() = %#v, %v", got, err)
	}
	unknown := "sha256:" + strings.Repeat("a", 64)
	if _, err := registry.Get(ctx, projectID, owner, "crm.leads", unknown); !errors.Is(err, application.ErrRevisionNotFound) {
		t.Fatalf("Get(valid unknown) error = %v, want ErrRevisionNotFound", err)
	}

	concurrentProject := mustProjectID(t, "01981234-5678-7abc-8def-0123456789b1")
	concurrentRegistry, err := application.NewRevisionRegistry(
		store, integrationRevisionClock{at: registeredAt.Add(4 * time.Hour)},
	)
	if err != nil {
		t.Fatal(err)
	}
	const writers = 8
	start := make(chan struct{})
	type registrationResult struct {
		revision domain.Revision
		created  bool
		err      error
	}
	results := make(chan registrationResult, writers)
	var group sync.WaitGroup
	for writer := range writers {
		source := firstSource
		module := firstModule
		if writer%2 == 1 {
			source = equivalentSource
			module = equivalentModule
		}
		group.Add(1)
		go func(source []byte, module *application.CompiledModule) {
			defer group.Done()
			<-start
			revision, created, err := concurrentRegistry.RegisterBootstrap(
				ctx, concurrentProject, module, source, spec.FormatYAML,
			)
			results <- registrationResult{revision: revision, created: created, err: err}
		}(source, module)
	}
	close(start)
	group.Wait()
	close(results)
	createdCount := 0
	var winner domain.Revision
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent RegisterBootstrap() error = %v", result.err)
		}
		if result.created {
			createdCount++
		}
		if winner.RevisionHash() == "" {
			winner = result.revision
			continue
		}
		if result.revision.RevisionHash() != winner.RevisionHash() ||
			result.revision.SourceHash() != winner.SourceHash() ||
			result.revision.RegisteredAt() != winner.RegisteredAt() ||
			!bytes.Equal(result.revision.Source(), winner.Source()) {
			t.Fatalf("concurrent registration returned different winners: got %#v want %#v", result.revision, winner)
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent RegisterBootstrap() created count = %d, want 1", createdCount)
	}
	if !bytes.Equal(winner.Source(), firstSource) && !bytes.Equal(winner.Source(), equivalentSource) {
		t.Fatalf("concurrent winner source = %q, want one submitted source", winner.Source())
	}
	assertRevisionRowCounts(t, ctx, pool, concurrentProject.String(), first.RevisionHash(), 1, 1)

	mutations := []struct {
		statement string
		arguments []any
	}{
		{`UPDATE panvara_module_revision SET registered_by = 'changed' WHERE project_id = $1 AND module_name = $2`, []any{projectID.String(), "crm.leads"}},
		{`DELETE FROM panvara_module_revision WHERE project_id = $1 AND module_name = $2`, []any{projectID.String(), "crm.leads"}},
		{`UPDATE panvara_module_revision_data_schema SET data_schema_fingerprint = $4 WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3`, []any{projectID.String(), "crm.leads", first.RevisionHash(), "sha256:" + strings.Repeat("e", 64)}},
		{`DELETE FROM panvara_module_revision_data_schema WHERE project_id = $1 AND module_name = $2`, []any{projectID.String(), "crm.leads"}},
	}
	for _, mutation := range mutations {
		_, err := pool.Exec(ctx, mutation.statement, mutation.arguments...)
		var postgresError *pgconn.PgError
		if !errors.As(err, &postgresError) || postgresError.Code != "55000" {
			t.Fatalf("immutable mutation %q error = %v, want SQLSTATE 55000", mutation.statement, err)
		}
		assertRevisionRowCounts(t, ctx, pool, projectID.String(), first.RevisionHash(), 1, 2)
	}
	for _, statement := range []string{
		`TRUNCATE panvara_module_revision CASCADE`,
		`TRUNCATE panvara_module_revision_data_schema`,
	} {
		_, err := pool.Exec(ctx, statement)
		var postgresError *pgconn.PgError
		if !errors.As(err, &postgresError) || postgresError.Code != "55000" {
			t.Fatalf("immutable truncate %q error = %v, want SQLSTATE 55000", statement, err)
		}
		assertRevisionRowCounts(t, ctx, pool, projectID.String(), first.RevisionHash(), 1, 2)
	}

	if _, err := pool.Exec(ctx, `ALTER TABLE panvara_module_revision DISABLE TRIGGER panvara_module_revision_immutable_rows`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_module_revision
		SET canonical_ir_bytes = $4
		WHERE project_id = $1 AND module_name = $2 AND revision_hash = $3
	`, projectID.String(), "crm.leads", first.RevisionHash(), []byte(`{"corrupt":true}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE panvara_module_revision ENABLE TRIGGER panvara_module_revision_immutable_rows`); err != nil {
		t.Fatal(err)
	}
	metadata, err := registry.List(ctx, projectID, owner, "crm.leads", 100)
	if err != nil || len(metadata) != 1 {
		t.Fatalf("metadata-only List(corrupt parent artifact) = %#v, %v", metadata, err)
	}
	if _, err := registry.Get(ctx, projectID, owner, "crm.leads", first.RevisionHash()); !errors.Is(err, application.ErrRevisionCorrupt) {
		t.Fatalf("Get(corrupt) error = %v, want ErrRevisionCorrupt", err)
	}
}

type integrationRevisionClock struct{ at time.Time }

func (clock integrationRevisionClock) Now() time.Time { return clock.at }

func revisionWithDataSchemaIdentities(
	t *testing.T,
	base domain.Revision,
	source []byte,
	identities []domain.DataSchemaIdentity,
	registeredAt time.Time,
) domain.Revision {
	t.Helper()
	value, err := domain.NewRevision(domain.RevisionMaterial{
		ProjectID: base.ProjectID(), ModuleName: base.ModuleName(), ModuleVersion: base.ModuleVersion(),
		RevisionHash: base.RevisionHash(), DataSchemaIdentities: identities,
		SpecVersion: base.SpecVersion(), IRFormat: base.IRFormat(), SourceFormat: base.SourceFormat(),
		SourceHash: integrationRevisionHash(source), Source: source, CanonicalIR: base.CanonicalIR(),
		OpenAPI: base.OpenAPI(), ManagerSchema: base.ManagerSchema(), Origin: base.Origin(),
		RegisteredBy: base.RegisteredBy(), RegisteredAt: registeredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func assertRevisionRowCounts(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	projectID string,
	revisionHash string,
	wantParent int,
	wantChildren int,
) {
	t.Helper()
	var parentCount, childCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_module_revision
		WHERE project_id = $1 AND module_name = 'crm.leads' AND revision_hash = $2
	`, projectID, revisionHash).Scan(&parentCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_module_revision_data_schema
		WHERE project_id = $1 AND module_name = 'crm.leads' AND revision_hash = $2
	`, projectID, revisionHash).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	if parentCount != wantParent || childCount != wantChildren {
		t.Fatalf("revision row counts = parent %d child %d, want %d/%d", parentCount, childCount, wantParent, wantChildren)
	}
}

func integrationRevisionHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func revisionRegistrySource(prefix string) []byte {
	return []byte(prefix + `apiVersion: panvara.dev/v1alpha1
kind: AppModule
metadata:
  name: crm.leads
  version: 1.0.0
spec:
  resources: []
`)
}

var _ application.RevisionClock = integrationRevisionClock{}
