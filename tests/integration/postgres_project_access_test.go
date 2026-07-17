//go:build integration

/*
   Panvara
   tests/integration/postgres_project_access_test.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shezw/panvara/internal/domain/project"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type execQueryRower interface {
	queryRower
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func TestPostgresProjectEnvironmentAccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	databaseURL := integrationDatabaseURL(t, ctx)
	pool := isolatedPool(t, ctx, databaseURL)

	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() fresh error = %v", err)
	}
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() idempotent error = %v", err)
	}
	store, err := panvarapg.NewProjectAccessStore(pool)
	if err != nil {
		t.Fatalf("NewProjectAccessStore() error = %v", err)
	}

	definition := mustProjectAccessDefinition(
		t,
		"01981234-5678-7abc-8def-0123456789ab",
		"demo",
		"en-US",
		"Asia/Shanghai",
		"CNY",
	)
	scope, err := store.EnsureBootstrapScope(ctx, definition, "default", "bootstrap-admin")
	if err != nil {
		t.Fatalf("EnsureBootstrapScope() fresh error = %v", err)
	}
	assertBootstrapRows(t, ctx, pool, scope, definition)

	restartedStore, err := panvarapg.NewProjectAccessStore(pool)
	if err != nil {
		t.Fatalf("NewProjectAccessStore() restart error = %v", err)
	}
	restartedScope, err := restartedStore.EnsureBootstrapScope(
		ctx,
		definition,
		"default",
		"bootstrap-admin",
	)
	if err != nil {
		t.Fatalf("EnsureBootstrapScope() restart error = %v", err)
	}
	if restartedScope.ProjectID() != scope.ProjectID() || restartedScope.EnvironmentID() != scope.EnvironmentID() {
		t.Fatalf("restart scope = %s/%s, want %s/%s",
			restartedScope.ProjectID().String(), restartedScope.EnvironmentID().String(),
			scope.ProjectID().String(), scope.EnvironmentID().String(),
		)
	}
	if active, err := store.ScopeActive(ctx, scope); err != nil || !active {
		t.Fatalf("ScopeActive(default) = %t, %v", active, err)
	}
	if granted, err := store.HasActiveGrant(ctx, scope, "bootstrap-admin", "project.owner"); err != nil || !granted {
		t.Fatalf("HasActiveGrant(owner) = %t, %v", granted, err)
	}
	if granted, err := store.HasActiveGrant(ctx, scope, "bootstrap-admin", "project.editor"); err != nil || granted {
		t.Fatalf("HasActiveGrant(unknown role) = %t, %v", granted, err)
	}

	assertProjectConfigurationDriftFails(t, ctx, store, definition)
	assertConcurrentSecondProjectBootstrapIsStable(t, ctx, store, scope)
	assertNonDefaultEnvironmentIsInactive(t, ctx, pool, store, scope)
	assertAccessSchemaContainsNoCredentials(t, ctx, pool)
	assertProjectAccessConstraints(t, ctx, pool, scope, definition)

	if _, err := pool.Exec(ctx, `
		UPDATE panvara_access_grant
		SET revoked_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
	`, scope.ProjectID().String(), scope.EnvironmentID().String()); err != nil {
		t.Fatalf("revoke bootstrap owner grant: %v", err)
	}
	afterRevokeScope, err := restartedStore.EnsureBootstrapScope(
		ctx,
		definition,
		"default",
		"bootstrap-admin",
	)
	if err != nil {
		t.Fatalf("EnsureBootstrapScope() after revoke error = %v", err)
	}
	if afterRevokeScope.EnvironmentID() != scope.EnvironmentID() {
		t.Fatalf("EnsureBootstrapScope() after revoke changed environment id")
	}
	if granted, err := store.HasActiveGrant(ctx, scope, "bootstrap-admin", "project.owner"); err != nil || granted {
		t.Fatalf("HasActiveGrant(revoked owner) = %t, %v", granted, err)
	}
	var revoked bool
	if err := pool.QueryRow(ctx, `
		SELECT revoked_at IS NOT NULL
		FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&revoked); err != nil {
		t.Fatalf("read revoked owner grant: %v", err)
	}
	if !revoked {
		t.Fatal("EnsureBootstrapScope() silently restored a revoked owner grant")
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
	`, scope.ProjectID().String(), scope.EnvironmentID().String()); err != nil {
		t.Fatalf("delete bootstrap owner grant: %v", err)
	}
	if _, err := restartedStore.EnsureBootstrapScope(
		ctx, definition, "default", "bootstrap-admin",
	); err != nil {
		t.Fatalf("EnsureBootstrapScope() after missing grant error = %v", err)
	}
	var grantCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&grantCount); err != nil {
		t.Fatalf("count missing bootstrap owner grant: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("EnsureBootstrapScope() silently recreated %d missing owner grants", grantCount)
	}
}

func assertBootstrapRows(
	t *testing.T,
	ctx context.Context,
	pool queryRower,
	scope project.Scope,
	definition project.Context,
) {
	t.Helper()
	var (
		projectKey, locale, timeZone, currency, projectStatus string
		environmentKey, environmentStatus, principalStatus    string
		isDefault                                             bool
		grantCount                                            int
	)
	if err := pool.QueryRow(ctx, `
		SELECT project_key, default_locale, default_time_zone, default_currency, status
		FROM panvara_project WHERE project_id = $1
	`, scope.ProjectID().String()).Scan(
		&projectKey, &locale, &timeZone, &currency, &projectStatus,
	); err != nil {
		t.Fatalf("read bootstrap project: %v", err)
	}
	if projectKey != definition.Key().String() || locale != definition.Locale() ||
		timeZone != definition.TimeZone() || currency != definition.Currency().String() ||
		projectStatus != "active" {
		t.Fatalf("persisted project = %q %q %q %q %q", projectKey, locale, timeZone, currency, projectStatus)
	}
	if err := pool.QueryRow(ctx, `
		SELECT environment_key, is_default, status
		FROM panvara_environment
		WHERE project_id = $1 AND environment_id = $2
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(
		&environmentKey, &isDefault, &environmentStatus,
	); err != nil {
		t.Fatalf("read bootstrap environment: %v", err)
	}
	if environmentKey != "default" || !isDefault || environmentStatus != "active" {
		t.Fatalf("persisted environment = %q default=%t status=%q", environmentKey, isDefault, environmentStatus)
	}
	if err := pool.QueryRow(ctx, `
		SELECT status FROM panvara_principal
		WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
	`, scope.ProjectID().String()).Scan(&principalStatus); err != nil {
		t.Fatalf("read bootstrap principal: %v", err)
	}
	if principalStatus != "active" {
		t.Fatalf("bootstrap principal status = %q", principalStatus)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = 'bootstrap-admin' AND role = 'project.owner'
		  AND revoked_at IS NULL
	`, scope.ProjectID().String(), scope.EnvironmentID().String()).Scan(&grantCount); err != nil {
		t.Fatalf("read bootstrap owner grant: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("active bootstrap owner grant count = %d, want 1", grantCount)
	}
}

func assertProjectConfigurationDriftFails(
	t *testing.T,
	ctx context.Context,
	store *panvarapg.ProjectAccessStore,
	definition project.Context,
) {
	t.Helper()
	tests := []struct {
		name, key, locale, timeZone, currency, environmentKey, principalID string
	}{
		{name: "project key", key: "changed", locale: "en-US", timeZone: "Asia/Shanghai", currency: "CNY", environmentKey: "default", principalID: "bootstrap-admin"},
		{name: "locale", key: "demo", locale: "zh-CN", timeZone: "Asia/Shanghai", currency: "CNY", environmentKey: "default", principalID: "bootstrap-admin"},
		{name: "time zone", key: "demo", locale: "en-US", timeZone: "UTC", currency: "CNY", environmentKey: "default", principalID: "bootstrap-admin"},
		{name: "currency", key: "demo", locale: "en-US", timeZone: "Asia/Shanghai", currency: "USD", environmentKey: "default", principalID: "bootstrap-admin"},
		{name: "environment key", key: "demo", locale: "en-US", timeZone: "Asia/Shanghai", currency: "CNY", environmentKey: "production", principalID: "bootstrap-admin"},
		{name: "bootstrap principal", key: "demo", locale: "en-US", timeZone: "Asia/Shanghai", currency: "CNY", environmentKey: "default", principalID: "other-admin"},
	}
	for _, test := range tests {
		t.Run("configuration drift/"+test.name, func(t *testing.T) {
			changed, err := project.NewContext(
				definition.ID().String(), test.key, test.locale, test.timeZone, test.currency,
			)
			if err != nil {
				t.Fatalf("NewContext() drift fixture error = %v", err)
			}
			if _, err := store.EnsureBootstrapScope(
				ctx, changed, test.environmentKey, test.principalID,
			); err == nil {
				t.Fatal("EnsureBootstrapScope() configuration drift error = nil")
			}
		})
	}
}

func assertConcurrentSecondProjectBootstrapIsStable(
	t *testing.T,
	ctx context.Context,
	store *panvarapg.ProjectAccessStore,
	firstScope project.Scope,
) {
	t.Helper()
	definition := mustProjectAccessDefinition(
		t,
		"01981234-5678-7abc-8def-0123456789b0",
		"concurrent",
		"en-US",
		"UTC",
		"USD",
	)
	const callers = 8
	start := make(chan struct{})
	results := make(chan project.Scope, callers)
	errorsFound := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			scope, err := store.EnsureBootstrapScope(ctx, definition, "default", "second-admin")
			if err != nil {
				errorsFound <- err
				return
			}
			results <- scope
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatalf("concurrent EnsureBootstrapScope() error = %v", err)
	}
	var environmentID project.EnvironmentID
	count := 0
	for scope := range results {
		count++
		if !environmentID.Valid() {
			environmentID = scope.EnvironmentID()
			continue
		}
		if scope.EnvironmentID() != environmentID {
			t.Fatalf("concurrent bootstrap environments = %s and %s", environmentID.String(), scope.EnvironmentID().String())
		}
	}
	if count != callers {
		t.Fatalf("concurrent bootstrap result count = %d, want %d", count, callers)
	}
	if environmentID == firstScope.EnvironmentID() {
		t.Fatal("separate projects unexpectedly share an environment id")
	}
	secondScope, err := project.NewScope(definition.ID(), environmentID)
	if err != nil {
		t.Fatal(err)
	}
	if active, err := store.ScopeActive(ctx, firstScope); err != nil || !active {
		t.Fatalf("ScopeActive(first project after second bootstrap) = %t, %v", active, err)
	}
	if active, err := store.ScopeActive(ctx, secondScope); err != nil || !active {
		t.Fatalf("ScopeActive(second project) = %t, %v", active, err)
	}
	for _, test := range []struct {
		name      string
		scope     project.Scope
		principal string
		want      bool
	}{
		{name: "first exact grant", scope: firstScope, principal: "bootstrap-admin", want: true},
		{name: "first rejects second principal", scope: firstScope, principal: "second-admin"},
		{name: "second exact grant", scope: secondScope, principal: "second-admin", want: true},
		{name: "second rejects first principal", scope: secondScope, principal: "bootstrap-admin"},
	} {
		granted, err := store.HasActiveGrant(ctx, test.scope, test.principal, "project.owner")
		if err != nil || granted != test.want {
			t.Fatalf("%s HasActiveGrant() = %t, %v, want %t", test.name, granted, err, test.want)
		}
	}
}

func assertNonDefaultEnvironmentIsInactive(
	t *testing.T,
	ctx context.Context,
	pool execQueryRower,
	store *panvarapg.ProjectAccessStore,
	defaultScope project.Scope,
) {
	t.Helper()
	otherID, err := project.ParseEnvironmentID("019f5c36-b322-7c52-9325-ec59f95c8faf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO panvara_environment (
			project_id, environment_id, environment_key, is_default
		) VALUES ($1, $2, 'secondary', false)
	`, defaultScope.ProjectID().String(), otherID.String()); err != nil {
		t.Fatalf("insert non-default environment: %v", err)
	}
	otherScope, err := project.NewScope(defaultScope.ProjectID(), otherID)
	if err != nil {
		t.Fatal(err)
	}
	if active, err := store.ScopeActive(ctx, otherScope); err != nil || active {
		t.Fatalf("ScopeActive(non-default) = %t, %v", active, err)
	}
	if granted, err := store.HasActiveGrant(ctx, otherScope, "bootstrap-admin", "project.owner"); err != nil || granted {
		t.Fatalf("HasActiveGrant(non-default) = %t, %v", granted, err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE panvara_environment SET status = 'disabled', updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
	`, defaultScope.ProjectID().String(), defaultScope.EnvironmentID().String()); err != nil {
		t.Fatalf("disable default environment: %v", err)
	}
	if active, err := store.ScopeActive(ctx, defaultScope); err != nil || active {
		t.Fatalf("ScopeActive(disabled environment) = %t, %v", active, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_environment SET status = 'active', updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
	`, defaultScope.ProjectID().String(), defaultScope.EnvironmentID().String()); err != nil {
		t.Fatalf("restore default environment: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE panvara_project SET status = 'disabled', updated_at = clock_timestamp()
		WHERE project_id = $1
	`, defaultScope.ProjectID().String()); err != nil {
		t.Fatalf("disable project: %v", err)
	}
	if active, err := store.ScopeActive(ctx, defaultScope); err != nil || active {
		t.Fatalf("ScopeActive(disabled project) = %t, %v", active, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_project SET status = 'active', updated_at = clock_timestamp()
		WHERE project_id = $1
	`, defaultScope.ProjectID().String()); err != nil {
		t.Fatalf("restore project: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE panvara_principal SET status = 'disabled', updated_at = clock_timestamp()
		WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
	`, defaultScope.ProjectID().String()); err != nil {
		t.Fatalf("disable bootstrap principal: %v", err)
	}
	if granted, err := store.HasActiveGrant(ctx, defaultScope, "bootstrap-admin", "project.owner"); err != nil || granted {
		t.Fatalf("HasActiveGrant(disabled principal) = %t, %v", granted, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_principal SET status = 'active', updated_at = clock_timestamp()
		WHERE project_id = $1 AND principal_id = 'bootstrap-admin'
	`, defaultScope.ProjectID().String()); err != nil {
		t.Fatalf("restore bootstrap principal: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE panvara_environment
		SET is_default = false, updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
	`, defaultScope.ProjectID().String(), defaultScope.EnvironmentID().String()); err != nil {
		t.Fatalf("remove default environment marker: %v", err)
	}
	if active, err := store.ScopeActive(ctx, defaultScope); err != nil || active {
		t.Fatalf("ScopeActive(project without default environment) = %t, %v", active, err)
	}
	definition := mustProjectAccessDefinition(
		t,
		defaultScope.ProjectID().String(),
		"demo",
		"en-US",
		"Asia/Shanghai",
		"CNY",
	)
	if _, err := store.EnsureBootstrapScope(
		ctx, definition, "default", "bootstrap-admin",
	); err == nil {
		t.Fatal("EnsureBootstrapScope(project without default environment) error = nil")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_environment
		SET is_default = true, updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2
	`, defaultScope.ProjectID().String(), defaultScope.EnvironmentID().String()); err != nil {
		t.Fatalf("restore default environment marker: %v", err)
	}
	if active, err := store.ScopeActive(ctx, defaultScope); err != nil || !active {
		t.Fatalf("ScopeActive(restored default environment) = %t, %v", active, err)
	}
}

func assertAccessSchemaContainsNoCredentials(t *testing.T, ctx context.Context, pool queryRower) {
	t.Helper()
	var sensitiveColumnCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name IN (
			'panvara_project', 'panvara_environment', 'panvara_principal', 'panvara_access_grant'
		  )
		  AND column_name ~ '(token|secret|password|credential)'
	`).Scan(&sensitiveColumnCount); err != nil {
		t.Fatalf("inspect project/access credential columns: %v", err)
	}
	if sensitiveColumnCount != 0 {
		t.Fatalf("project/access sensitive credential column count = %d, want 0", sensitiveColumnCount)
	}
}

func assertProjectAccessConstraints(
	t *testing.T,
	ctx context.Context,
	pool execQueryRower,
	scope project.Scope,
	definition project.Context,
) {
	t.Helper()
	assertPostgresCode(t, "23514", func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO panvara_access_grant (
				project_id, environment_id, principal_id, role
			) VALUES ($1, $2, 'bootstrap-admin', 'project.editor')
		`, scope.ProjectID().String(), scope.EnvironmentID().String())
		return err
	})
	assertPostgresCode(t, "23503", func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO panvara_access_grant (
				project_id, environment_id, principal_id, role
			) VALUES ($1, $2, 'missing-principal', 'project.owner')
		`, scope.ProjectID().String(), scope.EnvironmentID().String())
		return err
	})
	assertPostgresCode(t, "23505", func() error {
		_, err := pool.Exec(ctx, `
			UPDATE panvara_environment SET is_default = true
			WHERE project_id = $1 AND environment_key = 'secondary'
		`, scope.ProjectID().String())
		return err
	})
	assertPostgresCode(t, "23505", func() error {
		_, err := pool.Exec(ctx, `
			INSERT INTO panvara_project (
				project_id, project_key, default_locale, default_time_zone, default_currency
			) VALUES ($1, $2, 'en-US', 'UTC', 'USD')
		`, "01981234-5678-7abc-8def-0123456789b1", definition.Key().String())
		return err
	})
	assertPostgresCode(t, "23514", func() error {
		_, err := pool.Exec(ctx, `
			UPDATE panvara_project SET status = 'unknown' WHERE project_id = $1
		`, scope.ProjectID().String())
		return err
	})
}

func assertPostgresCode(t *testing.T, want string, operation func() error) {
	t.Helper()
	err := operation()
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != want {
		t.Fatalf("PostgreSQL constraint error = %v, want SQLSTATE %s", err, want)
	}
}

func mustProjectAccessDefinition(
	t *testing.T,
	id string,
	key string,
	locale string,
	timeZone string,
	currency string,
) project.Context {
	t.Helper()
	definition, err := project.NewContext(id, key, locale, timeZone, currency)
	if err != nil {
		t.Fatalf("NewContext() error = %v", err)
	}
	return definition
}
