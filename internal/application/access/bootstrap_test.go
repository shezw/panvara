/*
   Panvara
   internal/application/access/bootstrap_test.go    2026-07-19
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
	"errors"
	"testing"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
)

type markerBootstrapRepository struct {
	marked     bool
	digest     SecretDigest
	credential domainaccess.Credential
	calls      int
	audits     int
	last       BootstrapCredentialRegistration
}

func (repository *markerBootstrapRepository) RegisterBootstrapCredential(
	_ context.Context,
	registration BootstrapCredentialRegistration,
) (domainaccess.Credential, error) {
	repository.calls++
	repository.last = registration
	if !repository.marked {
		if registration.Candidate == nil {
			return domainaccess.Credential{}, ErrInvalid
		}
		repository.marked = true
		repository.digest = registration.Candidate.SecretDigest
		repository.credential = registration.Candidate.Credential
		repository.audits++
		return repository.credential, nil
	}
	if registration.Candidate != nil && registration.Candidate.SecretDigest != repository.digest {
		return domainaccess.Credential{}, ErrConflict
	}
	return repository.credential, nil
}

func TestBootstrapRegistrarEnforcesMarkerInitializationAndDrift(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	repository := &markerBootstrapRepository{}
	registrar, err := NewBootstrapCredentialRegistrar(
		repository,
		&fixedAccessIDGenerator{id: id},
		fixedAccessClock{at: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)},
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registrar.Register(
		context.Background(), scope, "bootstrap-admin", "",
	); !errors.Is(err, ErrInvalid) {
		t.Fatalf("marker-absent empty Register() error = %v, want ErrInvalid", err)
	}
	token := "bootstrap-secret-value-with-at-least-thirty-two-bytes"
	credential, err := registrar.Register(
		context.Background(), scope, "bootstrap-admin", token,
	)
	if err != nil {
		t.Fatal(err)
	}
	if repository.last.Candidate == nil || repository.last.Candidate.Credential.ID() != id ||
		credential.Hint() != credentialHint(repository.digest) || repository.audits != 1 {
		t.Fatalf("first registration did not persist candidate, marker, and one audit")
	}

	restarted, err := registrar.Register(context.Background(), scope, "bootstrap-admin", "")
	if err != nil || restarted.ID() != credential.ID() || repository.audits != 1 ||
		repository.last.Candidate != nil {
		t.Fatalf("marker restart = %q, %v, audits=%d, candidate=%v",
			restarted.ID().String(), err, repository.audits, repository.last.Candidate)
	}
	if _, err := registrar.Register(
		context.Background(), scope, "bootstrap-admin",
		"different-bootstrap-secret-with-at-least-thirty-two-bytes",
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("drift Register() error = %v, want ErrConflict", err)
	}
}

func TestBootstrapRegistrarNeverRestoresRevokedCredential(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	id := mustAccessID(t, testCredentialID)
	repository := &markerBootstrapRepository{}
	clock := fixedAccessClock{at: time.Date(2026, 7, 19, 8, 0, 0, 0, time.UTC)}
	registrar, err := NewBootstrapCredentialRegistrar(
		repository, &fixedAccessIDGenerator{id: id}, clock,
	)
	if err != nil {
		t.Fatal(err)
	}
	token := "bootstrap-secret-value-with-at-least-thirty-two-bytes"
	credential, err := registrar.Register(context.Background(), scope, "bootstrap-admin", token)
	if err != nil {
		t.Fatal(err)
	}
	repository.credential, err = credential.Revoke("bootstrap-admin", clock.at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	for _, restartToken := range []string{"", token} {
		persisted, registerErr := registrar.Register(
			context.Background(), scope, "bootstrap-admin", restartToken,
		)
		if registerErr != nil {
			t.Fatal(registerErr)
		}
		if persisted.Active() || persisted.Status() != domainaccess.CredentialStatusRevoked {
			t.Fatal("bootstrap restart restored a revoked credential")
		}
	}
	if repository.audits != 1 {
		t.Fatalf("bootstrap restarts appended %d audits, want 1 total", repository.audits)
	}
}

func TestBootstrapRegistrarRejectsMalformedRawTokenBeforeRepository(t *testing.T) {
	scope := testScope(t, testProjectID, testEnvironmentID)
	repository := &markerBootstrapRepository{}
	registrar, err := NewBootstrapCredentialRegistrar(
		repository,
		&fixedAccessIDGenerator{id: mustAccessID(t, testCredentialID)},
		fixedAccessClock{at: time.Now().UTC()},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"token with whitespace and sufficient length here",
		"bootstrap-token-with-comma,not-http-safe",
		"bootstrap-token-with-control-\x7f-not-safe",
		"bootstrap-token-padding=followed-by-data",
		"pvk1.01981234-5678-7abc-8def-0123456789ef.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if _, err := registrar.Register(
			context.Background(), scope, "bootstrap-admin", token,
		); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Register() error = %v, want ErrInvalid", err)
		}
	}
	if repository.calls != 0 {
		t.Fatalf("malformed bootstrap token caused %d repository calls", repository.calls)
	}
}
