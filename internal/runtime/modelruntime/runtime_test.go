/*
   Panvara
   internal/runtime/modelruntime/runtime_test.go    2026-08-02
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package modelruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func TestRuntimePrepareVerifiesRevisionAndBuildsInertHandler(t *testing.T) {
	t.Parallel()
	revision := modelRuntimeRevision(t, "1.0.0", nil)
	namespace := modelRuntimeHash("a")
	var buildCalls atomic.Int32
	runtime := mustModelRuntime(t, BuilderFunc(func(
		_ context.Context,
		module *appmodule.CompiledModule,
		gotNamespace string,
	) (http.Handler, error) {
		buildCalls.Add(1)
		if module.RevisionHash() != revision.RevisionHash() || gotNamespace != namespace {
			t.Fatalf("Build() target = %s/%s", module.RevisionHash(), gotNamespace)
		}
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(module.Version() + "/" + gotNamespace))
		}), nil
	}))

	prepared, err := runtime.Prepare(context.Background(), revision, namespace)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if prepared.RuntimeRevision() != revision.RevisionHash() ||
		prepared.RecordNamespaceRevision() != namespace {
		t.Fatalf("Prepare() identity = %s/%s", prepared.RuntimeRevision(), prepared.RecordNamespaceRevision())
	}
	if calls := buildCalls.Load(); calls != 1 {
		t.Fatalf("Build() calls = %d, want 1", calls)
	}
	if runtime.Ready() || runtime.Current() != nil {
		t.Fatal("Prepare() changed active runtime")
	}
	before := httptest.NewRecorder()
	runtime.ServeHTTP(before, httptest.NewRequest(http.MethodGet, "/", nil))
	if before.Code != http.StatusServiceUnavailable {
		t.Fatalf("ServeHTTP(before install) status = %d", before.Code)
	}

	active := modelRuntimeActive(t, revision, namespace, 17)
	if err := runtime.Install(active, prepared); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !runtime.Ready() || runtime.Degraded() {
		t.Fatalf("installed readiness = %v, degraded = %v", runtime.Ready(), runtime.Degraded())
	}
	current := runtime.Current()
	if current == nil || current.Epoch() != 17 || current.ModuleName() != revision.ModuleName() ||
		current.RuntimeRevision() != revision.RevisionHash() || current.RecordNamespaceRevision() != namespace {
		t.Fatalf("Current() = %#v", current)
	}
	after := httptest.NewRecorder()
	runtime.ServeHTTP(after, httptest.NewRequest(http.MethodGet, "/", nil))
	if after.Code != http.StatusOK || after.Body.String() != "1.0.0/"+namespace {
		t.Fatalf("ServeHTTP(after install) = %d %q", after.Code, after.Body.String())
	}
}

func TestRuntimePrepareFailsClosedOnCorruptArtifacts(t *testing.T) {
	t.Parallel()
	revision := modelRuntimeRevision(t, "1.0.0", func(material *domainappmodule.RevisionMaterial) {
		material.OpenAPI = []byte(`{"openapi":"3.1.0","info":{"title":"corrupt","version":"1.0.0"}}`)
	})
	var buildCalls atomic.Int32
	runtime := mustModelRuntime(t, BuilderFunc(func(
		context.Context,
		*appmodule.CompiledModule,
		string,
	) (http.Handler, error) {
		buildCalls.Add(1)
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil
	}))
	if _, err := runtime.Prepare(context.Background(), revision, modelRuntimeHash("a")); !errors.Is(err, ErrRevisionCorrupt) {
		t.Fatalf("Prepare(corrupt artifacts) error = %v, want ErrRevisionCorrupt", err)
	}
	if calls := buildCalls.Load(); calls != 0 {
		t.Fatalf("Build() calls after corrupt revision = %d, want 0", calls)
	}
}

func TestRuntimePrepareRejectsInvalidInputsAndBuilderResults(t *testing.T) {
	t.Parallel()
	revision := modelRuntimeRevision(t, "1.0.0", nil)
	namespace := modelRuntimeHash("a")
	var nilBuilder BuilderFunc
	if _, err := New(nilBuilder); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("New(typed nil) error = %v, want ErrInvalidArgument", err)
	}

	var calls atomic.Int32
	runtime := mustModelRuntime(t, BuilderFunc(func(
		context.Context,
		*appmodule.CompiledModule,
		string,
	) (http.Handler, error) {
		calls.Add(1)
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil
	}))
	if _, err := runtime.Prepare(context.Background(), revision, "invalid"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare(invalid namespace) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := runtime.Prepare(context.Background(), domainappmodule.Revision{}, namespace); !errors.Is(err, ErrRevisionCorrupt) {
		t.Fatalf("Prepare(zero revision) error = %v, want ErrRevisionCorrupt", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.Prepare(cancelled, revision, namespace); !errors.Is(err, context.Canceled) {
		t.Fatalf("Prepare(cancelled) error = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("Build() calls for rejected inputs = %d, want 0", calls.Load())
	}

	buildError := mustModelRuntime(t, BuilderFunc(func(
		context.Context,
		*appmodule.CompiledModule,
		string,
	) (http.Handler, error) {
		return nil, errors.New("dependency unavailable")
	}))
	if _, err := buildError.Prepare(context.Background(), revision, namespace); !errors.Is(err, ErrPrepare) {
		t.Fatalf("Prepare(builder error) error = %v, want ErrPrepare", err)
	}
	typedNil := mustModelRuntime(t, BuilderFunc(func(
		context.Context,
		*appmodule.CompiledModule,
		string,
	) (http.Handler, error) {
		var handler *nilHTTPHandler
		return handler, nil
	}))
	if _, err := typedNil.Prepare(context.Background(), revision, namespace); !errors.Is(err, ErrPrepare) {
		t.Fatalf("Prepare(typed nil handler) error = %v, want ErrPrepare", err)
	}
}

func TestRuntimeInstallEpochRules(t *testing.T) {
	t.Parallel()
	revision := modelRuntimeRevision(t, "1.0.0", nil)
	namespace := modelRuntimeHash("a")
	var handlerNumber atomic.Int32
	runtime := mustModelRuntime(t, BuilderFunc(func(
		context.Context,
		*appmodule.CompiledModule,
		string,
	) (http.Handler, error) {
		number := handlerNumber.Add(1)
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(writer, "handler-%d", number)
		}), nil
	}))
	first := mustPrepare(t, runtime, revision, namespace)
	second := mustPrepare(t, runtime, revision, namespace)
	active := modelRuntimeActive(t, revision, namespace, 10)
	if err := runtime.Install(active, first); err != nil {
		t.Fatal(err)
	}
	installed := runtime.Current()
	if err := runtime.Install(active, second); err != nil {
		t.Fatalf("Install(idempotent) error = %v", err)
	}
	if runtime.Current() != installed {
		t.Fatal("same epoch and target replaced the installed Snapshot")
	}
	response := httptest.NewRecorder()
	runtime.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Body.String() != "handler-1" {
		t.Fatalf("idempotent handler response = %q", response.Body.String())
	}
	if err := runtime.Install(modelRuntimeActive(t, revision, namespace, 9), second); !errors.Is(err, ErrStaleEpoch) {
		t.Fatalf("Install(stale) error = %v, want ErrStaleEpoch", err)
	}
	if runtime.Current() != installed || !runtime.Ready() || runtime.Degraded() {
		t.Fatal("stale install changed or degraded the active Snapshot")
	}
}

func TestRuntimeInstallConflictDegradesAndFailsClosed(t *testing.T) {
	t.Parallel()
	firstRevision := modelRuntimeRevision(t, "1.0.0", nil)
	secondRevision := modelRuntimeRevision(t, "1.1.0", nil)
	namespace := modelRuntimeHash("a")
	runtime := mustModelRuntime(t, responseModelRuntimeBuilder())
	first := mustPrepare(t, runtime, firstRevision, namespace)
	second := mustPrepare(t, runtime, secondRevision, namespace)
	firstActive := modelRuntimeActive(t, firstRevision, namespace, 12)
	secondActive := modelRuntimeActive(t, secondRevision, namespace, 12)
	if err := runtime.Install(firstActive, first); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Install(secondActive, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("Install(conflict) error = %v, want ErrConflict", err)
	}
	if !runtime.Degraded() || runtime.Ready() {
		t.Fatalf("conflict readiness = %v, degraded = %v", runtime.Ready(), runtime.Degraded())
	}
	response := httptest.NewRecorder()
	runtime.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("ServeHTTP(degraded) status = %d", response.Code)
	}
	if err := runtime.Install(firstActive, first); !errors.Is(err, ErrDegraded) {
		t.Fatalf("Install(after conflict) error = %v, want ErrDegraded", err)
	}
}

func TestRuntimeInstallRejectsPreparedSnapshotMismatch(t *testing.T) {
	t.Parallel()
	firstRevision := modelRuntimeRevision(t, "1.0.0", nil)
	secondRevision := modelRuntimeRevision(t, "1.1.0", nil)
	namespace := modelRuntimeHash("a")
	runtime := mustModelRuntime(t, responseModelRuntimeBuilder())
	prepared := mustPrepare(t, runtime, firstRevision, namespace)
	if err := runtime.Install(modelRuntimeActive(t, secondRevision, namespace, 1), prepared); !errors.Is(err, ErrConflict) {
		t.Fatalf("Install(mismatched prepared) error = %v, want ErrConflict", err)
	}
	if !runtime.Degraded() || runtime.Current() != nil {
		t.Fatal("mismatched prepared candidate did not fail closed")
	}
}

func TestRuntimeInstallRejectsCrossEnvironmentEpoch(t *testing.T) {
	t.Parallel()
	revision := modelRuntimeRevision(t, "1.0.0", nil)
	namespace := modelRuntimeHash("a")
	runtime := mustModelRuntime(t, responseModelRuntimeBuilder())
	prepared := mustPrepare(t, runtime, revision, namespace)
	if err := runtime.Install(modelRuntimeActive(t, revision, namespace, 1), prepared); err != nil {
		t.Fatal(err)
	}
	other := modelRuntimeActiveInEnvironment(
		t,
		revision,
		namespace,
		2,
		"01981234-5678-7abc-8def-0123456789ff",
	)
	if err := runtime.Install(other, prepared); !errors.Is(err, ErrConflict) {
		t.Fatalf("Install(other environment) error = %v, want ErrConflict", err)
	}
	if !runtime.Degraded() || runtime.Ready() {
		t.Fatal("cross-environment activation did not fail closed")
	}
}

func TestRuntimeRequestPinsSnapshotAcrossInstall(t *testing.T) {
	t.Parallel()
	firstRevision := modelRuntimeRevision(t, "1.0.0", nil)
	secondRevision := modelRuntimeRevision(t, "1.1.0", nil)
	namespace := modelRuntimeHash("a")
	started := make(chan struct{})
	release := make(chan struct{})
	var startOnce sync.Once
	runtime := mustModelRuntime(t, BuilderFunc(func(
		_ context.Context,
		module *appmodule.CompiledModule,
		_ string,
	) (http.Handler, error) {
		version := module.Version()
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			if version == "1.0.0" {
				startOnce.Do(func() { close(started) })
				<-release
			}
			_, _ = writer.Write([]byte(version))
		}), nil
	}))
	first := mustPrepare(t, runtime, firstRevision, namespace)
	second := mustPrepare(t, runtime, secondRevision, namespace)
	if err := runtime.Install(modelRuntimeActive(t, firstRevision, namespace, 1), first); err != nil {
		t.Fatal(err)
	}

	firstResponse := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.ServeHTTP(firstResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not reach its pinned handler")
	}
	if err := runtime.Install(modelRuntimeActive(t, secondRevision, namespace, 2), second); err != nil {
		t.Fatalf("Install(second) error = %v", err)
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("first request did not complete")
	}
	if firstResponse.Body.String() != "1.0.0" {
		t.Fatalf("in-flight response = %q, want first Snapshot", firstResponse.Body.String())
	}
	secondResponse := httptest.NewRecorder()
	runtime.ServeHTTP(secondResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if secondResponse.Body.String() != "1.1.0" {
		t.Fatalf("next response = %q, want second Snapshot", secondResponse.Body.String())
	}
}

func mustModelRuntime(t *testing.T, builder Builder) *Runtime {
	t.Helper()
	runtime, err := New(builder)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return runtime
}

func mustPrepare(
	t *testing.T,
	runtime *Runtime,
	revision domainappmodule.Revision,
	namespace string,
) releaseapp.PreparedRuntime {
	t.Helper()
	prepared, err := runtime.Prepare(context.Background(), revision, namespace)
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	return prepared
}

func responseModelRuntimeBuilder() Builder {
	return BuilderFunc(func(
		_ context.Context,
		module *appmodule.CompiledModule,
		namespace string,
	) (http.Handler, error) {
		response := module.Version() + "/" + namespace
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			_, _ = writer.Write([]byte(response))
		}), nil
	})
}

func modelRuntimeRevision(
	t *testing.T,
	version string,
	mutate func(*domainappmodule.RevisionMaterial),
) domainappmodule.Revision {
	t.Helper()
	source := []byte(
		"apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n" +
			"  name: runtime.test\n  version: " + version + "\nspec:\n  resources: []\n",
	)
	compiled, err := appmodule.NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	projectID, err := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domainappmodule.NewDataSchemaIdentity(
		compiled.DataSchemaFormat(),
		compiled.DataSchemaFingerprint(),
	)
	if err != nil {
		t.Fatal(err)
	}
	material := domainappmodule.RevisionMaterial{
		ProjectID: projectID, ModuleName: compiled.Name(), ModuleVersion: compiled.Version(),
		RevisionHash: compiled.RevisionHash(), DataSchemaIdentities: []domainappmodule.DataSchemaIdentity{identity},
		SpecVersion: spec.APIVersion,
		IRFormat:    appmodule.IRFormatVersion, SourceFormat: domainappmodule.SourceFormatYAML,
		SourceHash: modelRuntimeBytesHash(source), Source: source, CanonicalIR: compiled.CanonicalIR(),
		OpenAPI: compiled.OpenAPI(), ManagerSchema: compiled.ManagerUISchema(),
		Origin: domainappmodule.RevisionOriginBootstrap, RegisteredBy: "system:bootstrap",
		RegisteredAt: time.Date(2026, time.August, 2, 0, 0, 0, 0, time.UTC),
	}
	if mutate != nil {
		mutate(&material)
	}
	revision, err := domainappmodule.NewRevision(material)
	if err != nil {
		t.Fatalf("NewRevision() error = %v", err)
	}
	return revision
}

func modelRuntimeActive(
	t *testing.T,
	revision domainappmodule.Revision,
	namespace string,
	epoch uint64,
) domainrelease.ActiveSnapshot {
	t.Helper()
	return modelRuntimeActiveInEnvironment(
		t,
		revision,
		namespace,
		epoch,
		"01981234-5678-7abc-8def-0123456789fe",
	)
}

func modelRuntimeActiveInEnvironment(
	t *testing.T,
	revision domainappmodule.Revision,
	namespace string,
	epoch uint64,
	environment string,
) domainrelease.ActiveSnapshot {
	t.Helper()
	identity, found := revision.DataSchemaIdentity(appmodule.DataSchemaFormatVersion)
	if !found {
		t.Fatal("test revision is missing current data schema identity")
	}
	binding, err := domainrelease.NewModuleBinding(domainrelease.ModuleBindingMaterial{
		ModuleName: revision.ModuleName(), RuntimeRevision: revision.RevisionHash(),
		RecordNamespaceRevision: namespace, DataSchemaFormat: identity.Format(),
		DataSchemaFingerprint: identity.Fingerprint(),
	})
	if err != nil {
		t.Fatalf("NewModuleBinding() error = %v", err)
	}
	environmentID, err := project.ParseEnvironmentID(environment)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(revision.ProjectID(), environmentID)
	if err != nil {
		t.Fatal(err)
	}
	active, err := domainrelease.NewActiveSnapshot(domainrelease.ActiveSnapshotMaterial{
		Scope: scope, Epoch: epoch, Origin: domainrelease.ActiveSnapshotOriginBootstrap,
		Binding: binding, ActivatedBy: "system:bootstrap", RequestID: "system:bootstrap",
		ActivatedAt: time.Date(2026, time.August, 2, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NewActiveSnapshot() error = %v", err)
	}
	return active
}

func modelRuntimeHash(value string) string {
	return "sha256:" + strings.Repeat(value, 64)
}

func modelRuntimeBytesHash(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

type nilHTTPHandler struct{}

func (*nilHTTPHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}
