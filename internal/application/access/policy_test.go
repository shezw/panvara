/*
   Panvara
   internal/application/access/policy_test.go    2026-07-18
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
	"context"
	"errors"
	"testing"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

var errGrantStore = errors.New("grant store offline")

type fakeGrantReader struct {
	active          bool
	scopeError      error
	credentialOff   bool
	credentialError error
	grant           bool
	grantError      error
	grantScope      project.Scope
	scopeCalls      int
	grantCalls      int
	credentialCalls int
	lastScope       project.Scope
	lastPrincipalID string
	lastRole        string
}

func (reader *fakeGrantReader) ScopeActive(_ context.Context, scope project.Scope) (bool, error) {
	reader.scopeCalls++
	reader.lastScope = scope
	return reader.active, reader.scopeError
}

func (reader *fakeGrantReader) CredentialActive(
	_ context.Context,
	scope project.Scope,
	principalID string,
	_ domainaccess.ID,
) (bool, error) {
	reader.credentialCalls++
	reader.lastScope = scope
	reader.lastPrincipalID = principalID
	if reader.credentialError != nil {
		return false, reader.credentialError
	}
	return !reader.credentialOff, nil
}

func (reader *fakeGrantReader) HasActiveGrant(
	_ context.Context,
	scope project.Scope,
	principalID string,
	role string,
) (bool, error) {
	reader.grantCalls++
	reader.lastScope = scope
	reader.lastPrincipalID = principalID
	reader.lastRole = role
	if reader.grantError != nil {
		return false, reader.grantError
	}
	if !reader.grant {
		return false, nil
	}
	if err := reader.grantScope.Validate(); err != nil {
		return true, nil
	}
	return sameScope(reader.grantScope, scope), nil
}

func TestPolicyAllowsPublicRecordOperationsWithoutRoleGrant(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(t, scope, testAnonymous(t, testProjectID), SurfacePublic)
	reader := &fakeGrantReader{active: true}
	policy := mustPolicy(t, reader)

	for _, operation := range []Operation{
		OperationRecordList,
		OperationRecordGet,
		OperationRecordCreate,
		OperationRecordPatch,
		OperationRecordDelete,
	} {
		if err := policy.Authorize(context.Background(), execution, operation); err != nil {
			t.Fatalf("Authorize(%q) error = %v", operation, err)
		}
	}
	if reader.scopeCalls != 5 || reader.grantCalls != 0 {
		t.Fatalf("scope calls = %d, grant calls = %d", reader.scopeCalls, reader.grantCalls)
	}
}

func TestPolicyAllowsAdminOnlyFromAuthoritativeExactGrant(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	subject := testActor(t, testProjectID, "owner-1", nil)
	execution := mustExecution(t, scope, subject, SurfaceAdmin)
	reader := &fakeGrantReader{active: true, grant: true, grantScope: scope}
	policy := mustPolicy(t, reader)

	for _, operation := range []Operation{OperationDraftPlan, OperationReleaseActivate, OperationReleaseGetActive} {
		if err := policy.Authorize(context.Background(), execution, operation); err != nil {
			t.Fatalf("Authorize(%q) error = %v", operation, err)
		}
	}
	if reader.lastPrincipalID != "owner-1" ||
		reader.lastRole != RoleProjectOwner ||
		!sameScope(reader.lastScope, scope) {
		t.Fatalf(
			"grant query = scope %v, principal %q, role %q",
			reader.lastScope,
			reader.lastPrincipalID,
			reader.lastRole,
		)
	}
}

func TestPolicyRejectsAnonymousAdmin(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	if _, err := NewExecution(scope, testAnonymous(t, testProjectID), SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewExecution() error = %v, want ErrInvalid", err)
	}
}

func TestPolicyRejectsInactiveCredentialBeforeGrant(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(t, scope, testActor(t, testProjectID, "owner-1", nil), SurfaceAdmin)
	reader := &fakeGrantReader{active: true, credentialOff: true, grant: true, grantScope: scope}

	err := mustPolicy(t, reader).Authorize(context.Background(), execution, OperationRecordList)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("Authorize() error = %v, want ErrUnauthenticated", err)
	}
	if reader.credentialCalls != 1 || reader.grantCalls != 0 {
		t.Fatalf("credential calls = %d, grant calls = %d", reader.credentialCalls, reader.grantCalls)
	}
}

func TestPolicyIgnoresActorSelfReportedRolesAndRejectsRevokedGrant(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	subject := testActor(t, testProjectID, "owner-1", []string{RoleProjectOwner})
	execution := mustExecution(t, scope, subject, SurfaceAdmin)
	reader := &fakeGrantReader{active: true, grant: false}

	err := mustPolicy(t, reader).Authorize(context.Background(), execution, OperationRevisionList)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Authorize() error = %v, want ErrForbidden", err)
	}
	if reader.grantCalls != 1 {
		t.Fatalf("grant calls = %d, want 1", reader.grantCalls)
	}
}

func TestPolicyRejectsGrantFromAnotherEnvironment(t *testing.T) {
	requestScope := testScope(t, testProjectID, otherEnvironment)
	grantedScope := testScope(t, testProjectID, testEnvironmentID)
	subject := testActor(t, testProjectID, "owner-1", nil)
	execution := mustExecution(t, requestScope, subject, SurfaceAdmin)
	reader := &fakeGrantReader{active: true, grant: true, grantScope: grantedScope}

	err := mustPolicy(t, reader).Authorize(context.Background(), execution, OperationDraftGet)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-environment Authorize() error = %v, want ErrForbidden", err)
	}
}

func TestPolicyRejectsInactiveScopeBeforeSurfacePolicy(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(t, scope, testAnonymous(t, testProjectID), SurfacePublic)
	reader := &fakeGrantReader{active: false}

	err := mustPolicy(t, reader).Authorize(context.Background(), execution, OperationRecordCreate)
	if !errors.Is(err, ErrScopeInactive) {
		t.Fatalf("Authorize() error = %v, want ErrScopeInactive", err)
	}
	if reader.scopeCalls != 1 || reader.grantCalls != 0 {
		t.Fatalf("scope calls = %d, grant calls = %d", reader.scopeCalls, reader.grantCalls)
	}
}

func TestPolicyMapsGrantReaderFailuresToUnavailable(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	admin := mustExecution(
		t,
		scope,
		testActor(t, testProjectID, "owner-1", nil),
		SurfaceAdmin,
	)

	tests := []struct {
		name   string
		reader *fakeGrantReader
	}{
		{name: "scope", reader: &fakeGrantReader{scopeError: errGrantStore}},
		{name: "credential", reader: &fakeGrantReader{active: true, credentialError: errGrantStore}},
		{name: "grant", reader: &fakeGrantReader{active: true, grantError: errGrantStore}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := mustPolicy(t, test.reader).Authorize(
				context.Background(),
				admin,
				OperationRecordPatch,
			)
			if !errors.Is(err, ErrUnavailable) || !errors.Is(err, errGrantStore) {
				t.Fatalf("Authorize() error = %v, want ErrUnavailable wrapping store error", err)
			}
		})
	}
}

func TestPolicyUnknownOperationFailsClosedAfterScopeCheck(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(
		t,
		scope,
		testActor(t, testProjectID, "owner-1", nil),
		SurfaceAdmin,
	)
	reader := &fakeGrantReader{active: true, grant: true, grantScope: scope}

	err := mustPolicy(t, reader).Authorize(
		context.Background(),
		execution,
		Operation("draft.publish"),
	)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Authorize() error = %v, want ErrForbidden", err)
	}
	if reader.scopeCalls != 1 || reader.grantCalls != 0 {
		t.Fatalf("scope calls = %d, grant calls = %d", reader.scopeCalls, reader.grantCalls)
	}
}

func TestPolicyPublicSurfaceRejectsRevisionAndDraftOperations(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(t, scope, testAnonymous(t, testProjectID), SurfacePublic)
	reader := &fakeGrantReader{active: true}
	policy := mustPolicy(t, reader)

	operations := []Operation{
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
		OperationReleasePublish,
		OperationReleaseGet,
		OperationReleaseActivate,
		OperationReleaseGetActive,
	}
	for _, operation := range operations {
		if err := policy.Authorize(context.Background(), execution, operation); !errors.Is(err, ErrForbidden) {
			t.Fatalf("Authorize(%q) error = %v, want ErrForbidden", operation, err)
		}
	}
	if reader.grantCalls != 0 {
		t.Fatalf("public authorization caused %d grant reads", reader.grantCalls)
	}
}

func TestPolicyRejectsInvalidConstructionAndExecution(t *testing.T) {
	if _, err := NewPolicy(nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("NewPolicy(nil) error = %v, want ErrInvalidRequest", err)
	}
	var policy *Policy
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(t, scope, testAnonymous(t, testProjectID), SurfacePublic)
	if err := policy.Authorize(context.Background(), execution, OperationRecordList); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil Policy.Authorize() error = %v, want ErrUnavailable", err)
	}
	reader := &fakeGrantReader{active: true}
	if err := mustPolicy(t, reader).Authorize(context.Background(), Execution{}, OperationRecordList); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("zero Execution Authorize() error = %v, want ErrInvalidRequest", err)
	}
}

func mustPolicy(t *testing.T, reader GrantReader) *Policy {
	t.Helper()
	policy, err := NewPolicy(reader)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func mustExecution(
	t *testing.T,
	scope project.Scope,
	subject actor.Context,
	surface Surface,
) Execution {
	t.Helper()
	var execution Execution
	var err error
	if surface == SurfaceAdmin {
		credentialID, parseErr := domainaccess.ParseID("019f5c36-b326-7c52-9325-ec59f95c8fae")
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		principal, principalErr := newAuthenticatedPrincipal(scope, subject, credentialID)
		if principalErr != nil {
			t.Fatal(principalErr)
		}
		execution, err = NewAdminExecution(scope, principal)
	} else {
		execution, err = NewExecution(scope, subject, surface)
	}
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

func sameScope(left, right project.Scope) bool {
	return left.ProjectID().String() == right.ProjectID().String() &&
		left.EnvironmentID().String() == right.EnvironmentID().String()
}
