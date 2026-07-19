/*
   Panvara
   internal/application/access/administration_test.go    2026-07-19
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
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

type fakeAuthorizer struct {
	err       error
	calls     int
	operation Operation
}

func (authorizer *fakeAuthorizer) Authorize(
	_ context.Context,
	_ Execution,
	operation Operation,
) error {
	authorizer.calls++
	authorizer.operation = operation
	return authorizer.err
}

type fakeDeniedAuditor struct {
	err        error
	calls      int
	attempt    DeniedAttempt
	contextErr error
}

func (auditor *fakeDeniedAuditor) AuditDenied(
	ctx context.Context,
	attempt DeniedAttempt,
) error {
	auditor.calls++
	auditor.attempt = attempt
	auditor.contextErr = ctx.Err()
	return auditor.err
}

type fixedAccessIDGenerator struct {
	id    domainaccess.ID
	err   error
	calls int
}

func (generator *fixedAccessIDGenerator) New() (domainaccess.ID, error) {
	generator.calls++
	return generator.id, generator.err
}

type fixedAccessClock struct{ at time.Time }

func (clock fixedAccessClock) Now() time.Time { return clock.at }

type fakeAdminRepository struct {
	issue            CredentialIssue
	mutation         MutationContext
	issueCalls       int
	createCalls      int
	mutationError    error
	issuedCredential domainaccess.Credential
	createdPrincipal domainaccess.Principal
	principalList    []domainaccess.Principal
	credentialList   []domainaccess.Credential
	projectOwnerList []domainaccess.OwnerGrant
}

func (repository *fakeAdminRepository) ListPrincipals(
	_ context.Context,
	_ project.Scope,
) ([]domainaccess.Principal, error) {
	return repository.principalList, nil
}

func (repository *fakeAdminRepository) CreatePrincipal(
	_ context.Context,
	mutation MutationContext,
	principal domainaccess.Principal,
) (domainaccess.Principal, error) {
	repository.createCalls++
	repository.mutation = mutation
	if repository.mutationError != nil {
		return domainaccess.Principal{}, repository.mutationError
	}
	repository.createdPrincipal = principal
	return principal, nil
}

func (repository *fakeAdminRepository) DisablePrincipal(
	_ context.Context,
	_ MutationContext,
	_ string,
	_ time.Time,
) (domainaccess.Principal, error) {
	return domainaccess.Principal{}, repository.mutationError
}

func (repository *fakeAdminRepository) ListCredentials(
	_ context.Context,
	_ project.Scope,
	_ string,
) ([]domainaccess.Credential, error) {
	return repository.credentialList, nil
}

func (repository *fakeAdminRepository) IssueCredential(
	_ context.Context,
	mutation MutationContext,
	issue CredentialIssue,
) (domainaccess.Credential, error) {
	repository.issueCalls++
	repository.mutation = mutation
	repository.issue = issue
	if repository.mutationError != nil {
		return domainaccess.Credential{}, repository.mutationError
	}
	if repository.issuedCredential.Active() {
		return repository.issuedCredential, nil
	}
	return issue.Credential, nil
}

func (repository *fakeAdminRepository) RevokeCredential(
	_ context.Context,
	_ MutationContext,
	_ domainaccess.ID,
	_ time.Time,
) (domainaccess.Credential, error) {
	return domainaccess.Credential{}, repository.mutationError
}

func (repository *fakeAdminRepository) ListProjectOwners(
	_ context.Context,
	_ project.Scope,
) ([]domainaccess.OwnerGrant, error) {
	return repository.projectOwnerList, nil
}

func (repository *fakeAdminRepository) GrantProjectOwner(
	_ context.Context,
	_ MutationContext,
	_ string,
	_ time.Time,
) (domainaccess.OwnerGrant, error) {
	return domainaccess.OwnerGrant{}, repository.mutationError
}

func (repository *fakeAdminRepository) RevokeProjectOwner(
	_ context.Context,
	_ MutationContext,
	_ string,
	_ time.Time,
) (domainaccess.OwnerGrant, error) {
	return domainaccess.OwnerGrant{}, repository.mutationError
}

func TestAdministrationIssuesOneTimeTokenWithoutPassingRawSecretToRepository(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	invocation := mustInvocation(t, scope)
	id := mustAccessID(t, testCredentialID)
	repository := &fakeAdminRepository{}
	authorizer := &fakeAuthorizer{}
	auditor := &fakeDeniedAuditor{}
	ids := &fixedAccessIDGenerator{id: id}
	clock := fixedAccessClock{at: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)}
	service, err := NewAdministration(
		repository, authorizer, auditor, ids,
		bytes.NewReader(bytes.Repeat([]byte{0x5a}, CredentialSecretBytes)), clock,
	)
	if err != nil {
		t.Fatal(err)
	}

	issued, err := service.IssueCredential(context.Background(), invocation, IssueCredentialInput{
		PrincipalID: "svc:" + id.String(), Label: "deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if issued.Token == "" || issued.Credential.ID() != id || repository.issueCalls != 1 {
		t.Fatalf("issued result or repository call is incomplete")
	}
	digest := SecretDigest(sha256.Sum256([]byte(issued.Token)))
	if repository.issue.SecretDigest != digest ||
		repository.issue.Credential.Hint() != credentialHint(digest) {
		t.Fatal("repository did not receive digest-derived non-secret metadata")
	}
	if authorizer.operation != OperationCredentialIssue ||
		repository.mutation.Operation() != OperationCredentialIssue ||
		repository.mutation.CredentialID() != invocation.Execution().CredentialID() ||
		repository.mutation.RequestID() != invocation.RequestID() {
		t.Fatal("fixed authorization or transaction reauthorization evidence was lost")
	}
	if auditor.calls != 0 {
		t.Fatalf("successful issue wrote %d denied audits", auditor.calls)
	}
}

func TestAdministrationAuthorizesBeforeInputAndDeniedAuditIsBestEffort(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	invocation := mustInvocation(t, scope)
	repository := &fakeAdminRepository{}
	authorizer := &fakeAuthorizer{err: ErrForbidden}
	auditor := &fakeDeniedAuditor{err: errors.New("audit offline")}
	ids := &fixedAccessIDGenerator{id: mustAccessID(t, testCredentialID)}
	service := mustAdministration(t, repository, authorizer, auditor, ids)

	_, err := service.IssueCredential(context.Background(), invocation, IssueCredentialInput{})
	if !errors.Is(err, ErrForbidden) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("IssueCredential() error = %v, want original ErrForbidden only", err)
	}
	if authorizer.calls != 1 || ids.calls != 0 || repository.issueCalls != 0 {
		t.Fatalf("authorization ordering calls: auth=%d ids=%d repository=%d",
			authorizer.calls, ids.calls, repository.issueCalls)
	}
	if auditor.calls != 1 || auditor.attempt.Operation != OperationCredentialIssue ||
		auditor.attempt.Reason != "forbidden" || auditor.contextErr != nil {
		t.Fatalf("denied audit = %+v, calls=%d, context=%v",
			auditor.attempt, auditor.calls, auditor.contextErr)
	}
}

func TestAdministrationAuditsTransactionReauthorizationDenial(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	invocation := mustInvocation(t, scope)
	repository := &fakeAdminRepository{mutationError: ErrUnauthenticated}
	authorizer := &fakeAuthorizer{}
	auditor := &fakeDeniedAuditor{}
	ids := &fixedAccessIDGenerator{id: mustAccessID(t, testCredentialID)}
	service := mustAdministration(t, repository, authorizer, auditor, ids)

	_, err := service.CreatePrincipal(context.Background(), invocation, CreatePrincipalInput{
		DisplayName: "worker",
	})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("CreatePrincipal() error = %v, want ErrUnauthenticated", err)
	}
	if authorizer.calls != 1 || repository.createCalls != 1 || auditor.calls != 1 ||
		auditor.attempt.Operation != OperationPrincipalCreate ||
		auditor.attempt.Reason != "unauthenticated" {
		t.Fatalf("transaction denial was not audited: %+v", auditor.attempt)
	}
}

func TestInvocationRejectsMissingOrMalformedRequestID(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	execution := mustExecution(
		t, scope, testActor(t, testProjectID, "bootstrap-admin", nil), SurfaceAdmin,
	)
	for _, requestID := range []string{"", "bad request", string(bytes.Repeat([]byte{'a'}, 129))} {
		if _, err := NewInvocation(execution, requestID); !errors.Is(err, ErrInvalid) {
			t.Fatalf("NewInvocation(%q) error = %v, want ErrInvalid", requestID, err)
		}
	}
}

func mustAdministration(
	t *testing.T,
	repository AccessAdminRepository,
	authorizer Authorizer,
	auditor DeniedAuditor,
	ids AccessIDGenerator,
) *Administration {
	t.Helper()
	service, err := NewAdministration(
		repository, authorizer, auditor, ids,
		bytes.NewReader(bytes.Repeat([]byte{0x5a}, CredentialSecretBytes)),
		fixedAccessClock{at: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func mustInvocation(t *testing.T, scope project.Scope) Invocation {
	t.Helper()
	execution := mustExecution(
		t, scope, testActor(t, scope.ProjectID().String(), "bootstrap-admin", nil), SurfaceAdmin,
	)
	invocation, err := NewInvocation(execution, "request-01")
	if err != nil {
		t.Fatal(err)
	}
	return invocation
}
