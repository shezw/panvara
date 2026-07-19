//go:build integration

/*
   Panvara
   tests/integration/postgres_access_admin_test.go    2026-07-19
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
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
	panvarapg "github.com/shezw/panvara/internal/infrastructure/postgres"
)

func TestPostgresAccessAdministrationSecurityTransactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool := isolatedPool(t, ctx, integrationDatabaseURL(t, ctx))
	if err := panvarapg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	definition := mustProjectAccessDefinition(
		t, "019f5c36-b322-7c52-9325-ec59f95c8fe0", "access-admin",
		"en-US", "UTC", "USD",
	)
	projectAccess, err := panvarapg.NewProjectAccessStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := projectAccess.EnsureBootstrapScope(
		ctx, definition, "default", "bootstrap-admin",
	)
	if err != nil {
		t.Fatalf("EnsureBootstrapScope() error = %v", err)
	}
	accessStore, err := panvarapg.NewAccessAdminStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := applicationaccess.NewDefaultBootstrapCredentialRegistrar(accessStore)
	if err != nil {
		t.Fatal(err)
	}
	const bootstrapToken = "postgres-access-bootstrap-token-canary-0123456789"
	bootstrapCredential, err := registrar.Register(
		ctx, scope, "bootstrap-admin", bootstrapToken,
	)
	if err != nil {
		t.Fatalf("Register(bootstrap) error = %v", err)
	}
	authenticator, err := applicationaccess.NewCredentialAuthenticator(accessStore)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := applicationaccess.NewPolicy(projectAccess)
	if err != nil {
		t.Fatal(err)
	}
	administration, err := applicationaccess.NewDefaultAdministration(
		accessStore, policy, accessStore,
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapInvocation := mustAccessAdminInvocation(
		t, ctx, authenticator, scope, bootstrapToken, "request:bootstrap",
	)

	assertAuditFailureRollsBackPrincipal(
		t, ctx, pool, administration, bootstrapInvocation,
	)

	firstPrincipal, err := administration.CreatePrincipal(
		ctx, bootstrapInvocation,
		applicationaccess.CreatePrincipalInput{DisplayName: "First owner"},
	)
	if err != nil {
		t.Fatalf("CreatePrincipal(first) error = %v", err)
	}
	secondPrincipal, err := administration.CreatePrincipal(
		ctx, bootstrapInvocation,
		applicationaccess.CreatePrincipalInput{DisplayName: "Second owner"},
	)
	if err != nil {
		t.Fatalf("CreatePrincipal(second) error = %v", err)
	}
	firstIssued, err := administration.IssueCredential(
		ctx, bootstrapInvocation,
		applicationaccess.IssueCredentialInput{
			PrincipalID: firstPrincipal.ID(), Label: "First owner key",
		},
	)
	if err != nil {
		t.Fatalf("IssueCredential(first) error = %v", err)
	}
	secondIssued, err := administration.IssueCredential(
		ctx, bootstrapInvocation,
		applicationaccess.IssueCredentialInput{
			PrincipalID: secondPrincipal.ID(), Label: "Second owner key",
		},
	)
	if err != nil {
		t.Fatalf("IssueCredential(second) error = %v", err)
	}
	if _, err := administration.GrantProjectOwner(
		ctx, bootstrapInvocation, firstPrincipal.ID(),
	); err != nil {
		t.Fatalf("GrantProjectOwner(first) error = %v", err)
	}
	if _, err := administration.GrantProjectOwner(
		ctx, bootstrapInvocation, secondPrincipal.ID(),
	); err != nil {
		t.Fatalf("GrantProjectOwner(second) error = %v", err)
	}

	firstInvocation := mustAccessAdminInvocation(
		t, ctx, authenticator, scope, firstIssued.Token, "request:mutual-first",
	)
	secondInvocation := mustAccessAdminInvocation(
		t, ctx, authenticator, scope, secondIssued.Token, "request:mutual-second",
	)
	survivor := assertMutualCredentialRevocationSerializes(
		t, ctx, pool, administration,
		firstInvocation, firstIssued.Credential.ID(),
		secondInvocation, secondIssued.Credential.ID(),
	)

	if _, err := administration.RevokeCredential(
		ctx, survivor.invocation, bootstrapCredential.ID(),
	); err != nil {
		t.Fatalf("RevokeCredential(bootstrap with service survivor) error = %v", err)
	}
	if _, err := administration.RevokeCredential(
		ctx, survivor.invocation, survivor.credentialID,
	); !errors.Is(err, applicationaccess.ErrLastOwnerPath) {
		t.Fatalf("RevokeCredential(last owner path) error = %v", err)
	}
	assertCredentialStatus(t, ctx, pool, scope, survivor.credentialID, "active")

	assertExplicitOwnerRegrantIsAuditedOnce(
		t, ctx, pool, accessStore, policy, administration,
		survivor.invocation, survivor.otherPrincipalID,
	)
	if _, err := administration.DisablePrincipal(
		ctx, survivor.invocation, survivor.otherPrincipalID,
	); err != nil {
		t.Fatalf("DisablePrincipal(non-effective owner) error = %v", err)
	}
	terminalPrincipal, err := administration.CreatePrincipal(
		ctx, survivor.invocation,
		applicationaccess.CreatePrincipalInput{DisplayName: "Unreferenced terminal principal"},
	)
	if err != nil {
		t.Fatalf("CreatePrincipal(unreferenced terminal) error = %v", err)
	}
	if _, err := administration.DisablePrincipal(
		ctx, survivor.invocation, terminalPrincipal.ID(),
	); err != nil {
		t.Fatalf("DisablePrincipal(unreferenced terminal) error = %v", err)
	}
	credentialPrincipal, err := administration.CreatePrincipal(
		ctx, survivor.invocation,
		applicationaccess.CreatePrincipalInput{DisplayName: "Revoked credential owner"},
	)
	if err != nil {
		t.Fatalf("CreatePrincipal(revoked credential owner) error = %v", err)
	}
	terminalCredential, err := administration.IssueCredential(
		ctx, survivor.invocation,
		applicationaccess.IssueCredentialInput{
			PrincipalID: credentialPrincipal.ID(), Label: "Terminal credential",
		},
	)
	if err != nil {
		t.Fatalf("IssueCredential(terminal) error = %v", err)
	}
	if _, err := administration.RevokeCredential(
		ctx, survivor.invocation, terminalCredential.Credential.ID(),
	); err != nil {
		t.Fatalf("RevokeCredential(terminal) error = %v", err)
	}
	assertTerminalAccessFactsRejectDirectResurrection(
		t, ctx, pool, scope, terminalPrincipal.ID(),
		terminalCredential.Credential.ID(), survivor.credentialID,
	)
	assertBootstrapRestartDoesNotRestore(
		t, ctx, registrar, scope, bootstrapCredential.ID(), bootstrapToken,
	)
	assertAccessSecurityFactsAreImmutableAndSecretFree(
		t, ctx, pool, scope, bootstrapToken,
	)
}

func assertTerminalAccessFactsRejectDirectResurrection(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	scope project.Scope,
	disabledPrincipalID string,
	revokedCredentialID domainaccess.ID,
	activeCredentialID domainaccess.ID,
) {
	t.Helper()
	projectID := scope.ProjectID().String()
	environmentID := scope.EnvironmentID().String()

	_, err := pool.Exec(ctx, `
		UPDATE panvara_principal
		SET status = 'active', disabled_at = NULL, updated_at = clock_timestamp()
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, disabledPrincipalID)
	assertPostgresSQLState(t, "reactivate disabled principal", err, "55000")
	var principalStatus string
	var disabledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT status, disabled_at FROM panvara_principal
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, disabledPrincipalID).Scan(&principalStatus, &disabledAt); err != nil {
		t.Fatalf("read disabled principal after rejected reactivation: %v", err)
	}
	if principalStatus != "disabled" || disabledAt == nil {
		t.Fatalf("principal after rejected reactivation = status %q disabled_at %v", principalStatus, disabledAt)
	}
	_, err = pool.Exec(ctx, `
		UPDATE panvara_principal
		SET display_name = 'rewritten terminal principal'
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, disabledPrincipalID)
	assertPostgresSQLState(t, "rewrite disabled principal", err, "55000")
	_, err = pool.Exec(ctx, `
		DELETE FROM panvara_principal
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, disabledPrincipalID)
	assertPostgresSQLState(t, "delete disabled principal", err, "55000")
	if err := pool.QueryRow(ctx, `
		SELECT status, disabled_at FROM panvara_principal
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, disabledPrincipalID).Scan(&principalStatus, &disabledAt); err != nil {
		t.Fatalf("read disabled principal after rejected delete: %v", err)
	}
	if principalStatus != "disabled" || disabledAt == nil {
		t.Fatalf("principal after rejected delete = status %q disabled_at %v", principalStatus, disabledAt)
	}

	_, err = pool.Exec(ctx, `
		UPDATE panvara_api_credential
		SET status = 'active', revoked_by_principal_id = NULL,
		    revoked_at = NULL, updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, revokedCredentialID.String())
	assertPostgresSQLState(t, "reactivate revoked credential", err, "55000")
	assertCredentialStatus(t, ctx, pool, scope, revokedCredentialID, "revoked")
	_, err = pool.Exec(ctx, `
		UPDATE panvara_api_credential
		SET label = 'rewritten terminal credential'
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, revokedCredentialID.String())
	assertPostgresSQLState(t, "rewrite revoked credential", err, "55000")
	_, err = pool.Exec(ctx, `
		DELETE FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, revokedCredentialID.String())
	assertPostgresSQLState(t, "delete revoked credential", err, "55000")
	assertCredentialStatus(t, ctx, pool, scope, revokedCredentialID, "revoked")

	var originalDigest []byte
	var activePrincipalID, originalIssuer string
	if err := pool.QueryRow(ctx, `
		SELECT secret_digest, principal_id, issued_by_principal_id
		FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, activeCredentialID.String()).Scan(
		&originalDigest, &activePrincipalID, &originalIssuer,
	); err != nil {
		t.Fatalf("read active credential digest: %v", err)
	}
	_, err = pool.Exec(ctx, `
		UPDATE panvara_principal
		SET kind = 'bootstrap'
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, activePrincipalID)
	assertPostgresSQLState(t, "rewrite active principal identity", err, "55000")
	_, err = pool.Exec(ctx, `
		UPDATE panvara_api_credential
		SET issued_by_principal_id = $4
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, activeCredentialID.String(), disabledPrincipalID)
	assertPostgresSQLState(t, "rewrite active credential provenance", err, "55000")
	_, err = pool.Exec(ctx, `
		UPDATE panvara_api_credential
		SET secret_digest = decode(repeat('cd', 32), 'hex'),
		    secret_hint = 'sha256:cdcdcdcdcdcd', updated_at = clock_timestamp()
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, activeCredentialID.String())
	assertPostgresSQLState(t, "rewrite credential secret evidence", err, "55000")
	var persistedDigest []byte
	if err := pool.QueryRow(ctx, `
		SELECT secret_digest FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, activeCredentialID.String()).Scan(&persistedDigest); err != nil {
		t.Fatalf("read active credential digest after rejected rewrite: %v", err)
	}
	if !bytes.Equal(persistedDigest, originalDigest) {
		t.Fatal("active credential digest changed after rejected rewrite")
	}
	var persistedIssuer, persistedKind string
	if err := pool.QueryRow(ctx, `
		SELECT issued_by_principal_id FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, projectID, environmentID, activeCredentialID.String()).Scan(&persistedIssuer); err != nil {
		t.Fatalf("read active credential issuer after rejected rewrite: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT kind FROM panvara_principal
		WHERE project_id = $1 AND principal_id = $2
	`, projectID, activePrincipalID).Scan(&persistedKind); err != nil {
		t.Fatalf("read active principal kind after rejected rewrite: %v", err)
	}
	if persistedIssuer != originalIssuer || persistedKind != "service" {
		t.Fatalf(
			"immutable access facts changed: issuer %q/%q kind %q",
			persistedIssuer, originalIssuer, persistedKind,
		)
	}
}

func assertPostgresSQLState(t *testing.T, operation string, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s error = nil, want SQLSTATE %s", operation, want)
	}
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		t.Fatalf("%s error = %T %v, want PostgreSQL error", operation, err, err)
	}
	if databaseError.Code != want {
		t.Fatalf("%s SQLSTATE = %s, want %s", operation, databaseError.Code, want)
	}
}

func assertAuditFailureRollsBackPrincipal(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	administration *applicationaccess.Administration,
	invocation applicationaccess.Invocation,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION panvara_test_reject_audit_insert()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			RAISE EXCEPTION 'forced audit insert failure';
		END;
		$$
	`); err != nil {
		t.Fatalf("create forced audit failure function: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TRIGGER panvara_test_reject_audit_insert
		BEFORE INSERT ON panvara_security_audit_event
		FOR EACH ROW EXECUTE FUNCTION panvara_test_reject_audit_insert()
	`); err != nil {
		t.Fatalf("create forced audit failure trigger: %v", err)
	}
	if _, err := administration.CreatePrincipal(
		ctx, invocation,
		applicationaccess.CreatePrincipalInput{DisplayName: "Must roll back"},
	); !errors.Is(err, applicationaccess.ErrUnavailable) {
		t.Fatalf("CreatePrincipal(forced audit failure) error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		DROP TRIGGER panvara_test_reject_audit_insert ON panvara_security_audit_event
	`); err != nil {
		t.Fatalf("drop forced audit failure trigger: %v", err)
	}
	var serviceCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_principal WHERE kind = 'service'
	`).Scan(&serviceCount); err != nil {
		t.Fatalf("count service principals after audit failure: %v", err)
	}
	if serviceCount != 0 {
		t.Fatalf("service principals after audit failure = %d, want 0", serviceCount)
	}
}

type accessAdminSurvivor struct {
	invocation       applicationaccess.Invocation
	credentialID     domainaccess.ID
	otherPrincipalID string
}

func assertMutualCredentialRevocationSerializes(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	administration *applicationaccess.Administration,
	firstInvocation applicationaccess.Invocation,
	firstCredentialID domainaccess.ID,
	secondInvocation applicationaccess.Invocation,
	secondCredentialID domainaccess.ID,
) accessAdminSurvivor {
	t.Helper()
	type result struct {
		caller string
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	go func() {
		<-start
		_, err := administration.RevokeCredential(ctx, firstInvocation, secondCredentialID)
		results <- result{caller: "first", err: err}
	}()
	go func() {
		<-start
		_, err := administration.RevokeCredential(ctx, secondInvocation, firstCredentialID)
		results <- result{caller: "second", err: err}
	}()
	close(start)
	firstResult := <-results
	secondResult := <-results
	successes := 0
	var winner string
	for _, value := range []result{firstResult, secondResult} {
		if value.err == nil {
			successes++
			winner = value.caller
			continue
		}
		if !errors.Is(value.err, applicationaccess.ErrUnauthenticated) &&
			!errors.Is(value.err, applicationaccess.ErrForbidden) {
			t.Fatalf("mutual revoke %s error = %v", value.caller, value.err)
		}
	}
	if successes != 1 {
		t.Fatalf("mutual revoke successes = %d, want 1 (%v, %v)", successes, firstResult.err, secondResult.err)
	}
	var revokedCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_api_credential
		WHERE credential_id IN ($1, $2) AND status = 'revoked'
	`, firstCredentialID.String(), secondCredentialID.String()).Scan(&revokedCount); err != nil {
		t.Fatalf("count mutually revoked credentials: %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("mutually revoked credentials = %d, want 1", revokedCount)
	}
	var revokeAuditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE action = $1 AND outcome = 'success'
	`, string(applicationaccess.OperationCredentialRevoke)).Scan(&revokeAuditCount); err != nil {
		t.Fatalf("count successful credential revoke audits: %v", err)
	}
	if revokeAuditCount != 1 {
		t.Fatalf("successful credential revoke audits = %d, want 1", revokeAuditCount)
	}
	if winner == "first" {
		return accessAdminSurvivor{
			invocation: firstInvocation, credentialID: firstCredentialID,
			otherPrincipalID: secondInvocation.Execution().Actor().ActorID(),
		}
	}
	return accessAdminSurvivor{
		invocation: secondInvocation, credentialID: secondCredentialID,
		otherPrincipalID: firstInvocation.Execution().Actor().ActorID(),
	}
}

func assertExplicitOwnerRegrantIsAuditedOnce(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	accessStore *panvarapg.AccessAdminStore,
	policy *applicationaccess.Policy,
	administration *applicationaccess.Administration,
	invocation applicationaccess.Invocation,
	principalID string,
) {
	t.Helper()
	revoked, err := administration.RevokeProjectOwner(ctx, invocation, principalID)
	if err != nil {
		t.Fatalf("RevokeProjectOwner(non-effective owner) error = %v", err)
	}
	revokedAt := revoked.RevokedAt()
	if revokedAt == nil {
		t.Fatal("RevokeProjectOwner(non-effective owner) returned no revoked_at")
	}
	regressedAdministration, err := applicationaccess.NewAdministration(
		accessStore, policy, accessStore, domainaccess.NewDefaultIDGenerator(), rand.Reader,
		fixedIntegrationAccessClock{at: revokedAt.Add(-time.Nanosecond)},
	)
	if err != nil {
		t.Fatalf("NewAdministration(regressed clock) error = %v", err)
	}
	var beforeRegression int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE action = $1 AND outcome = 'success' AND target_id = $2
	`, string(applicationaccess.OperationProjectOwnerGrant), principalID).Scan(&beforeRegression); err != nil {
		t.Fatalf("count owner grant audits before regressed regrant: %v", err)
	}
	if _, err := regressedAdministration.GrantProjectOwner(
		ctx, invocation, principalID,
	); !errors.Is(err, applicationaccess.ErrConflict) {
		t.Fatalf("GrantProjectOwner(regressed clock) error = %v", err)
	}
	var afterRegression int
	var persistedRevokedAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT revoked_at FROM panvara_access_grant
		WHERE project_id = $1 AND environment_id = $2
		  AND principal_id = $3 AND role = $4
	`, invocation.Execution().Scope().ProjectID().String(),
		invocation.Execution().Scope().EnvironmentID().String(), principalID,
		applicationaccess.RoleProjectOwner).Scan(&persistedRevokedAt); err != nil {
		t.Fatalf("read owner grant after regressed regrant: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE action = $1 AND outcome = 'success' AND target_id = $2
	`, string(applicationaccess.OperationProjectOwnerGrant), principalID).Scan(&afterRegression); err != nil {
		t.Fatalf("count owner grant audits after regressed regrant: %v", err)
	}
	if persistedRevokedAt == nil || !persistedRevokedAt.Equal(*revokedAt) ||
		afterRegression != beforeRegression {
		t.Fatalf(
			"regressed regrant changed grant/audit: revoked_at=%v want=%v audits=%d/%d",
			persistedRevokedAt, revokedAt, beforeRegression, afterRegression,
		)
	}
	if _, err := administration.GrantProjectOwner(ctx, invocation, principalID); err != nil {
		t.Fatalf("GrantProjectOwner(explicit regrant) error = %v", err)
	}
	var before int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE action = $1 AND outcome = 'success' AND target_id = $2
	`, string(applicationaccess.OperationProjectOwnerGrant), principalID).Scan(&before); err != nil {
		t.Fatalf("count owner grant audits before idempotent put: %v", err)
	}
	if _, err := administration.GrantProjectOwner(ctx, invocation, principalID); err != nil {
		t.Fatalf("GrantProjectOwner(idempotent active) error = %v", err)
	}
	var after int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM panvara_security_audit_event
		WHERE action = $1 AND outcome = 'success' AND target_id = $2
	`, string(applicationaccess.OperationProjectOwnerGrant), principalID).Scan(&after); err != nil {
		t.Fatalf("count owner grant audits after idempotent put: %v", err)
	}
	if before != 2 || after != before {
		t.Fatalf("owner grant audits before/after idempotent put = %d/%d, want 2/2", before, after)
	}
}

type fixedIntegrationAccessClock struct{ at time.Time }

func (clock fixedIntegrationAccessClock) Now() time.Time { return clock.at }

func assertBootstrapRestartDoesNotRestore(
	t *testing.T,
	ctx context.Context,
	registrar *applicationaccess.BootstrapCredentialRegistrar,
	scope project.Scope,
	credentialID domainaccess.ID,
	rawToken string,
) {
	t.Helper()
	withoutToken, err := registrar.Register(ctx, scope, "bootstrap-admin", "")
	if err != nil {
		t.Fatalf("Register(restart without token) error = %v", err)
	}
	if withoutToken.ID() != credentialID || withoutToken.Active() {
		t.Fatalf("restart without token credential = %s active=%t", withoutToken.ID().String(), withoutToken.Active())
	}
	withSameToken, err := registrar.Register(ctx, scope, "bootstrap-admin", rawToken)
	if err != nil {
		t.Fatalf("Register(restart same token) error = %v", err)
	}
	if withSameToken.ID() != credentialID || withSameToken.Active() {
		t.Fatalf("restart same token credential = %s active=%t", withSameToken.ID().String(), withSameToken.Active())
	}
	if _, err := registrar.Register(
		ctx, scope, "bootstrap-admin", "different-bootstrap-token-canary-0123456789",
	); !errors.Is(err, applicationaccess.ErrConflict) {
		t.Fatalf("Register(restart different token) error = %v", err)
	}
}

func assertCredentialStatus(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	scope project.Scope,
	credentialID domainaccess.ID,
	want string,
) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM panvara_api_credential
		WHERE project_id = $1 AND environment_id = $2 AND credential_id = $3
	`, scope.ProjectID().String(), scope.EnvironmentID().String(), credentialID.String()).Scan(&status); err != nil {
		t.Fatalf("read credential status: %v", err)
	}
	if status != want {
		t.Fatalf("credential status = %q, want %q", status, want)
	}
}

func assertAccessSecurityFactsAreImmutableAndSecretFree(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	scope project.Scope,
	secretCanary string,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		DELETE FROM panvara_access_bootstrap_marker
		WHERE project_id = $1 AND environment_id = $2
	`, scope.ProjectID().String(), scope.EnvironmentID().String()); err == nil {
		t.Fatal("DELETE bootstrap marker error = nil")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE panvara_security_audit_event SET reason_code = 'tampered'
		WHERE project_id = $1 AND environment_id = $2
	`, scope.ProjectID().String(), scope.EnvironmentID().String()); err == nil {
		t.Fatal("UPDATE security audit error = nil")
	}
	var leaks int
	pattern := "%" + secretCanary + "%"
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM (
			SELECT to_jsonb(credential)::text AS payload FROM panvara_api_credential AS credential
			UNION ALL
			SELECT to_jsonb(marker)::text FROM panvara_access_bootstrap_marker AS marker
			UNION ALL
			SELECT to_jsonb(audit)::text FROM panvara_security_audit_event AS audit
		) AS security_fact
		WHERE payload LIKE $1
	`, pattern).Scan(&leaks); err != nil {
		t.Fatalf("scan security facts for secret canary: %v", err)
	}
	if leaks != 0 {
		t.Fatalf("security facts containing bootstrap token = %d, want 0", leaks)
	}
}

func mustAccessAdminInvocation(
	t *testing.T,
	ctx context.Context,
	authenticator applicationaccess.PrincipalAuthenticator,
	scope project.Scope,
	token string,
	requestID string,
) applicationaccess.Invocation {
	t.Helper()
	principal, err := authenticator.Authenticate(ctx, scope, token)
	if err != nil {
		t.Fatalf("Authenticate(%s) error = %v", requestID, err)
	}
	execution, err := applicationaccess.NewAdminExecution(scope, principal)
	if err != nil {
		t.Fatalf("NewAdminExecution(%s) error = %v", requestID, err)
	}
	invocation, err := applicationaccess.NewInvocation(execution, requestID)
	if err != nil {
		t.Fatalf("NewInvocation(%s) error = %v", requestID, err)
	}
	return invocation
}
