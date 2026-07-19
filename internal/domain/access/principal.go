/*
   Panvara
   internal/domain/access/principal.go    2026-07-19
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
	// MaxPrincipalDisplayNameBytes bounds a human-readable principal name.
	MaxPrincipalDisplayNameBytes = 128
)

var bootstrapPrincipalIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// PrincipalKind identifies a project-local principal category.
type PrincipalKind string

const (
	// PrincipalKindBootstrap is the one bootstrap administration principal.
	PrincipalKindBootstrap PrincipalKind = "bootstrap"
	// PrincipalKindService is a managed service principal.
	PrincipalKindService PrincipalKind = "service"
)

// Valid reports whether the principal kind is supported.
func (kind PrincipalKind) Valid() bool {
	return kind == PrincipalKindBootstrap || kind == PrincipalKindService
}

// PrincipalStatus is the terminal lifecycle state of a project principal.
type PrincipalStatus string

const (
	// PrincipalStatusActive permits credentials and grants to be evaluated.
	PrincipalStatusActive PrincipalStatus = "active"
	// PrincipalStatusDisabled is terminal and cannot be reactivated.
	PrincipalStatusDisabled PrincipalStatus = "disabled"
)

// Valid reports whether the principal status is supported.
func (status PrincipalStatus) Valid() bool {
	return status == PrincipalStatusActive || status == PrincipalStatusDisabled
}

// PrincipalMaterial restores a project-local principal from authoritative facts.
type PrincipalMaterial struct {
	ProjectID   project.ID
	ID          string
	Kind        PrincipalKind
	DisplayName string
	Status      PrincipalStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DisabledAt  *time.Time
}

// Principal is an immutable project-local identity view.
type Principal struct {
	projectID   project.ID
	id          string
	kind        PrincipalKind
	displayName string
	status      PrincipalStatus
	createdAt   time.Time
	updatedAt   time.Time
	disabledAt  *time.Time
}

// NewPrincipal validates and restores a principal view.
func NewPrincipal(material PrincipalMaterial) (Principal, error) {
	displayName, err := normalizeDisplayName(material.DisplayName)
	if err != nil {
		return Principal{}, err
	}
	if !material.ProjectID.Valid() {
		return Principal{}, fmt.Errorf("principal project id is invalid")
	}
	if !material.Kind.Valid() {
		return Principal{}, fmt.Errorf("principal kind %q is invalid", material.Kind)
	}
	if !material.Status.Valid() {
		return Principal{}, fmt.Errorf("principal status %q is invalid", material.Status)
	}
	if err := validatePrincipalID(material.ID, material.Kind); err != nil {
		return Principal{}, err
	}
	createdAt := material.CreatedAt.UTC()
	updatedAt := material.UpdatedAt.UTC()
	if createdAt.IsZero() || updatedAt.Before(createdAt) {
		return Principal{}, fmt.Errorf("principal timestamps are invalid")
	}
	disabledAt := cloneTime(material.DisabledAt)
	if material.Status == PrincipalStatusActive && disabledAt != nil {
		return Principal{}, fmt.Errorf("active principal cannot have a disabled timestamp")
	}
	if material.Status == PrincipalStatusDisabled {
		if disabledAt == nil || disabledAt.Before(createdAt) || updatedAt.Before(*disabledAt) {
			return Principal{}, fmt.Errorf("disabled principal timestamps are invalid")
		}
	}
	return Principal{
		projectID: material.ProjectID, id: material.ID, kind: material.Kind,
		displayName: displayName, status: material.Status,
		createdAt: createdAt, updatedAt: updatedAt, disabledAt: disabledAt,
	}, nil
}

// NewServicePrincipal creates an active service principal with ID svc:<uuidv7>.
func NewServicePrincipal(
	projectID project.ID,
	id ID,
	displayName string,
	at time.Time,
) (Principal, error) {
	if !id.Valid() {
		return Principal{}, fmt.Errorf("service principal access id is invalid")
	}
	at = at.UTC()
	return NewPrincipal(PrincipalMaterial{
		ProjectID: projectID, ID: "svc:" + id.String(), Kind: PrincipalKindService,
		DisplayName: displayName, Status: PrincipalStatusActive,
		CreatedAt: at, UpdatedAt: at,
	})
}

// ProjectID returns the exact project boundary.
func (principal Principal) ProjectID() project.ID { return principal.projectID }

// ID returns the stable project-local principal identifier.
func (principal Principal) ID() string { return principal.id }

// Kind returns the bootstrap or service category.
func (principal Principal) Kind() PrincipalKind { return principal.kind }

// DisplayName returns the human-readable display name.
func (principal Principal) DisplayName() string { return principal.displayName }

// Status returns the active or terminal disabled state.
func (principal Principal) Status() PrincipalStatus { return principal.status }

// CreatedAt returns the creation timestamp.
func (principal Principal) CreatedAt() time.Time { return principal.createdAt }

// UpdatedAt returns the last lifecycle update timestamp.
func (principal Principal) UpdatedAt() time.Time { return principal.updatedAt }

// DisabledAt returns a defensive copy of the terminal disable timestamp.
func (principal Principal) DisabledAt() *time.Time { return cloneTime(principal.disabledAt) }

// Active reports whether the principal may participate in authentication.
func (principal Principal) Active() bool { return principal.status == PrincipalStatusActive }

// Disable transitions an active principal into its terminal disabled state.
func (principal Principal) Disable(at time.Time) (Principal, error) {
	if !principal.Active() {
		return Principal{}, fmt.Errorf("principal is already disabled")
	}
	at = at.UTC()
	if at.Before(principal.createdAt) || at.Before(principal.updatedAt) {
		return Principal{}, fmt.Errorf("principal disable timestamp is invalid")
	}
	principal.status = PrincipalStatusDisabled
	principal.updatedAt = at
	principal.disabledAt = &at
	return principal, nil
}

func validatePrincipalID(value string, kind PrincipalKind) error {
	if strings.TrimSpace(value) != value || value == "" {
		return fmt.Errorf("principal id is invalid")
	}
	switch kind {
	case PrincipalKindBootstrap:
		// 0004 allowed any project-local principal ID matching the shared
		// pattern. Kind is authoritative after migration, so bootstrap must
		// continue to restore legacy values such as "svc:legacy" unchanged.
		if !bootstrapPrincipalIDPattern.MatchString(value) {
			return fmt.Errorf("bootstrap principal id %q is invalid", value)
		}
	case PrincipalKindService:
		if !strings.HasPrefix(value, "svc:") {
			return fmt.Errorf("service principal id %q is invalid", value)
		}
		if _, err := ParseID(strings.TrimPrefix(value, "svc:")); err != nil {
			return fmt.Errorf("service principal id %q is invalid", value)
		}
	default:
		return fmt.Errorf("principal kind %q is invalid", kind)
	}
	return nil
}

func normalizeDisplayName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > MaxPrincipalDisplayNameBytes || !utf8.ValidString(value) {
		return "", fmt.Errorf("principal display name is invalid")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("principal display name contains control characters")
		}
	}
	return value, nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.UTC()
	return &result
}
