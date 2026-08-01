/*
   Panvara
   internal/runtime/modelruntime/runtime.go    2026-08-02
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package modelruntime prepares and atomically installs immutable model-driven
// HTTP runtime graphs without allowing requests to observe mixed revisions.
package modelruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

var (
	// ErrInvalidArgument reports an invalid runtime dependency or candidate.
	ErrInvalidArgument = errors.New("invalid model runtime argument")
	// ErrRevisionCorrupt reports a persisted Revision that does not reproduce.
	ErrRevisionCorrupt = errors.New("model runtime revision is corrupt")
	// ErrPrepare reports failure to construct a complete candidate handler.
	ErrPrepare = errors.New("model runtime preparation failed")
	// ErrStaleEpoch reports an install older than the currently served epoch.
	ErrStaleEpoch = errors.New("model runtime epoch is stale")
	// ErrConflict reports two different targets claiming the same epoch, or a
	// prepared candidate that contradicts its authoritative ActiveSnapshot.
	ErrConflict = errors.New("model runtime activation conflict")
	// ErrDegraded reports a fail-closed runtime after an activation conflict.
	ErrDegraded = errors.New("model runtime is degraded")
)

var (
	_ releaseapp.Runtime = (*Runtime)(nil)
	_ http.Handler       = (*Runtime)(nil)
)

// Runtime owns the process-local active handler. Request dispatch is lock-free;
// installation is serialized and publishes one fully built Snapshot at a time.
type Runtime struct {
	builder  Builder
	compiler *appmodule.Compiler

	installMu sync.Mutex
	current   atomic.Pointer[Snapshot]
	degraded  atomic.Bool
}

// Snapshot is one immutable, process-local activation. Its handler and target
// metadata are never mutated after publication through Runtime.current.
type Snapshot struct {
	epoch                   uint64
	runtimeRevision         string
	recordNamespaceRevision string
	moduleName              string
	handler                 http.Handler
	target                  activationTarget
}

// New constructs an empty runtime. It is not ready until an initial Snapshot
// has been prepared and installed.
func New(builder Builder) (*Runtime, error) {
	if isNilInterface(builder) {
		return nil, fmt.Errorf("%w: nil runtime builder", ErrInvalidArgument)
	}
	return &Runtime{builder: builder, compiler: appmodule.NewCompiler()}, nil
}

// Prepare recompiles all persisted source and compares every current compiler
// artifact before asking Builder to construct a complete candidate handler.
// It has no effect on the active Snapshot.
func (runtime *Runtime) Prepare(
	ctx context.Context,
	revision domainappmodule.Revision,
	recordNamespaceRevision string,
) (releaseapp.PreparedRuntime, error) {
	if runtime == nil || isNilInterface(runtime.builder) || runtime.compiler == nil {
		return nil, fmt.Errorf("%w: uninitialized runtime", ErrInvalidArgument)
	}
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil context", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.degraded.Load() {
		return nil, ErrDegraded
	}
	recordNamespaceRevision = strings.ToLower(strings.TrimSpace(recordNamespaceRevision))
	if !domainappmodule.ValidContentHash(recordNamespaceRevision) {
		return nil, fmt.Errorf("%w: invalid Record namespace revision", ErrInvalidArgument)
	}
	compiled, err := runtime.recompileAndVerify(revision)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handler, err := runtime.builder.Build(ctx, compiled, recordNamespaceRevision)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, fmt.Errorf("%w: build candidate handler: %w", ErrPrepare, err)
	}
	if isNilInterface(handler) {
		return nil, fmt.Errorf("%w: builder returned a nil handler", ErrPrepare)
	}
	return &preparedRuntime{
		owner:                   runtime,
		projectID:               revision.ProjectID().String(),
		moduleName:              compiled.Name(),
		runtimeRevision:         compiled.RevisionHash(),
		recordNamespaceRevision: recordNamespaceRevision,
		dataSchemaFormat:        compiled.DataSchemaFormat(),
		dataSchemaFingerprint:   compiled.DataSchemaFingerprint(),
		handler:                 handler,
	}, nil
}

// Install publishes a prepared handler under an authoritative ActiveSnapshot.
// Epochs must increase. An exact same-epoch target is idempotent, while a
// contradictory same-epoch target permanently degrades this process.
func (runtime *Runtime) Install(
	active domainrelease.ActiveSnapshot,
	prepared releaseapp.PreparedRuntime,
) error {
	if runtime == nil {
		return fmt.Errorf("%w: nil runtime", ErrInvalidArgument)
	}
	candidate, ok := prepared.(*preparedRuntime)
	if !ok || candidate == nil || candidate.owner != runtime || isNilInterface(candidate.handler) {
		return fmt.Errorf("%w: foreign or invalid prepared runtime", ErrInvalidArgument)
	}
	if err := active.Validate(); err != nil {
		return fmt.Errorf("%w: invalid active snapshot: %v", ErrInvalidArgument, err)
	}
	target := targetFromActive(active)

	runtime.installMu.Lock()
	defer runtime.installMu.Unlock()
	if runtime.degraded.Load() {
		return ErrDegraded
	}
	current := runtime.current.Load()
	if current != nil && !current.target.sameStream(target) {
		runtime.degraded.Store(true)
		return fmt.Errorf("%w: activation crossed the installed project, environment, or module", ErrConflict)
	}
	if current != nil && active.Epoch() < current.epoch {
		return fmt.Errorf("%w: got %d, current %d", ErrStaleEpoch, active.Epoch(), current.epoch)
	}
	if !candidate.matches(target) {
		runtime.degraded.Store(true)
		return fmt.Errorf("%w: prepared runtime contradicts active snapshot", ErrConflict)
	}
	if current != nil {
		switch {
		case active.Epoch() == current.epoch && current.target.equal(target):
			return nil
		case active.Epoch() == current.epoch:
			runtime.degraded.Store(true)
			return fmt.Errorf("%w: epoch %d names different activation targets", ErrConflict, active.Epoch())
		}
	}
	runtime.current.Store(&Snapshot{
		epoch:                   active.Epoch(),
		runtimeRevision:         candidate.runtimeRevision,
		recordNamespaceRevision: candidate.recordNamespaceRevision,
		moduleName:              candidate.moduleName,
		handler:                 candidate.handler,
		target:                  target,
	})
	return nil
}

// ServeHTTP pins one immutable Snapshot pointer for the complete request.
func (runtime *Runtime) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if runtime == nil || runtime.degraded.Load() {
		serveUnavailable(writer)
		return
	}
	snapshot := runtime.current.Load()
	if snapshot == nil {
		serveUnavailable(writer)
		return
	}
	snapshot.handler.ServeHTTP(writer, request)
}

// Ready reports whether a non-conflicting active Snapshot is installed.
func (runtime *Runtime) Ready() bool {
	return runtime != nil && !runtime.degraded.Load() && runtime.current.Load() != nil
}

// Degraded reports whether this process failed closed after an activation conflict.
func (runtime *Runtime) Degraded() bool {
	return runtime != nil && runtime.degraded.Load()
}

// FailClosed permanently rejects new work in this process. It is used when a
// database COMMIT may have advanced the authoritative epoch but its result was
// lost, so serving the previously installed Snapshot would be unsafe.
func (runtime *Runtime) FailClosed() {
	if runtime != nil {
		runtime.degraded.Store(true)
	}
}

// Current returns the currently published immutable Snapshot, or nil.
func (runtime *Runtime) Current() *Snapshot {
	if runtime == nil {
		return nil
	}
	return runtime.current.Load()
}

// Epoch returns the authoritative activation epoch of the Snapshot.
func (snapshot *Snapshot) Epoch() uint64 {
	if snapshot == nil {
		return 0
	}
	return snapshot.epoch
}

// RuntimeRevision returns the compiled AppModule revision served by the Snapshot.
func (snapshot *Snapshot) RuntimeRevision() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.runtimeRevision
}

// RecordNamespaceRevision returns the independently selected persistence namespace.
func (snapshot *Snapshot) RecordNamespaceRevision() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.recordNamespaceRevision
}

// ModuleName returns the canonical module route identity.
func (snapshot *Snapshot) ModuleName() string {
	if snapshot == nil {
		return ""
	}
	return snapshot.moduleName
}

type preparedRuntime struct {
	owner                   *Runtime
	projectID               string
	moduleName              string
	runtimeRevision         string
	recordNamespaceRevision string
	dataSchemaFormat        int
	dataSchemaFingerprint   string
	handler                 http.Handler
}

func (prepared *preparedRuntime) RuntimeRevision() string {
	if prepared == nil {
		return ""
	}
	return prepared.runtimeRevision
}

func (prepared *preparedRuntime) RecordNamespaceRevision() string {
	if prepared == nil {
		return ""
	}
	return prepared.recordNamespaceRevision
}

func (prepared *preparedRuntime) matches(target activationTarget) bool {
	return prepared != nil && prepared.projectID == target.projectID &&
		prepared.moduleName == target.moduleName && prepared.runtimeRevision == target.runtimeRevision &&
		prepared.recordNamespaceRevision == target.recordNamespaceRevision &&
		prepared.dataSchemaFormat == target.dataSchemaFormat &&
		prepared.dataSchemaFingerprint == target.dataSchemaFingerprint
}

type activationTarget struct {
	projectID               string
	environmentID           string
	origin                  string
	releaseID               string
	moduleName              string
	runtimeRevision         string
	recordNamespaceRevision string
	dataSchemaFormat        int
	dataSchemaFingerprint   string
}

func targetFromActive(active domainrelease.ActiveSnapshot) activationTarget {
	releaseID := ""
	if value, found := active.ReleaseID(); found {
		releaseID = value.String()
	}
	return activationTarget{
		projectID:               active.Scope().ProjectID().String(),
		environmentID:           active.Scope().EnvironmentID().String(),
		origin:                  string(active.Origin()),
		releaseID:               releaseID,
		moduleName:              active.ModuleName(),
		runtimeRevision:         active.RuntimeRevision(),
		recordNamespaceRevision: active.RecordNamespaceRevision(),
		dataSchemaFormat:        active.DataSchemaFormat(),
		dataSchemaFingerprint:   active.DataSchemaFingerprint(),
	}
}

func (target activationTarget) equal(other activationTarget) bool {
	return target == other
}

func (target activationTarget) sameStream(other activationTarget) bool {
	return target.projectID == other.projectID && target.environmentID == other.environmentID &&
		target.moduleName == other.moduleName
}

func (runtime *Runtime) recompileAndVerify(
	revision domainappmodule.Revision,
) (*appmodule.CompiledModule, error) {
	if !revision.ProjectID().Valid() || !domainappmodule.ValidModuleName(revision.ModuleName()) ||
		!domainappmodule.ValidContentHash(revision.RevisionHash()) {
		return nil, fmt.Errorf("%w: invalid persisted revision identity", ErrRevisionCorrupt)
	}
	format, err := sourceFormat(revision.SourceFormat())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRevisionCorrupt, err)
	}
	compiled, err := runtime.compiler.Compile(revision.Source(), format)
	if err != nil {
		return nil, fmt.Errorf("%w: recompile stored source: %v", ErrRevisionCorrupt, err)
	}
	identity, found := revision.DataSchemaIdentity(appmodule.DataSchemaFormatVersion)
	if !found {
		return nil, fmt.Errorf(
			"%w: current data schema format %d is missing",
			ErrRevisionCorrupt,
			appmodule.DataSchemaFormatVersion,
		)
	}
	if compiled.Name() != revision.ModuleName() || compiled.Version() != revision.ModuleVersion() ||
		compiled.RevisionHash() != revision.RevisionHash() ||
		compiled.DataSchemaFormat() != identity.Format() ||
		compiled.DataSchemaFingerprint() != identity.Fingerprint() ||
		revision.SpecVersion() != spec.APIVersion || revision.IRFormat() != appmodule.IRFormatVersion ||
		!bytes.Equal(compiled.CanonicalIR(), revision.CanonicalIR()) ||
		!bytes.Equal(compiled.OpenAPI(), revision.OpenAPI()) ||
		!bytes.Equal(compiled.ManagerUISchema(), revision.ManagerSchema()) {
		return nil, fmt.Errorf("%w: persisted source does not reproduce compiled artifacts", ErrRevisionCorrupt)
	}
	return compiled, nil
}

func sourceFormat(format domainappmodule.SourceFormat) (spec.Format, error) {
	switch format {
	case domainappmodule.SourceFormatJSON:
		return spec.FormatJSON, nil
	case domainappmodule.SourceFormatYAML:
		return spec.FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported persisted source format %q", format)
	}
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func serveUnavailable(writer http.ResponseWriter) {
	http.Error(writer, "service unavailable", http.StatusServiceUnavailable)
}
