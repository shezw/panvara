/*
   Panvara
   internal/infrastructure/postgres/access_admin_store_test.go    2026-07-19
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
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	applicationaccess "github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

func TestNewAccessAdminStoreRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewAccessAdminStore(nil); err == nil {
		t.Fatal("NewAccessAdminStore(nil) error = nil")
	}
	if _, err := newAccessAdminStore(&pgxpool.Pool{}, nil); err == nil {
		t.Fatal("newAccessAdminStore(pool, nil) error = nil")
	}
	if _, err := newAccessAdminStore(&pgxpool.Pool{}, fixedAccessAuditIDs{}); err != nil {
		t.Fatalf("newAccessAdminStore(valid dependencies) error = %v", err)
	}
}

func TestValidateCredentialFactsRequiresDigestDerivedHint(t *testing.T) {
	t.Parallel()

	scope := mustAccessAdminScope(t)
	digest := applicationaccess.SecretDigest(sha256.Sum256([]byte("credential-token-canary")))
	credential := mustAccessAdminCredential(t, scope, digest)
	if err := validateCredentialFacts(credential, digest); err != nil {
		t.Fatalf("validateCredentialFacts(valid) error = %v", err)
	}

	other := applicationaccess.SecretDigest(sha256.Sum256([]byte("different-token")))
	if err := validateCredentialFacts(credential, other); !errors.Is(err, applicationaccess.ErrInvalid) {
		t.Fatalf("validateCredentialFacts(mismatch) error = %v", err)
	}
}

func TestAppendAuditUsesOnlyFixedNonSecretColumns(t *testing.T) {
	t.Parallel()

	scope := mustAccessAdminScope(t)
	credentialID := mustAccessID(t, "019f5c36-b322-7c52-9325-ec59f95c8fb0")
	executor := &recordingAuditExecutor{}
	store, err := newAccessAdminStore(&pgxpool.Pool{}, fixedAccessAuditIDs{})
	if err != nil {
		t.Fatal(err)
	}
	err = store.appendAudit(context.Background(), executor, auditFact{
		scope: scope, actorID: "bootstrap-admin", credentialID: credentialID,
		requestID: "request:unit-1", action: string(applicationaccess.OperationPrincipalCreate),
		outcome: auditOutcomeSuccess, targetKind: "principal",
		targetID:   "svc:019f5c36-b322-7c52-9325-ec59f95c8fb1",
		occurredAt: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("appendAudit() error = %v", err)
	}
	if executor.calls != 1 || len(executor.arguments) != 12 {
		t.Fatalf("appendAudit() calls = %d args = %d", executor.calls, len(executor.arguments))
	}
	for _, argument := range executor.arguments {
		if value, ok := argument.(string); ok && value == "credential-token-canary" {
			t.Fatal("appendAudit() persisted raw credential token")
		}
	}
}

func TestMapAccessWriteErrorPreservesStableLifecycleSemantics(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		code string
		want error
	}{
		{code: "23505", want: applicationaccess.ErrConflict},
		{code: "23503", want: applicationaccess.ErrNotFound},
		{code: "23514", want: applicationaccess.ErrInvalid},
	} {
		err := mapAccessWriteError("test mutation", &pgconn.PgError{Code: test.code})
		if !errors.Is(err, test.want) {
			t.Fatalf("mapAccessWriteError(%s) = %v, want %v", test.code, err, test.want)
		}
	}
}

func TestOwnerRegrantTimeCannotPrecedePriorRevocation(t *testing.T) {
	t.Parallel()

	grantedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	grant, err := domainaccess.NewOwnerGrant(domainaccess.OwnerGrantMaterial{
		Scope: mustAccessAdminScope(t), PrincipalID: "bootstrap-admin",
		GrantedBy: "bootstrap-admin", GrantedAt: grantedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	revokedAt := grantedAt.Add(time.Minute)
	revoked, err := grant.Revoke("bootstrap-admin", revokedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := requireMonotonicOwnerRegrant(
		revoked, revokedAt.Add(-time.Nanosecond),
	); !errors.Is(err, applicationaccess.ErrConflict) {
		t.Fatalf("requireMonotonicOwnerRegrant(regressed) error = %v", err)
	}
	if err := requireMonotonicOwnerRegrant(revoked, revokedAt); err != nil {
		t.Fatalf("requireMonotonicOwnerRegrant(equal) error = %v", err)
	}
}

type fixedAccessAuditIDs struct{}

func (fixedAccessAuditIDs) New() (domainaccess.ID, error) {
	return domainaccess.ParseID("019f5c36-b322-7c52-9325-ec59f95c8fb2")
}

type recordingAuditExecutor struct {
	calls     int
	arguments []any
}

func (executor *recordingAuditExecutor) Exec(
	_ context.Context,
	_ string,
	arguments ...any,
) (pgconn.CommandTag, error) {
	executor.calls++
	executor.arguments = append([]any(nil), arguments...)
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func mustAccessAdminScope(t *testing.T) project.Scope {
	t.Helper()
	projectID, err := project.ParseID(projectAccessTestID)
	if err != nil {
		t.Fatal(err)
	}
	environmentID, err := project.ParseEnvironmentID("019f5c36-b322-7c52-9325-ec59f95c8faf")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func mustAccessAdminCredential(
	t *testing.T,
	scope project.Scope,
	digest applicationaccess.SecretDigest,
) domainaccess.Credential {
	t.Helper()
	id := mustAccessID(t, "019f5c36-b322-7c52-9325-ec59f95c8fb0")
	credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: scope, ID: id, PrincipalID: "bootstrap-admin",
		Label: "bootstrap administration", Hint: fmt.Sprintf("sha256:%x", digest[:6]),
		Status: domainaccess.CredentialStatusActive, IssuedBy: "bootstrap-admin",
		IssuedAt: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func mustAccessID(t *testing.T, value string) domainaccess.ID {
	t.Helper()
	id, err := domainaccess.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
