/*
   Panvara
   internal/application/appmodule/revision_registry.go    2026-07-15
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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

const (
	// DefaultRevisionListLimit bounds a registry query when no limit is selected.
	DefaultRevisionListLimit = 20
	// MaxRevisionListLimit is the largest registry page accepted by application.
	MaxRevisionListLimit = 100
)

var (
	// ErrRevisionInvalid reports malformed registry input.
	ErrRevisionInvalid = errors.New("invalid module revision request")
	// ErrRevisionForbidden aliases the shared access denial for compatibility.
	ErrRevisionForbidden = access.ErrForbidden
	// ErrRevisionNotFound reports an absent project/module/revision fact.
	ErrRevisionNotFound = errors.New("module revision not found")
	// ErrRevisionCorrupt reports persisted bytes that fail identity verification.
	ErrRevisionCorrupt = errors.New("module revision registry is corrupt")
)

// RevisionStore is the append-only persistence port consumed by RevisionRegistry.
// It intentionally exposes no update or delete operation.
type RevisionStore interface {
	Register(context.Context, domain.Revision) (domain.Revision, bool, error)
	List(context.Context, project.ID, string, int) ([]domain.RevisionSummary, error)
	Get(context.Context, project.ID, string, string) (domain.Revision, error)
}

// RevisionClock supplies deterministic registration timestamps in tests.
type RevisionClock interface {
	Now() time.Time
}

// SystemRevisionClock reads the host clock for bootstrap registration.
type SystemRevisionClock struct{}

// Now returns the current UTC-compatible timestamp.
func (SystemRevisionClock) Now() time.Time { return time.Now() }

// RevisionSource is a verified defensive source response.
type RevisionSource struct {
	Format domain.SourceFormat
	Hash   string
	Bytes  []byte
}

// RevisionRegistry coordinates bootstrap registration and authorized, verified
// read access without owning publication or activation state.
type RevisionRegistry struct {
	store      RevisionStore
	authorizer access.Authorizer
	clock      RevisionClock
	compiler   *Compiler
}

// NewRevisionRegistry constructs the immutable revision use-case boundary.
func NewRevisionRegistry(
	store RevisionStore,
	authorizer access.Authorizer,
	clock RevisionClock,
) (*RevisionRegistry, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: nil revision store", ErrRevisionInvalid)
	}
	if authorizer == nil {
		return nil, fmt.Errorf("%w: nil revision authorizer", ErrRevisionInvalid)
	}
	if clock == nil {
		return nil, fmt.Errorf("%w: nil revision clock", ErrRevisionInvalid)
	}
	return &RevisionRegistry{
		store: store, authorizer: authorizer, clock: clock, compiler: NewCompiler(),
	}, nil
}

// RegisterBootstrap registers the currently compiled Server source as an
// immutable bootstrap fact. Equivalent sources never replace the first source.
// This composition-root-only system path intentionally bypasses user
// authorization and must never be exposed through an interface adapter.
func (registry *RevisionRegistry) RegisterBootstrap(
	ctx context.Context,
	projectID project.ID,
	current *CompiledModule,
	source []byte,
	format spec.Format,
) (domain.Revision, bool, error) {
	if registry == nil || registry.store == nil || registry.clock == nil || registry.compiler == nil {
		return domain.Revision{}, false, fmt.Errorf("%w: uninitialized revision registry", ErrRevisionInvalid)
	}
	if err := ctx.Err(); err != nil {
		return domain.Revision{}, false, err
	}
	if !projectID.Valid() || current == nil {
		return domain.Revision{}, false, fmt.Errorf("%w: invalid project or compiled module", ErrRevisionInvalid)
	}
	recompiled, err := registry.compiler.Compile(source, format)
	if err != nil {
		return domain.Revision{}, false, fmt.Errorf("%w: recompile bootstrap source: %v", ErrRevisionInvalid, err)
	}
	if !sameCompiledModule(current, recompiled) {
		return domain.Revision{}, false, fmt.Errorf("%w: bootstrap source does not match runtime module", ErrRevisionCorrupt)
	}
	sourceFormat, err := makeDomainSourceFormat(format)
	if err != nil {
		return domain.Revision{}, false, err
	}
	dataSchemaIdentity, err := domain.NewDataSchemaIdentity(
		current.DataSchemaFormat(), current.DataSchemaFingerprint(),
	)
	if err != nil {
		return domain.Revision{}, false, fmt.Errorf("%w: compiler data schema identity: %v", ErrRevisionCorrupt, err)
	}
	revision, err := domain.NewRevision(domain.RevisionMaterial{
		ProjectID: projectID, ModuleName: current.Name(), ModuleVersion: current.Version(),
		RevisionHash: current.RevisionHash(), DataSchemaIdentities: []domain.DataSchemaIdentity{dataSchemaIdentity},
		SpecVersion: spec.APIVersion,
		IRFormat:    IRFormatVersion, SourceFormat: sourceFormat, SourceHash: revisionHashBytes(source),
		Source: append([]byte(nil), source...), CanonicalIR: current.CanonicalIR(), OpenAPI: current.OpenAPI(),
		ManagerSchema: current.ManagerUISchema(), Origin: domain.RevisionOriginBootstrap,
		RegisteredBy: "system:bootstrap", RegisteredAt: registry.clock.Now().UTC(),
	})
	if err != nil {
		return domain.Revision{}, false, fmt.Errorf("%w: construct bootstrap revision: %v", ErrRevisionInvalid, err)
	}
	stored, created, err := registry.store.Register(ctx, revision)
	if err != nil {
		return domain.Revision{}, false, fmt.Errorf("register bootstrap module revision: %w", err)
	}
	if err := registry.verifyStored(stored, projectID, current.Name(), current.RevisionHash()); err != nil {
		return domain.Revision{}, false, err
	}
	if !stored.SameParentArtifacts(revision) {
		return domain.Revision{}, false, fmt.Errorf("%w: stored revision artifacts differ from bootstrap compile", ErrRevisionCorrupt)
	}
	return stored, created, nil
}

// List returns bounded immutable metadata after application-layer owner authorization.
// It intentionally does not load or recompile large stored artifacts.
func (registry *RevisionRegistry) List(
	ctx context.Context,
	execution access.Execution,
	module string,
	limit int,
) ([]domain.RevisionSummary, error) {
	if err := registry.authorize(ctx, execution, access.OperationRevisionList); err != nil {
		return nil, err
	}
	projectID := execution.Scope().ProjectID()
	module = strings.TrimSpace(module)
	if !domain.ValidModuleName(module) {
		return nil, fmt.Errorf("%w: module identity is invalid", ErrRevisionInvalid)
	}
	if limit == 0 {
		limit = DefaultRevisionListLimit
	}
	if limit < 1 || limit > MaxRevisionListLimit {
		return nil, fmt.Errorf("%w: revision list limit must be between 1 and %d", ErrRevisionInvalid, MaxRevisionListLimit)
	}
	values, err := registry.store.List(ctx, projectID, module, limit)
	if err != nil {
		return nil, fmt.Errorf("list module revisions: %w", err)
	}
	result := make([]domain.RevisionSummary, 0, len(values))
	for _, value := range values {
		if err := validateRevisionSummary(value, projectID, module); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// Get returns one verified immutable revision after application-layer owner authorization.
func (registry *RevisionRegistry) Get(
	ctx context.Context,
	execution access.Execution,
	module string,
	revisionHash string,
) (domain.Revision, error) {
	if err := registry.authorize(ctx, execution, access.OperationRevisionGet); err != nil {
		return domain.Revision{}, err
	}
	projectID := execution.Scope().ProjectID()
	return registry.getVerified(ctx, projectID, strings.TrimSpace(module), strings.TrimSpace(revisionHash))
}

// GetSource returns the first registered source only after full stored artifact verification.
func (registry *RevisionRegistry) GetSource(
	ctx context.Context,
	execution access.Execution,
	module string,
	revisionHash string,
) (RevisionSource, error) {
	if err := registry.authorize(ctx, execution, access.OperationRevisionGetSource); err != nil {
		return RevisionSource{}, err
	}
	projectID := execution.Scope().ProjectID()
	value, err := registry.getVerified(ctx, projectID, strings.TrimSpace(module), strings.TrimSpace(revisionHash))
	if err != nil {
		return RevisionSource{}, err
	}
	return RevisionSource{Format: value.SourceFormat(), Hash: value.SourceHash(), Bytes: value.Source()}, nil
}

func (registry *RevisionRegistry) getVerified(
	ctx context.Context,
	projectID project.ID,
	module string,
	revisionHash string,
) (domain.Revision, error) {
	if module == "" || revisionHash == "" {
		return domain.Revision{}, fmt.Errorf("%w: module and revision are required", ErrRevisionInvalid)
	}
	if !domain.ValidModuleName(module) || !domain.ValidContentHash(revisionHash) {
		return domain.Revision{}, fmt.Errorf("%w: module or revision identity is invalid", ErrRevisionInvalid)
	}
	value, err := registry.store.Get(ctx, projectID, module, revisionHash)
	if err != nil {
		return domain.Revision{}, fmt.Errorf("get module revision: %w", err)
	}
	if err := registry.verifyStored(value, projectID, module, revisionHash); err != nil {
		return domain.Revision{}, err
	}
	return value, nil
}

func (registry *RevisionRegistry) verifyStored(
	value domain.Revision,
	projectID project.ID,
	module string,
	revisionHash string,
) error {
	if value.ProjectID().String() != projectID.String() || value.ModuleName() != module ||
		value.RevisionHash() != revisionHash {
		return fmt.Errorf("%w: stored revision crossed its requested identity", ErrRevisionCorrupt)
	}
	format, err := makeSpecSourceFormat(value.SourceFormat())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrRevisionCorrupt, err)
	}
	recompiled, err := registry.compiler.Compile(value.Source(), format)
	if err != nil {
		return fmt.Errorf("%w: recompile stored source: %v", ErrRevisionCorrupt, err)
	}
	storedIdentity, found := value.DataSchemaIdentity(DataSchemaFormatVersion)
	if !found {
		return fmt.Errorf("%w: current data schema format %d is missing", ErrRevisionCorrupt, DataSchemaFormatVersion)
	}
	if recompiled.Name() != value.ModuleName() || recompiled.Version() != value.ModuleVersion() ||
		recompiled.RevisionHash() != value.RevisionHash() ||
		recompiled.DataSchemaFormat() != storedIdentity.Format() ||
		recompiled.DataSchemaFingerprint() != storedIdentity.Fingerprint() ||
		value.SpecVersion() != spec.APIVersion || value.IRFormat() != IRFormatVersion ||
		!bytes.Equal(recompiled.CanonicalIR(), value.CanonicalIR()) ||
		!bytes.Equal(recompiled.OpenAPI(), value.OpenAPI()) ||
		!bytes.Equal(recompiled.ManagerUISchema(), value.ManagerSchema()) {
		return fmt.Errorf("%w: stored revision does not reproduce its compiled artifacts", ErrRevisionCorrupt)
	}
	return nil
}

func validateRevisionSummary(value domain.RevisionSummary, projectID project.ID, module string) error {
	if value.ProjectID().String() != projectID.String() || value.ModuleName() != module {
		return fmt.Errorf("%w: stored revision summary crossed its requested identity", ErrRevisionCorrupt)
	}
	if _, found := findSummaryDataSchemaIdentity(value.DataSchemaIdentities(), DataSchemaFormatVersion); !found {
		return fmt.Errorf("%w: current data schema format %d is missing", ErrRevisionCorrupt, DataSchemaFormatVersion)
	}
	return nil
}

func findSummaryDataSchemaIdentity(values []domain.DataSchemaIdentity, format int) (domain.DataSchemaIdentity, bool) {
	for _, value := range values {
		if value.Format() == format {
			return value, true
		}
	}
	return domain.DataSchemaIdentity{}, false
}

func (registry *RevisionRegistry) authorize(
	ctx context.Context,
	execution access.Execution,
	operation access.Operation,
) error {
	if registry == nil || registry.authorizer == nil {
		return fmt.Errorf("%w: uninitialized revision registry", ErrRevisionInvalid)
	}
	if err := execution.Validate(); err != nil {
		return err
	}
	return registry.authorizer.Authorize(ctx, execution, operation)
}

func sameCompiledModule(left, right *CompiledModule) bool {
	return left != nil && right != nil && left.Name() == right.Name() && left.Version() == right.Version() &&
		left.RevisionHash() == right.RevisionHash() &&
		left.DataSchemaFormat() == right.DataSchemaFormat() &&
		left.DataSchemaFingerprint() == right.DataSchemaFingerprint() &&
		bytes.Equal(left.CanonicalIR(), right.CanonicalIR()) &&
		bytes.Equal(left.OpenAPI(), right.OpenAPI()) &&
		bytes.Equal(left.ManagerUISchema(), right.ManagerUISchema())
}

func makeDomainSourceFormat(format spec.Format) (domain.SourceFormat, error) {
	switch format {
	case spec.FormatJSON:
		return domain.SourceFormatJSON, nil
	case spec.FormatYAML:
		return domain.SourceFormatYAML, nil
	default:
		return "", fmt.Errorf("%w: unsupported source format %q", ErrRevisionInvalid, format)
	}
}

func makeSpecSourceFormat(format domain.SourceFormat) (spec.Format, error) {
	switch format {
	case domain.SourceFormatJSON:
		return spec.FormatJSON, nil
	case domain.SourceFormatYAML:
		return spec.FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported stored source format %q", format)
	}
}

func revisionHashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
