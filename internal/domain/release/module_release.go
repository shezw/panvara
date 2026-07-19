/*
   Panvara
   internal/domain/release/module_release.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package release

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const maxDraftGeneration uint64 = 1<<63 - 1

var provenancePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// Outcome is the reviewed plan classification retained by a publication fact.
type Outcome string

const (
	// OutcomeCompatible permits publishing a plan classified as compatible.
	OutcomeCompatible Outcome = "compatible"
	// OutcomeReviewRequired permits publishing after an owner accepts review risk.
	OutcomeReviewRequired Outcome = "review_required"
	// OutcomeMigrationRequired records that later activation requires migration.
	OutcomeMigrationRequired Outcome = "migration_required"
)

// Valid reports whether the outcome is publishable in this release contract.
func (outcome Outcome) Valid() bool {
	switch outcome {
	case OutcomeCompatible, OutcomeReviewRequired, OutcomeMigrationRequired:
		return true
	default:
		return false
	}
}

// ModuleReleaseMaterial contains every immutable fact bound by publication.
// It deliberately has no active pointer, epoch, migration, or runtime state.
type ModuleReleaseMaterial struct {
	ID                    ID
	Scope                 project.Scope
	ModuleName            string
	DraftID               appmodule.DraftID
	DraftGeneration       uint64
	ValidationID          string
	PlanID                string
	PlanHash              string
	BaselineRevision      string
	CandidateRevision     string
	DataSchemaFormat      int
	DataSchemaFingerprint string
	SourceHash            string
	Outcome               Outcome
	Risk                  string
	PublishedBy           string
	PublishedCredentialID domainaccess.ID
	RequestID             string
	PublishedAt           time.Time
}

// ModuleRelease is an immutable environment-scoped publication fact.
type ModuleRelease struct{ material ModuleReleaseMaterial }

// NewModuleRelease validates and constructs an immutable publication fact.
func NewModuleRelease(material ModuleReleaseMaterial) (ModuleRelease, error) {
	material.ModuleName = strings.TrimSpace(material.ModuleName)
	material.ValidationID = strings.ToLower(strings.TrimSpace(material.ValidationID))
	material.PlanID = strings.ToLower(strings.TrimSpace(material.PlanID))
	material.PlanHash = strings.ToLower(strings.TrimSpace(material.PlanHash))
	material.BaselineRevision = strings.ToLower(strings.TrimSpace(material.BaselineRevision))
	material.CandidateRevision = strings.ToLower(strings.TrimSpace(material.CandidateRevision))
	material.DataSchemaFingerprint = strings.ToLower(strings.TrimSpace(material.DataSchemaFingerprint))
	material.SourceHash = strings.ToLower(strings.TrimSpace(material.SourceHash))
	material.Risk = strings.TrimSpace(material.Risk)
	material.PublishedBy = strings.TrimSpace(material.PublishedBy)
	material.RequestID = strings.TrimSpace(material.RequestID)
	material.PublishedAt = material.PublishedAt.UTC()

	if !material.ID.Valid() || material.Scope.Validate() != nil || !appmodule.ValidModuleName(material.ModuleName) ||
		!material.DraftID.Valid() || material.DraftGeneration == 0 || material.DraftGeneration > maxDraftGeneration {
		return ModuleRelease{}, fmt.Errorf("module release identity or scope is invalid")
	}
	if !appmodule.ValidContentHash(material.ValidationID) || !appmodule.ValidContentHash(material.PlanID) ||
		!appmodule.ValidContentHash(material.PlanHash) || !appmodule.ValidContentHash(material.CandidateRevision) ||
		!appmodule.ValidContentHash(material.DataSchemaFingerprint) || !appmodule.ValidContentHash(material.SourceHash) {
		return ModuleRelease{}, fmt.Errorf("module release content identity is invalid")
	}
	if material.BaselineRevision != "" && !appmodule.ValidContentHash(material.BaselineRevision) {
		return ModuleRelease{}, fmt.Errorf("module release baseline identity is invalid")
	}
	if material.DataSchemaFormat <= 0 || !validClassification(material.Outcome, material.Risk) {
		return ModuleRelease{}, fmt.Errorf("module release plan classification is invalid")
	}
	if !provenancePattern.MatchString(material.PublishedBy) || !material.PublishedCredentialID.Valid() ||
		!provenancePattern.MatchString(material.RequestID) || material.PublishedAt.IsZero() {
		return ModuleRelease{}, fmt.Errorf("module release publication provenance is invalid")
	}
	return ModuleRelease{material: material}, nil
}

// Validate rechecks every persisted identity, classification, and provenance field.
func (release ModuleRelease) Validate() error {
	_, err := NewModuleRelease(release.material)
	return err
}

// ID returns the UUIDv7 release identity.
func (release ModuleRelease) ID() ID { return release.material.ID }

// Scope returns the exact project/environment publication boundary.
func (release ModuleRelease) Scope() project.Scope { return release.material.Scope }

// ModuleName returns the canonical module route identity.
func (release ModuleRelease) ModuleName() string { return release.material.ModuleName }

// DraftID returns the source draft identity.
func (release ModuleRelease) DraftID() appmodule.DraftID { return release.material.DraftID }

// DraftGeneration returns the exact source generation that was published.
func (release ModuleRelease) DraftGeneration() uint64 { return release.material.DraftGeneration }

// ValidationID returns the exact validation snapshot identity.
func (release ModuleRelease) ValidationID() string { return release.material.ValidationID }

// PlanID returns the exact change-plan snapshot identity.
func (release ModuleRelease) PlanID() string { return release.material.PlanID }

// PlanHash returns the deterministic semantic plan identity.
func (release ModuleRelease) PlanHash() string { return release.material.PlanHash }

// BaselineRevision returns the exact baseline revision or empty for none.
func (release ModuleRelease) BaselineRevision() string { return release.material.BaselineRevision }

// CandidateRevision returns the immutable candidate revision identity.
func (release ModuleRelease) CandidateRevision() string { return release.material.CandidateRevision }

// DataSchemaFormat returns the candidate data-schema projection format.
func (release ModuleRelease) DataSchemaFormat() int { return release.material.DataSchemaFormat }

// DataSchemaFingerprint returns the candidate persistence-affecting identity.
func (release ModuleRelease) DataSchemaFingerprint() string {
	return release.material.DataSchemaFingerprint
}

// SourceHash returns the exact authoring source identity.
func (release ModuleRelease) SourceHash() string { return release.material.SourceHash }

// Outcome returns the accepted plan outcome.
func (release ModuleRelease) Outcome() Outcome { return release.material.Outcome }

// Risk returns the accepted plan risk classification.
func (release ModuleRelease) Risk() string { return release.material.Risk }

// PublishedBy returns the project-local actor that published the plan.
func (release ModuleRelease) PublishedBy() string { return release.material.PublishedBy }

// PublishedCredentialID returns the exact credential used for publication.
func (release ModuleRelease) PublishedCredentialID() domainaccess.ID {
	return release.material.PublishedCredentialID
}

// RequestID returns the transport correlation identity recorded at publication.
func (release ModuleRelease) RequestID() string { return release.material.RequestID }

// PublishedAt returns the UTC publication time.
func (release ModuleRelease) PublishedAt() time.Time { return release.material.PublishedAt }

// SamePublishIntent reports whether two facts bind the same scoped plan and candidate.
// Publication identity and provenance are intentionally excluded for idempotent replay.
func (release ModuleRelease) SamePublishIntent(other ModuleRelease) bool {
	return sameScope(release.Scope(), other.Scope()) && release.ModuleName() == other.ModuleName() &&
		release.DraftID().String() == other.DraftID().String() &&
		release.DraftGeneration() == other.DraftGeneration() && release.ValidationID() == other.ValidationID() &&
		release.PlanID() == other.PlanID() && release.PlanHash() == other.PlanHash() &&
		release.BaselineRevision() == other.BaselineRevision() &&
		release.CandidateRevision() == other.CandidateRevision() &&
		release.DataSchemaFormat() == other.DataSchemaFormat() &&
		release.DataSchemaFingerprint() == other.DataSchemaFingerprint() &&
		release.SourceHash() == other.SourceHash() && release.Outcome() == other.Outcome() &&
		release.Risk() == other.Risk()
}

func validClassification(outcome Outcome, risk string) bool {
	switch outcome {
	case OutcomeCompatible:
		return risk == "none" || risk == "low"
	case OutcomeReviewRequired:
		return risk == "medium"
	case OutcomeMigrationRequired:
		return risk == "high"
	default:
		return false
	}
}

func sameScope(left, right project.Scope) bool {
	return left.ProjectID().String() == right.ProjectID().String() &&
		left.EnvironmentID().String() == right.EnvironmentID().String()
}
