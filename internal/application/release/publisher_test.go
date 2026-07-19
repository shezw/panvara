/*
   Panvara
   internal/application/release/publisher_test.go    2026-07-19
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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

var releaseTestTime = time.Date(2026, 7, 19, 10, 30, 0, 0, time.UTC)

func TestPublisherPublishesVerifiedFactAndReadsExactRelease(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	authorizer := &releaseTestAuthorizer{}
	denied := &releaseTestDeniedAuditor{}
	publisher := releaseTestPublisher(t, fixture.reader, store, authorizer, denied)

	published, created, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-1",
	})
	if err != nil || !created {
		t.Fatalf("Publish() = %#v, %v, %v", published, created, err)
	}
	if store.resolveCalls != 1 || store.resolveMutation.Operation() != access.OperationReleasePublish ||
		store.resolveModule != "notes" || store.resolvePlanID != fixture.plan.ID ||
		store.resolveKey != "publish-1" || !domainmodule.ValidContentHash(store.resolveIntentHash) ||
		!store.resolveAt.Equal(releaseTestTime) || store.publishCalls != 1 ||
		store.mutation.Operation() != access.OperationReleasePublish ||
		store.mutation.RequestID() != "request-1" || store.key != "publish-1" ||
		!domainmodule.ValidContentHash(store.intentHash) || store.revision.Origin() != domainmodule.RevisionOriginPublish ||
		store.revision.RegisteredBy() != "owner" || !bytes.Equal(store.revision.Source(), fixture.draft.Source()) {
		t.Fatalf("store publish input = %#v/%q/%q/%#v", store.mutation, store.key, store.intentHash, store.revision)
	}
	if err := store.mutation.Validate(); err != nil {
		t.Fatalf("MutationContext.Validate() = %v", err)
	}
	if published.Scope().EnvironmentID().String() != fixture.scope.EnvironmentID().String() ||
		published.PlanID() != fixture.plan.ID || published.ValidationID() != fixture.validation.ID ||
		published.CandidateRevision() != fixture.plan.Candidate.RevisionHash ||
		published.PublishedBy() != "owner" || published.RequestID() != "request-1" ||
		published.PublishedCredentialID().String() != fixture.invocation.Execution().CredentialID().String() ||
		published.Outcome() != domainrelease.OutcomeReviewRequired {
		t.Fatalf("published release = %#v", published)
	}
	if len(authorizer.operations) != 1 || authorizer.operations[0] != access.OperationReleasePublish || denied.calls != 0 {
		t.Fatalf("authorization operations/audits = %#v/%d", authorizer.operations, denied.calls)
	}

	store.getValue = published
	got, err := publisher.Get(context.Background(), fixture.invocation, "notes", published.ID().String())
	if err != nil || got.ID().String() != published.ID().String() || store.getCalls != 1 {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if len(authorizer.operations) != 2 || authorizer.operations[1] != access.OperationReleaseGet {
		t.Fatalf("Get authorization operations = %#v", authorizer.operations)
	}
}

func TestPublisherAcceptsStoreConvergenceForPlanReplay(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
	first, created, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-first",
	})
	if err != nil || !created {
		t.Fatalf("first Publish() = %#v, %v, %v", first, created, err)
	}
	store.stored = first
	replayed, created, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-second-key",
	})
	if err != nil || created || replayed.ID().String() != first.ID().String() {
		t.Fatalf("replayed Publish() = %#v, %v, %v", replayed, created, err)
	}
}

func TestPublisherResolvesPublishedPlanBeforeReadingChangedSnapshot(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	publisher := releaseTestPublisher(
		t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{},
	)
	first, created, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-original",
	})
	if err != nil || !created {
		t.Fatalf("first Publish() = %#v, %v, %v", first, created, err)
	}

	store.resolved, store.resolveFound = first, true
	fixture.reader.draft = replaceFixtureDraft(t, fixture.draft)
	fixture.reader.err = moduleapp.ErrPlanNotFound
	fixture.reader.calls = 0
	for _, key := range []string{"publish-original", "publish-alias"} {
		replayed, replayCreated, replayErr := publisher.Publish(
			context.Background(), fixture.invocation, "notes",
			PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: key},
		)
		if replayErr != nil || replayCreated || replayed.ID() != first.ID() {
			t.Fatalf("Publish(replay %q) = %#v, %v, %v", key, replayed, replayCreated, replayErr)
		}
	}
	if fixture.reader.calls != 0 || store.resolveCalls != 3 || store.publishCalls != 1 {
		t.Fatalf(
			"replay snapshot/resolve/publish calls = %d/%d/%d",
			fixture.reader.calls, store.resolveCalls, store.publishCalls,
		)
	}
}

func TestPublisherPrioritizesBoundKeyConflictBeforePlanState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		fixture func(*testing.T) publishFixture
		planID  func(publishFixture) string
		prepare func(*testing.T, *publishFixture)
	}{
		{
			name:    "missing plan",
			fixture: func(t *testing.T) publishFixture { return newPublishFixture(t, false) },
			planID: func(publishFixture) string {
				return "sha256:" + strings.Repeat("f", 64)
			},
			prepare: func(_ *testing.T, fixture *publishFixture) {
				fixture.reader.err = moduleapp.ErrPlanNotFound
			},
		},
		{
			name:    "stale plan",
			fixture: func(t *testing.T) publishFixture { return newPublishFixture(t, false) },
			planID:  func(fixture publishFixture) string { return fixture.plan.ID },
			prepare: func(t *testing.T, fixture *publishFixture) {
				fixture.reader.draft = replaceFixtureDraft(t, fixture.draft)
			},
		},
		{
			name:    "unsupported plan",
			fixture: func(t *testing.T) publishFixture { return newPublishFixture(t, true) },
			planID:  func(fixture publishFixture) string { return fixture.plan.ID },
			prepare: func(*testing.T, *publishFixture) {},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := test.fixture(t)
			test.prepare(t, &fixture)
			store := &releaseTestStore{resolveErr: ErrIdempotencyConflict}
			publisher := releaseTestPublisher(
				t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{},
			)
			_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
				PlanID: test.planID(fixture), IdempotencyKey: "publish-bound",
			})
			if !errors.Is(err, ErrIdempotencyConflict) || fixture.reader.calls != 0 ||
				store.resolveCalls != 1 || store.publishCalls != 0 {
				t.Fatalf(
					"Publish() error/snapshot/resolve/publish = %v/%d/%d/%d",
					err, fixture.reader.calls, store.resolveCalls, store.publishCalls,
				)
			}
		})
	}
}

func TestPublisherFailsClosedWhenReplayAuthorityIsUnavailable(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{resolveErr: errors.New("authority database unavailable")}
	publisher := releaseTestPublisher(
		t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{},
	)
	_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-replay-unavailable",
	})
	if !errors.Is(err, ErrUnavailable) || fixture.reader.calls != 0 ||
		store.resolveCalls != 1 || store.publishCalls != 0 {
		t.Fatalf(
			"Publish() error/snapshot/resolve/publish = %v/%d/%d/%d",
			err, fixture.reader.calls, store.resolveCalls, store.publishCalls,
		)
	}
}

func TestNewPublisherAndMethodsRejectIncompleteRuntime(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	authorizer := &releaseTestAuthorizer{}
	denied := &releaseTestDeniedAuditor{}
	clock := fixedReleaseClock{at: releaseTestTime}
	ids := fixedReleaseIDGenerator{id: releaseTestID(t, "019f5c36-b328-7c52-9325-ec59f95c8fae")}
	constructors := []func() error{
		func() error { _, err := NewPublisher(nil, store, authorizer, denied, clock, ids); return err },
		func() error { _, err := NewPublisher(fixture.reader, nil, authorizer, denied, clock, ids); return err },
		func() error { _, err := NewPublisher(fixture.reader, store, nil, denied, clock, ids); return err },
		func() error { _, err := NewPublisher(fixture.reader, store, authorizer, nil, clock, ids); return err },
		func() error { _, err := NewPublisher(fixture.reader, store, authorizer, denied, nil, ids); return err },
		func() error {
			_, err := NewPublisher(fixture.reader, store, authorizer, denied, clock, nil)
			return err
		},
	}
	for index, constructor := range constructors {
		if err := constructor(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("constructor %d error = %v, want ErrInvalid", index, err)
		}
	}
	var nilPublisher *Publisher
	if _, _, err := nilPublisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil Publisher.Publish() error = %v", err)
	}
	publisher := releaseTestPublisher(t, fixture.reader, store, authorizer, denied)
	if _, err := publisher.Get(nil, fixture.invocation, "notes", "invalid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil-context Publisher.Get() error = %v", err)
	}
}

func TestPublisherRejectsStaleAndUnsupportedPlansBeforePublishWrite(t *testing.T) {
	t.Parallel()
	t.Run("stale current draft", func(t *testing.T) {
		fixture := newPublishFixture(t, false)
		fixture.reader.draft = replaceFixtureDraft(t, fixture.draft)
		store := &releaseTestStore{}
		publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
		_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
			PlanID: fixture.plan.ID, IdempotencyKey: "publish-stale",
		})
		if !errors.Is(err, ErrStale) || store.resolveCalls != 1 || store.publishCalls != 0 {
			t.Fatalf("Publish(stale) error/resolve/publish calls = %v/%d/%d", err, store.resolveCalls, store.publishCalls)
		}
	})
	t.Run("unsupported outcome", func(t *testing.T) {
		fixture := newPublishFixture(t, true)
		if fixture.plan.Outcome != "unsupported" {
			t.Fatalf("fixture outcome = %q", fixture.plan.Outcome)
		}
		store := &releaseTestStore{}
		publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
		_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
			PlanID: fixture.plan.ID, IdempotencyKey: "publish-unsupported",
		})
		if !errors.Is(err, ErrNotPublishable) || store.resolveCalls != 1 || store.publishCalls != 0 {
			t.Fatalf("Publish(unsupported) error/resolve/publish calls = %v/%d/%d", err, store.resolveCalls, store.publishCalls)
		}
	})
}

func TestPublisherRejectsCrossedSnapshotAndInvalidInput(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
	otherPlan := "sha256:" + strings.Repeat("f", 64)
	if _, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: otherPlan, IdempotencyKey: "publish-crossed",
	}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("crossed plan error = %v, want ErrCorrupt", err)
	}
	for _, input := range []struct {
		module string
		value  PublishInput
	}{
		{module: "INVALID", value: PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "publish-1"}},
		{module: "notes", value: PublishInput{PlanID: "invalid", IdempotencyKey: "publish-1"}},
		{module: "notes", value: PublishInput{PlanID: fixture.plan.ID, IdempotencyKey: "bad key"}},
	} {
		if _, _, err := publisher.Publish(context.Background(), fixture.invocation, input.module, input.value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Publish(%q,%#v) error = %v, want ErrInvalid", input.module, input.value, err)
		}
	}
	if store.resolveCalls != 1 || store.publishCalls != 0 {
		t.Fatalf("invalid/crossed requests reached Store resolve/publish %d/%d times", store.resolveCalls, store.publishCalls)
	}
}

func TestPublisherAuditsPreauthorizationAndRepositoryReauthorizationFailures(t *testing.T) {
	t.Parallel()
	t.Run("preauthorization", func(t *testing.T) {
		fixture := newPublishFixture(t, false)
		authorizer := &releaseTestAuthorizer{err: access.ErrForbidden}
		denied := &releaseTestDeniedAuditor{}
		store := &releaseTestStore{}
		publisher := releaseTestPublisher(t, fixture.reader, store, authorizer, denied)
		_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
			PlanID: fixture.plan.ID, IdempotencyKey: "publish-denied",
		})
		if !errors.Is(err, access.ErrForbidden) || fixture.reader.calls != 0 || store.resolveCalls != 0 || store.publishCalls != 0 ||
			denied.calls != 1 || denied.attempt.Operation != access.OperationReleasePublish || denied.attempt.Reason != "forbidden" {
			t.Fatalf("preauthorization = %v, snapshot/resolve/publish/audit %d/%d/%d/%d %#v", err, fixture.reader.calls, store.resolveCalls, store.publishCalls, denied.calls, denied.attempt)
		}
	})
	t.Run("transaction reauthorization", func(t *testing.T) {
		fixture := newPublishFixture(t, false)
		denied := &releaseTestDeniedAuditor{}
		store := &releaseTestStore{resolveErr: access.ErrScopeInactive}
		publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, denied)
		_, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
			PlanID: fixture.plan.ID, IdempotencyKey: "publish-reauth",
		})
		if !errors.Is(err, access.ErrScopeInactive) || fixture.reader.calls != 0 ||
			store.resolveCalls != 1 || store.publishCalls != 0 || denied.calls != 1 ||
			denied.attempt.Reason != "scope_inactive" {
			t.Fatalf("repository reauthorization = %v/%d/%d/%d/%d/%#v", err, fixture.reader.calls, store.resolveCalls, store.publishCalls, denied.calls, denied.attempt)
		}
	})
}

func TestPublisherNormalizesSnapshotStoreAndGetFailures(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	fixture.reader.err = moduleapp.ErrPlanNotFound
	publisher := releaseTestPublisher(t, fixture.reader, &releaseTestStore{}, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
	if _, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-missing",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing snapshot error = %v", err)
	}

	fixture = newPublishFixture(t, false)
	store := &releaseTestStore{publishErr: ErrIdempotencyConflict}
	publisher = releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
	if _, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-conflict",
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("idempotency error = %v", err)
	}
	store.publishErr = errors.New("database offline")
	if _, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-offline",
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("store unavailable error = %v", err)
	}
	store.getErr = ErrNotFound
	if _, err := publisher.Get(context.Background(), fixture.invocation, "notes", "019f5c36-b328-7c52-9325-ec59f95c8fae"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(not found) error = %v", err)
	}
}

func TestPublisherRejectsCorruptStoreResults(t *testing.T) {
	t.Parallel()
	fixture := newPublishFixture(t, false)
	store := &releaseTestStore{}
	publisher := releaseTestPublisher(t, fixture.reader, store, &releaseTestAuthorizer{}, &releaseTestDeniedAuditor{})
	valid, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-valid",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherScope := releaseTestScope(t, fixture.scope.ProjectID().String(), "019f5c36-b399-7c52-9325-ec59f95c8fae")
	store.getValue = cloneReleaseInScope(t, valid, otherScope)
	if _, err := publisher.Get(context.Background(), fixture.invocation, "notes", valid.ID().String()); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("cross-scope Get() error = %v", err)
	}
	store.stored = cloneReleaseCandidate(t, valid, "sha256:"+strings.Repeat("9", 64))
	if _, _, err := publisher.Publish(context.Background(), fixture.invocation, "notes", PublishInput{
		PlanID: fixture.plan.ID, IdempotencyKey: "publish-corrupt",
	}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("cross-intent Store result error = %v", err)
	}
}

func TestUUIDv7GeneratorIsDeterministicAndRejectsInvalidDependencies(t *testing.T) {
	t.Parallel()
	if _, err := NewUUIDv7Generator(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewUUIDv7Generator(nil) error = %v", err)
	}
	generator, err := NewUUIDv7Generator(bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}))
	if err != nil {
		t.Fatal(err)
	}
	id, err := generator.New(time.UnixMilli(1_700_000_000_123))
	if err != nil || !id.Valid() || !strings.HasPrefix(id.String(), "018bcfe5-687b-7") {
		t.Fatalf("UUIDv7Generator.New() = %q, %v", id.String(), err)
	}
	if _, err := generator.New(time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero timestamp error = %v", err)
	}
	exhausted, _ := NewUUIDv7Generator(bytes.NewReader(nil))
	if _, err := exhausted.New(releaseTestTime); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("exhausted entropy error = %v", err)
	}
}

type publishFixture struct {
	scope      project.Scope
	invocation access.Invocation
	draft      domainmodule.Draft
	validation moduleapp.DraftValidation
	plan       moduleapp.DraftPlan
	reader     *releaseTestSnapshotReader
}

func newPublishFixture(t *testing.T, destructive bool) publishFixture {
	t.Helper()
	scope := releaseTestScope(t, "019f5c36-b322-7c52-9325-ec59f95c8fae", "019f5c36-b323-7c52-9325-ec59f95c8fae")
	invocation := releaseTestInvocation(t, scope)
	store := &releaseTestDraftStore{}
	revisions := &releaseTestRevisionReader{}
	baselineText := "none"
	if destructive {
		baselineSource := releaseTestSource("notes", "1.0.0", "\n    - name: note\n      fields:\n        - name: title\n          type: string")
		revisions.value = releaseTestRevision(t, scope.ProjectID(), baselineSource)
		baselineText = revisions.value.RevisionHash()
	}
	workflow, err := moduleapp.NewDraftWorkflow(
		store, revisions, &releaseTestAuthorizer{}, fixedReleaseClock{at: releaseTestTime}, fixedDraftIDGenerator{},
	)
	if err != nil {
		t.Fatal(err)
	}
	source := releaseTestSource("notes", "2.0.0", " []")
	draft, _, err := workflow.Create(context.Background(), invocation.Execution(), "notes", moduleapp.CreateDraftInput{
		BaselineRevision: baselineText, Format: domainmodule.SourceFormatYAML, Source: source, IdempotencyKey: "draft-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	validation, _, err := workflow.Validate(context.Background(), invocation.Execution(), "notes", draft.ID().String(), draft.Generation())
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := workflow.Plan(context.Background(), invocation.Execution(), "notes", draft.ID().String(), validation.ID, draft.Generation())
	if err != nil {
		t.Fatal(err)
	}
	reader := &releaseTestSnapshotReader{draft: draft, validation: validation, plan: plan}
	return publishFixture{scope: scope, invocation: invocation, draft: draft, validation: validation, plan: plan, reader: reader}
}

func releaseTestPublisher(
	t *testing.T,
	snapshots SnapshotReader,
	store ReleaseStore,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
) *Publisher {
	t.Helper()
	value, err := NewPublisher(
		snapshots, store, authorizer, denied, fixedReleaseClock{at: releaseTestTime},
		fixedReleaseIDGenerator{id: releaseTestID(t, "019f5c36-b328-7c52-9325-ec59f95c8fae")},
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func releaseTestInvocation(t *testing.T, scope project.Scope) access.Invocation {
	t.Helper()
	credentialID := releaseTestAccessID(t, "019f5c36-b326-7c52-9325-ec59f95c8fae")
	secret := bytes.Repeat([]byte{7}, access.CredentialSecretBytes)
	token := access.CredentialTokenPrefix + "." + credentialID.String() + "." + base64.RawURLEncoding.EncodeToString(secret)
	digest := sha256.Sum256([]byte(token))
	credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: scope, ID: credentialID, PrincipalID: "owner", Label: "test publisher",
		Hint: "sha256:123456789abc", Status: domainaccess.CredentialStatusActive,
		IssuedBy: "owner", IssuedAt: releaseTestTime.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := access.NewCredentialAuthenticator(releaseTestCredentialLookup{
		candidate: access.CredentialCandidate{Credential: credential, SecretDigest: access.SecretDigest(digest)},
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.Authenticate(context.Background(), scope, token)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := access.NewAdminExecution(scope, principal)
	if err != nil {
		t.Fatal(err)
	}
	invocation, err := access.NewInvocation(execution, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}

func replaceFixtureDraft(t *testing.T, current domainmodule.Draft) domainmodule.Draft {
	t.Helper()
	source := append([]byte("# changed current generation\n"), current.Source()...)
	value, err := domainmodule.NewDraft(domainmodule.DraftMaterial{
		ID: current.ID(), ProjectID: current.ProjectID(), ModuleName: current.ModuleName(), Baseline: current.Baseline(),
		SourceFormat: current.SourceFormat(), SourceHash: domainmodule.DraftSourceHash(source), Source: source,
		Generation: current.Generation() + 1, CreatedBy: current.CreatedBy(), CreatedAt: current.CreatedAt(),
		UpdatedBy: "owner", UpdatedAt: current.UpdatedAt().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func releaseTestSource(module, version, resources string) []byte {
	return []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\nmetadata:\n  name: " + module +
		"\n  version: " + version + "\nspec:\n  resources:" + resources + "\n")
}

func releaseTestRevision(t *testing.T, projectID project.ID, source []byte) domainmodule.Revision {
	t.Helper()
	compiled, err := moduleapp.NewCompiler().Compile(source, spec.FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := domainmodule.NewDataSchemaIdentity(compiled.DataSchemaFormat(), compiled.DataSchemaFingerprint())
	if err != nil {
		t.Fatal(err)
	}
	value, err := domainmodule.NewRevision(domainmodule.RevisionMaterial{
		ProjectID: projectID, ModuleName: compiled.Name(), ModuleVersion: compiled.Version(),
		RevisionHash: compiled.RevisionHash(), DataSchemaIdentities: []domainmodule.DataSchemaIdentity{identity},
		SpecVersion: spec.APIVersion, IRFormat: moduleapp.IRFormatVersion, SourceFormat: domainmodule.SourceFormatYAML,
		SourceHash: domainmodule.DraftSourceHash(source), Source: source, CanonicalIR: compiled.CanonicalIR(),
		OpenAPI: compiled.OpenAPI(), ManagerSchema: compiled.ManagerUISchema(),
		Origin: domainmodule.RevisionOriginBootstrap, RegisteredBy: "system:bootstrap", RegisteredAt: releaseTestTime.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func cloneReleaseInScope(t *testing.T, value domainrelease.ModuleRelease, scope project.Scope) domainrelease.ModuleRelease {
	t.Helper()
	return releaseFromMaterial(t, value, scope, value.CandidateRevision())
}

func cloneReleaseCandidate(t *testing.T, value domainrelease.ModuleRelease, candidate string) domainrelease.ModuleRelease {
	t.Helper()
	return releaseFromMaterial(t, value, value.Scope(), candidate)
}

func releaseFromMaterial(t *testing.T, value domainrelease.ModuleRelease, scope project.Scope, candidate string) domainrelease.ModuleRelease {
	t.Helper()
	result, err := domainrelease.NewModuleRelease(domainrelease.ModuleReleaseMaterial{
		ID: value.ID(), Scope: scope, ModuleName: value.ModuleName(), DraftID: value.DraftID(),
		DraftGeneration: value.DraftGeneration(), ValidationID: value.ValidationID(), PlanID: value.PlanID(),
		PlanHash: value.PlanHash(), BaselineRevision: value.BaselineRevision(), CandidateRevision: candidate,
		DataSchemaFormat: value.DataSchemaFormat(), DataSchemaFingerprint: value.DataSchemaFingerprint(),
		SourceHash: value.SourceHash(), Outcome: value.Outcome(), Risk: value.Risk(), PublishedBy: value.PublishedBy(),
		PublishedCredentialID: value.PublishedCredentialID(), RequestID: value.RequestID(), PublishedAt: value.PublishedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type releaseTestSnapshotReader struct {
	draft      domainmodule.Draft
	validation moduleapp.DraftValidation
	plan       moduleapp.DraftPlan
	err        error
	calls      int
}

func (reader *releaseTestSnapshotReader) GetPublishSnapshot(
	_ context.Context,
	_ project.ID,
	_ string,
	_ string,
) (domainmodule.Draft, moduleapp.DraftValidation, moduleapp.DraftPlan, error) {
	reader.calls++
	return reader.draft, reader.validation, reader.plan, reader.err
}

type releaseTestStore struct {
	resolveErr        error
	publishErr        error
	getErr            error
	resolved          domainrelease.ModuleRelease
	resolveFound      bool
	stored            domainrelease.ModuleRelease
	getValue          domainrelease.ModuleRelease
	resolveMutation   access.MutationContext
	mutation          access.MutationContext
	revision          domainmodule.Revision
	resolveModule     string
	resolvePlanID     string
	resolveKey        string
	resolveIntentHash string
	resolveAt         time.Time
	key               string
	intentHash        string
	resolveCalls      int
	publishCalls      int
	getCalls          int
}

func (store *releaseTestStore) ResolveReplay(
	_ context.Context,
	mutation access.MutationContext,
	module string,
	planID string,
	key string,
	intentHash string,
	at time.Time,
) (domainrelease.ModuleRelease, bool, error) {
	store.resolveCalls++
	store.resolveMutation = mutation
	store.resolveModule, store.resolvePlanID = module, planID
	store.resolveKey, store.resolveIntentHash, store.resolveAt = key, intentHash, at
	if store.resolveErr != nil {
		return domainrelease.ModuleRelease{}, false, store.resolveErr
	}
	return store.resolved, store.resolveFound, nil
}

func (store *releaseTestStore) Publish(
	_ context.Context,
	mutation access.MutationContext,
	revision domainmodule.Revision,
	proposed domainrelease.ModuleRelease,
	key string,
	intentHash string,
) (domainrelease.ModuleRelease, bool, error) {
	store.publishCalls++
	store.mutation, store.revision, store.key, store.intentHash = mutation, revision, key, intentHash
	if store.publishErr != nil {
		return domainrelease.ModuleRelease{}, false, store.publishErr
	}
	if store.stored.ID().Valid() {
		return store.stored, false, nil
	}
	return proposed, true, nil
}

func (store *releaseTestStore) Get(
	_ context.Context,
	_ project.Scope,
	_ string,
	_ domainrelease.ID,
) (domainrelease.ModuleRelease, error) {
	store.getCalls++
	return store.getValue, store.getErr
}

type releaseTestAuthorizer struct {
	err        error
	operations []access.Operation
}

func (authorizer *releaseTestAuthorizer) Authorize(
	_ context.Context,
	_ access.Execution,
	operation access.Operation,
) error {
	authorizer.operations = append(authorizer.operations, operation)
	return authorizer.err
}

type releaseTestDeniedAuditor struct {
	calls   int
	attempt access.DeniedAttempt
}

func (auditor *releaseTestDeniedAuditor) AuditDenied(_ context.Context, attempt access.DeniedAttempt) error {
	auditor.calls++
	auditor.attempt = attempt
	return nil
}

type fixedReleaseClock struct{ at time.Time }

func (clock fixedReleaseClock) Now() time.Time { return clock.at }

type fixedReleaseIDGenerator struct{ id domainrelease.ID }

func (generator fixedReleaseIDGenerator) New(time.Time) (domainrelease.ID, error) {
	return generator.id, nil
}

type fixedDraftIDGenerator struct{}

func (fixedDraftIDGenerator) New(time.Time) (domainmodule.DraftID, error) {
	return domainmodule.ParseDraftID("019f5c36-b324-7c52-9325-ec59f95c8fae")
}

type releaseTestCredentialLookup struct{ candidate access.CredentialCandidate }

func (lookup releaseTestCredentialLookup) LookupCredential(
	context.Context,
	project.Scope,
	access.CredentialSelector,
) (access.CredentialCandidate, error) {
	return lookup.candidate, nil
}

type releaseTestRevisionReader struct{ value domainmodule.Revision }

func (reader *releaseTestRevisionReader) Get(
	_ context.Context,
	_ access.Execution,
	_ string,
	_ string,
) (domainmodule.Revision, error) {
	if !reader.value.ProjectID().Valid() {
		return domainmodule.Revision{}, moduleapp.ErrRevisionNotFound
	}
	return reader.value, nil
}

type releaseTestDraftStore struct {
	draft      domainmodule.Draft
	validation moduleapp.DraftValidation
	plan       moduleapp.DraftPlan
}

func (store *releaseTestDraftStore) Create(
	_ context.Context,
	value domainmodule.Draft,
	_ string,
	_ string,
) (domainmodule.Draft, bool, error) {
	store.draft = value
	return value, true, nil
}

func (store *releaseTestDraftStore) Get(
	_ context.Context,
	_ project.ID,
	_ string,
	_ domainmodule.DraftID,
) (domainmodule.Draft, error) {
	return store.draft, nil
}

func (store *releaseTestDraftStore) Replace(
	context.Context,
	project.ID,
	string,
	domainmodule.DraftID,
	uint64,
	domainmodule.DraftReplacement,
) (domainmodule.Draft, bool, error) {
	return domainmodule.Draft{}, false, errors.New("not implemented")
}

func (store *releaseTestDraftStore) SaveValidation(
	_ context.Context,
	value moduleapp.DraftValidation,
) (moduleapp.DraftValidation, bool, error) {
	store.validation = value
	return value, true, nil
}

func (store *releaseTestDraftStore) GetValidation(
	_ context.Context,
	_ project.ID,
	_ string,
	_ domainmodule.DraftID,
	_ string,
) (moduleapp.DraftValidation, error) {
	return store.validation, nil
}

func (store *releaseTestDraftStore) SavePlan(
	_ context.Context,
	value moduleapp.DraftPlan,
	_ uint64,
) (moduleapp.DraftPlan, bool, error) {
	store.plan = value
	return value, true, nil
}

func (store *releaseTestDraftStore) GetPlan(
	_ context.Context,
	_ project.ID,
	_ string,
	_ domainmodule.DraftID,
	_ string,
) (moduleapp.DraftPlan, error) {
	return store.plan, nil
}

func releaseTestScope(t *testing.T, projectText, environmentText string) project.Scope {
	t.Helper()
	projectID, err := project.ParseID(projectText)
	if err != nil {
		t.Fatal(err)
	}
	environmentID, err := project.ParseEnvironmentID(environmentText)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func releaseTestID(t *testing.T, value string) domainrelease.ID {
	t.Helper()
	id, err := domainrelease.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func releaseTestAccessID(t *testing.T, value string) domainaccess.ID {
	t.Helper()
	id, err := domainaccess.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
