/*
   Panvara
   internal/domain/access/grant.go    2026-07-19
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
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

// RoleProjectOwner is the only grant role supported by the P0-01b slice.
const RoleProjectOwner = "project.owner"

// OwnerGrantMaterial restores one exact project/environment owner grant view.
type OwnerGrantMaterial struct {
	Scope       project.Scope
	PrincipalID string
	GrantedBy   string
	GrantedAt   time.Time
	RevokedBy   string
	RevokedAt   *time.Time
}

// OwnerGrant is an immutable view of one exact project-owner grant.
type OwnerGrant struct {
	scope       project.Scope
	principalID string
	grantedBy   string
	grantedAt   time.Time
	revokedBy   string
	revokedAt   *time.Time
}

// NewOwnerGrant validates and restores a project-owner grant view.
func NewOwnerGrant(material OwnerGrantMaterial) (OwnerGrant, error) {
	if err := material.Scope.Validate(); err != nil {
		return OwnerGrant{}, fmt.Errorf("owner grant scope is invalid: %w", err)
	}
	if !bootstrapPrincipalIDPattern.MatchString(material.PrincipalID) ||
		!bootstrapPrincipalIDPattern.MatchString(material.GrantedBy) {
		return OwnerGrant{}, fmt.Errorf("owner grant principal identity is invalid")
	}
	grantedAt := material.GrantedAt.UTC()
	if grantedAt.IsZero() {
		return OwnerGrant{}, fmt.Errorf("owner grant timestamp is required")
	}
	revokedAt := cloneTime(material.RevokedAt)
	if revokedAt == nil && material.RevokedBy != "" {
		return OwnerGrant{}, fmt.Errorf("active owner grant cannot have a revoker")
	}
	if revokedAt != nil && (revokedAt.Before(grantedAt) ||
		!bootstrapPrincipalIDPattern.MatchString(material.RevokedBy)) {
		return OwnerGrant{}, fmt.Errorf("revoked owner grant facts are invalid")
	}
	return OwnerGrant{
		scope: material.Scope, principalID: material.PrincipalID,
		grantedBy: material.GrantedBy, grantedAt: grantedAt,
		revokedBy: material.RevokedBy, revokedAt: revokedAt,
	}, nil
}

// Scope returns the exact project/environment grant boundary.
func (grant OwnerGrant) Scope() project.Scope { return grant.scope }

// PrincipalID returns the granted project-local principal.
func (grant OwnerGrant) PrincipalID() string { return grant.principalID }

// Role returns project.owner.
func (grant OwnerGrant) Role() string { return RoleProjectOwner }

// GrantedBy returns the principal that issued the grant.
func (grant OwnerGrant) GrantedBy() string { return grant.grantedBy }

// GrantedAt returns the grant timestamp.
func (grant OwnerGrant) GrantedAt() time.Time { return grant.grantedAt }

// RevokedBy returns the principal that revoked the grant.
func (grant OwnerGrant) RevokedBy() string { return grant.revokedBy }

// RevokedAt returns a defensive copy of the optional revocation timestamp.
func (grant OwnerGrant) RevokedAt() *time.Time { return cloneTime(grant.revokedAt) }

// Active reports whether the owner grant has not been revoked.
func (grant OwnerGrant) Active() bool { return grant.revokedAt == nil }

// Revoke records an explicit revocation. A later authorized grant command may
// create a new active grant view while append-only audit retains both actions.
func (grant OwnerGrant) Revoke(principalID string, at time.Time) (OwnerGrant, error) {
	if !grant.Active() {
		return OwnerGrant{}, fmt.Errorf("owner grant is already revoked")
	}
	if !bootstrapPrincipalIDPattern.MatchString(principalID) {
		return OwnerGrant{}, fmt.Errorf("owner grant revoker is invalid")
	}
	at = at.UTC()
	if at.Before(grant.grantedAt) {
		return OwnerGrant{}, fmt.Errorf("owner grant revocation timestamp is invalid")
	}
	grant.revokedBy = principalID
	grant.revokedAt = &at
	return grant, nil
}
