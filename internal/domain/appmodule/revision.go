/*
   Panvara
   internal/domain/appmodule/revision.go    2026-07-15
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
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

const (
	maxRevisionSourceBytes   = 1 << 20
	maxRevisionArtifactBytes = 16 << 20
)

var contentHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// RevisionOrigin identifies the trusted workflow that first registered an
// immutable module revision.
type RevisionOrigin string

const (
	// RevisionOriginBootstrap records the module loaded by Server bootstrap.
	RevisionOriginBootstrap RevisionOrigin = "bootstrap"
)

// SourceFormat identifies the exact authoring bytes retained for provenance.
type SourceFormat string

const (
	// SourceFormatJSON identifies a JSON AppModule source.
	SourceFormatJSON SourceFormat = "json"
	// SourceFormatYAML identifies a YAML AppModule source.
	SourceFormatYAML SourceFormat = "yaml"
)

// DataSchemaIdentity identifies one versioned, deterministic projection of a
// module's persistence-affecting structure.
type DataSchemaIdentity struct {
	format      int
	fingerprint string
}

// NewDataSchemaIdentity validates and constructs an immutable projection identity.
func NewDataSchemaIdentity(format int, fingerprint string) (DataSchemaIdentity, error) {
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	if format <= 0 || !contentHashPattern.MatchString(fingerprint) {
		return DataSchemaIdentity{}, fmt.Errorf("data schema identity is invalid")
	}
	return DataSchemaIdentity{format: format, fingerprint: fingerprint}, nil
}

// Format returns the versioned projection format.
func (identity DataSchemaIdentity) Format() int { return identity.format }

// Fingerprint returns the projection's SHA-256 content identity.
func (identity DataSchemaIdentity) Fingerprint() string { return identity.fingerprint }

// RevisionMaterial contains all immutable bytes and identity metadata used to
// construct a Revision. Callers retain no ownership of byte slices after
// construction.
type RevisionMaterial struct {
	ProjectID            project.ID
	ModuleName           string
	ModuleVersion        string
	RevisionHash         string
	DataSchemaIdentities []DataSchemaIdentity
	SpecVersion          string
	IRFormat             int
	SourceFormat         SourceFormat
	SourceHash           string
	Source               []byte
	CanonicalIR          []byte
	OpenAPI              []byte
	ManagerSchema        []byte
	Origin               RevisionOrigin
	RegisteredBy         string
	RegisteredAt         time.Time
}

// Revision is an immutable, project-scoped compiled AppModule fact. It is not
// a Draft, publication state, active binding, or Record namespace.
type Revision struct {
	material RevisionMaterial
}

// NewRevision validates hashes, canonical identity, bounds, and provenance,
// then returns an immutable defensive copy.
func NewRevision(material RevisionMaterial) (Revision, error) {
	material.ModuleName = strings.TrimSpace(material.ModuleName)
	material.ModuleVersion = strings.TrimSpace(material.ModuleVersion)
	material.RevisionHash = strings.ToLower(strings.TrimSpace(material.RevisionHash))
	material.SpecVersion = strings.TrimSpace(material.SpecVersion)
	material.SourceHash = strings.ToLower(strings.TrimSpace(material.SourceHash))
	material.RegisteredBy = strings.TrimSpace(material.RegisteredBy)
	material.RegisteredAt = material.RegisteredAt.UTC()
	if !material.ProjectID.Valid() {
		return Revision{}, fmt.Errorf("revision project id is invalid")
	}
	if !validName(moduleNamePattern, material.ModuleName) {
		return Revision{}, fmt.Errorf("revision module name %q is invalid", material.ModuleName)
	}
	if len(material.ModuleVersion) > maxNameBytes || !semverPattern.MatchString(material.ModuleVersion) {
		return Revision{}, fmt.Errorf("revision module version %q is invalid", material.ModuleVersion)
	}
	if !contentHashPattern.MatchString(material.RevisionHash) {
		return Revision{}, fmt.Errorf("revision hash %q is invalid", material.RevisionHash)
	}
	identities, err := normalizeDataSchemaIdentities(material.DataSchemaIdentities)
	if err != nil {
		return Revision{}, fmt.Errorf("revision %w", err)
	}
	material.DataSchemaIdentities = identities
	if material.IRFormat <= 0 || material.SpecVersion == "" || len(material.SpecVersion) > maxNameBytes {
		return Revision{}, fmt.Errorf("revision IR or spec version is invalid")
	}
	if material.SourceFormat != SourceFormatJSON && material.SourceFormat != SourceFormatYAML {
		return Revision{}, fmt.Errorf("revision source format %q is invalid", material.SourceFormat)
	}
	if len(material.Source) == 0 || len(material.Source) > maxRevisionSourceBytes {
		return Revision{}, fmt.Errorf("revision source must contain 1..%d bytes", maxRevisionSourceBytes)
	}
	if !contentHashPattern.MatchString(material.SourceHash) || material.SourceHash != hashBytes(material.Source) {
		return Revision{}, fmt.Errorf("revision source hash does not match source bytes")
	}
	if err := validateRevisionArtifact("canonical IR", material.CanonicalIR); err != nil {
		return Revision{}, err
	}
	if err := validateRevisionArtifact("OpenAPI", material.OpenAPI); err != nil {
		return Revision{}, err
	}
	if err := validateRevisionArtifact("Manager schema", material.ManagerSchema); err != nil {
		return Revision{}, err
	}
	if material.RevisionHash != hashBytes(material.CanonicalIR) {
		return Revision{}, fmt.Errorf("revision hash does not match canonical IR bytes")
	}
	if err := validateCanonicalIdentity(material); err != nil {
		return Revision{}, err
	}
	if material.Origin != RevisionOriginBootstrap || material.RegisteredBy != "system:bootstrap" {
		return Revision{}, fmt.Errorf("revision bootstrap provenance is invalid")
	}
	if material.RegisteredAt.IsZero() {
		return Revision{}, fmt.Errorf("revision registration time is required")
	}
	material.Source = append([]byte(nil), material.Source...)
	material.CanonicalIR = append([]byte(nil), material.CanonicalIR...)
	material.OpenAPI = append([]byte(nil), material.OpenAPI...)
	material.ManagerSchema = append([]byte(nil), material.ManagerSchema...)
	return Revision{material: material}, nil
}

// ProjectID returns the owning project boundary.
func (revision Revision) ProjectID() project.ID { return revision.material.ProjectID }

// ModuleName returns the canonical module name.
func (revision Revision) ModuleName() string { return revision.material.ModuleName }

// ModuleVersion returns the author-controlled semantic version.
func (revision Revision) ModuleVersion() string { return revision.material.ModuleVersion }

// RevisionHash returns the complete canonical Module IR identity.
func (revision Revision) RevisionHash() string { return revision.material.RevisionHash }

// DataSchemaIdentities returns a format-ascending defensive copy. These
// identities are not Record namespace identifiers.
func (revision Revision) DataSchemaIdentities() []DataSchemaIdentity {
	return append([]DataSchemaIdentity(nil), revision.material.DataSchemaIdentities...)
}

// DataSchemaIdentity returns the identity for one projection format.
func (revision Revision) DataSchemaIdentity(format int) (DataSchemaIdentity, bool) {
	return findDataSchemaIdentity(revision.material.DataSchemaIdentities, format)
}

// SpecVersion returns the AppModule authoring contract version.
func (revision Revision) SpecVersion() string { return revision.material.SpecVersion }

// IRFormat returns the complete canonical IR format version.
func (revision Revision) IRFormat() int { return revision.material.IRFormat }

// SourceFormat returns the retained authoring source format.
func (revision Revision) SourceFormat() SourceFormat { return revision.material.SourceFormat }

// SourceHash returns the content hash of the retained source bytes.
func (revision Revision) SourceHash() string { return revision.material.SourceHash }

// Source returns a defensive copy of the first registered authoring source.
func (revision Revision) Source() []byte { return append([]byte(nil), revision.material.Source...) }

// CanonicalIR returns a defensive copy of the compiled canonical Module IR.
func (revision Revision) CanonicalIR() []byte {
	return append([]byte(nil), revision.material.CanonicalIR...)
}

// OpenAPI returns a defensive copy of the generated OpenAPI artifact.
func (revision Revision) OpenAPI() []byte { return append([]byte(nil), revision.material.OpenAPI...) }

// ManagerSchema returns a defensive copy of the generated Manager schema.
func (revision Revision) ManagerSchema() []byte {
	return append([]byte(nil), revision.material.ManagerSchema...)
}

// Origin returns the trusted first-registration workflow.
func (revision Revision) Origin() RevisionOrigin { return revision.material.Origin }

// RegisteredBy returns the stable actor identifier that registered the fact.
func (revision Revision) RegisteredBy() string { return revision.material.RegisteredBy }

// RegisteredAt returns the UTC first-registration time.
func (revision Revision) RegisteredAt() time.Time { return revision.material.RegisteredAt }

// SameParentArtifacts reports whether two revisions carry the same immutable
// parent fact. Source, registration provenance, and append-only data-schema
// child identities are intentionally excluded.
func (revision Revision) SameParentArtifacts(other Revision) bool {
	return revision.ProjectID().String() == other.ProjectID().String() &&
		revision.ModuleName() == other.ModuleName() &&
		revision.ModuleVersion() == other.ModuleVersion() &&
		revision.RevisionHash() == other.RevisionHash() &&
		revision.SpecVersion() == other.SpecVersion() &&
		revision.IRFormat() == other.IRFormat() &&
		bytes.Equal(revision.CanonicalIR(), other.CanonicalIR()) &&
		bytes.Equal(revision.OpenAPI(), other.OpenAPI()) &&
		bytes.Equal(revision.ManagerSchema(), other.ManagerSchema())
}

// SameArtifacts reports whether two revisions carry the same parent fact and
// the same format-ordered data-schema identities. Source and first-registration
// provenance remain intentionally excluded.
func (revision Revision) SameArtifacts(other Revision) bool {
	return revision.SameParentArtifacts(other) &&
		dataSchemaIdentitiesEqual(revision.material.DataSchemaIdentities, other.material.DataSchemaIdentities)
}

// Summary returns immutable metadata without Source or compiled artifact bytes.
func (revision Revision) Summary() RevisionSummary {
	return RevisionSummary{material: RevisionSummaryMaterial{
		ProjectID: revision.ProjectID(), ModuleName: revision.ModuleName(), ModuleVersion: revision.ModuleVersion(),
		RevisionHash: revision.RevisionHash(), DataSchemaIdentities: revision.DataSchemaIdentities(),
		SpecVersion: revision.SpecVersion(), IRFormat: revision.IRFormat(), SourceFormat: revision.SourceFormat(),
		SourceHash: revision.SourceHash(), Origin: revision.Origin(), RegisteredBy: revision.RegisteredBy(),
		RegisteredAt: revision.RegisteredAt(),
	}}
}

// RevisionSummaryMaterial contains bounded registry metadata and no large artifacts.
type RevisionSummaryMaterial struct {
	ProjectID            project.ID
	ModuleName           string
	ModuleVersion        string
	RevisionHash         string
	DataSchemaIdentities []DataSchemaIdentity
	SpecVersion          string
	IRFormat             int
	SourceFormat         SourceFormat
	SourceHash           string
	Origin               RevisionOrigin
	RegisteredBy         string
	RegisteredAt         time.Time
}

// RevisionSummary is immutable registry metadata suitable for bounded lists.
type RevisionSummary struct {
	material RevisionSummaryMaterial
}

// NewRevisionSummary validates and constructs immutable registry metadata.
func NewRevisionSummary(material RevisionSummaryMaterial) (RevisionSummary, error) {
	material.ModuleName = strings.TrimSpace(material.ModuleName)
	material.ModuleVersion = strings.TrimSpace(material.ModuleVersion)
	material.RevisionHash = strings.ToLower(strings.TrimSpace(material.RevisionHash))
	material.SpecVersion = strings.TrimSpace(material.SpecVersion)
	material.SourceHash = strings.ToLower(strings.TrimSpace(material.SourceHash))
	material.RegisteredBy = strings.TrimSpace(material.RegisteredBy)
	material.RegisteredAt = material.RegisteredAt.UTC()
	if !material.ProjectID.Valid() || !ValidModuleName(material.ModuleName) {
		return RevisionSummary{}, fmt.Errorf("revision summary project or module is invalid")
	}
	if len(material.ModuleVersion) > maxNameBytes || !semverPattern.MatchString(material.ModuleVersion) ||
		!ValidContentHash(material.RevisionHash) || !ValidContentHash(material.SourceHash) {
		return RevisionSummary{}, fmt.Errorf("revision summary content identity is invalid")
	}
	identities, err := normalizeDataSchemaIdentities(material.DataSchemaIdentities)
	if err != nil {
		return RevisionSummary{}, fmt.Errorf("revision summary %w", err)
	}
	material.DataSchemaIdentities = identities
	if material.IRFormat <= 0 || material.SpecVersion == "" || len(material.SpecVersion) > maxNameBytes ||
		(material.SourceFormat != SourceFormatJSON && material.SourceFormat != SourceFormatYAML) ||
		material.Origin != RevisionOriginBootstrap || material.RegisteredBy != "system:bootstrap" ||
		material.RegisteredAt.IsZero() {
		return RevisionSummary{}, fmt.Errorf("revision summary metadata is invalid")
	}
	return RevisionSummary{material: material}, nil
}

// ProjectID returns the owning project boundary.
func (summary RevisionSummary) ProjectID() project.ID { return summary.material.ProjectID }

// ModuleName returns the canonical module name.
func (summary RevisionSummary) ModuleName() string { return summary.material.ModuleName }

// ModuleVersion returns the author-controlled semantic version.
func (summary RevisionSummary) ModuleVersion() string { return summary.material.ModuleVersion }

// RevisionHash returns the complete canonical Module IR identity.
func (summary RevisionSummary) RevisionHash() string { return summary.material.RevisionHash }

// DataSchemaIdentities returns a format-ascending defensive copy.
func (summary RevisionSummary) DataSchemaIdentities() []DataSchemaIdentity {
	return append([]DataSchemaIdentity(nil), summary.material.DataSchemaIdentities...)
}

// SpecVersion returns the AppModule authoring contract version.
func (summary RevisionSummary) SpecVersion() string { return summary.material.SpecVersion }

// IRFormat returns the complete canonical IR format version.
func (summary RevisionSummary) IRFormat() int { return summary.material.IRFormat }

// SourceFormat returns the retained authoring source format.
func (summary RevisionSummary) SourceFormat() SourceFormat { return summary.material.SourceFormat }

// SourceHash returns the retained authoring source identity.
func (summary RevisionSummary) SourceHash() string { return summary.material.SourceHash }

// Origin returns the trusted first-registration workflow.
func (summary RevisionSummary) Origin() RevisionOrigin { return summary.material.Origin }

// RegisteredBy returns the stable actor identifier that registered the fact.
func (summary RevisionSummary) RegisteredBy() string { return summary.material.RegisteredBy }

// RegisteredAt returns the UTC first-registration time.
func (summary RevisionSummary) RegisteredAt() time.Time { return summary.material.RegisteredAt }

// ValidModuleName reports whether value follows the shared AppModule name grammar.
func ValidModuleName(value string) bool {
	return validName(moduleNamePattern, strings.TrimSpace(value))
}

// ValidContentHash reports whether value is a canonical lowercase SHA-256 identity.
func ValidContentHash(value string) bool {
	return contentHashPattern.MatchString(strings.TrimSpace(value))
}

func normalizeDataSchemaIdentities(values []DataSchemaIdentity) ([]DataSchemaIdentity, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("data schema identities are required")
	}
	result := append([]DataSchemaIdentity(nil), values...)
	for index, value := range result {
		validated, err := NewDataSchemaIdentity(value.Format(), value.Fingerprint())
		if err != nil {
			return nil, err
		}
		result[index] = validated
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Format() < result[right].Format() })
	for index := 1; index < len(result); index++ {
		if result[index-1].Format() == result[index].Format() {
			return nil, fmt.Errorf("data schema format %d is duplicated", result[index].Format())
		}
	}
	return result, nil
}

func findDataSchemaIdentity(values []DataSchemaIdentity, format int) (DataSchemaIdentity, bool) {
	index := sort.Search(len(values), func(index int) bool { return values[index].Format() >= format })
	if index >= len(values) || values[index].Format() != format {
		return DataSchemaIdentity{}, false
	}
	return values[index], true
}

func dataSchemaIdentitiesEqual(left, right []DataSchemaIdentity) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validateRevisionArtifact(label string, artifact []byte) error {
	if len(artifact) == 0 || len(artifact) > maxRevisionArtifactBytes || !json.Valid(artifact) {
		return fmt.Errorf("revision %s must be valid JSON within 1..%d bytes", label, maxRevisionArtifactBytes)
	}
	return nil
}

func validateCanonicalIdentity(material RevisionMaterial) error {
	var identity struct {
		FormatVersion int    `json:"formatVersion"`
		SpecVersion   string `json:"specVersion"`
		Name          string `json:"name"`
		Version       string `json:"version"`
	}
	if err := json.Unmarshal(material.CanonicalIR, &identity); err != nil {
		return fmt.Errorf("decode revision canonical identity: %w", err)
	}
	if identity.FormatVersion != material.IRFormat || identity.SpecVersion != material.SpecVersion ||
		identity.Name != material.ModuleName || identity.Version != material.ModuleVersion {
		return fmt.Errorf("revision canonical IR identity does not match registry identity")
	}
	return nil
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}
