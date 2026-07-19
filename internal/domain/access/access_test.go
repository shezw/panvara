/*
   Panvara
   internal/domain/access/access_test.go    2026-07-19
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
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

const (
	testProjectID     = "019f5c36-b322-7c52-9325-ec59f95c8fae"
	testEnvironmentID = "019f5c36-b324-7c52-9325-ec59f95c8fae"
	testAccessID      = "019f5c36-b326-7c52-9325-ec59f95c8fae"
)

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

func TestIDGeneratorCreatesUUIDv7WithInjectedClockAndEntropy(t *testing.T) {
	generator, err := NewIDGenerator(
		bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}),
		fixedClock{at: time.UnixMilli(1_700_000_000_123)},
	)
	if err != nil {
		t.Fatal(err)
	}
	id, err := generator.New()
	if err != nil {
		t.Fatal(err)
	}
	if !id.Valid() || id.String()[14] != '7' || !strings.Contains("89ab", id.String()[19:20]) {
		t.Fatalf("generated id %q is not UUIDv7", id.String())
	}
	if parsed, err := ParseID(strings.ToUpper(id.String())); err != nil || parsed != id {
		t.Fatalf("ParseID() = %q, %v", parsed.String(), err)
	}
}

func TestServicePrincipalLifecycleIsTerminal(t *testing.T) {
	projectID := mustProjectID(t)
	id := mustID(t, testAccessID)
	createdAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	principal, err := NewServicePrincipal(projectID, id, " Worker API ", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID() != "svc:"+id.String() || principal.DisplayName() != "Worker API" ||
		principal.Kind() != PrincipalKindService || !principal.Active() {
		t.Fatalf("unexpected principal: id=%q name=%q kind=%q status=%q",
			principal.ID(), principal.DisplayName(), principal.Kind(), principal.Status())
	}
	disabledAt := createdAt.Add(time.Hour)
	disabled, err := principal.Disable(disabledAt)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Active() || disabled.DisabledAt() == nil || !disabled.DisabledAt().Equal(disabledAt) {
		t.Fatalf("disabled principal lifecycle facts are incomplete")
	}
	if _, err := disabled.Disable(disabledAt.Add(time.Hour)); err == nil {
		t.Fatal("disabled principal was re-transitioned")
	}
}

func TestBootstrapPrincipalRestoresLegacy0004Identifier(t *testing.T) {
	createdAt := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	principal, err := NewPrincipal(PrincipalMaterial{
		ProjectID: mustProjectID(t), ID: "svc:legacy", Kind: PrincipalKindBootstrap,
		DisplayName: "svc:legacy", Status: PrincipalStatusActive,
		CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID() != "svc:legacy" || principal.Kind() != PrincipalKindBootstrap {
		t.Fatalf("restored legacy principal = id %q kind %q", principal.ID(), principal.Kind())
	}
}

func TestCredentialRequiresExactDerivedHintAndTerminalRevocation(t *testing.T) {
	scope := mustScope(t)
	issuedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	material := CredentialMaterial{
		Scope: scope, ID: mustID(t, testAccessID), PrincipalID: "bootstrap-admin",
		Label: " deploy ", Hint: "sha256:012345abcdef", Status: CredentialStatusActive,
		IssuedBy: "bootstrap-admin", IssuedAt: issuedAt,
	}
	credential, err := NewCredential(material)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Label() != "deploy" || credential.Hint() != material.Hint ||
		credential.Scope().EnvironmentID() != scope.EnvironmentID() {
		t.Fatalf("unexpected credential metadata")
	}
	for _, hint := range []string{"012345abcdef", "sha256:012345ABCDEf", "sha256:012345abcdef0"} {
		invalid := material
		invalid.Hint = hint
		if _, err := NewCredential(invalid); err == nil {
			t.Fatalf("NewCredential() accepted hint %q", hint)
		}
	}
	revokedAt := issuedAt.Add(time.Minute)
	revoked, err := credential.Revoke("bootstrap-admin", revokedAt)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Active() || revoked.RevokedAt() == nil || revoked.RevokedBy() != "bootstrap-admin" {
		t.Fatal("credential revocation facts are incomplete")
	}
	if _, err := revoked.Revoke("bootstrap-admin", revokedAt.Add(time.Minute)); err == nil {
		t.Fatal("revoked credential was re-transitioned")
	}
}

func TestOwnerGrantLifecyclePreservesExactScope(t *testing.T) {
	scope := mustScope(t)
	grantedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	grant, err := NewOwnerGrant(OwnerGrantMaterial{
		Scope: scope, PrincipalID: "bootstrap-admin",
		GrantedBy: "bootstrap-admin", GrantedAt: grantedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !grant.Active() || grant.Role() != RoleProjectOwner || grant.Scope() != scope {
		t.Fatalf("unexpected owner grant")
	}
	revoked, err := grant.Revoke("bootstrap-admin", grantedAt.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Active() || revoked.RevokedAt() == nil {
		t.Fatal("owner grant revocation facts are incomplete")
	}
}

func mustID(t *testing.T, value string) ID {
	t.Helper()
	id, err := ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustProjectID(t *testing.T) project.ID {
	t.Helper()
	id, err := project.ParseID(testProjectID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustScope(t *testing.T) project.Scope {
	t.Helper()
	environmentID, err := project.ParseEnvironmentID(testEnvironmentID)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(mustProjectID(t), environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
