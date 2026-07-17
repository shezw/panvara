/*
   Panvara
   internal/domain/appmodule/draft.go    2026-07-16
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package appmodule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shezw/panvara/internal/domain/project"
)

const (
	maxDraftSourceBytes = 1 << 20
	// MaxDraftGeneration is the largest generation representable by PostgreSQL bigint.
	MaxDraftGeneration uint64 = 1<<63 - 1
)

var draftIDPattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)
var draftActorPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// DraftID is an opaque UUIDv7 identity scoped by project and module.
type DraftID struct{ value string }

// ParseDraftID validates and normalizes a UUIDv7 draft identity.
func ParseDraftID(value string) (DraftID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !draftIDPattern.MatchString(value) {
		return DraftID{}, fmt.Errorf("invalid UUIDv7 draft id %q", value)
	}
	return DraftID{value: value}, nil
}

// String returns the normalized draft identity.
func (id DraftID) String() string { return id.value }

// Valid reports whether the ID was constructed by ParseDraftID.
func (id DraftID) Valid() bool { return draftIDPattern.MatchString(id.value) }

// DraftBaseline is either no baseline or one exact immutable revision hash.
type DraftBaseline struct{ revisionHash string }

// NewDraftBaseline constructs a baseline. An empty value explicitly means none.
func NewDraftBaseline(revisionHash string) (DraftBaseline, error) {
	revisionHash = strings.ToLower(strings.TrimSpace(revisionHash))
	if revisionHash != "" && !ValidContentHash(revisionHash) {
		return DraftBaseline{}, fmt.Errorf("draft baseline revision is invalid")
	}
	return DraftBaseline{revisionHash: revisionHash}, nil
}

// None reports whether this draft intentionally has no baseline.
func (baseline DraftBaseline) None() bool { return baseline.revisionHash == "" }

// RevisionHash returns the exact baseline hash or an empty string for none.
func (baseline DraftBaseline) RevisionHash() string { return baseline.revisionHash }

// DraftMaterial contains persisted mutable draft state.
type DraftMaterial struct {
	ID           DraftID
	ProjectID    project.ID
	ModuleName   string
	Baseline     DraftBaseline
	SourceFormat SourceFormat
	SourceHash   string
	Source       []byte
	Generation   uint64
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedBy    string
	UpdatedAt    time.Time
}

// Draft is mutable only through generation-checked replacement in application storage.
type Draft struct{ material DraftMaterial }

// NewDraft validates and defensively copies one persisted draft generation.
func NewDraft(material DraftMaterial) (Draft, error) {
	material.ModuleName = strings.TrimSpace(material.ModuleName)
	material.SourceHash = strings.ToLower(strings.TrimSpace(material.SourceHash))
	material.CreatedBy = strings.TrimSpace(material.CreatedBy)
	material.UpdatedBy = strings.TrimSpace(material.UpdatedBy)
	material.CreatedAt = material.CreatedAt.UTC()
	material.UpdatedAt = material.UpdatedAt.UTC()
	if !material.ID.Valid() || !material.ProjectID.Valid() || !ValidModuleName(material.ModuleName) {
		return Draft{}, fmt.Errorf("draft identity or scope is invalid")
	}
	if _, err := NewDraftBaseline(material.Baseline.RevisionHash()); err != nil {
		return Draft{}, err
	}
	if err := validateDraftSource(material.SourceFormat, material.SourceHash, material.Source); err != nil {
		return Draft{}, err
	}
	if material.Generation == 0 || material.Generation > MaxDraftGeneration {
		return Draft{}, fmt.Errorf("draft generation must be within 1..%d", MaxDraftGeneration)
	}
	if !validDraftActor(material.CreatedBy) || !validDraftActor(material.UpdatedBy) ||
		material.CreatedAt.IsZero() || material.UpdatedAt.IsZero() || material.UpdatedAt.Before(material.CreatedAt) {
		return Draft{}, fmt.Errorf("draft audit metadata is invalid")
	}
	material.Source = append([]byte(nil), material.Source...)
	return Draft{material: material}, nil
}

// ID returns the UUIDv7 draft identity.
func (draft Draft) ID() DraftID { return draft.material.ID }

// ProjectID returns the owning project boundary.
func (draft Draft) ProjectID() project.ID { return draft.material.ProjectID }

// ModuleName returns the fixed module routing scope.
func (draft Draft) ModuleName() string { return draft.material.ModuleName }

// Baseline returns the immutable baseline selected at creation.
func (draft Draft) Baseline() DraftBaseline { return draft.material.Baseline }

// SourceFormat returns the explicit authoring format.
func (draft Draft) SourceFormat() SourceFormat { return draft.material.SourceFormat }

// SourceHash returns the exact source content identity.
func (draft Draft) SourceHash() string { return draft.material.SourceHash }

// Source returns a defensive copy of the current raw source.
func (draft Draft) Source() []byte { return append([]byte(nil), draft.material.Source...) }

// Generation returns the positive optimistic-concurrency version.
func (draft Draft) Generation() uint64 { return draft.material.Generation }

// CreatedBy returns the creating actor identity.
func (draft Draft) CreatedBy() string { return draft.material.CreatedBy }

// CreatedAt returns the UTC creation time.
func (draft Draft) CreatedAt() time.Time { return draft.material.CreatedAt }

// UpdatedBy returns the actor that last changed the source.
func (draft Draft) UpdatedBy() string { return draft.material.UpdatedBy }

// UpdatedAt returns the UTC time of the last material source change.
func (draft Draft) UpdatedAt() time.Time { return draft.material.UpdatedAt }

// SameSource reports whether a replacement would be an idempotent no-op.
func (draft Draft) SameSource(replacement DraftReplacement) bool {
	return draft.SourceFormat() == replacement.Format() && draft.SourceHash() == replacement.SourceHash() &&
		bytes.Equal(draft.material.Source, replacement.source)
}

// DraftReplacement is a validated, bounded source replacement command.
type DraftReplacement struct {
	format     SourceFormat
	sourceHash string
	source     []byte
	actorID    string
	at         time.Time
}

// NewDraftReplacement validates source bytes and audit metadata before storage CAS.
func NewDraftReplacement(format SourceFormat, source []byte, actorID string, at time.Time) (DraftReplacement, error) {
	actorID = strings.TrimSpace(actorID)
	at = at.UTC()
	hash := draftSourceHash(source)
	if err := validateDraftSource(format, hash, source); err != nil {
		return DraftReplacement{}, err
	}
	if !validDraftActor(actorID) || at.IsZero() {
		return DraftReplacement{}, fmt.Errorf("draft replacement audit metadata is invalid")
	}
	return DraftReplacement{format: format, sourceHash: hash, source: append([]byte(nil), source...), actorID: actorID, at: at}, nil
}

// Format returns the replacement source format.
func (replacement DraftReplacement) Format() SourceFormat { return replacement.format }

// SourceHash returns the replacement source identity.
func (replacement DraftReplacement) SourceHash() string { return replacement.sourceHash }

// Source returns a defensive copy of replacement bytes.
func (replacement DraftReplacement) Source() []byte {
	return append([]byte(nil), replacement.source...)
}

// ActorID returns the replacing actor identity.
func (replacement DraftReplacement) ActorID() string { return replacement.actorID }

// At returns the replacement timestamp.
func (replacement DraftReplacement) At() time.Time { return replacement.at }

// DraftSourceHash returns the canonical SHA-256 identity for source bytes.
func DraftSourceHash(source []byte) string { return draftSourceHash(source) }

func validateDraftSource(format SourceFormat, sourceHash string, source []byte) error {
	if format != SourceFormatJSON && format != SourceFormatYAML {
		return fmt.Errorf("draft source format %q is invalid", format)
	}
	if len(source) > maxDraftSourceBytes || !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return fmt.Errorf("draft source must be NUL-free valid UTF-8 within 0..%d bytes", maxDraftSourceBytes)
	}
	if !ValidContentHash(sourceHash) || sourceHash != draftSourceHash(source) {
		return fmt.Errorf("draft source hash does not match source bytes")
	}
	return nil
}

func validDraftActor(value string) bool {
	return draftActorPattern.MatchString(value)
}

func draftSourceHash(source []byte) string {
	digest := sha256.Sum256(source)
	return "sha256:" + hex.EncodeToString(digest[:])
}
