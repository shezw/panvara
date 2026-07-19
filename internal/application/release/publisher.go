/*
   Panvara
   internal/application/release/publisher.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package release

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

const deniedAuditTimeout = 2 * time.Second

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Publisher verifies and publishes immutable module facts without activating them.
type Publisher struct {
	snapshots  SnapshotReader
	store      ReleaseStore
	authorizer access.Authorizer
	denied     access.DeniedAuditor
	clock      Clock
	ids        ReleaseIDGenerator
	compiler   *moduleapp.Compiler
}

// NewPublisher constructs the publication use-case boundary with explicit dependencies.
func NewPublisher(
	snapshots SnapshotReader,
	store ReleaseStore,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
	clock Clock,
	ids ReleaseIDGenerator,
) (*Publisher, error) {
	if snapshots == nil || store == nil || authorizer == nil || denied == nil || clock == nil || ids == nil {
		return nil, fmt.Errorf("%w: publisher dependency is nil", ErrInvalid)
	}
	return &Publisher{
		snapshots: snapshots, store: store, authorizer: authorizer, denied: denied,
		clock: clock, ids: ids, compiler: moduleapp.NewCompiler(),
	}, nil
}

// NewDefaultPublisher constructs a production publisher with wall-clock time
// and cryptographically random UUIDv7 release identities.
func NewDefaultPublisher(
	snapshots SnapshotReader,
	store ReleaseStore,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
) (*Publisher, error) {
	return NewPublisher(
		snapshots, store, authorizer, denied, SystemClock{}, NewDefaultUUIDv7Generator(),
	)
}

// Publish first resolves an authorized replay, then verifies one exact snapshot
// chain and asks storage to atomically record a new publication if needed.
func (publisher *Publisher) Publish(
	ctx context.Context,
	invocation access.Invocation,
	module string,
	input PublishInput,
) (domainrelease.ModuleRelease, bool, error) {
	if err := publisher.ready(ctx, invocation); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	if err := publisher.authorize(ctx, invocation, access.OperationReleasePublish); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	module = strings.TrimSpace(module)
	planID := strings.ToLower(strings.TrimSpace(input.PlanID))
	key := strings.TrimSpace(input.IdempotencyKey)
	if !domainmodule.ValidModuleName(module) || !domainmodule.ValidContentHash(planID) ||
		!idempotencyKeyPattern.MatchString(key) {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: module, plan id, or idempotency key is invalid", ErrInvalid)
	}

	mutation, err := access.NewMutationContext(invocation, access.OperationReleasePublish)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: construct release mutation: %v", ErrInvalid, err)
	}
	at := publisher.clock.Now().UTC()
	if at.IsZero() {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: publication clock returned zero time", ErrUnavailable)
	}
	intentHash := publishIntentHash(invocation, module, planID)
	replayed, resolved, err := publisher.store.ResolveReplay(
		ctx, mutation, module, planID, key, intentHash, at,
	)
	if err != nil {
		normalized := normalizeStoreError("resolve module release replay", err)
		if isAuthorizationFailure(normalized) {
			publisher.auditDenied(ctx, invocation, access.OperationReleasePublish, normalized)
		}
		return domainrelease.ModuleRelease{}, false, normalized
	}
	if resolved {
		if err := verifyReplayedRelease(replayed, invocation.Execution().Scope(), module, planID); err != nil {
			return domainrelease.ModuleRelease{}, false, err
		}
		return replayed, false, nil
	}

	projectID := invocation.Execution().Scope().ProjectID()
	draft, validation, plan, err := publisher.snapshots.GetPublishSnapshot(ctx, projectID, module, planID)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, normalizeSnapshotError("read publish snapshot", err)
	}
	if err := verifySnapshotChain(projectID.String(), module, planID, draft, validation, plan); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	outcome, err := publishableOutcome(plan.Outcome)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}

	compiled, err := publisher.compiler.Compile(draft.Source(), specFormat(draft.SourceFormat()))
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: recompile publish source: %v", ErrCorrupt, err)
	}
	if err := verifyCompiledCandidate(module, compiled, validation, plan); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}

	releaseID, err := publisher.ids.New(at)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, normalizeGeneratedError("generate release id", err)
	}
	dataIdentity, err := domainmodule.NewDataSchemaIdentity(
		compiled.DataSchemaFormat(), compiled.DataSchemaFingerprint(),
	)
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: construct data schema identity: %v", ErrCorrupt, err)
	}
	subject := invocation.Execution().Actor()
	revision, err := domainmodule.NewRevision(domainmodule.RevisionMaterial{
		ProjectID: projectID, ModuleName: module, ModuleVersion: compiled.Version(),
		RevisionHash: compiled.RevisionHash(), DataSchemaIdentities: []domainmodule.DataSchemaIdentity{dataIdentity},
		SpecVersion: spec.APIVersion, IRFormat: moduleapp.IRFormatVersion,
		SourceFormat: draft.SourceFormat(), SourceHash: draft.SourceHash(), Source: draft.Source(),
		CanonicalIR: compiled.CanonicalIR(), OpenAPI: compiled.OpenAPI(), ManagerSchema: compiled.ManagerUISchema(),
		Origin: domainmodule.RevisionOriginPublish, RegisteredBy: subject.ActorID(), RegisteredAt: at,
	})
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: construct published revision: %v", ErrCorrupt, err)
	}
	releaseFact, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: releaseID, Scope: invocation.Execution().Scope(), ModuleName: module,
		DraftID: draft.ID(), DraftGeneration: draft.Generation(), ValidationID: validation.ID,
		PlanID: plan.ID, PlanHash: plan.PlanHash, BaselineRevision: plan.BaselineRevision,
		CandidateRevision: compiled.RevisionHash(), DataSchemaFormat: compiled.DataSchemaFormat(),
		DataSchemaFingerprint: compiled.DataSchemaFingerprint(), SourceHash: draft.SourceHash(),
		Outcome: outcome, Risk: plan.Risk, PublishedBy: subject.ActorID(),
		PublishedCredentialID: invocation.Execution().CredentialID(), RequestID: invocation.RequestID(), PublishedAt: at,
	})
	if err != nil {
		return domainrelease.ModuleRelease{}, false, fmt.Errorf("%w: construct module release: %v", ErrCorrupt, err)
	}
	stored, created, err := publisher.store.Publish(ctx, mutation, revision, releaseFact, key, intentHash)
	if err != nil {
		normalized := normalizeStoreError("publish module release", err)
		if isAuthorizationFailure(normalized) {
			publisher.auditDenied(ctx, invocation, access.OperationReleasePublish, normalized)
		}
		return domainrelease.ModuleRelease{}, false, normalized
	}
	if err := verifyStoredRelease(stored, releaseFact, created); err != nil {
		return domainrelease.ModuleRelease{}, false, err
	}
	return stored, created, nil
}

// Get returns one exact immutable release after fixed owner authorization.
func (publisher *Publisher) Get(
	ctx context.Context,
	invocation access.Invocation,
	module string,
	releaseID string,
) (domainrelease.ModuleRelease, error) {
	if err := publisher.ready(ctx, invocation); err != nil {
		return domainrelease.ModuleRelease{}, err
	}
	if err := publisher.authorize(ctx, invocation, access.OperationReleaseGet); err != nil {
		return domainrelease.ModuleRelease{}, err
	}
	module = strings.TrimSpace(module)
	id, err := domainrelease.ParseID(releaseID)
	if !domainmodule.ValidModuleName(module) || err != nil {
		return domainrelease.ModuleRelease{}, fmt.Errorf("%w: module or release id is invalid", ErrInvalid)
	}
	value, err := publisher.store.Get(ctx, invocation.Execution().Scope(), module, id)
	if err != nil {
		return domainrelease.ModuleRelease{}, normalizeStoreError("get module release", err)
	}
	if err := value.Validate(); err != nil || !sameScope(value.Scope(), invocation.Execution().Scope()) ||
		value.ModuleName() != module || value.ID().String() != id.String() {
		return domainrelease.ModuleRelease{}, fmt.Errorf("%w: stored release crossed its requested identity", ErrCorrupt)
	}
	return value, nil
}

func (publisher *Publisher) ready(ctx context.Context, invocation access.Invocation) error {
	if publisher == nil || publisher.snapshots == nil || publisher.store == nil || publisher.authorizer == nil ||
		publisher.denied == nil || publisher.clock == nil || publisher.ids == nil || publisher.compiler == nil {
		return fmt.Errorf("%w: publisher is not initialized", ErrUnavailable)
	}
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := invocation.Validate(); err != nil {
		return fmt.Errorf("%w: invalid publication invocation: %v", ErrInvalid, err)
	}
	return nil
}

func (publisher *Publisher) authorize(ctx context.Context, invocation access.Invocation, operation access.Operation) error {
	err := publisher.authorizer.Authorize(ctx, invocation.Execution(), operation)
	if err == nil {
		return nil
	}
	publisher.auditDenied(ctx, invocation, operation, err)
	return err
}

func (publisher *Publisher) auditDenied(
	ctx context.Context,
	invocation access.Invocation,
	operation access.Operation,
	cause error,
) {
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), deniedAuditTimeout)
	defer cancel()
	execution := invocation.Execution()
	_ = publisher.denied.AuditDenied(auditContext, access.DeniedAttempt{
		Scope: execution.Scope(), ActorID: execution.Actor().ActorID(), CredentialID: execution.CredentialID(),
		Operation: operation, RequestID: invocation.RequestID(), Reason: denialReason(cause),
		DeniedAt: publisher.clock.Now().UTC(),
	})
}
