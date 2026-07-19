/*
   Panvara
   internal/application/access/authentication_test.go    2026-07-19
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
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const testCredentialID = "019f5c36-b326-7c52-9325-ec59f95c8fae"

type fakeCredentialLookup struct {
	candidate CredentialCandidate
	err       error
	calls     int
	scope     project.Scope
	selector  CredentialSelector
}

func (lookup *fakeCredentialLookup) LookupCredential(
	_ context.Context,
	scope project.Scope,
	selector CredentialSelector,
) (CredentialCandidate, error) {
	lookup.calls++
	lookup.scope = scope
	lookup.selector = selector
	return lookup.candidate, lookup.err
}

func TestCredentialAuthenticatorVerifiesIssuedTokenAndReturnsOpaqueEvidence(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	raw := issuedToken(id, make([]byte, CredentialSecretBytes))
	lookup := &fakeCredentialLookup{candidate: CredentialCandidate{
		Credential:   mustCredential(t, scope, id, domainaccess.CredentialStatusActive),
		SecretDigest: SecretDigest(sha256.Sum256([]byte(raw))),
	}}
	authenticator, err := NewCredentialAuthenticator(lookup)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.Authenticate(context.Background(), scope, raw)
	if err != nil {
		t.Fatal(err)
	}
	selectedID, ok := lookup.selector.CredentialID()
	if !ok || selectedID != id || lookup.selector.Bootstrap() {
		t.Fatalf("selector = id %q, bootstrap %v", selectedID.String(), lookup.selector.Bootstrap())
	}
	if !principal.Valid() || principal.Actor().ActorID() != "svc:"+id.String() ||
		principal.CredentialID() != id || !sameScope(principal.Scope(), scope) {
		t.Fatalf("unexpected authenticated principal evidence")
	}
	execution, err := NewAdminExecution(scope, principal)
	if err != nil || execution.CredentialID() != id {
		t.Fatalf("NewAdminExecution() = %+v, %v", execution, err)
	}
}

func TestAdminExecutionRejectsCredentialEvidenceFromAnotherEnvironment(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	raw := issuedToken(id, make([]byte, CredentialSecretBytes))
	lookup := &fakeCredentialLookup{candidate: CredentialCandidate{
		Credential:   mustCredential(t, scope, id, domainaccess.CredentialStatusActive),
		SecretDigest: SecretDigest(sha256.Sum256([]byte(raw))),
	}}
	authenticator, err := NewCredentialAuthenticator(lookup)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.Authenticate(context.Background(), scope, raw)
	if err != nil {
		t.Fatal(err)
	}

	otherScope := testScope(t, testProjectID, "019f5c36-b32a-7c52-9325-ec59f95c8fae")
	if _, err := NewAdminExecution(otherScope, principal); !errors.Is(err, ErrInvalid) {
		t.Fatalf("NewAdminExecution() error = %v, want ErrInvalid", err)
	}
}

func TestCredentialAuthenticatorSupportsMarkerSelectedBootstrapToken(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	raw := "legacy-bootstrap-token-with-at-least-thirty-two-bytes"
	lookup := &fakeCredentialLookup{candidate: CredentialCandidate{
		Credential:   mustBootstrapCredential(t, scope, id, domainaccess.CredentialStatusActive),
		SecretDigest: SecretDigest(sha256.Sum256([]byte(raw))),
	}}
	authenticator, err := NewCredentialAuthenticator(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(context.Background(), scope, raw); err != nil {
		t.Fatal(err)
	}
	if !lookup.selector.Bootstrap() {
		t.Fatal("legacy token did not select bootstrap marker")
	}
}

func TestCredentialAuthenticatorFailsClosed(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	raw := issuedToken(id, make([]byte, CredentialSecretBytes))
	active := mustCredential(t, scope, id, domainaccess.CredentialStatusActive)
	revoked, err := active.Revoke("bootstrap-admin", active.IssuedAt().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		raw    string
		lookup *fakeCredentialLookup
		want   error
	}{
		{
			name: "digest mismatch", raw: raw,
			lookup: &fakeCredentialLookup{candidate: CredentialCandidate{Credential: active}},
			want:   ErrUnauthenticated,
		},
		{
			name: "revoked", raw: raw,
			lookup: &fakeCredentialLookup{candidate: CredentialCandidate{
				Credential: revoked, SecretDigest: SecretDigest(sha256.Sum256([]byte(raw))),
			}},
			want: ErrUnauthenticated,
		},
		{
			name: "not found", raw: raw,
			lookup: &fakeCredentialLookup{err: ErrNotFound}, want: ErrUnauthenticated,
		},
		{
			name: "store unavailable", raw: raw,
			lookup: &fakeCredentialLookup{err: errors.New("offline")}, want: ErrUnavailable,
		},
		{
			name: "whitespace", raw: raw + "\n",
			lookup: &fakeCredentialLookup{}, want: ErrUnauthenticated,
		},
		{
			name: "comma", raw: strings.Repeat("a", MinimumBootstrapTokenBytes) + ",",
			lookup: &fakeCredentialLookup{}, want: ErrUnauthenticated,
		},
		{
			name: "control", raw: strings.Repeat("a", MinimumBootstrapTokenBytes) + "\x7f",
			lookup: &fakeCredentialLookup{}, want: ErrUnauthenticated,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authenticator, err := NewCredentialAuthenticator(test.lookup)
			if err != nil {
				t.Fatal(err)
			}
			_, err = authenticator.Authenticate(context.Background(), scope, test.raw)
			if !errors.Is(err, test.want) {
				t.Fatalf("Authenticate() error = %v, want %v", err, test.want)
			}
			if (test.name == "whitespace" || test.name == "comma" || test.name == "control") &&
				test.lookup.calls != 0 {
				t.Fatalf("malformed token caused %d lookup calls", test.lookup.calls)
			}
		})
	}
}

func issuedToken(id domainaccess.ID, secret []byte) string {
	return CredentialTokenPrefix + "." + id.String() + "." +
		base64.RawURLEncoding.EncodeToString(secret)
}

func mustCredential(
	t *testing.T,
	scope project.Scope,
	id domainaccess.ID,
	status domainaccess.CredentialStatus,
) domainaccess.Credential {
	t.Helper()
	issuedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	material := domainaccess.CredentialMaterial{
		Scope: scope, ID: id, PrincipalID: "svc:" + id.String(), Label: "deploy",
		Hint: "sha256:012345abcdef", Status: status,
		IssuedBy: "bootstrap-admin", IssuedAt: issuedAt,
	}
	if status == domainaccess.CredentialStatusRevoked {
		revokedAt := issuedAt.Add(time.Minute)
		material.RevokedBy = "bootstrap-admin"
		material.RevokedAt = &revokedAt
	}
	credential, err := domainaccess.NewCredential(material)
	if err != nil {
		t.Fatal(err)
	}
	return credential
}

func mustBootstrapCredential(
	t *testing.T,
	scope project.Scope,
	id domainaccess.ID,
	status domainaccess.CredentialStatus,
) domainaccess.Credential {
	t.Helper()
	issuedAt := time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)
	material := domainaccess.CredentialMaterial{
		Scope: scope, ID: id, PrincipalID: "bootstrap-admin", Label: "bootstrap administration",
		Hint: "sha256:012345abcdef", Status: status,
		IssuedBy: "bootstrap-admin", IssuedAt: issuedAt,
	}
	if status == domainaccess.CredentialStatusRevoked {
		revokedAt := issuedAt.Add(time.Minute)
		material.RevokedBy = "bootstrap-admin"
		material.RevokedAt = &revokedAt
	}
	credential, err := domainaccess.NewCredential(material)
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
