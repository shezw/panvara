/*
   Panvara
   internal/application/appmodule/revision_registry_test.go    2026-07-15
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package appmodule

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/actor"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestRevisionRegistryBootstrapIsIdempotentAndRetainsFirstSource(t *testing.T) {
	t.Parallel()
	store := newFakeRevisionStore()
	registry := mustRevisionRegistry(t, store)
	projectID := revisionTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	firstSource := revisionTestYAML("notes", "1.0.0", "")
	firstModule := mustCompileRevisionSource(t, firstSource)
	first, created, err := registry.RegisterBootstrap(context.Background(), projectID, firstModule, firstSource, spec.FormatYAML)
	if err != nil || !created {
		t.Fatalf("first RegisterBootstrap() = created %v, error %v", created, err)
	}
	secondSource := revisionTestYAML("notes", "1.0.0", "# equivalent comment\n")
	secondModule := mustCompileRevisionSource(t, secondSource)
	second, created, err := registry.RegisterBootstrap(context.Background(), projectID, secondModule, secondSource, spec.FormatYAML)
	if err != nil || created {
		t.Fatalf("second RegisterBootstrap() = created %v, error %v", created, err)
	}
	if first.RevisionHash() != second.RevisionHash() || !bytes.Equal(second.Source(), firstSource) {
		t.Fatal("idempotent registration did not retain the first source")
	}
}

func TestRevisionRegistryQueriesEnforceApplicationOwnerBoundary(t *testing.T) {
	t.Parallel()
	store := newFakeRevisionStore()
	registry := mustRevisionRegistry(t, store)
	projectID := revisionTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	otherProject := revisionTestProject(t, "01981234-5678-7abc-8def-0123456789ac")
	owner := revisionTestActor(t, projectID, "owner", []string{"project.owner"})
	source := revisionTestYAML("notes", "1.0.0", "")
	module := mustCompileRevisionSource(t, source)
	revision, _, err := registry.RegisterBootstrap(context.Background(), projectID, module, source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	anonymous, err := actor.NewAnonymous(projectID.String())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		actor actor.Context
	}{
		{name: "zero", actor: actor.Context{}},
		{name: "anonymous", actor: anonymous},
		{name: "non-owner", actor: revisionTestActor(t, projectID, "member", []string{"project.member"})},
		{name: "other project", actor: revisionTestActor(t, otherProject, "owner", []string{"project.owner"})},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := registry.List(context.Background(), projectID, test.actor, "notes", 20); !errors.Is(err, ErrRevisionForbidden) {
				t.Fatalf("List() error = %v, want ErrRevisionForbidden", err)
			}
			if _, err := registry.Get(context.Background(), projectID, test.actor, "notes", revision.RevisionHash()); !errors.Is(err, ErrRevisionForbidden) {
				t.Fatalf("Get() error = %v, want ErrRevisionForbidden", err)
			}
			if _, err := registry.GetSource(context.Background(), projectID, test.actor, "notes", revision.RevisionHash()); !errors.Is(err, ErrRevisionForbidden) {
				t.Fatalf("GetSource() error = %v, want ErrRevisionForbidden", err)
			}
		})
	}
	values, err := registry.List(context.Background(), projectID, owner, "notes", 20)
	if err != nil || len(values) != 1 || values[0].RevisionHash() != revision.RevisionHash() {
		t.Fatalf("authorized List() = %#v, %v", values, err)
	}
	got, err := registry.Get(context.Background(), projectID, owner, "notes", revision.RevisionHash())
	if err != nil || got.RevisionHash() != revision.RevisionHash() {
		t.Fatalf("authorized Get() = %#v, %v", got, err)
	}
	gotSource, err := registry.GetSource(context.Background(), projectID, owner, "notes", revision.RevisionHash())
	if err != nil || gotSource.Format != domain.SourceFormatYAML || !bytes.Equal(gotSource.Bytes, source) {
		t.Fatalf("authorized GetSource() = %#v, %v", gotSource, err)
	}
}

func TestRevisionRegistryFailsClosedWhenStoredSourceDoesNotReproduceArtifacts(t *testing.T) {
	t.Parallel()
	store := newFakeRevisionStore()
	registry := mustRevisionRegistry(t, store)
	projectID := revisionTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	owner := revisionTestActor(t, projectID, "owner", []string{"project.owner"})
	source := revisionTestYAML("notes", "1.0.0", "")
	module := mustCompileRevisionSource(t, source)
	corrupt, err := domain.NewRevision(domain.RevisionMaterial{
		ProjectID: projectID, ModuleName: module.Name(), ModuleVersion: module.Version(),
		RevisionHash: module.RevisionHash(), DataSchemaIdentities: revisionTestDataSchemaIdentities(t, module),
		SpecVersion: spec.APIVersion,
		IRFormat:    IRFormatVersion, SourceFormat: domain.SourceFormatYAML,
		Source: []byte("not: [valid"), SourceHash: revisionHashBytes([]byte("not: [valid")),
		CanonicalIR: module.CanonicalIR(), OpenAPI: module.OpenAPI(), ManagerSchema: module.ManagerUISchema(),
		Origin: domain.RevisionOriginBootstrap, RegisteredBy: "system:bootstrap",
		RegisteredAt: time.Date(2026, 7, 15, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	store.values[revisionStoreKey(projectID, module.Name(), module.RevisionHash())] = corrupt
	if _, err := registry.Get(context.Background(), projectID, owner, module.Name(), module.RevisionHash()); !errors.Is(err, ErrRevisionCorrupt) {
		t.Fatalf("Get(corrupt) error = %v, want ErrRevisionCorrupt", err)
	}
	values, err := registry.List(context.Background(), projectID, owner, module.Name(), 20)
	if err != nil || len(values) != 1 {
		t.Fatalf("List(corrupt artifacts) = %#v, %v; metadata-only List must not recompile", values, err)
	}
}

func TestRevisionRegistryValidUnknownIdentityReturnsNotFound(t *testing.T) {
	t.Parallel()
	store := newFakeRevisionStore()
	registry := mustRevisionRegistry(t, store)
	projectID := revisionTestProject(t, "01981234-5678-7abc-8def-0123456789ab")
	owner := revisionTestActor(t, projectID, "owner", []string{"project.owner"})
	unknown := "sha256:" + strings.Repeat("f", 64)
	if _, err := registry.Get(context.Background(), projectID, owner, "notes", unknown); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("Get(valid unknown) error = %v, want ErrRevisionNotFound", err)
	}
	if _, err := registry.GetSource(context.Background(), projectID, owner, "notes", unknown); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("GetSource(valid unknown) error = %v, want ErrRevisionNotFound", err)
	}
	for _, identity := range []struct{ module, revision string }{
		{module: "Bad_Module", revision: unknown},
		{module: "notes", revision: "not-a-hash"},
	} {
		if _, err := registry.Get(context.Background(), projectID, owner, identity.module, identity.revision); !errors.Is(err, ErrRevisionInvalid) {
			t.Fatalf("Get(invalid %q/%q) error = %v, want ErrRevisionInvalid", identity.module, identity.revision, err)
		}
	}
}

type fixedRevisionClock struct{ at time.Time }

func (clock fixedRevisionClock) Now() time.Time { return clock.at }

type fakeRevisionStore struct {
	mu     sync.Mutex
	values map[string]domain.Revision
}

func newFakeRevisionStore() *fakeRevisionStore {
	return &fakeRevisionStore{values: make(map[string]domain.Revision)}
}

func (store *fakeRevisionStore) Register(_ context.Context, value domain.Revision) (domain.Revision, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	key := revisionStoreKey(value.ProjectID(), value.ModuleName(), value.RevisionHash())
	if current, found := store.values[key]; found {
		return current, false, nil
	}
	store.values[key] = value
	return value, true, nil
}

func (store *fakeRevisionStore) List(_ context.Context, projectID project.ID, module string, limit int) ([]domain.RevisionSummary, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	values := make([]domain.RevisionSummary, 0)
	for _, value := range store.values {
		if value.ProjectID().String() == projectID.String() && value.ModuleName() == module {
			values = append(values, value.Summary())
		}
	}
	sort.Slice(values, func(left, right int) bool {
		if !values[left].RegisteredAt().Equal(values[right].RegisteredAt()) {
			return values[left].RegisteredAt().After(values[right].RegisteredAt())
		}
		return values[left].RevisionHash() < values[right].RevisionHash()
	})
	if len(values) > limit {
		values = values[:limit]
	}
	return values, nil
}

func revisionTestDataSchemaIdentities(t *testing.T, module *CompiledModule) []domain.DataSchemaIdentity {
	t.Helper()
	identity, err := domain.NewDataSchemaIdentity(module.DataSchemaFormat(), module.DataSchemaFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	return []domain.DataSchemaIdentity{identity}
}

func (store *fakeRevisionStore) Get(_ context.Context, projectID project.ID, module, revision string) (domain.Revision, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, found := store.values[revisionStoreKey(projectID, module, revision)]
	if !found {
		return domain.Revision{}, ErrRevisionNotFound
	}
	return value, nil
}

func mustRevisionRegistry(t *testing.T, store RevisionStore) *RevisionRegistry {
	t.Helper()
	registry, err := NewRevisionRegistry(store, fixedRevisionClock{at: time.Date(2026, 7, 15, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func revisionTestProject(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func revisionTestActor(t *testing.T, projectID project.ID, id string, roles []string) actor.Context {
	t.Helper()
	value, err := actor.New(projectID.String(), id, roles)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func revisionStoreKey(projectID project.ID, module, revision string) string {
	return projectID.String() + "/" + module + "/" + revision
}

func revisionTestYAML(module, version, prefix string) []byte {
	return []byte(prefix + "apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: " + module + "\n  version: " + version + "\nspec:\n  resources: []\n")
}

func mustCompileRevisionSource(t *testing.T, source []byte) *CompiledModule {
	t.Helper()
	module, err := NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	return module
}
