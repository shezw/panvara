/*
   Panvara
   internal/infrastructure/postgres/project_access_store.go    2026-07-18
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
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/domain/project"
)

var (
	environmentKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	principalIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

var _ access.GrantReader = (*ProjectAccessStore)(nil)

// ProjectAccessStore persists the single-default-environment bootstrap scope
// and answers fail-closed access-kernel grant lookups.
type ProjectAccessStore struct {
	pool        *pgxpool.Pool
	idGenerator *project.EnvironmentIDGenerator
}

// NewProjectAccessStore constructs the PostgreSQL project and access adapter.
func NewProjectAccessStore(pool *pgxpool.Pool) (*ProjectAccessStore, error) {
	return newProjectAccessStore(pool, project.NewDefaultEnvironmentIDGenerator())
}

func newProjectAccessStore(
	pool *pgxpool.Pool,
	idGenerator *project.EnvironmentIDGenerator,
) (*ProjectAccessStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL project access store: nil pool")
	}
	if idGenerator == nil {
		return nil, fmt.Errorf("construct PostgreSQL project access store: nil environment id generator")
	}
	return &ProjectAccessStore{pool: pool, idGenerator: idGenerator}, nil
}

// EnsureBootstrapScope atomically creates the first persisted scope for the
// exact project ID: project, default environment, bootstrap principal, and
// owner grant. Once that project exists, it only verifies persisted identity
// and settings: missing or revoked access is never silently recreated. A
// different project ID and unique project key form a separate project scope.
func (store *ProjectAccessStore) EnsureBootstrapScope(
	ctx context.Context,
	definition project.Context,
	environmentKey string,
	principalID string,
) (project.Scope, error) {
	environmentKey, principalID, err := validateBootstrapInput(definition, environmentKey, principalID)
	if err != nil {
		return project.Scope{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return project.Scope{}, fmt.Errorf("begin PostgreSQL project bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, projectAccessLockID(definition.ID())); err != nil {
		return project.Scope{}, fmt.Errorf("lock PostgreSQL project bootstrap: %w", err)
	}

	persisted, err := readPersistedProject(ctx, tx, definition.ID())
	var scope project.Scope
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		scope, err = store.insertBootstrapScope(ctx, tx, definition, environmentKey, principalID)
	case err != nil:
		err = fmt.Errorf("read PostgreSQL bootstrap project: %w", err)
	default:
		if err = persisted.matches(definition); err == nil {
			scope, err = readPersistedBootstrapScope(ctx, tx, definition.ID(), environmentKey, principalID)
		}
	}
	if err != nil {
		return project.Scope{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return project.Scope{}, fmt.Errorf("commit PostgreSQL project bootstrap: %w", err)
	}
	return scope, nil
}

// ScopeActive reports whether the exact scope is the project's active default
// environment. P0-01a deliberately rejects non-default environments because
// the existing fact tables do not yet carry environment_id.
func (store *ProjectAccessStore) ScopeActive(ctx context.Context, scope project.Scope) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, fmt.Errorf("read PostgreSQL scope state: %w", err)
	}
	var active bool
	err := store.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM panvara_project AS project
			JOIN panvara_environment AS environment
			  ON environment.project_id = project.project_id
			WHERE project.project_id = $1
			  AND project.status = 'active'
			  AND environment.environment_id = $2
			  AND environment.is_default
			  AND environment.status = 'active'
		)
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("read PostgreSQL scope state: %w", err)
	}
	return active, nil
}

// HasActiveGrant reports whether an active principal has the exact unrevoked
// grant inside an active default environment.
func (store *ProjectAccessStore) HasActiveGrant(
	ctx context.Context,
	scope project.Scope,
	principalID string,
	role string,
) (bool, error) {
	if err := scope.Validate(); err != nil {
		return false, fmt.Errorf("read PostgreSQL access grant: %w", err)
	}
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) {
		return false, fmt.Errorf("read PostgreSQL access grant: invalid principal id %q", principalID)
	}
	if role != access.RoleProjectOwner {
		return false, nil
	}

	var granted bool
	err := store.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM panvara_access_grant AS access_grant
			JOIN panvara_project AS project
			  ON project.project_id = access_grant.project_id
			JOIN panvara_environment AS environment
			  ON environment.project_id = access_grant.project_id
			 AND environment.environment_id = access_grant.environment_id
			JOIN panvara_principal AS principal
			  ON principal.project_id = access_grant.project_id
			 AND principal.principal_id = access_grant.principal_id
			WHERE access_grant.project_id = $1
			  AND access_grant.environment_id = $2
			  AND access_grant.principal_id = $3
			  AND access_grant.role = $4
			  AND access_grant.revoked_at IS NULL
			  AND project.status = 'active'
			  AND environment.status = 'active'
			  AND environment.is_default
			  AND principal.status = 'active'
		)
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), principalID, role).Scan(&granted)
	if err != nil {
		return false, fmt.Errorf("read PostgreSQL access grant: %w", err)
	}
	return granted, nil
}

type persistedProject struct {
	key, locale, timeZone, currency string
}

func readPersistedProject(ctx context.Context, tx pgx.Tx, projectID project.ID) (persistedProject, error) {
	var value persistedProject
	err := tx.QueryRow(ctx, `
		SELECT project_key, default_locale, default_time_zone, default_currency
		FROM panvara_project
		WHERE project_id = $1
	`, projectID.String()).Scan(&value.key, &value.locale, &value.timeZone, &value.currency)
	return value, err
}

func (persisted persistedProject) matches(definition project.Context) error {
	if persisted.key != definition.Key().String() ||
		persisted.locale != definition.Locale() ||
		persisted.timeZone != definition.TimeZone() ||
		persisted.currency != definition.Currency().String() {
		return fmt.Errorf(
			"PostgreSQL project %q settings conflict with bootstrap configuration",
			definition.ID().String(),
		)
	}
	return nil
}

func (store *ProjectAccessStore) insertBootstrapScope(
	ctx context.Context,
	tx pgx.Tx,
	definition project.Context,
	environmentKey string,
	principalID string,
) (project.Scope, error) {
	environmentID, err := store.idGenerator.New()
	if err != nil {
		return project.Scope{}, fmt.Errorf("generate PostgreSQL bootstrap environment id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_project (
			project_id, project_key, default_locale, default_time_zone, default_currency
		) VALUES ($1, $2, $3, $4, $5)
	`, definition.ID().String(), definition.Key().String(), definition.Locale(),
		definition.TimeZone(), definition.Currency().String()); err != nil {
		return project.Scope{}, fmt.Errorf("insert PostgreSQL bootstrap project: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_environment (
			project_id, environment_id, environment_key, is_default
		) VALUES ($1, $2, $3, true)
	`, definition.ID().String(), environmentID.String(), environmentKey); err != nil {
		return project.Scope{}, fmt.Errorf("insert PostgreSQL bootstrap environment: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_principal (project_id, principal_id)
		VALUES ($1, $2)
	`, definition.ID().String(), principalID); err != nil {
		return project.Scope{}, fmt.Errorf("insert PostgreSQL bootstrap principal: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO panvara_access_grant (
			project_id, environment_id, principal_id, role
		) VALUES ($1, $2, $3, $4)
	`, definition.ID().String(), environmentID.String(), principalID, access.RoleProjectOwner); err != nil {
		return project.Scope{}, fmt.Errorf("insert PostgreSQL bootstrap owner grant: %w", err)
	}
	scope, err := project.NewScope(definition.ID(), environmentID)
	if err != nil {
		return project.Scope{}, fmt.Errorf("construct PostgreSQL bootstrap scope: %w", err)
	}
	return scope, nil
}

func readPersistedBootstrapScope(
	ctx context.Context,
	tx pgx.Tx,
	projectID project.ID,
	environmentKey string,
	principalID string,
) (project.Scope, error) {
	var environmentIDText, persistedKey string
	err := tx.QueryRow(ctx, `
		SELECT environment_id::text, environment_key
		FROM panvara_environment
		WHERE project_id = $1 AND is_default
	`, projectID.String()).Scan(&environmentIDText, &persistedKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return project.Scope{}, fmt.Errorf("PostgreSQL project %q has no default environment", projectID.String())
	}
	if err != nil {
		return project.Scope{}, fmt.Errorf("read PostgreSQL default environment: %w", err)
	}
	if persistedKey != environmentKey {
		return project.Scope{}, fmt.Errorf(
			"PostgreSQL project %q environment key %q conflicts with bootstrap configuration %q",
			projectID.String(), persistedKey, environmentKey,
		)
	}

	var principalExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM panvara_principal
			WHERE project_id = $1 AND principal_id = $2
		)
	`, projectID.String(), principalID).Scan(&principalExists); err != nil {
		return project.Scope{}, fmt.Errorf("read PostgreSQL bootstrap principal: %w", err)
	}
	if !principalExists {
		return project.Scope{}, fmt.Errorf(
			"PostgreSQL project %q has no bootstrap principal %q",
			projectID.String(), principalID,
		)
	}

	environmentID, err := project.ParseEnvironmentID(environmentIDText)
	if err != nil {
		return project.Scope{}, fmt.Errorf("parse PostgreSQL default environment id: %w", err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		return project.Scope{}, fmt.Errorf("construct persisted PostgreSQL bootstrap scope: %w", err)
	}
	return scope, nil
}

func validateBootstrapInput(
	definition project.Context,
	environmentKey string,
	principalID string,
) (string, string, error) {
	if !definition.ID().Valid() || !definition.Key().Valid() || !definition.Currency().Valid() ||
		definition.Locale() == "" || definition.TimeZone() == "" {
		return "", "", fmt.Errorf("bootstrap PostgreSQL project context is invalid")
	}
	environmentKey = strings.ToLower(strings.TrimSpace(environmentKey))
	if !environmentKeyPattern.MatchString(environmentKey) {
		return "", "", fmt.Errorf("invalid bootstrap environment key %q", environmentKey)
	}
	principalID = strings.TrimSpace(principalID)
	if !principalIDPattern.MatchString(principalID) {
		return "", "", fmt.Errorf("invalid bootstrap principal id %q", principalID)
	}
	return environmentKey, principalID, nil
}

func projectAccessLockID(projectID project.ID) int64 {
	digest := sha256.Sum256([]byte("panvara:project-access:" + projectID.String()))
	return int64(binary.BigEndian.Uint64(digest[:8]))
}
