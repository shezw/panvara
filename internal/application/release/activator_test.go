/*
   Panvara
   internal/application/release/activator_test.go    2026-08-02
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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

func TestActivatorAdvancesCompatibleReleaseAndInstallsCommittedRuntime(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)

	active, advanced, err := fixture.activator.Activate(
		context.Background(), fixture.invocation, " notes ", fixture.target.ID().String(),
	)
	if err != nil || !advanced {
		t.Fatalf("Activate() = %#v, %v, %v", active, advanced, err)
	}
	activeID, found := active.ReleaseID()
	if !found || activeID.String() != fixture.target.ID().String() || active.Epoch() != 2 ||
		active.RuntimeRevision() != fixture.candidate.RevisionHash() ||
		active.RecordNamespaceRevision() != fixture.baseline.RevisionHash() {
		t.Fatalf("active snapshot = %#v", active)
	}
	if fixture.store.activateCalls != 1 || fixture.store.mutation.Operation() != access.OperationReleaseActivate ||
		fixture.store.mutation.RequestID() != fixture.invocation.RequestID() ||
		!sameActiveSnapshot(fixture.store.command.Expected, fixture.current) ||
		fixture.store.command.Release.ID().String() != fixture.target.ID().String() ||
		fixture.store.command.Candidate.RevisionHash() != fixture.candidate.RevisionHash() ||
		!fixture.store.command.ActivatedAt.Equal(releaseTestTime) {
		t.Fatalf("activation store input = %#v / %#v", fixture.store.mutation, fixture.store.command)
	}
	if fixture.runtime.prepareCalls != 1 || fixture.runtime.prepareRevision != fixture.candidate.RevisionHash() ||
		fixture.runtime.prepareNamespace != fixture.baseline.RevisionHash() ||
		fixture.runtime.installCalls != 1 || !sameActiveSnapshot(fixture.runtime.installed, active) {
		t.Fatalf("runtime prepare/install = %d/%q/%q/%d", fixture.runtime.prepareCalls,
			fixture.runtime.prepareRevision, fixture.runtime.prepareNamespace, fixture.runtime.installCalls)
	}
	if len(fixture.authorizer.operations) != 1 || fixture.authorizer.operations[0] != access.OperationReleaseActivate ||
		fixture.denied.calls != 0 {
		t.Fatalf("authorization operations/audits = %#v/%d", fixture.authorizer.operations, fixture.denied.calls)
	}
}

func TestActivatorReplaysCurrentlyActiveReleaseWithoutAdvancingEpoch(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)
	fixture.current = fixture.store.activateValue
	fixture.store.current = fixture.current
	fixture.store.activateValue = fixture.current
	fixture.store.advanced = false

	active, advanced, err := fixture.activator.Activate(
		context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
	)
	if err != nil || advanced || !sameActiveSnapshot(active, fixture.current) {
		t.Fatalf("Activate(replay) = %#v, %v, %v", active, advanced, err)
	}
	if fixture.store.activateCalls != 1 || fixture.runtime.prepareCalls != 1 || fixture.runtime.installCalls != 1 {
		t.Fatalf("replay store/prepare/install calls = %d/%d/%d", fixture.store.activateCalls,
			fixture.runtime.prepareCalls, fixture.runtime.installCalls)
	}
}

func TestActivatorAcceptsConcurrentConvergenceOnTheSameExactRelease(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)
	fixture.store.advanced = false

	active, advanced, err := fixture.activator.Activate(
		context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
	)
	if err != nil || advanced || !sameActiveSnapshot(active, fixture.store.activateValue) ||
		fixture.runtime.installCalls != 1 {
		t.Fatalf("Activate(converged replay) = %#v, %v, %v, installs %d", active, advanced, err,
			fixture.runtime.installCalls)
	}
}

func TestActivatorRejectsUnsupportedOutcomeBeforeRevisionOrRuntimeWork(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		outcome domainrelease.Outcome
		risk    string
	}{
		{name: "review required", outcome: domainrelease.OutcomeReviewRequired, risk: "medium"},
		{name: "migration required", outcome: domainrelease.OutcomeMigrationRequired, risk: "high"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fixture := newActivationFixture(t)
			fixture.target = makeActivationRelease(
				t, fixture.scope, fixture.candidate, fixture.baseline.RevisionHash(),
				fixture.target.ID(), test.outcome, test.risk,
			)
			fixture.releases.value = fixture.target
			_, _, err := fixture.activator.Activate(
				context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
			)
			if !errors.Is(err, ErrNotActivatable) || fixture.revisions.calls != 0 ||
				fixture.runtime.prepareCalls != 0 || fixture.store.activateCalls != 0 {
				t.Fatalf("Activate() error/revision/prepare/store = %v/%d/%d/%d", err,
					fixture.revisions.calls, fixture.runtime.prepareCalls, fixture.store.activateCalls)
			}
		})
	}
}

func TestActivatorRejectsStaleOrAbsentBaselineAsConflict(t *testing.T) {
	t.Parallel()
	for _, baseline := range []string{"", "sha256:" + strings.Repeat("f", 64)} {
		fixture := newActivationFixture(t)
		fixture.target = makeActivationRelease(
			t, fixture.scope, fixture.candidate, baseline, fixture.target.ID(),
			domainrelease.OutcomeCompatible, "low",
		)
		fixture.releases.value = fixture.target
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrActivationConflict) || fixture.revisions.calls != 0 ||
			fixture.runtime.prepareCalls != 0 || fixture.store.activateCalls != 0 {
			t.Fatalf("Activate(baseline %q) = %v, calls %d/%d/%d", baseline, err,
				fixture.revisions.calls, fixture.runtime.prepareCalls, fixture.store.activateCalls)
		}
	}
}

func TestActivatorRejectsChangedDataIdentityDespiteCompatibleClassification(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)
	changed := releaseTestRevision(t, fixture.scope.ProjectID(), releaseTestSource(
		"notes", "3.0.0", "\n    - name: note\n      fields:\n        - name: title\n          type: string",
	))
	fixture.target = makeActivationRelease(
		t, fixture.scope, changed, fixture.baseline.RevisionHash(), fixture.target.ID(),
		domainrelease.OutcomeCompatible, "low",
	)
	fixture.releases.value = fixture.target
	fixture.revisions.values[changed.RevisionHash()] = changed

	_, _, err := fixture.activator.Activate(
		context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
	)
	if !errors.Is(err, ErrNotActivatable) || fixture.runtime.prepareCalls != 0 || fixture.store.activateCalls != 0 {
		t.Fatalf("Activate(changed data identity) error/prepare/store = %v/%d/%d", err,
			fixture.runtime.prepareCalls, fixture.store.activateCalls)
	}
}

func TestActivatorFailsClosedOnCrossedFactsAndRuntimeTargets(t *testing.T) {
	t.Parallel()
	t.Run("crossed Release", func(t *testing.T) {
		fixture := newActivationFixture(t)
		other := releaseTestScope(t, fixture.scope.ProjectID().String(), "019f5c36-b399-7c52-9325-ec59f95c8fae")
		fixture.releases.value = makeActivationRelease(
			t, other, fixture.candidate, fixture.baseline.RevisionHash(), fixture.target.ID(),
			domainrelease.OutcomeCompatible, "low",
		)
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrCorrupt) || fixture.revisions.calls != 0 || fixture.store.activateCalls != 0 {
			t.Fatalf("Activate(crossed release) = %v, calls %d/%d", err,
				fixture.revisions.calls, fixture.store.activateCalls)
		}
	})
	t.Run("crossed prepared runtime", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.runtime.preparedRevision = fixture.baseline.RevisionHash()
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrCorrupt) || fixture.store.activateCalls != 0 || fixture.runtime.installCalls != 0 {
			t.Fatalf("Activate(crossed runtime) = %v, calls %d/%d", err,
				fixture.store.activateCalls, fixture.runtime.installCalls)
		}
	})
	t.Run("non-advancing non-replay result", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.store.activateValue = fixture.current
		fixture.store.advanced = false
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrCorrupt) || fixture.runtime.installCalls != 0 {
			t.Fatalf("Activate(corrupt convergence) = %v, install calls %d", err, fixture.runtime.installCalls)
		}
	})
}

func TestActivatorNormalizesStoreRevisionAndRuntimeFailures(t *testing.T) {
	t.Parallel()
	t.Run("missing candidate", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.revisions.errs[fixture.candidate.RevisionHash()] = moduleapp.ErrRevisionNotFound
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrNotFound) || fixture.runtime.prepareCalls != 0 || fixture.store.activateCalls != 0 {
			t.Fatalf("Activate(missing candidate) = %v", err)
		}
	})
	t.Run("prepare unavailable", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.runtime.prepareErr = errors.New("builder offline")
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrUnavailable) || fixture.store.activateCalls != 0 {
			t.Fatalf("Activate(prepare unavailable) = %v", err)
		}
	})
	t.Run("CAS conflict", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.store.activateErr = ErrActivationConflict
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrActivationConflict) || fixture.runtime.prepareCalls != 1 ||
			fixture.runtime.installCalls != 0 {
			t.Fatalf("Activate(CAS conflict) = %v, runtime calls %d/%d", err,
				fixture.runtime.prepareCalls, fixture.runtime.installCalls)
		}
	})
	t.Run("install unavailable after commit", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.runtime.installErr = errors.New("atomic install rejected")
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, ErrUnavailable) || fixture.store.activateCalls != 1 || fixture.runtime.installCalls != 1 {
			t.Fatalf("Activate(install unavailable) = %v, calls %d/%d", err,
				fixture.store.activateCalls, fixture.runtime.installCalls)
		}
	})
}

func TestActivatorAuditsPreauthorizationAndTransactionReauthorizationDenials(t *testing.T) {
	t.Parallel()
	t.Run("preauthorization", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.authorizer.err = access.ErrForbidden
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, access.ErrForbidden) || fixture.store.getCalls != 0 || fixture.denied.calls != 1 ||
			fixture.denied.attempt.Operation != access.OperationReleaseActivate ||
			fixture.denied.attempt.Reason != "forbidden" {
			t.Fatalf("preauthorization = %v/%d/%d/%#v", err, fixture.store.getCalls,
				fixture.denied.calls, fixture.denied.attempt)
		}
	})
	t.Run("transaction reauthorization", func(t *testing.T) {
		fixture := newActivationFixture(t)
		fixture.store.activateErr = access.ErrScopeInactive
		_, _, err := fixture.activator.Activate(
			context.Background(), fixture.invocation, "notes", fixture.target.ID().String(),
		)
		if !errors.Is(err, access.ErrScopeInactive) || fixture.denied.calls != 1 ||
			fixture.denied.attempt.Reason != "scope_inactive" || fixture.runtime.installCalls != 0 {
			t.Fatalf("transaction reauthorization = %v/%d/%#v", err, fixture.denied.calls, fixture.denied.attempt)
		}
	})
}

func TestActivatorGetsExactActiveSnapshotAndRejectsInvalidUse(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)
	active, err := fixture.activator.GetActive(context.Background(), fixture.invocation, " notes ")
	if err != nil || !sameActiveSnapshot(active, fixture.current) ||
		len(fixture.authorizer.operations) != 1 || fixture.authorizer.operations[0] != access.OperationReleaseGetActive {
		t.Fatalf("GetActive() = %#v, %v, operations %#v", active, err, fixture.authorizer.operations)
	}
	if _, err := fixture.activator.GetActive(context.Background(), fixture.invocation, "INVALID"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("GetActive(invalid) error = %v", err)
	}
	fixture.store.getErr = ErrNotFound
	if _, err := fixture.activator.GetActive(context.Background(), fixture.invocation, "notes"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetActive(not found) error = %v", err)
	}

	var nilActivator *Activator
	if _, _, err := nilActivator.Activate(context.Background(), fixture.invocation, "notes", fixture.target.ID().String()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil Activator.Activate() error = %v", err)
	}
	if _, err := fixture.activator.GetActive(nil, fixture.invocation, "notes"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil-context GetActive() error = %v", err)
	}
}

func TestNewActivatorRejectsIncompleteDependencies(t *testing.T) {
	t.Parallel()
	fixture := newActivationFixture(t)
	constructors := []func() error{
		func() error {
			_, err := NewActivator(nil, fixture.releases, fixture.revisions, fixture.runtime,
				fixture.authorizer, fixture.denied, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, nil, fixture.revisions, fixture.runtime,
				fixture.authorizer, fixture.denied, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, fixture.releases, nil, fixture.runtime,
				fixture.authorizer, fixture.denied, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, fixture.releases, fixture.revisions, nil,
				fixture.authorizer, fixture.denied, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, fixture.releases, fixture.revisions, fixture.runtime,
				nil, fixture.denied, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, fixture.releases, fixture.revisions, fixture.runtime,
				fixture.authorizer, nil, fixedReleaseClock{at: releaseTestTime})
			return err
		},
		func() error {
			_, err := NewActivator(fixture.store, fixture.releases, fixture.revisions, fixture.runtime,
				fixture.authorizer, fixture.denied, nil)
			return err
		},
	}
	for index, constructor := range constructors {
		if err := constructor(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("constructor %d error = %v, want ErrInvalid", index, err)
		}
	}
}

type activationFixture struct {
	scope      project.Scope
	invocation access.Invocation
	baseline   domainmodule.Revision
	candidate  domainmodule.Revision
	target     domainrelease.ModuleRelease
	current    domainrelease.ActiveSnapshot
	store      *activationTestStore
	releases   *activationTestReleaseReader
	revisions  *activationTestRevisionReader
	runtime    *activationTestRuntime
	authorizer *releaseTestAuthorizer
	denied     *releaseTestDeniedAuditor
	activator  *Activator
}

func newActivationFixture(t *testing.T) *activationFixture {
	t.Helper()
	scope := releaseTestScope(t, "019f5c36-b322-7c52-9325-ec59f95c8fae", "019f5c36-b323-7c52-9325-ec59f95c8fae")
	invocation := releaseTestInvocation(t, scope)
	baseline := releaseTestRevision(t, scope.ProjectID(), releaseTestSource("notes", "1.0.0", " []"))
	candidate := releaseTestRevision(t, scope.ProjectID(), releaseTestSource("notes", "2.0.0", " []"))
	target := makeActivationRelease(
		t, scope, candidate, baseline.RevisionHash(),
		releaseTestID(t, "019f5c36-b328-7c52-9325-ec59f95c8fae"),
		domainrelease.OutcomeCompatible, "low",
	)
	current := makeActivationSnapshot(t, scope, 1, nil, baseline, baseline.RevisionHash(), invocation, releaseTestTime.Add(-time.Hour))
	stored := makeActivationSnapshot(t, scope, 2, &target, candidate, baseline.RevisionHash(), invocation, releaseTestTime)
	store := &activationTestStore{current: current, activateValue: stored, advanced: true}
	releases := &activationTestReleaseReader{value: target}
	revisions := &activationTestRevisionReader{
		values: map[string]domainmodule.Revision{
			baseline.RevisionHash():  baseline,
			candidate.RevisionHash(): candidate,
		},
		errs: make(map[string]error),
	}
	runtime := &activationTestRuntime{}
	authorizer := &releaseTestAuthorizer{}
	denied := &releaseTestDeniedAuditor{}
	activator, err := NewActivator(
		store, releases, revisions, runtime, authorizer, denied, fixedReleaseClock{at: releaseTestTime},
	)
	if err != nil {
		t.Fatal(err)
	}
	return &activationFixture{
		scope: scope, invocation: invocation, baseline: baseline, candidate: candidate,
		target: target, current: current, store: store, releases: releases,
		revisions: revisions, runtime: runtime, authorizer: authorizer, denied: denied,
		activator: activator,
	}
}

func makeActivationRelease(
	t *testing.T,
	scope project.Scope,
	candidate domainmodule.Revision,
	baseline string,
	id domainrelease.ID,
	outcome domainrelease.Outcome,
	risk string,
) domainrelease.ModuleRelease {
	t.Helper()
	draftID, err := domainmodule.ParseDraftID("019f5c36-b324-7c52-9325-ec59f95c8fae")
	if err != nil {
		t.Fatal(err)
	}
	identity, found := candidate.DataSchemaIdentity(moduleapp.DataSchemaFormatVersion)
	if !found {
		t.Fatalf("candidate is missing data schema format %d", moduleapp.DataSchemaFormatVersion)
	}
	value, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: id, Scope: scope, ModuleName: "notes", DraftID: draftID, DraftGeneration: 1,
		ValidationID:     "sha256:" + strings.Repeat("1", 64),
		PlanID:           "sha256:" + strings.Repeat("2", 64),
		PlanHash:         "sha256:" + strings.Repeat("3", 64),
		BaselineRevision: baseline, CandidateRevision: candidate.RevisionHash(),
		DataSchemaFormat: identity.Format(), DataSchemaFingerprint: identity.Fingerprint(),
		SourceHash: candidate.SourceHash(), Outcome: outcome, Risk: risk,
		PublishedBy: "owner", PublishedCredentialID: releaseTestAccessID(t, "019f5c36-b326-7c52-9325-ec59f95c8fae"),
		RequestID: "publish-1", PublishedAt: releaseTestTime.Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func makeActivationSnapshot(
	t *testing.T,
	scope project.Scope,
	epoch uint64,
	target *domainrelease.ModuleRelease,
	runtime domainmodule.Revision,
	namespace string,
	invocation access.Invocation,
	at time.Time,
) domainrelease.ActiveSnapshot {
	t.Helper()
	identity, found := runtime.DataSchemaIdentity(moduleapp.DataSchemaFormatVersion)
	if !found {
		t.Fatalf("runtime is missing data schema format %d", moduleapp.DataSchemaFormatVersion)
	}
	var releaseID *domainrelease.ID
	origin := domainrelease.ActiveSnapshotOriginBootstrap
	actor := "system:bootstrap"
	requestID := "system:bootstrap"
	if target != nil {
		copied := target.ID()
		releaseID = &copied
		origin = domainrelease.ActiveSnapshotOriginRelease
		actor = invocation.Execution().Actor().ActorID()
		requestID = invocation.RequestID()
	}
	binding, err := domainrelease.NewModuleBinding(domainrelease.ModuleBindingMaterial{
		ModuleName: "notes", ReleaseID: releaseID, RuntimeRevision: runtime.RevisionHash(),
		RecordNamespaceRevision: namespace, DataSchemaFormat: identity.Format(),
		DataSchemaFingerprint: identity.Fingerprint(),
	})
	if err != nil {
		t.Fatal(err)
	}
	material := domainrelease.ActiveSnapshotMaterial{
		Scope: scope, Epoch: epoch, Origin: origin, Binding: binding,
		ActivatedBy: actor, RequestID: requestID, ActivatedAt: at,
	}
	if target != nil {
		value := invocation.Execution().CredentialID()
		material.ActivatedCredentialID = &value
	}
	value, err := domainrelease.NewActiveSnapshot(material)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type activationTestStore struct {
	current       domainrelease.ActiveSnapshot
	activateValue domainrelease.ActiveSnapshot
	advanced      bool
	getErr        error
	activateErr   error
	mutation      access.MutationContext
	command       ActivateCompatibleCommand
	getCalls      int
	activateCalls int
}

func (store *activationTestStore) GetActive(
	_ context.Context,
	_ project.Scope,
	_ string,
) (domainrelease.ActiveSnapshot, error) {
	store.getCalls++
	return store.current, store.getErr
}

func (store *activationTestStore) ActivateCompatible(
	_ context.Context,
	mutation access.MutationContext,
	command ActivateCompatibleCommand,
) (domainrelease.ActiveSnapshot, bool, error) {
	store.activateCalls++
	store.mutation, store.command = mutation, command
	return store.activateValue, store.advanced, store.activateErr
}

type activationTestReleaseReader struct {
	value domainrelease.ModuleRelease
	err   error
	calls int
}

func (reader *activationTestReleaseReader) Get(
	_ context.Context,
	_ project.Scope,
	_ string,
	_ domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	reader.calls++
	return reader.value, reader.err
}

type activationTestRevisionReader struct {
	values map[string]domainmodule.Revision
	errs   map[string]error
	calls  int
}

func (reader *activationTestRevisionReader) Get(
	_ context.Context,
	_ project.ID,
	_ string,
	revision string,
) (domainmodule.Revision, error) {
	reader.calls++
	if err := reader.errs[revision]; err != nil {
		return domainmodule.Revision{}, err
	}
	value, found := reader.values[revision]
	if !found {
		return domainmodule.Revision{}, moduleapp.ErrRevisionNotFound
	}
	return value, nil
}

type activationTestPrepared struct {
	runtimeRevision string
	namespace       string
}

func (prepared *activationTestPrepared) RuntimeRevision() string { return prepared.runtimeRevision }

func (prepared *activationTestPrepared) RecordNamespaceRevision() string { return prepared.namespace }

type activationTestRuntime struct {
	preparedRevision  string
	preparedNamespace string
	prepareErr        error
	installErr        error
	prepareRevision   string
	prepareNamespace  string
	installed         domainrelease.ActiveSnapshot
	prepareCalls      int
	installCalls      int
}

func (runtime *activationTestRuntime) Prepare(
	_ context.Context,
	revision domainmodule.Revision,
	namespace string,
) (PreparedRuntime, error) {
	runtime.prepareCalls++
	runtime.prepareRevision, runtime.prepareNamespace = revision.RevisionHash(), namespace
	if runtime.prepareErr != nil {
		return nil, runtime.prepareErr
	}
	preparedRevision := runtime.preparedRevision
	if preparedRevision == "" {
		preparedRevision = revision.RevisionHash()
	}
	preparedNamespace := runtime.preparedNamespace
	if preparedNamespace == "" {
		preparedNamespace = namespace
	}
	return &activationTestPrepared{runtimeRevision: preparedRevision, namespace: preparedNamespace}, nil
}

func (runtime *activationTestRuntime) Install(
	active domainrelease.ActiveSnapshot,
	_ PreparedRuntime,
) error {
	runtime.installCalls++
	runtime.installed = active
	return runtime.installErr
}
