/*
   Panvara
   internal/infrastructure/postgres/access_admin_query.go    2026-07-19
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
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

// ListPrincipals returns deterministic project-local principal lifecycle views.
func (store *AccessAdminStore) ListPrincipals(
	ctx context.Context,
	scope project.Scope,
) ([]domainaccess.Principal, error) {
	if err := validateAccessScope(scope); err != nil {
		return nil, err
	}
	rows, err := store.pool.Query(ctx, `
		SELECT principal_id, kind, display_name, status, created_at, updated_at, disabled_at
		FROM panvara_principal
		WHERE project_id = $1
		ORDER BY created_at, principal_id
	`, scope.ProjectID().String())
	if err != nil {
		return nil, fmt.Errorf("list PostgreSQL principals: %w", err)
	}
	defer rows.Close()
	values := make([]domainaccess.Principal, 0)
	for rows.Next() {
		value, err := scanPrincipal(rows, scope.ProjectID())
		if err != nil {
			return nil, fmt.Errorf("scan PostgreSQL principal: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL principals: %w", err)
	}
	return values, nil
}

// ListCredentials returns non-secret metadata in deterministic issue order.
func (store *AccessAdminStore) ListCredentials(
	ctx context.Context,
	scope project.Scope,
	principalID string,
) ([]domainaccess.Credential, error) {
	if err := validateAccessScope(scope); err != nil {
		return nil, err
	}
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) {
		return nil, fmt.Errorf("%w: invalid principal id", applicationaccess.ErrInvalid)
	}
	rows, err := store.pool.Query(ctx, `
		SELECT credential_id::text, principal_id, label, secret_hint, status,
		       issued_by_principal_id, created_at, revoked_by_principal_id, revoked_at
		FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND principal_id = $3
		ORDER BY created_at, credential_id
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), principalID)
	if err != nil {
		return nil, fmt.Errorf("list PostgreSQL credentials: %w", err)
	}
	defer rows.Close()
	values := make([]domainaccess.Credential, 0)
	for rows.Next() {
		value, err := scanCredential(rows, scope)
		if err != nil {
			return nil, fmt.Errorf("scan PostgreSQL credential: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL credentials: %w", err)
	}
	return values, nil
}

// ListProjectOwners returns active and revoked exact-scope owner grant views.
func (store *AccessAdminStore) ListProjectOwners(
	ctx context.Context,
	scope project.Scope,
) ([]domainaccess.OwnerGrant, error) {
	if err := validateAccessScope(scope); err != nil {
		return nil, err
	}
	rows, err := store.pool.Query(ctx, `
		SELECT principal_id, granted_by_principal_id, granted_at,
		       revoked_by_principal_id, revoked_at
		FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2 AND role = $3
		ORDER BY granted_at, principal_id
	`, scope.ProjectID().String(), scope.EnvironmentID().String(),
		applicationaccess.RoleProjectOwner)
	if err != nil {
		return nil, fmt.Errorf("list PostgreSQL project owners: %w", err)
	}
	defer rows.Close()
	values := make([]domainaccess.OwnerGrant, 0)
	for rows.Next() {
		value, err := scanOwnerGrant(rows, scope)
		if err != nil {
			return nil, fmt.Errorf("scan PostgreSQL project owner: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PostgreSQL project owners: %w", err)
	}
	return values, nil
}

type accessRowScanner interface {
	Scan(...any) error
}

func scanPrincipal(
	scanner accessRowScanner,
	projectID project.ID,
) (domainaccess.Principal, error) {
	var (
		principalID, kind, displayName, status string
		createdAt, updatedAt                   time.Time
		disabledAt                             *time.Time
	)
	if err := scanner.Scan(
		&principalID, &kind, &displayName, &status, &createdAt, &updatedAt, &disabledAt,
	); err != nil {
		return domainaccess.Principal{}, err
	}
	return domainaccess.NewPrincipal(domainaccess.PrincipalMaterial{
		ProjectID: projectID, ID: principalID, Kind: domainaccess.PrincipalKind(kind),
		DisplayName: displayName, Status: domainaccess.PrincipalStatus(status),
		CreatedAt: createdAt, UpdatedAt: updatedAt, DisabledAt: disabledAt,
	})
}

type credentialRow struct {
	id, principalID, label, hint, status, issuedBy string
	issuedAt                                       time.Time
	revokedBy                                      *string
	revokedAt                                      *time.Time
}

func (row credentialRow) credential(scope project.Scope) (domainaccess.Credential, error) {
	id, err := domainaccess.ParseID(row.id)
	if err != nil {
		return domainaccess.Credential{}, err
	}
	revokedBy := ""
	if row.revokedBy != nil {
		revokedBy = *row.revokedBy
	}
	return domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: scope, ID: id, PrincipalID: row.principalID, Label: row.label,
		Hint: row.hint, Status: domainaccess.CredentialStatus(row.status),
		IssuedBy: row.issuedBy, IssuedAt: row.issuedAt,
		RevokedBy: revokedBy, RevokedAt: row.revokedAt,
	})
}

func scanCredential(
	scanner accessRowScanner,
	scope project.Scope,
) (domainaccess.Credential, error) {
	var row credentialRow
	if err := scanner.Scan(
		&row.id, &row.principalID, &row.label, &row.hint, &row.status,
		&row.issuedBy, &row.issuedAt, &row.revokedBy, &row.revokedAt,
	); err != nil {
		return domainaccess.Credential{}, err
	}
	return row.credential(scope)
}

func scanCredentialCandidate(
	scanner accessRowScanner,
	scope project.Scope,
) (domainaccess.Credential, applicationaccess.SecretDigest, error) {
	var row credentialRow
	var digestBytes []byte
	if err := scanner.Scan(
		&row.id, &row.principalID, &row.label, &row.hint, &row.status,
		&row.issuedBy, &row.issuedAt, &row.revokedBy, &row.revokedAt, &digestBytes,
	); err != nil {
		return domainaccess.Credential{}, applicationaccess.SecretDigest{}, err
	}
	if len(digestBytes) != sha256.Size {
		return domainaccess.Credential{}, applicationaccess.SecretDigest{},
			fmt.Errorf("stored credential digest length is %d", len(digestBytes))
	}
	credential, err := row.credential(scope)
	if err != nil {
		return domainaccess.Credential{}, applicationaccess.SecretDigest{}, err
	}
	var digest applicationaccess.SecretDigest
	copy(digest[:], digestBytes)
	return credential, digest, nil
}

func scanOwnerGrant(
	scanner accessRowScanner,
	scope project.Scope,
) (domainaccess.OwnerGrant, error) {
	var principalID, grantedBy string
	var grantedAt time.Time
	var revokedBy *string
	var revokedAt *time.Time
	if err := scanner.Scan(&principalID, &grantedBy, &grantedAt, &revokedBy, &revokedAt); err != nil {
		return domainaccess.OwnerGrant{}, err
	}
	revoker := ""
	if revokedBy != nil {
		revoker = *revokedBy
	}
	return domainaccess.NewOwnerGrant(domainaccess.OwnerGrantMaterial{
		Scope: scope, PrincipalID: principalID, GrantedBy: grantedBy,
		GrantedAt: grantedAt, RevokedBy: revoker, RevokedAt: revokedAt,
	})
}

func readBootstrapCredential(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
) (domainaccess.Credential, applicationaccess.SecretDigest, error) {
	return scanCredentialCandidate(tx.QueryRow(ctx, `
		SELECT credential.credential_id::text,
		       credential.principal_id,
		       credential.label,
		       credential.secret_hint,
		       credential.status,
		       credential.issued_by_principal_id,
		       credential.created_at,
		       credential.revoked_by_principal_id,
		       credential.revoked_at,
		       marker.secret_digest
		FROM panvara_access_bootstrap_marker AS marker
		JOIN panvara_api_credential AS credential
		  ON credential.project_id = marker.project_id
		 AND credential.environment_id = marker.environment_id
		 AND credential.credential_id = marker.credential_id
		 AND credential.principal_id = marker.principal_id
		 AND credential.secret_digest = marker.secret_digest
		 AND credential.secret_hint = marker.secret_hint
		WHERE marker.project_id = $1 AND marker.environment_id = $2
	`, scope.ProjectID().String(), scope.EnvironmentID().String()), scope)
}

func readPrincipalForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	projectID project.ID,
	principalID string,
) (domainaccess.Principal, error) {
	return scanPrincipal(tx.QueryRow(ctx, `
		SELECT principal_id, kind, display_name, status, created_at, updated_at, disabled_at
		FROM panvara_principal
		WHERE project_id = $1 AND principal_id = $2
		FOR UPDATE
	`, projectID.String(), principalID), projectID)
}

func readCredentialForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	credentialID domainaccess.ID,
) (domainaccess.Credential, error) {
	return scanCredential(tx.QueryRow(ctx, `
		SELECT credential_id::text, principal_id, label, secret_hint, status,
		       issued_by_principal_id, created_at, revoked_by_principal_id, revoked_at
		FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
		FOR UPDATE
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), credentialID.String()), scope)
}

func readOwnerGrantForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	scope project.Scope,
	principalID string,
) (domainaccess.OwnerGrant, error) {
	return scanOwnerGrant(tx.QueryRow(ctx, `
		SELECT principal_id, granted_by_principal_id, granted_at,
		       revoked_by_principal_id, revoked_at
		FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = $3 AND role = $4
		FOR UPDATE
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), principalID,
		applicationaccess.RoleProjectOwner), scope)
}
