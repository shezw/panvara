/*
   Panvara
   internal/infrastructure/postgres/access_admin_mutation.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

// CreatePrincipal inserts one active service principal and its success audit
// after transaction-local caller reauthorization.
func (store *AccessAdminStore) CreatePrincipal(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	principal domainaccess.Principal,
) (domainaccess.Principal, error) {
	if err := validateCreatedPrincipal(mutation.Scope(), principal); err != nil {
		return domainaccess.Principal{}, err
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationPrincipalCreate)
	if err != nil {
		return domainaccess.Principal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_principal (
			project_id, principal_id, kind, display_name, status,
			created_at, updated_at, disabled_at
		) VALUES ($1, $2, $3, $4, $5, $6, $6, NULL)
	`, principal.ProjectID().String(), principal.ID(), string(principal.Kind()),
		principal.DisplayName(), string(principal.Status()), principal.CreatedAt()); err != nil {
		return domainaccess.Principal{}, mapAccessWriteError("insert service principal", err)
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "principal", principal.ID(), principal.CreatedAt(),
	); err != nil {
		return domainaccess.Principal{}, err
	}
	if err := commitAccessMutation(ctx, tx, "create service principal"); err != nil {
		return domainaccess.Principal{}, err
	}
	return principal, nil
}

// DisablePrincipal terminally disables a principal while preserving at least
// one effective owner credential path in the project.
func (store *AccessAdminStore) DisablePrincipal(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	principalID string,
	at time.Time,
) (domainaccess.Principal, error) {
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) || at.IsZero() {
		return domainaccess.Principal{}, fmt.Errorf(
			"%w: invalid principal disable input", applicationaccess.ErrInvalid,
		)
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationPrincipalDisable)
	if err != nil {
		return domainaccess.Principal{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := readPrincipalForUpdate(ctx, tx, mutation.Scope().ProjectID(), principalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainaccess.Principal{}, applicationaccess.ErrNotFound
	}
	if err != nil {
		return domainaccess.Principal{}, fmt.Errorf("read PostgreSQL principal for disable: %w", err)
	}
	disabled, err := current.Disable(at)
	if err != nil {
		return domainaccess.Principal{}, fmt.Errorf("%w: %v", applicationaccess.ErrConflict, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE panvara_principal
		SET status = 'disabled', disabled_at = $3, updated_at = $3
		WHERE project_id = $1 AND principal_id = $2 AND status = 'active'
	`, mutation.Scope().ProjectID().String(), principalID, disabled.UpdatedAt()); err != nil {
		return domainaccess.Principal{}, mapAccessWriteError("disable principal", err)
	}
	if err := requireRemainingOwnerPath(ctx, tx, mutation.Scope()); err != nil {
		return domainaccess.Principal{}, err
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "principal", principalID, disabled.UpdatedAt(),
	); err != nil {
		return domainaccess.Principal{}, err
	}
	if err := commitAccessMutation(ctx, tx, "disable principal"); err != nil {
		return domainaccess.Principal{}, err
	}
	return disabled, nil
}

// IssueCredential inserts digest-only credential facts for an active target
// principal and appends the success audit in the same transaction.
func (store *AccessAdminStore) IssueCredential(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	issue applicationaccess.CredentialIssue,
) (domainaccess.Credential, error) {
	credential := issue.Credential
	if err := validateIssuedCredential(mutation, credential, issue.SecretDigest); err != nil {
		return domainaccess.Credential{}, err
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationCredentialIssue)
	if err != nil {
		return domainaccess.Credential{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActivePrincipal(ctx, tx, credential.ProjectID(), credential.PrincipalID()); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := insertCredential(ctx, tx, credential, issue.SecretDigest); err != nil {
		return domainaccess.Credential{}, mapAccessWriteError("issue credential", err)
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "credential", credential.ID().String(), credential.IssuedAt(),
	); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := commitAccessMutation(ctx, tx, "issue credential"); err != nil {
		return domainaccess.Credential{}, err
	}
	return credential, nil
}

// RevokeCredential terminally revokes one exact credential while retaining an
// effective owner path for project administration.
func (store *AccessAdminStore) RevokeCredential(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	credentialID domainaccess.ID,
	at time.Time,
) (domainaccess.Credential, error) {
	if !credentialID.Valid() || at.IsZero() {
		return domainaccess.Credential{}, fmt.Errorf(
			"%w: invalid credential revoke input", applicationaccess.ErrInvalid,
		)
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationCredentialRevoke)
	if err != nil {
		return domainaccess.Credential{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := readCredentialForUpdate(ctx, tx, mutation.Scope(), credentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainaccess.Credential{}, applicationaccess.ErrNotFound
	}
	if err != nil {
		return domainaccess.Credential{}, fmt.Errorf("read PostgreSQL credential for revoke: %w", err)
	}
	revoked, err := current.Revoke(mutation.ActorID(), at)
	if err != nil {
		return domainaccess.Credential{}, fmt.Errorf("%w: %v", applicationaccess.ErrConflict, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE panvara_api_credential
		SET status = 'revoked', revoked_by_principal_id = $4,
		    revoked_at = $5, updated_at = $5
		WHERE project_id = $1 AND environment_id = $2
		  AND credential_id = $3 AND status = 'active'
	`, mutation.Scope().ProjectID().String(), mutation.Scope().EnvironmentID().String(),
		credentialID.String(), mutation.ActorID(), at.UTC()); err != nil {
		return domainaccess.Credential{}, mapAccessWriteError("revoke credential", err)
	}
	if err := requireRemainingOwnerPath(ctx, tx, mutation.Scope()); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "credential", credentialID.String(), at.UTC(),
	); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := commitAccessMutation(ctx, tx, "revoke credential"); err != nil {
		return domainaccess.Credential{}, err
	}
	return revoked, nil
}

// GrantProjectOwner creates, idempotently retains, or explicitly re-grants one
// exact-scope owner grant. Restart never invokes this mutation implicitly.
func (store *AccessAdminStore) GrantProjectOwner(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	principalID string,
	at time.Time,
) (domainaccess.OwnerGrant, error) {
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) || at.IsZero() {
		return domainaccess.OwnerGrant{}, fmt.Errorf(
			"%w: invalid project owner grant input", applicationaccess.ErrInvalid,
		)
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationProjectOwnerGrant)
	if err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := requireActivePrincipal(ctx, tx, mutation.Scope().ProjectID(), principalID); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	current, readErr := readOwnerGrantForUpdate(ctx, tx, mutation.Scope(), principalID)
	if readErr == nil && current.Active() {
		if err := commitAccessMutation(ctx, tx, "retain project owner"); err != nil {
			return domainaccess.OwnerGrant{}, err
		}
		return current, nil
	}
	if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
		return domainaccess.OwnerGrant{}, fmt.Errorf("read PostgreSQL owner grant for grant: %w", readErr)
	}
	if readErr == nil {
		if err := requireMonotonicOwnerRegrant(current, at); err != nil {
			return domainaccess.OwnerGrant{}, err
		}
	}
	grant, err := domainaccess.NewOwnerGrant(domainaccess.OwnerGrantMaterial{
		Scope: mutation.Scope(), PrincipalID: principalID,
		GrantedBy: mutation.ActorID(), GrantedAt: at.UTC(),
	})
	if err != nil {
		return domainaccess.OwnerGrant{}, fmt.Errorf("%w: %v", applicationaccess.ErrInvalid, err)
	}
	if errors.Is(readErr, pgx.ErrNoRows) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO panvara_access_grant (
				project_id, environment_id, principal_id, role,
				granted_by_principal_id, granted_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $6)
		`, mutation.Scope().ProjectID().String(), mutation.Scope().EnvironmentID().String(),
			principalID, applicationaccess.RoleProjectOwner, mutation.ActorID(), at.UTC()); err != nil {
			return domainaccess.OwnerGrant{}, mapAccessWriteError("grant project owner", err)
		}
	} else {
		// Owner grants are deliberately reversible policy facts, unlike disabled
		// principals and revoked credentials. An authorized explicit re-grant
		// starts a new grant cycle and replaces the prior revoke lifecycle facts.
		if _, err := tx.Exec(ctx, `
			UPDATE panvara_access_grant
			SET granted_by_principal_id = $5, granted_at = $6,
			    revoked_by_principal_id = NULL, revoked_at = NULL, updated_at = $6
			WHERE project_id = $1 AND environment_id = $2
			  AND principal_id = $3 AND role = $4 AND revoked_at IS NOT NULL
		`, mutation.Scope().ProjectID().String(), mutation.Scope().EnvironmentID().String(),
			principalID, applicationaccess.RoleProjectOwner, mutation.ActorID(), at.UTC()); err != nil {
			return domainaccess.OwnerGrant{}, mapAccessWriteError("re-grant project owner", err)
		}
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "project_owner", principalID, at.UTC(),
	); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	if err := commitAccessMutation(ctx, tx, "grant project owner"); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	return grant, nil
}

func requireMonotonicOwnerRegrant(current domainaccess.OwnerGrant, at time.Time) error {
	revokedAt := current.RevokedAt()
	if revokedAt == nil {
		return fmt.Errorf("%w: owner grant is not revoked", applicationaccess.ErrConflict)
	}
	if at.UTC().Before(revokedAt.UTC()) {
		return fmt.Errorf(
			"%w: owner re-grant timestamp precedes the prior revocation",
			applicationaccess.ErrConflict,
		)
	}
	return nil
}

// RevokeProjectOwner ends the current owner grant cycle while retaining
// another effective owner credential path. A later authorized explicit grant
// may start a new cycle; bootstrap and restart never do so implicitly.
func (store *AccessAdminStore) RevokeProjectOwner(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	principalID string,
	at time.Time,
) (domainaccess.OwnerGrant, error) {
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) || at.IsZero() {
		return domainaccess.OwnerGrant{}, fmt.Errorf(
			"%w: invalid project owner revoke input", applicationaccess.ErrInvalid,
		)
	}
	tx, err := store.beginMutation(ctx, mutation, applicationaccess.OperationProjectOwnerRevoke)
	if err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := readOwnerGrantForUpdate(ctx, tx, mutation.Scope(), principalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domainaccess.OwnerGrant{}, applicationaccess.ErrNotFound
	}
	if err != nil {
		return domainaccess.OwnerGrant{}, fmt.Errorf("read PostgreSQL owner grant for revoke: %w", err)
	}
	revoked, err := current.Revoke(mutation.ActorID(), at)
	if err != nil {
		return domainaccess.OwnerGrant{}, fmt.Errorf("%w: %v", applicationaccess.ErrConflict, err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE panvara_access_grant
		SET revoked_by_principal_id = $5, revoked_at = $6, updated_at = $6
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = $3 AND role = $4 AND revoked_at IS NULL
	`, mutation.Scope().ProjectID().String(), mutation.Scope().EnvironmentID().String(),
		principalID, applicationaccess.RoleProjectOwner, mutation.ActorID(), at.UTC()); err != nil {
		return domainaccess.OwnerGrant{}, mapAccessWriteError("revoke project owner", err)
	}
	if err := requireRemainingOwnerPath(ctx, tx, mutation.Scope()); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	if err := store.appendMutationAudit(
		ctx, tx, mutation, "project_owner", principalID, at.UTC(),
	); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	if err := commitAccessMutation(ctx, tx, "revoke project owner"); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	return revoked, nil
}

func (store *AccessAdminStore) beginMutation(
	ctx context.Context,
	mutation applicationaccess.MutationContext,
	expected applicationaccess.Operation,
) (pgx.Tx, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: nil mutation context", applicationaccess.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateMutationContext(mutation, expected); err != nil {
		return nil, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin PostgreSQL access mutation: %w", err)
	}
	if err := lockAccessProject(ctx, tx, mutation.Scope().ProjectID()); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	if err := reauthorizeMutation(ctx, tx, mutation); err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func lockAccessProject(ctx context.Context, tx pgx.Tx, projectID project.ID) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, projectAccessLockID(projectID)); err != nil {
		return fmt.Errorf("lock PostgreSQL access project: %w", err)
	}
	return nil
}

func reauthorizeMutation(
	ctx context.Context,
	tx pgx.Tx,
	mutation applicationaccess.MutationContext,
) error {
	scope := mutation.Scope()
	var scopeActive, credentialActive, ownerActive bool
	err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM panvara_project AS project
				JOIN panvara_environment AS environment ON environment.project_id = project.project_id
				WHERE project.project_id = $1 AND project.status = 'active'
				  AND environment.environment_id = $2 AND environment.status = 'active'
				  AND environment.is_default
			),
			EXISTS (
				SELECT 1 FROM panvara_api_credential AS credential
				JOIN panvara_principal AS principal
				  ON principal.project_id = credential.project_id
				 AND principal.principal_id = credential.principal_id
				WHERE credential.project_id = $1 AND credential.environment_id = $2
				  AND credential.credential_id = $3 AND credential.principal_id = $4
				  AND credential.status = 'active' AND credential.revoked_at IS NULL
				  AND principal.status = 'active'
			),
			EXISTS (
				SELECT 1 FROM panvara_access_grant AS access_grant
				JOIN panvara_principal AS principal
				  ON principal.project_id = access_grant.project_id
				 AND principal.principal_id = access_grant.principal_id
				WHERE access_grant.project_id = $1 AND access_grant.environment_id = $2
				  AND access_grant.principal_id = $4 AND access_grant.role = $5
				  AND access_grant.revoked_at IS NULL AND principal.status = 'active'
			)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(),
		mutation.CredentialID().String(), mutation.ActorID(),
		applicationaccess.RoleProjectOwner).Scan(&scopeActive, &credentialActive, &ownerActive)
	if err != nil {
		return fmt.Errorf("reauthorize PostgreSQL access mutation: %w", err)
	}
	switch {
	case !scopeActive:
		return applicationaccess.ErrScopeInactive
	case !credentialActive:
		return applicationaccess.ErrUnauthenticated
	case !ownerActive:
		return applicationaccess.ErrForbidden
	default:
		return nil
	}
}

func requireBootstrapFacts(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	principalID string,
) error {
	var factsExist bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM panvara_project AS project
			JOIN panvara_environment AS environment ON environment.project_id = project.project_id
			JOIN panvara_principal AS principal ON principal.project_id = project.project_id
			WHERE project.project_id = $1
			  AND environment.environment_id = $2
			  AND environment.is_default AND principal.principal_id = $3
			  AND principal.kind = 'bootstrap'
		)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), principalID).Scan(&factsExist)
	if err != nil {
		return fmt.Errorf("read PostgreSQL bootstrap facts: %w", err)
	}
	if !factsExist {
		return applicationaccess.ErrNotFound
	}
	return nil
}

func requireActivePrincipal(
	ctx context.Context,
	tx pgx.Tx,
	projectID project.ID,
	principalID string,
) error {
	var active bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM panvara_principal
			WHERE project_id = $1 AND principal_id = $2 AND status = 'active'
		)
	`, projectID.String(), principalID).Scan(&active); err != nil {
		return fmt.Errorf("read PostgreSQL active principal: %w", err)
	}
	if !active {
		return applicationaccess.ErrNotFound
	}
	return nil
}

func requireRemainingOwnerPath(ctx context.Context, tx pgx.Tx, scope project.Scope) error {
	var remains bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM panvara_project AS project
			JOIN panvara_environment AS environment ON environment.project_id = project.project_id
			JOIN panvara_access_grant AS access_grant
			  ON access_grant.project_id = project.project_id
			 AND access_grant.environment_id = environment.environment_id
			JOIN panvara_principal AS principal
			  ON principal.project_id = access_grant.project_id
			 AND principal.principal_id = access_grant.principal_id
			JOIN panvara_api_credential AS credential
			  ON credential.project_id = access_grant.project_id
			 AND credential.environment_id = access_grant.environment_id
			 AND credential.principal_id = access_grant.principal_id
			WHERE project.project_id = $1 AND project.status = 'active'
			  AND environment.environment_id = $2 AND environment.status = 'active'
			  AND environment.is_default AND access_grant.role = $3
			  AND access_grant.revoked_at IS NULL AND principal.status = 'active'
			  AND credential.status = 'active' AND credential.revoked_at IS NULL
		)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(),
		applicationaccess.RoleProjectOwner).Scan(&remains)
	if err != nil {
		return fmt.Errorf("count PostgreSQL effective owner paths: %w", err)
	}
	if !remains {
		return applicationaccess.ErrLastOwnerPath
	}
	return nil
}

func insertCredential(
	ctx context.Context,
	tx pgx.Tx,
	credential domainaccess.Credential,
	digest applicationaccess.SecretDigest,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO panvara_api_credential (
			project_id, environment_id, credential_id, principal_id,
			label, secret_digest, secret_hint, status,
			issued_by_principal_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
	`, credential.ProjectID().String(), credential.EnvironmentID().String(),
		credential.ID().String(), credential.PrincipalID(), credential.Label(), digest[:],
		credential.Hint(), string(credential.Status()), credential.IssuedBy(), credential.IssuedAt())
	return err
}

func commitAccessMutation(ctx context.Context, tx pgx.Tx, action string) error {
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL %s: %w", action, err)
	}
	return nil
}

func mapAccessWriteError(action string, err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		switch databaseError.Code {
		case "23505":
			return fmt.Errorf("%w: %s", applicationaccess.ErrConflict, action)
		case "23503":
			return fmt.Errorf("%w: %s dependency", applicationaccess.ErrNotFound, action)
		case "23502", "23514", "22P02":
			return fmt.Errorf("%w: %s", applicationaccess.ErrInvalid, action)
		}
	}
	return fmt.Errorf("%s: %w", action, err)
}

func validateAccessScope(scope project.Scope) error {
	if err := scope.Validate(); err != nil {
		return fmt.Errorf("%w: invalid access scope", applicationaccess.ErrInvalid)
	}
	return nil
}

func validateMutationContext(
	mutation applicationaccess.MutationContext,
	expected applicationaccess.Operation,
) error {
	if err := validateAccessScope(mutation.Scope()); err != nil {
		return err
	}
	if !expected.Valid() || mutation.Operation() != expected {
		return fmt.Errorf("%w: access mutation operation mismatch", applicationaccess.ErrInvalid)
	}
	if !principalIDPattern.MatchString(mutation.ActorID()) || !mutation.CredentialID().Valid() {
		return fmt.Errorf("%w: invalid access mutation actor", applicationaccess.ErrInvalid)
	}
	if !auditRequestIDPattern.MatchString(mutation.RequestID()) {
		return fmt.Errorf("%w: invalid access mutation request id", applicationaccess.ErrInvalid)
	}
	return nil
}

func validateCreatedPrincipal(scope project.Scope, principal domainaccess.Principal) error {
	if err := validateAccessScope(scope); err != nil {
		return err
	}
	if principal.ProjectID().String() != scope.ProjectID().String() ||
		principal.Kind() != domainaccess.PrincipalKindService || !principal.Active() ||
		!principal.CreatedAt().Equal(principal.UpdatedAt()) || principal.DisabledAt() != nil {
		return fmt.Errorf("%w: invalid service principal facts", applicationaccess.ErrInvalid)
	}
	_, err := domainaccess.NewPrincipal(domainaccess.PrincipalMaterial{
		ProjectID: principal.ProjectID(), ID: principal.ID(), Kind: principal.Kind(),
		DisplayName: principal.DisplayName(), Status: principal.Status(),
		CreatedAt: principal.CreatedAt(), UpdatedAt: principal.UpdatedAt(),
		DisabledAt: principal.DisabledAt(),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", applicationaccess.ErrInvalid, err)
	}
	return nil
}

func validateIssuedCredential(
	mutation applicationaccess.MutationContext,
	credential domainaccess.Credential,
	digest applicationaccess.SecretDigest,
) error {
	if err := validateCredentialFacts(credential, digest); err != nil {
		return err
	}
	if credential.Scope() != mutation.Scope() || credential.IssuedBy() != mutation.ActorID() ||
		!credential.Active() {
		return fmt.Errorf("%w: credential issue boundary mismatch", applicationaccess.ErrInvalid)
	}
	return nil
}

func validateBootstrapCandidate(
	scope project.Scope,
	principalID string,
	candidate applicationaccess.BootstrapCredentialCandidate,
) error {
	if err := validateCredentialFacts(candidate.Credential, candidate.SecretDigest); err != nil {
		return err
	}
	credential := candidate.Credential
	if credential.Scope() != scope || credential.PrincipalID() != principalID ||
		credential.IssuedBy() != principalID || !credential.Active() {
		return fmt.Errorf("%w: bootstrap credential boundary mismatch", applicationaccess.ErrInvalid)
	}
	return nil
}

func validateCredentialFacts(
	credential domainaccess.Credential,
	digest applicationaccess.SecretDigest,
) error {
	if err := credential.Scope().Validate(); err != nil || !credential.ID().Valid() {
		return fmt.Errorf("%w: invalid credential identity", applicationaccess.ErrInvalid)
	}
	expectedHint := fmt.Sprintf("sha256:%x", digest[:6])
	if credential.Hint() != expectedHint {
		return fmt.Errorf("%w: credential hint does not match digest", applicationaccess.ErrInvalid)
	}
	_, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: credential.Scope(), ID: credential.ID(), PrincipalID: credential.PrincipalID(),
		Label: credential.Label(), Hint: credential.Hint(), Status: credential.Status(),
		IssuedBy: credential.IssuedBy(), IssuedAt: credential.IssuedAt(),
		RevokedBy: credential.RevokedBy(), RevokedAt: credential.RevokedAt(),
	})
	if err != nil {
		return fmt.Errorf("%w: %v", applicationaccess.ErrInvalid, err)
	}
	return nil
}
