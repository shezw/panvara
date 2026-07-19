/*
   Panvara
   internal/domain/access/credential.go    2026-07-19
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
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/shezw/panvara/internal/domain/project"
)

const (
	// MaxCredentialLabelBytes bounds a credential's operator-facing label.
	MaxCredentialLabelBytes = 128
	// CredentialHintBytes is the exact byte length of a SHA-256-derived hint.
	CredentialHintBytes = 19
)

var credentialHintPattern = regexp.MustCompile(`\Asha256:[0-9a-f]{12}\z`)

// CredentialStatus is the terminal lifecycle state of an API credential.
type CredentialStatus string

const (
	// CredentialStatusActive permits secret verification.
	CredentialStatusActive CredentialStatus = "active"
	// CredentialStatusRevoked is terminal and cannot be restored.
	CredentialStatusRevoked CredentialStatus = "revoked"
)

// Valid reports whether the credential status is supported.
func (status CredentialStatus) Valid() bool {
	return status == CredentialStatusActive || status == CredentialStatusRevoked
}

// CredentialMaterial restores non-secret credential metadata.
type CredentialMaterial struct {
	Scope       project.Scope
	ID          ID
	PrincipalID string
	Label       string
	Hint        string
	Status      CredentialStatus
	IssuedBy    string
	IssuedAt    time.Time
	RevokedBy   string
	RevokedAt   *time.Time
}

// Credential is immutable non-secret API credential metadata.
type Credential struct {
	scope       project.Scope
	id          ID
	principalID string
	label       string
	hint        string
	status      CredentialStatus
	issuedBy    string
	issuedAt    time.Time
	revokedBy   string
	revokedAt   *time.Time
}

// NewCredential validates and restores credential metadata.
func NewCredential(material CredentialMaterial) (Credential, error) {
	if err := material.Scope.Validate(); err != nil || !material.ID.Valid() {
		return Credential{}, fmt.Errorf("credential identity is invalid")
	}
	if !bootstrapPrincipalIDPattern.MatchString(material.PrincipalID) ||
		!bootstrapPrincipalIDPattern.MatchString(material.IssuedBy) {
		return Credential{}, fmt.Errorf("credential principal identity is invalid")
	}
	label, err := normalizeCredentialText("label", material.Label, MaxCredentialLabelBytes)
	if err != nil {
		return Credential{}, err
	}
	if len(material.Hint) != CredentialHintBytes || !credentialHintPattern.MatchString(material.Hint) {
		return Credential{}, fmt.Errorf("credential hint is invalid")
	}
	hint := material.Hint
	if !material.Status.Valid() {
		return Credential{}, fmt.Errorf("credential status %q is invalid", material.Status)
	}
	issuedAt := material.IssuedAt.UTC()
	if issuedAt.IsZero() {
		return Credential{}, fmt.Errorf("credential issue timestamp is required")
	}
	revokedAt := cloneTime(material.RevokedAt)
	if material.Status == CredentialStatusActive && (revokedAt != nil || material.RevokedBy != "") {
		return Credential{}, fmt.Errorf("active credential cannot have revocation facts")
	}
	if material.Status == CredentialStatusRevoked {
		if revokedAt == nil || revokedAt.Before(issuedAt) ||
			!bootstrapPrincipalIDPattern.MatchString(material.RevokedBy) {
			return Credential{}, fmt.Errorf("revoked credential facts are invalid")
		}
	}
	return Credential{
		scope: material.Scope, id: material.ID, principalID: material.PrincipalID,
		label: label, hint: hint, status: material.Status, issuedBy: material.IssuedBy,
		issuedAt: issuedAt, revokedBy: material.RevokedBy, revokedAt: revokedAt,
	}, nil
}

// Scope returns the exact project/environment credential boundary.
func (credential Credential) Scope() project.Scope { return credential.scope }

// ProjectID returns the exact project boundary.
func (credential Credential) ProjectID() project.ID { return credential.scope.ProjectID() }

// EnvironmentID returns the exact environment boundary.
func (credential Credential) EnvironmentID() project.EnvironmentID {
	return credential.scope.EnvironmentID()
}

// ID returns the stable UUIDv7 credential identifier.
func (credential Credential) ID() ID { return credential.id }

// PrincipalID returns the project-local principal owning this credential.
func (credential Credential) PrincipalID() string { return credential.principalID }

// Label returns the operator-facing label.
func (credential Credential) Label() string { return credential.label }

// Hint returns the non-secret token hint.
func (credential Credential) Hint() string { return credential.hint }

// Status returns active or terminal revoked state.
func (credential Credential) Status() CredentialStatus { return credential.status }

// IssuedBy returns the principal that issued the credential.
func (credential Credential) IssuedBy() string { return credential.issuedBy }

// IssuedAt returns the issue timestamp.
func (credential Credential) IssuedAt() time.Time { return credential.issuedAt }

// RevokedBy returns the principal that revoked the credential.
func (credential Credential) RevokedBy() string { return credential.revokedBy }

// RevokedAt returns a defensive copy of the terminal revocation timestamp.
func (credential Credential) RevokedAt() *time.Time { return cloneTime(credential.revokedAt) }

// Active reports whether the credential may authenticate.
func (credential Credential) Active() bool { return credential.status == CredentialStatusActive }

// Revoke transitions an active credential into its terminal revoked state.
func (credential Credential) Revoke(principalID string, at time.Time) (Credential, error) {
	if credential.status != CredentialStatusActive {
		return Credential{}, fmt.Errorf("credential is already revoked")
	}
	if !bootstrapPrincipalIDPattern.MatchString(principalID) {
		return Credential{}, fmt.Errorf("credential revoker is invalid")
	}
	at = at.UTC()
	if at.Before(credential.issuedAt) {
		return Credential{}, fmt.Errorf("credential revocation timestamp is invalid")
	}
	credential.status = CredentialStatusRevoked
	credential.revokedBy = principalID
	credential.revokedAt = &at
	return credential, nil
}

func normalizeCredentialText(field, value string, limit int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return "", fmt.Errorf("credential %s is invalid", field)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("credential %s contains control characters", field)
		}
	}
	return value, nil
}
