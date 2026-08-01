/*
   Panvara
   internal/application/release/activator.go    2026-08-02
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
	"strings"

	"github.com/shezw/panvara/internal/application/access"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const maxActivationEpoch uint64 = 1<<63 - 1

// Activator safely moves one environment's authoritative active pointer to an
// exact compatible Release, then installs the committed runtime snapshot.
type Activator struct {
	store      ActivationStore
	releases   ReleaseReader
	revisions  RevisionReader
	runtime    Runtime
	authorizer access.Authorizer
	denied     access.DeniedAuditor
	clock      Clock
}

// NewActivator constructs the compatible-activation use-case boundary.
func NewActivator(
	store ActivationStore,
	releases ReleaseReader,
	revisions RevisionReader,
	runtime Runtime,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
	clock Clock,
) (*Activator, error) {
	if store == nil || releases == nil || revisions == nil || runtime == nil ||
		authorizer == nil || denied == nil || clock == nil {
		return nil, fmt.Errorf("%w: activator dependency is nil", ErrInvalid)
	}
	return &Activator{
		store: store, releases: releases, revisions: revisions, runtime: runtime,
		authorizer: authorizer, denied: denied, clock: clock,
	}, nil
}

// NewDefaultActivator constructs a production Activator with UTC wall-clock time.
func NewDefaultActivator(
	store ActivationStore,
	releases ReleaseReader,
	revisions RevisionReader,
	runtime Runtime,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
) (*Activator, error) {
	return NewActivator(store, releases, revisions, runtime, authorizer, denied, SystemClock{})
}

// Activate prepares and atomically activates one exact compatible Release.
// The bool reports whether storage advanced the authoritative activation epoch.
func (activator *Activator) Activate(
	ctx context.Context,
	invocation access.Invocation,
	module string,
	releaseID string,
) (domainrelease.ActiveSnapshot, bool, error) {
	if err := activator.ready(ctx, invocation); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if err := activator.authorize(ctx, invocation, access.OperationReleaseActivate); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	module = strings.TrimSpace(module)
	id, err := domainrelease.ParseID(strings.TrimSpace(releaseID))
	if !domainmodule.ValidModuleName(module) || err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("%w: module or release id is invalid", ErrInvalid)
	}
	mutation, err := access.NewMutationContext(invocation, access.OperationReleaseActivate)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("%w: construct activation mutation: %v", ErrInvalid, err)
	}

	scope := invocation.Execution().Scope()
	current, err := activator.store.GetActive(ctx, scope, module)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeStoreError("get active module snapshot", err)
	}
	if err := verifyRequestedActive(current, scope, module); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	target, err := activator.releases.Get(ctx, scope, module, id)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeStoreError("get activation release", err)
	}
	if err := verifyRequestedRelease(target, scope, module, id); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if target.Outcome() != domainrelease.OutcomeCompatible {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: outcome %q requires a later activation workflow", ErrNotActivatable, target.Outcome(),
		)
	}

	replay, err := verifyActivationBaseline(current, target)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if !replay && current.Epoch() == maxActivationEpoch {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("%w: activation epoch is exhausted", ErrUnavailable)
	}
	projectID := scope.ProjectID()
	candidate, err := activator.revisions.Get(ctx, projectID, module, target.CandidateRevision())
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeRevisionError("get activation candidate", err)
	}
	namespace, err := activator.revisions.Get(ctx, projectID, module, current.RecordNamespaceRevision())
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeRevisionError("get Record namespace revision", err)
	}
	if err := verifyActivationRevisions(current, target, candidate, namespace, replay); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}

	prepared, err := activator.runtime.Prepare(ctx, candidate, current.RecordNamespaceRevision())
	if err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeRuntimeError("prepare activation runtime", err)
	}
	if prepared == nil || prepared.RuntimeRevision() != candidate.RevisionHash() ||
		prepared.RecordNamespaceRevision() != current.RecordNamespaceRevision() {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf(
			"%w: prepared runtime crossed the verified activation target", ErrCorrupt,
		)
	}

	at := activator.clock.Now().UTC().Truncate(activationTimestampPrecision)
	if at.IsZero() {
		return domainrelease.ActiveSnapshot{}, false, fmt.Errorf("%w: activation clock returned zero time", ErrUnavailable)
	}
	stored, advanced, err := activator.store.ActivateCompatible(ctx, mutation, ActivateCompatibleCommand{
		Expected: current, Release: target, Candidate: candidate, ActivatedAt: at,
	})
	if err != nil {
		normalized := normalizeStoreError("activate compatible module release", err)
		if isAuthorizationFailure(normalized) {
			activator.auditDenied(ctx, invocation, access.OperationReleaseActivate, normalized)
		}
		return domainrelease.ActiveSnapshot{}, false, normalized
	}
	if err := verifyActivationResult(stored, current, target, invocation, at, advanced, replay); err != nil {
		return domainrelease.ActiveSnapshot{}, false, err
	}
	if err := activator.runtime.Install(stored, prepared); err != nil {
		return domainrelease.ActiveSnapshot{}, false, normalizeRuntimeError("install committed activation runtime", err)
	}
	return stored, advanced, nil
}

// GetActive returns the authoritative active module snapshot after owner authorization.
func (activator *Activator) GetActive(
	ctx context.Context,
	invocation access.Invocation,
	module string,
) (domainrelease.ActiveSnapshot, error) {
	if err := activator.ready(ctx, invocation); err != nil {
		return domainrelease.ActiveSnapshot{}, err
	}
	if err := activator.authorize(ctx, invocation, access.OperationReleaseGetActive); err != nil {
		return domainrelease.ActiveSnapshot{}, err
	}
	module = strings.TrimSpace(module)
	if !domainmodule.ValidModuleName(module) {
		return domainrelease.ActiveSnapshot{}, fmt.Errorf("%w: module is invalid", ErrInvalid)
	}
	scope := invocation.Execution().Scope()
	value, err := activator.store.GetActive(ctx, scope, module)
	if err != nil {
		return domainrelease.ActiveSnapshot{}, normalizeStoreError("get active module snapshot", err)
	}
	if err := verifyRequestedActive(value, scope, module); err != nil {
		return domainrelease.ActiveSnapshot{}, err
	}
	return value, nil
}

func (activator *Activator) ready(ctx context.Context, invocation access.Invocation) error {
	if activator == nil || activator.store == nil || activator.releases == nil || activator.revisions == nil ||
		activator.runtime == nil || activator.authorizer == nil || activator.denied == nil || activator.clock == nil {
		return fmt.Errorf("%w: activator is not initialized", ErrUnavailable)
	}
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := invocation.Validate(); err != nil {
		return fmt.Errorf("%w: invalid activation invocation: %v", ErrInvalid, err)
	}
	return nil
}

func (activator *Activator) authorize(
	ctx context.Context,
	invocation access.Invocation,
	operation access.Operation,
) error {
	err := activator.authorizer.Authorize(ctx, invocation.Execution(), operation)
	if err == nil {
		return nil
	}
	activator.auditDenied(ctx, invocation, operation, err)
	return err
}

func (activator *Activator) auditDenied(
	ctx context.Context,
	invocation access.Invocation,
	operation access.Operation,
	cause error,
) {
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), deniedAuditTimeout)
	defer cancel()
	execution := invocation.Execution()
	_ = activator.denied.AuditDenied(auditContext, access.DeniedAttempt{
		Scope: execution.Scope(), ActorID: execution.Actor().ActorID(), CredentialID: execution.CredentialID(),
		Operation: operation, RequestID: invocation.RequestID(), Reason: denialReason(cause),
		DeniedAt: activator.clock.Now().UTC(),
	})
}
