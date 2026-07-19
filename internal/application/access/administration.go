/*
   Panvara
   internal/application/access/administration.go    2026-07-19
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
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	maxRequestIDBytes  = 128
	deniedAuditTimeout = 2 * time.Second
)

var requestIDPattern = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9._:-]{0,127}\z`)

// SecretDigest is the fixed SHA-256 representation persisted for a raw token.
type SecretDigest [sha256.Size]byte

// Invocation binds an authorized execution to one transport correlation ID.
// Its fields are private so adapters cannot replace authentication evidence.
type Invocation struct {
	execution Execution
	requestID string
}

// NewInvocation validates and constructs an administration invocation.
func NewInvocation(execution Execution, requestID string) (Invocation, error) {
	if err := execution.Validate(); err != nil {
		return Invocation{}, err
	}
	if len(requestID) > maxRequestIDBytes || !requestIDPattern.MatchString(requestID) {
		return Invocation{}, fmt.Errorf("%w: invalid request id", ErrInvalid)
	}
	return Invocation{execution: execution, requestID: requestID}, nil
}

// Execution returns the immutable authorization evidence.
func (invocation Invocation) Execution() Execution { return invocation.execution }

// RequestID returns the transport correlation ID.
func (invocation Invocation) RequestID() string { return invocation.requestID }

// Validate rejects incomplete invocation values.
func (invocation Invocation) Validate() error {
	if err := invocation.execution.Validate(); err != nil {
		return err
	}
	if len(invocation.requestID) > maxRequestIDBytes ||
		!requestIDPattern.MatchString(invocation.requestID) {
		return fmt.Errorf("%w: invalid request id", ErrInvalid)
	}
	return nil
}

// MutationContext is the repository's trusted instruction to reauthorize one
// exact mutation in its transaction before mutating and appending its audit.
type MutationContext struct {
	invocation Invocation
	operation  Operation
}

func newMutationContext(invocation Invocation, operation Operation) MutationContext {
	return MutationContext{invocation: invocation, operation: operation}
}

// Scope returns the exact project/environment mutation boundary.
func (mutation MutationContext) Scope() project.Scope {
	return mutation.invocation.Execution().Scope()
}

// ActorID returns the principal performing the mutation.
func (mutation MutationContext) ActorID() string {
	return mutation.invocation.Execution().Actor().ActorID()
}

// CredentialID returns the credential that must remain active in transaction.
func (mutation MutationContext) CredentialID() domainaccess.ID {
	return mutation.invocation.Execution().CredentialID()
}

// Operation returns the fixed use-case operation written to the audit event.
func (mutation MutationContext) Operation() Operation { return mutation.operation }

// RequestID returns the transport correlation ID written to the audit event.
func (mutation MutationContext) RequestID() string { return mutation.invocation.RequestID() }

// CreatePrincipalInput contains operator input for a service principal.
type CreatePrincipalInput struct {
	DisplayName string
}

// IssueCredentialInput contains operator input for a scoped API credential.
type IssueCredentialInput struct {
	PrincipalID string
	Label       string
}

// CredentialIssue contains only non-secret metadata and an irreversible digest.
// Repositories never receive the one-time raw token.
type CredentialIssue struct {
	Credential   domainaccess.Credential
	SecretDigest SecretDigest
}

// IssuedCredential is the sole application result that contains a raw token.
type IssuedCredential struct {
	Credential domainaccess.Credential
	Token      string
}

// DeniedAttempt is append-only security audit input for failed authorization.
type DeniedAttempt struct {
	Scope        project.Scope
	ActorID      string
	CredentialID domainaccess.ID
	Operation    Operation
	RequestID    string
	Reason       string
	DeniedAt     time.Time
}

// DeniedAuditor persists failed authorization independently from mutations.
type DeniedAuditor interface {
	AuditDenied(context.Context, DeniedAttempt) error
}

// AccessAdminRepository stores access views. Every mutation implementation
// MUST, in one transaction: lock the project, re-check MutationContext's exact
// credential and active project.owner grant, perform the mutation, append its
// successful security audit, and commit both or neither.
type AccessAdminRepository interface {
	ListPrincipals(context.Context, project.Scope) ([]domainaccess.Principal, error)
	CreatePrincipal(context.Context, MutationContext, domainaccess.Principal) (domainaccess.Principal, error)
	DisablePrincipal(context.Context, MutationContext, string, time.Time) (domainaccess.Principal, error)

	ListCredentials(context.Context, project.Scope, string) ([]domainaccess.Credential, error)
	IssueCredential(context.Context, MutationContext, CredentialIssue) (domainaccess.Credential, error)
	RevokeCredential(context.Context, MutationContext, domainaccess.ID, time.Time) (domainaccess.Credential, error)

	ListProjectOwners(context.Context, project.Scope) ([]domainaccess.OwnerGrant, error)
	GrantProjectOwner(context.Context, MutationContext, string, time.Time) (domainaccess.OwnerGrant, error)
	RevokeProjectOwner(context.Context, MutationContext, string, time.Time) (domainaccess.OwnerGrant, error)
}

// AccessIDGenerator creates UUIDv7 identities for principals and credentials.
type AccessIDGenerator interface {
	New() (domainaccess.ID, error)
}

// Clock supplies deterministic administration timestamps.
type Clock interface {
	Now() time.Time
}

// SystemClock supplies UTC wall-clock timestamps.
type SystemClock struct{}

// Now returns the current UTC timestamp.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// Administration implements fixed Principal, Credential, and ProjectOwner use
// cases. It never accepts caller-selected authorization operations.
type Administration struct {
	repository AccessAdminRepository
	authorizer Authorizer
	denied     DeniedAuditor
	ids        AccessIDGenerator
	entropy    io.Reader
	clock      Clock
}

// NewAdministration constructs an access administration service with explicit
// security dependencies. Use crypto/rand.Reader for production entropy.
func NewAdministration(
	repository AccessAdminRepository,
	authorizer Authorizer,
	denied DeniedAuditor,
	ids AccessIDGenerator,
	entropy io.Reader,
	clock Clock,
) (*Administration, error) {
	if repository == nil || authorizer == nil || denied == nil || ids == nil || entropy == nil || clock == nil {
		return nil, fmt.Errorf("%w: incomplete access administration dependencies", ErrInvalid)
	}
	return &Administration{
		repository: repository, authorizer: authorizer, denied: denied,
		ids: ids, entropy: entropy, clock: clock,
	}, nil
}

// NewDefaultAdministration constructs a production service using UUIDv7 IDs,
// crypto/rand entropy, and the UTC wall clock.
func NewDefaultAdministration(
	repository AccessAdminRepository,
	authorizer Authorizer,
	denied DeniedAuditor,
) (*Administration, error) {
	return NewAdministration(
		repository,
		authorizer,
		denied,
		domainaccess.NewDefaultIDGenerator(),
		rand.Reader,
		SystemClock{},
	)
}

// ListPrincipals returns project-local principals after fixed authorization.
func (administration *Administration) ListPrincipals(
	ctx context.Context,
	invocation Invocation,
) ([]domainaccess.Principal, error) {
	if err := administration.authorize(ctx, invocation, OperationPrincipalList); err != nil {
		return nil, err
	}
	values, err := administration.repository.ListPrincipals(ctx, invocation.Execution().Scope())
	if err != nil {
		return nil, normalizeRepositoryError("list principals", err)
	}
	return values, nil
}

// CreatePrincipal creates an active service principal after fixed authorization.
func (administration *Administration) CreatePrincipal(
	ctx context.Context,
	invocation Invocation,
	input CreatePrincipalInput,
) (domainaccess.Principal, error) {
	if err := administration.authorize(ctx, invocation, OperationPrincipalCreate); err != nil {
		return domainaccess.Principal{}, err
	}
	id, err := administration.ids.New()
	if err != nil {
		return domainaccess.Principal{}, fmt.Errorf("%w: generate principal id: %v", ErrUnavailable, err)
	}
	principal, err := domainaccess.NewServicePrincipal(
		invocation.Execution().Scope().ProjectID(), id, input.DisplayName, administration.clock.Now(),
	)
	if err != nil {
		return domainaccess.Principal{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	value, err := administration.repository.CreatePrincipal(
		ctx, newMutationContext(invocation, OperationPrincipalCreate), principal,
	)
	if err != nil {
		return domainaccess.Principal{}, administration.mutationRepositoryError(
			ctx, invocation, OperationPrincipalCreate, "create principal", err,
		)
	}
	return value, nil
}

// DisablePrincipal terminally disables a project-local principal.
func (administration *Administration) DisablePrincipal(
	ctx context.Context,
	invocation Invocation,
	principalID string,
) (domainaccess.Principal, error) {
	if err := administration.authorize(ctx, invocation, OperationPrincipalDisable); err != nil {
		return domainaccess.Principal{}, err
	}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return domainaccess.Principal{}, fmt.Errorf("%w: principal id is required", ErrInvalid)
	}
	value, err := administration.repository.DisablePrincipal(
		ctx,
		newMutationContext(invocation, OperationPrincipalDisable),
		principalID,
		administration.clock.Now().UTC(),
	)
	if err != nil {
		return domainaccess.Principal{}, administration.mutationRepositoryError(
			ctx, invocation, OperationPrincipalDisable, "disable principal", err,
		)
	}
	return value, nil
}

// ListCredentials returns non-secret metadata for one project-local principal.
func (administration *Administration) ListCredentials(
	ctx context.Context,
	invocation Invocation,
	principalID string,
) ([]domainaccess.Credential, error) {
	if err := administration.authorize(ctx, invocation, OperationCredentialList); err != nil {
		return nil, err
	}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return nil, fmt.Errorf("%w: principal id is required", ErrInvalid)
	}
	values, err := administration.repository.ListCredentials(
		ctx, invocation.Execution().Scope(), principalID,
	)
	if err != nil {
		return nil, normalizeRepositoryError("list credentials", err)
	}
	return values, nil
}

// IssueCredential creates a scoped credential and returns its raw token once.
func (administration *Administration) IssueCredential(
	ctx context.Context,
	invocation Invocation,
	input IssueCredentialInput,
) (IssuedCredential, error) {
	if err := administration.authorize(ctx, invocation, OperationCredentialIssue); err != nil {
		return IssuedCredential{}, err
	}
	id, err := administration.ids.New()
	if err != nil {
		return IssuedCredential{}, fmt.Errorf("%w: generate credential id: %v", ErrUnavailable, err)
	}
	token, digest, err := generateCredentialToken(administration.entropy, id)
	if err != nil {
		return IssuedCredential{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: invocation.Execution().Scope(), ID: id,
		PrincipalID: strings.TrimSpace(input.PrincipalID), Label: input.Label,
		Hint: credentialHint(digest), Status: domainaccess.CredentialStatusActive,
		IssuedBy: invocation.Execution().Actor().ActorID(), IssuedAt: administration.clock.Now(),
	})
	if err != nil {
		return IssuedCredential{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	stored, err := administration.repository.IssueCredential(
		ctx,
		newMutationContext(invocation, OperationCredentialIssue),
		CredentialIssue{Credential: credential, SecretDigest: digest},
	)
	if err != nil {
		return IssuedCredential{}, administration.mutationRepositoryError(
			ctx, invocation, OperationCredentialIssue, "issue credential", err,
		)
	}
	return IssuedCredential{Credential: stored, Token: token}, nil
}

// RevokeCredential terminally revokes one credential.
func (administration *Administration) RevokeCredential(
	ctx context.Context,
	invocation Invocation,
	credentialID domainaccess.ID,
) (domainaccess.Credential, error) {
	if err := administration.authorize(ctx, invocation, OperationCredentialRevoke); err != nil {
		return domainaccess.Credential{}, err
	}
	if !credentialID.Valid() {
		return domainaccess.Credential{}, fmt.Errorf("%w: invalid credential id", ErrInvalid)
	}
	value, err := administration.repository.RevokeCredential(
		ctx,
		newMutationContext(invocation, OperationCredentialRevoke),
		credentialID,
		administration.clock.Now().UTC(),
	)
	if err != nil {
		return domainaccess.Credential{}, administration.mutationRepositoryError(
			ctx, invocation, OperationCredentialRevoke, "revoke credential", err,
		)
	}
	return value, nil
}

// ListProjectOwners returns all project-owner grant lifecycle views.
func (administration *Administration) ListProjectOwners(
	ctx context.Context,
	invocation Invocation,
) ([]domainaccess.OwnerGrant, error) {
	if err := administration.authorize(ctx, invocation, OperationProjectOwnerList); err != nil {
		return nil, err
	}
	values, err := administration.repository.ListProjectOwners(ctx, invocation.Execution().Scope())
	if err != nil {
		return nil, normalizeRepositoryError("list project owners", err)
	}
	return values, nil
}

// GrantProjectOwner grants owner authority to one active principal.
func (administration *Administration) GrantProjectOwner(
	ctx context.Context,
	invocation Invocation,
	principalID string,
) (domainaccess.OwnerGrant, error) {
	if err := administration.authorize(ctx, invocation, OperationProjectOwnerGrant); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return domainaccess.OwnerGrant{}, fmt.Errorf("%w: principal id is required", ErrInvalid)
	}
	value, err := administration.repository.GrantProjectOwner(
		ctx,
		newMutationContext(invocation, OperationProjectOwnerGrant),
		principalID,
		administration.clock.Now().UTC(),
	)
	if err != nil {
		return domainaccess.OwnerGrant{}, administration.mutationRepositoryError(
			ctx, invocation, OperationProjectOwnerGrant, "grant project owner", err,
		)
	}
	return value, nil
}

// RevokeProjectOwner revokes one owner's exact-scope grant. A later authorized
// GrantProjectOwner call may explicitly grant that principal again.
func (administration *Administration) RevokeProjectOwner(
	ctx context.Context,
	invocation Invocation,
	principalID string,
) (domainaccess.OwnerGrant, error) {
	if err := administration.authorize(ctx, invocation, OperationProjectOwnerRevoke); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return domainaccess.OwnerGrant{}, fmt.Errorf("%w: principal id is required", ErrInvalid)
	}
	value, err := administration.repository.RevokeProjectOwner(
		ctx,
		newMutationContext(invocation, OperationProjectOwnerRevoke),
		principalID,
		administration.clock.Now().UTC(),
	)
	if err != nil {
		return domainaccess.OwnerGrant{}, administration.mutationRepositoryError(
			ctx, invocation, OperationProjectOwnerRevoke, "revoke project owner", err,
		)
	}
	return value, nil
}

func (administration *Administration) authorize(
	ctx context.Context,
	invocation Invocation,
	operation Operation,
) error {
	if administration == nil || administration.repository == nil || administration.authorizer == nil ||
		administration.denied == nil || administration.ids == nil || administration.entropy == nil ||
		administration.clock == nil {
		return fmt.Errorf("%w: access administration is not initialized", ErrUnavailable)
	}
	if ctx == nil {
		return fmt.Errorf("%w: nil context", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := invocation.Validate(); err != nil {
		return err
	}
	err := administration.authorizer.Authorize(ctx, invocation.Execution(), operation)
	if err == nil {
		return nil
	}
	administration.auditDenied(ctx, invocation, operation, err)
	return err
}

func (administration *Administration) mutationRepositoryError(
	ctx context.Context,
	invocation Invocation,
	operation Operation,
	action string,
	err error,
) error {
	normalized := normalizeRepositoryError(action, err)
	if errors.Is(normalized, ErrUnauthenticated) ||
		errors.Is(normalized, ErrForbidden) ||
		errors.Is(normalized, ErrScopeInactive) {
		administration.auditDenied(ctx, invocation, operation, normalized)
	}
	return normalized
}

func (administration *Administration) auditDenied(
	ctx context.Context,
	invocation Invocation,
	operation Operation,
	cause error,
) {
	auditContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), deniedAuditTimeout)
	defer cancel()
	attempt := newDeniedAttempt(invocation, operation, cause, administration.clock.Now())
	_ = administration.denied.AuditDenied(auditContext, attempt)
}

func newDeniedAttempt(
	invocation Invocation,
	operation Operation,
	cause error,
	at time.Time,
) DeniedAttempt {
	execution := invocation.Execution()
	return DeniedAttempt{
		Scope: execution.Scope(), ActorID: execution.Actor().ActorID(),
		CredentialID: execution.CredentialID(), Operation: operation,
		RequestID: invocation.RequestID(), Reason: denialReason(cause), DeniedAt: at.UTC(),
	}
}

func denialReason(err error) string {
	switch {
	case errors.Is(err, ErrUnauthenticated):
		return "unauthenticated"
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrScopeInactive):
		return "scope_inactive"
	case errors.Is(err, ErrUnavailable):
		return "unavailable"
	default:
		return "invalid"
	}
}

func generateCredentialToken(
	entropy io.Reader,
	id domainaccess.ID,
) (string, SecretDigest, error) {
	if entropy == nil || !id.Valid() {
		return "", SecretDigest{}, fmt.Errorf("credential token dependencies are invalid")
	}
	secret := make([]byte, CredentialSecretBytes)
	if _, err := io.ReadFull(entropy, secret); err != nil {
		return "", SecretDigest{}, fmt.Errorf("generate credential secret: %w", err)
	}
	token := CredentialTokenPrefix + "." + id.String() + "." +
		base64.RawURLEncoding.EncodeToString(secret)
	return token, SecretDigest(sha256.Sum256([]byte(token))), nil
}

func credentialHint(digest SecretDigest) string {
	return fmt.Sprintf("sha256:%x", digest[:6])
}

func normalizeRepositoryError(action string, err error) error {
	if err == nil {
		return nil
	}
	for _, stable := range []error{
		ErrInvalid, ErrNotFound, ErrConflict, ErrLastOwnerPath,
		ErrUnauthenticated, ErrForbidden, ErrScopeInactive, ErrUnavailable,
	} {
		if errors.Is(err, stable) {
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
}
