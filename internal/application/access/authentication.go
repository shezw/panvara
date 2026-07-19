/*
   Panvara
   internal/application/access/authentication.go    2026-07-19
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
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	// CredentialTokenPrefix identifies a Panvara API key.
	CredentialTokenPrefix = "pvk1"
	// CredentialSecretBytes is the cryptographic entropy required by issued keys.
	CredentialSecretBytes = 32
	// MinimumBootstrapTokenBytes preserves the existing bootstrap-token floor.
	MinimumBootstrapTokenBytes = 32
	// MaxBearerTokenBytes prevents unbounded authentication input.
	MaxBearerTokenBytes = 1024
)

// CredentialSelector selects either an encoded credential ID or the one
// bootstrap credential marker inside an exact project/environment scope.
type CredentialSelector struct {
	id        domainaccess.ID
	bootstrap bool
}

// CredentialID returns the encoded credential identity and whether it exists.
func (selector CredentialSelector) CredentialID() (domainaccess.ID, bool) {
	return selector.id, selector.id.Valid()
}

// Bootstrap reports whether the legacy bootstrap marker must be resolved.
func (selector CredentialSelector) Bootstrap() bool { return selector.bootstrap }

// CredentialCandidate contains authoritative metadata and its non-reversible
// digest. Infrastructure adapters construct candidates; transport adapters do not.
type CredentialCandidate struct {
	Credential   domainaccess.Credential
	SecretDigest SecretDigest
}

// CredentialLookup resolves one candidate in an exact execution scope.
type CredentialLookup interface {
	LookupCredential(context.Context, project.Scope, CredentialSelector) (CredentialCandidate, error)
}

// PrincipalAuthenticator authenticates one raw Bearer value in an exact scope.
type PrincipalAuthenticator interface {
	Authenticate(context.Context, project.Scope, string) (AuthenticatedPrincipal, error)
}

// CredentialAuthenticator verifies bounded Bearer tokens against stored digests.
type CredentialAuthenticator struct {
	lookup CredentialLookup
}

// NewCredentialAuthenticator constructs a credential-backed authenticator.
func NewCredentialAuthenticator(lookup CredentialLookup) (*CredentialAuthenticator, error) {
	if lookup == nil {
		return nil, fmt.Errorf("%w: nil credential lookup", ErrInvalid)
	}
	return &CredentialAuthenticator{lookup: lookup}, nil
}

// Authenticate verifies token syntax, exact scope, lifecycle, and digest before
// returning opaque authenticated-principal evidence.
func (authenticator *CredentialAuthenticator) Authenticate(
	ctx context.Context,
	scope project.Scope,
	rawBearer string,
) (AuthenticatedPrincipal, error) {
	if ctx == nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: nil authentication context", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return AuthenticatedPrincipal{}, err
	}
	if authenticator == nil || authenticator.lookup == nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: authenticator is not initialized", ErrUnavailable)
	}
	if err := scope.Validate(); err != nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: invalid authentication scope", ErrInvalid)
	}
	selector, err := parseCredentialSelector(rawBearer)
	if err != nil {
		return AuthenticatedPrincipal{}, ErrUnauthenticated
	}
	candidate, err := authenticator.lookup.LookupCredential(ctx, scope, selector)
	if errors.Is(err, ErrNotFound) {
		return AuthenticatedPrincipal{}, ErrUnauthenticated
	}
	if err != nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: lookup credential: %w", ErrUnavailable, err)
	}
	credential := candidate.Credential
	if err := validateCredentialCandidate(scope, selector, candidate); err != nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if !credential.Active() {
		return AuthenticatedPrincipal{}, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(rawBearer))
	if subtle.ConstantTimeCompare(digest[:], candidate.SecretDigest[:]) != 1 {
		return AuthenticatedPrincipal{}, ErrUnauthenticated
	}
	subject, err := actor.New(scope.ProjectID().String(), credential.PrincipalID(), nil)
	if err != nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: stored credential principal is corrupt", ErrUnavailable)
	}
	principal, err := newAuthenticatedPrincipal(scope, subject, credential.ID())
	if err != nil {
		return AuthenticatedPrincipal{}, fmt.Errorf("%w: stored credential evidence is corrupt", ErrUnavailable)
	}
	return principal, nil
}

func parseCredentialSelector(raw string) (CredentialSelector, error) {
	if !validBearerTokenSyntax(raw) {
		return CredentialSelector{}, fmt.Errorf("invalid bearer token")
	}
	if !strings.HasPrefix(raw, CredentialTokenPrefix+".") {
		return CredentialSelector{bootstrap: true}, nil
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 || parts[0] != CredentialTokenPrefix {
		return CredentialSelector{}, fmt.Errorf("invalid credential token format")
	}
	id, err := domainaccess.ParseID(parts[1])
	if err != nil {
		return CredentialSelector{}, err
	}
	secret, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(secret) != CredentialSecretBytes ||
		base64.RawURLEncoding.EncodeToString(secret) != parts[2] {
		return CredentialSelector{}, fmt.Errorf("invalid credential token secret")
	}
	return CredentialSelector{id: id}, nil
}

func validateCredentialCandidate(
	scope project.Scope,
	selector CredentialSelector,
	candidate CredentialCandidate,
) error {
	credential := candidate.Credential
	if err := credential.Scope().Validate(); err != nil ||
		credential.ProjectID().String() != scope.ProjectID().String() ||
		credential.EnvironmentID().String() != scope.EnvironmentID().String() ||
		!credential.ID().Valid() {
		return fmt.Errorf("credential candidate scope is inconsistent")
	}
	if selectedID, ok := selector.CredentialID(); ok && selectedID.String() != credential.ID().String() {
		return fmt.Errorf("credential candidate identity is inconsistent")
	}
	return nil
}

func validBearerTokenSyntax(value string) bool {
	if len(value) < MinimumBootstrapTokenBytes || len(value) > MaxBearerTokenBytes {
		return false
	}
	seenTokenCharacter := false
	seenPadding := false
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch {
		case character >= 'A' && character <= 'Z',
			character >= 'a' && character <= 'z',
			character >= '0' && character <= '9',
			strings.ContainsRune("-._~+/", rune(character)):
			if seenPadding {
				return false
			}
			seenTokenCharacter = true
		case character == '=':
			if !seenTokenCharacter {
				return false
			}
			seenPadding = true
		default:
			return false
		}
	}
	return seenTokenCharacter
}

var _ PrincipalAuthenticator = (*CredentialAuthenticator)(nil)
