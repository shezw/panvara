/*
   Panvara
   internal/application/access/types_test.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package access

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	testProjectID     = "019f5c36-b322-7c52-9325-ec59f95c8fae"
	otherProjectID    = "019f5c36-b323-7c52-9325-ec59f95c8fae"
	testEnvironmentID = "019f5c36-b324-7c52-9325-ec59f95c8fae"
	otherEnvironment  = "019f5c36-b325-7c52-9325-ec59f95c8fae"
)

type fixedEnvironmentClock struct {
	at time.Time
}

func (clock fixedEnvironmentClock) Now() time.Time {
	return clock.at
}

func TestEnvironmentIDGeneratorProducesParseableDeterministicUUIDv7(t *testing.T) {
	t.Parallel()

	generator, err := project.NewEnvironmentIDGenerator(
		bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}),
		fixedEnvironmentClock{at: time.UnixMilli(1_700_000_000_123)},
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := generator.New()
	if err != nil {
		t.Fatal(err)
	}
	if !id.Valid() {
		t.Fatalf("generated environment id %q is invalid", id.String())
	}
	parsed, err := project.ParseEnvironmentID("  " + id.String() + "  ")
	if err != nil || parsed.String() != id.String() {
		t.Fatalf("ParseEnvironmentID() = %q, %v", parsed.String(), err)
	}
}

func TestEnvironmentIDGeneratorRejectsInvalidDependenciesAndEntropy(t *testing.T) {
	t.Parallel()

	clock := fixedEnvironmentClock{at: time.UnixMilli(1)}
	if _, err := project.NewEnvironmentIDGenerator(nil, clock); err == nil {
		t.Fatal("NewEnvironmentIDGenerator() accepted nil entropy")
	}
	if _, err := project.NewEnvironmentIDGenerator(bytes.NewReader(nil), nil); err == nil {
		t.Fatal("NewEnvironmentIDGenerator() accepted nil clock")
	}
	generator, err := project.NewEnvironmentIDGenerator(bytes.NewReader(nil), clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generator.New(); err == nil {
		t.Fatal("EnvironmentIDGenerator.New() accepted exhausted entropy")
	}
	var zero *project.EnvironmentIDGenerator
	if _, err := zero.New(); err == nil {
		t.Fatal("nil EnvironmentIDGenerator.New() unexpectedly succeeded")
	}
}

func TestScopeAndExecutionPreserveExactBoundary(t *testing.T) {
	t.Parallel()

	scope := testScope(t, testProjectID, testEnvironmentID)
	subject := testActor(t, testProjectID, "owner", nil)
	execution := mustExecution(t, scope, subject, SurfaceAdmin)
	if execution.Scope().ProjectID().String() != testProjectID ||
		execution.Scope().EnvironmentID().String() != testEnvironmentID ||
		execution.Actor().ActorID() != "owner" ||
		execution.Surface() != SurfaceAdmin {
		t.Fatalf("execution = %+v", execution)
	}
}

func TestExecutionRejectsCrossProjectAndMalformedInput(t *testing.T) {
	t.Parallel()

	scope := testScope(t, testProjectID, testEnvironmentID)
	other := testActor(t, otherProjectID, "owner", nil)
	if _, err := NewExecution(scope, other, SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-project NewExecution() error = %v, want ErrInvalidRequest", err)
	}
	subject := testActor(t, testProjectID, "owner", nil)
	if _, err := NewExecution(scope, subject, Surface("unknown")); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid surface NewExecution() error = %v, want ErrInvalidRequest", err)
	}
	if _, err := NewExecution(project.Scope{}, subject, SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero scope NewExecution() error = %v, want ErrInvalidRequest", err)
	}
}

func TestAllDeclaredOperationsAreValid(t *testing.T) {
	t.Parallel()

	for _, operation := range allOperations() {
		if !operation.Valid() {
			t.Fatalf("operation %q is invalid", operation)
		}
	}
	if Operation("record.publish").Valid() {
		t.Fatal("unknown operation unexpectedly valid")
	}
}

func allOperations() []Operation {
	return []Operation{
		OperationRecordList,
		OperationRecordGet,
		OperationRecordCreate,
		OperationRecordPatch,
		OperationRecordDelete,
		OperationRevisionList,
		OperationRevisionGet,
		OperationRevisionGetSource,
		OperationDraftCreate,
		OperationDraftGet,
		OperationDraftGetSource,
		OperationDraftReplace,
		OperationDraftValidate,
		OperationDraftPlan,
		OperationDraftGetValidation,
		OperationDraftGetPlan,
		OperationPrincipalList,
		OperationPrincipalCreate,
		OperationPrincipalDisable,
		OperationCredentialList,
		OperationCredentialIssue,
		OperationCredentialRevoke,
		OperationCredentialBootstrap,
		OperationProjectOwnerList,
		OperationProjectOwnerGrant,
		OperationProjectOwnerRevoke,
	}
}

func testScope(t *testing.T, projectText, environmentText string) project.Scope {
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

func testActor(t *testing.T, projectID, actorID string, roles []string) actor.Context {
	t.Helper()
	subject, err := actor.New(projectID, actorID, roles)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}

func testAnonymous(t *testing.T, projectID string) actor.Context {
	t.Helper()
	subject, err := actor.NewAnonymous(projectID)
	if err != nil {
		t.Fatal(err)
	}
	return subject
}
