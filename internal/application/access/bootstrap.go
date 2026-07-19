/*
   Panvara
   internal/application/access/bootstrap.go    2026-07-19
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
	"fmt"
	"strings"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const bootstrapCredentialLabel = "bootstrap administration"

// BootstrapCredentialCandidate is the first-start credential proposal. It
// contains no raw token and is ignored after a registration marker exists.
type BootstrapCredentialCandidate struct {
	Credential   domainaccess.Credential
	SecretDigest SecretDigest
}

// BootstrapCredentialRegistration carries either a first-start candidate or
// an empty restart check for one exact bootstrap principal and scope.
type BootstrapCredentialRegistration struct {
	Scope       project.Scope
	PrincipalID string
	Candidate   *BootstrapCredentialCandidate
}

// BootstrapCredentialRepository owns the irreversible initialization marker.
// RegisterBootstrapCredential MUST run atomically with these semantics:
// marker absent plus nil Candidate => ErrInvalid; marker absent plus Candidate
// => insert candidate, marker, and an access.credential.bootstrap success audit
// in one transaction; marker present plus nil Candidate => return the persisted
// credential without another audit; marker present plus a different digest =>
// ErrConflict. It must never reactivate or replace a revoked credential.
type BootstrapCredentialRepository interface {
	RegisterBootstrapCredential(
		context.Context,
		BootstrapCredentialRegistration,
	) (domainaccess.Credential, error)
}

// BootstrapCredentialRegistrar converts configuration into a digest-only,
// marker-protected bootstrap registration.
type BootstrapCredentialRegistrar struct {
	repository BootstrapCredentialRepository
	ids        AccessIDGenerator
	clock      Clock
}

// NewBootstrapCredentialRegistrar constructs an injectable bootstrap registrar.
func NewBootstrapCredentialRegistrar(
	repository BootstrapCredentialRepository,
	ids AccessIDGenerator,
	clock Clock,
) (*BootstrapCredentialRegistrar, error) {
	if repository == nil || ids == nil || clock == nil {
		return nil, fmt.Errorf("%w: incomplete bootstrap credential dependencies", ErrInvalid)
	}
	return &BootstrapCredentialRegistrar{repository: repository, ids: ids, clock: clock}, nil
}

// NewDefaultBootstrapCredentialRegistrar constructs a production registrar.
func NewDefaultBootstrapCredentialRegistrar(
	repository BootstrapCredentialRepository,
) (*BootstrapCredentialRegistrar, error) {
	return NewBootstrapCredentialRegistrar(
		repository,
		domainaccess.NewDefaultIDGenerator(),
		SystemClock{},
	)
}

// Register registers the first raw bootstrap token or verifies a marker-aware
// restart. Raw token data is hashed here and never crosses the repository port.
func (registrar *BootstrapCredentialRegistrar) Register(
	ctx context.Context,
	scope project.Scope,
	principalID string,
	rawToken string,
) (domainaccess.Credential, error) {
	if registrar == nil || registrar.repository == nil || registrar.ids == nil || registrar.clock == nil {
		return domainaccess.Credential{}, fmt.Errorf("%w: bootstrap registrar is not initialized", ErrUnavailable)
	}
	if ctx == nil {
		return domainaccess.Credential{}, fmt.Errorf("%w: nil context", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return domainaccess.Credential{}, err
	}
	if err := scope.Validate(); err != nil {
		return domainaccess.Credential{}, fmt.Errorf("%w: invalid bootstrap scope", ErrInvalid)
	}
	principalID = strings.TrimSpace(principalID)
	if principalID == "" {
		return domainaccess.Credential{}, fmt.Errorf("%w: bootstrap principal is required", ErrInvalid)
	}

	registration := BootstrapCredentialRegistration{Scope: scope, PrincipalID: principalID}
	if rawToken != "" {
		if !validBearerTokenSyntax(rawToken) ||
			strings.HasPrefix(rawToken, CredentialTokenPrefix+".") {
			return domainaccess.Credential{}, fmt.Errorf("%w: invalid bootstrap token", ErrInvalid)
		}
		id, err := registrar.ids.New()
		if err != nil {
			return domainaccess.Credential{}, fmt.Errorf("%w: generate bootstrap credential id: %v", ErrUnavailable, err)
		}
		digest := SecretDigest(sha256.Sum256([]byte(rawToken)))
		credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
			Scope: scope, ID: id, PrincipalID: principalID,
			Label: bootstrapCredentialLabel, Hint: credentialHint(digest),
			Status:   domainaccess.CredentialStatusActive,
			IssuedBy: principalID, IssuedAt: registrar.clock.Now(),
		})
		if err != nil {
			return domainaccess.Credential{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		registration.Candidate = &BootstrapCredentialCandidate{
			Credential: credential, SecretDigest: digest,
		}
	}

	credential, err := registrar.repository.RegisterBootstrapCredential(ctx, registration)
	if err != nil {
		return domainaccess.Credential{}, normalizeRepositoryError("register bootstrap credential", err)
	}
	return credential, nil
}
