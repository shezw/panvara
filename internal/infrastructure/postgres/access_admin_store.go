/*
   Panvara
   internal/infrastructure/postgres/access_admin_store.go    2026-07-19
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
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

type accessAuditIDGenerator interface {
	New() (domainaccess.ID, error)
}

var (
	_ applicationaccess.AccessAdminRepository         = (*AccessAdminStore)(nil)
	_ applicationaccess.BootstrapCredentialRepository = (*AccessAdminStore)(nil)
	_ applicationaccess.CredentialLookup              = (*AccessAdminStore)(nil)
	_ applicationaccess.DeniedAuditor                 = (*AccessAdminStore)(nil)
)

// AccessAdminStore persists credential authentication, access administration,
// and append-only security audit facts.
type AccessAdminStore struct {
	pool     *pgxpool.Pool
	auditIDs accessAuditIDGenerator
}

// NewAccessAdminStore constructs the PostgreSQL access administration adapter.
func NewAccessAdminStore(pool *pgxpool.Pool) (*AccessAdminStore, error) {
	return newAccessAdminStore(pool, domainaccess.NewDefaultIDGenerator())
}

func newAccessAdminStore(
	pool *pgxpool.Pool,
	auditIDs accessAuditIDGenerator,
) (*AccessAdminStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL access administration store: nil pool")
	}
	if auditIDs == nil {
		return nil, fmt.Errorf("construct PostgreSQL access administration store: nil audit id generator")
	}
	return &AccessAdminStore{pool: pool, auditIDs: auditIDs}, nil
}

// LookupCredential returns one exact credential and its irreversible digest.
// It never returns a raw token or resolves a credential across scopes. Project
// and Environment activity are deliberately left to Policy so a valid
// credential in an inactive scope is authenticated and then rejected as 403.
func (store *AccessAdminStore) LookupCredential(
	ctx context.Context,
	scope project.Scope,
	selector applicationaccess.CredentialSelector,
) (applicationaccess.CredentialCandidate, error) {
	if err := validateAccessScope(scope); err != nil {
		return applicationaccess.CredentialCandidate{}, err
	}
	credentialID, selectedByID := selector.CredentialID()
	if selectedByID == selector.Bootstrap() {
		return applicationaccess.CredentialCandidate{}, fmt.Errorf(
			"%w: credential selector must choose one lookup mode", applicationaccess.ErrInvalid,
		)
	}

	query := `
		SELECT credential.credential_id::text,
		       credential.principal_id,
		       credential.label,
		       credential.secret_hint,
		       credential.status,
		       credential.issued_by_principal_id,
		       credential.created_at,
		       credential.revoked_by_principal_id,
		       credential.revoked_at,
		       credential.secret_digest
		FROM panvara_api_credential AS credential
		JOIN panvara_principal AS principal
		  ON principal.project_id = credential.project_id
		 AND principal.principal_id = credential.principal_id
		WHERE credential.project_id = $1
		  AND credential.environment_id = $2
		  AND principal.status = 'active'
	`
	arguments := []any{scope.ProjectID().String(), scope.EnvironmentID().String()}
	if selector.Bootstrap() {
		query += `
		  AND credential.credential_id = (
		      SELECT marker.credential_id
		      FROM panvara_access_bootstrap_marker AS marker
		      WHERE marker.project_id = $1 AND marker.environment_id = $2
		  )
		`
	} else {
		query += ` AND credential.credential_id = $3`
		arguments = append(arguments, credentialID.String())
	}

	credential, digest, err := scanCredentialCandidate(
		store.pool.QueryRow(ctx, query, arguments...), scope,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return applicationaccess.CredentialCandidate{}, applicationaccess.ErrNotFound
	}
	if err != nil {
		return applicationaccess.CredentialCandidate{}, fmt.Errorf(
			"read PostgreSQL credential candidate: %w", err,
		)
	}
	return applicationaccess.CredentialCandidate{
		Credential: credential, SecretDigest: digest,
	}, nil
}

// RegisterBootstrapCredential creates the permanent bootstrap marker once or
// verifies an already initialized marker without restoring revoked authority.
func (store *AccessAdminStore) RegisterBootstrapCredential(
	ctx context.Context,
	registration applicationaccess.BootstrapCredentialRegistration,
) (domainaccess.Credential, error) {
	if err := validateAccessScope(registration.Scope); err != nil {
		return domainaccess.Credential{}, err
	}
	principalID := strings.TrimSpace(registration.PrincipalID)
	if !principalIDPattern.MatchString(principalID) {
		return domainaccess.Credential{}, fmt.Errorf(
			"%w: invalid bootstrap principal", applicationaccess.ErrInvalid,
		)
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domainaccess.Credential{}, fmt.Errorf("begin PostgreSQL bootstrap credential: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockAccessProject(ctx, tx, registration.Scope.ProjectID()); err != nil {
		return domainaccess.Credential{}, err
	}

	persisted, persistedDigest, err := readBootstrapCredential(ctx, tx, registration.Scope)
	switch {
	case err == nil:
		if persisted.PrincipalID() != principalID {
			return domainaccess.Credential{}, fmt.Errorf(
				"%w: bootstrap principal conflicts with permanent marker",
				applicationaccess.ErrConflict,
			)
		}
		if registration.Candidate != nil && subtle.ConstantTimeCompare(
			persistedDigest[:], registration.Candidate.SecretDigest[:],
		) != 1 {
			return domainaccess.Credential{}, fmt.Errorf(
				"%w: bootstrap token conflicts with permanent marker",
				applicationaccess.ErrConflict,
			)
		}
		if err := tx.Commit(ctx); err != nil {
			return domainaccess.Credential{}, fmt.Errorf(
				"commit PostgreSQL bootstrap credential verification: %w", err,
			)
		}
		return persisted, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return domainaccess.Credential{}, fmt.Errorf("read PostgreSQL bootstrap marker: %w", err)
	case registration.Candidate == nil:
		return domainaccess.Credential{}, fmt.Errorf(
			"%w: bootstrap token is required before permanent initialization",
			applicationaccess.ErrInvalid,
		)
	}

	candidate := registration.Candidate
	if err := validateBootstrapCandidate(registration.Scope, principalID, *candidate); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := requireBootstrapFacts(ctx, tx, registration.Scope, principalID); err != nil {
		return domainaccess.Credential{}, err
	}
	credential := candidate.Credential
	if err := insertCredential(ctx, tx, credential, candidate.SecretDigest); err != nil {
		return domainaccess.Credential{}, mapAccessWriteError("insert bootstrap credential", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_access_bootstrap_marker (
			project_id, environment_id, principal_id, credential_id,
			secret_digest, secret_hint, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, credential.ProjectID().String(), credential.EnvironmentID().String(),
		credential.PrincipalID(), credential.ID().String(), candidate.SecretDigest[:],
		credential.Hint(), credential.IssuedAt()); err != nil {
		return domainaccess.Credential{}, mapAccessWriteError("insert bootstrap marker", err)
	}
	if err := store.appendAudit(ctx, tx, auditFact{
		scope: credential.Scope(), actorID: credential.PrincipalID(),
		credentialID: credential.ID(), requestID: "bootstrap:" + credential.ID().String(),
		action: string(applicationaccess.OperationCredentialBootstrap), outcome: auditOutcomeSuccess,
		targetKind: "credential", targetID: credential.ID().String(), occurredAt: credential.IssuedAt(),
	}); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domainaccess.Credential{}, fmt.Errorf("commit PostgreSQL bootstrap credential: %w", err)
	}
	return credential, nil
}
