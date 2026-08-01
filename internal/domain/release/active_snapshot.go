/*
   Panvara
   internal/domain/release/active_snapshot.go    2026-08-02
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
	"strings"
	"time"

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	maxReleaseEpoch uint64 = 1<<63 - 1
	bootstrapActor         = "system:bootstrap"
)

// ActiveSnapshotOrigin identifies the authority that first made a snapshot active.
type ActiveSnapshotOrigin string

const (
	// ActiveSnapshotOriginBootstrap identifies the initial persisted runtime seed.
	ActiveSnapshotOriginBootstrap ActiveSnapshotOrigin = "bootstrap"
	// ActiveSnapshotOriginRelease identifies an owner-authorized Release activation.
	ActiveSnapshotOriginRelease ActiveSnapshotOrigin = "release"
)

// Valid reports whether origin is an explicit active-snapshot provenance.
func (origin ActiveSnapshotOrigin) Valid() bool {
	return origin == ActiveSnapshotOriginBootstrap || origin == ActiveSnapshotOriginRelease
}

// ModuleBindingMaterial contains one module's immutable runtime and Record namespace binding.
type ModuleBindingMaterial struct {
	ModuleName              string
	ReleaseID               *ID
	RuntimeRevision         string
	RecordNamespaceRevision string
	DataSchemaFormat        int
	DataSchemaFingerprint   string
}

// ModuleBinding separates the executable Revision from the persistent Record namespace.
// P0-02b snapshots contain exactly one binding, while this value remains suitable
// for a later complete Project snapshot.
type ModuleBinding struct {
	moduleName              string
	releaseID               *ID
	runtimeRevision         string
	recordNamespaceRevision string
	dataSchemaFormat        int
	dataSchemaFingerprint   string
}

// NewModuleBinding validates and constructs an immutable active module binding.
func NewModuleBinding(material ModuleBindingMaterial) (ModuleBinding, error) {
	material.ModuleName = strings.TrimSpace(material.ModuleName)
	material.RuntimeRevision = strings.ToLower(strings.TrimSpace(material.RuntimeRevision))
	material.RecordNamespaceRevision = strings.ToLower(strings.TrimSpace(material.RecordNamespaceRevision))
	material.DataSchemaFingerprint = strings.ToLower(strings.TrimSpace(material.DataSchemaFingerprint))
	if !appmodule.ValidModuleName(material.ModuleName) ||
		!appmodule.ValidContentHash(material.RuntimeRevision) ||
		!appmodule.ValidContentHash(material.RecordNamespaceRevision) ||
		material.DataSchemaFormat <= 0 ||
		!appmodule.ValidContentHash(material.DataSchemaFingerprint) {
		return ModuleBinding{}, fmt.Errorf("active module binding is invalid")
	}
	var releaseID *ID
	if material.ReleaseID != nil {
		if !material.ReleaseID.Valid() {
			return ModuleBinding{}, fmt.Errorf("active module binding release id is invalid")
		}
		copied := *material.ReleaseID
		releaseID = &copied
	}
	return ModuleBinding{
		moduleName: material.ModuleName, releaseID: releaseID,
		runtimeRevision:         material.RuntimeRevision,
		recordNamespaceRevision: material.RecordNamespaceRevision,
		dataSchemaFormat:        material.DataSchemaFormat,
		dataSchemaFingerprint:   material.DataSchemaFingerprint,
	}, nil
}

// Validate rechecks all module, Release, Revision, namespace, and data-schema identities.
func (binding ModuleBinding) Validate() error {
	_, err := NewModuleBinding(ModuleBindingMaterial{
		ModuleName: binding.moduleName, ReleaseID: binding.releaseID,
		RuntimeRevision:         binding.runtimeRevision,
		RecordNamespaceRevision: binding.recordNamespaceRevision,
		DataSchemaFormat:        binding.dataSchemaFormat,
		DataSchemaFingerprint:   binding.dataSchemaFingerprint,
	})
	return err
}

// ModuleName returns the canonical module route identity.
func (binding ModuleBinding) ModuleName() string { return binding.moduleName }

// ReleaseID returns the active immutable Release identity, if the binding was activated.
func (binding ModuleBinding) ReleaseID() (ID, bool) {
	if binding.releaseID == nil {
		return ID{}, false
	}
	return *binding.releaseID, true
}

// RuntimeRevision returns the Revision whose rules and generated artifacts execute.
func (binding ModuleBinding) RuntimeRevision() string { return binding.runtimeRevision }

// RecordNamespaceRevision returns the stable physical Record namespace Revision.
func (binding ModuleBinding) RecordNamespaceRevision() string {
	return binding.recordNamespaceRevision
}

// DataSchemaFormat returns the versioned schema identity format shared by runtime and namespace.
func (binding ModuleBinding) DataSchemaFormat() int { return binding.dataSchemaFormat }

// DataSchemaFingerprint returns the schema identity shared by runtime and namespace.
func (binding ModuleBinding) DataSchemaFingerprint() string {
	return binding.dataSchemaFingerprint
}

// ActiveSnapshotMaterial contains one immutable environment-scoped active state.
type ActiveSnapshotMaterial struct {
	Scope                 project.Scope
	Epoch                 uint64
	Origin                ActiveSnapshotOrigin
	Binding               ModuleBinding
	ActivatedBy           string
	ActivatedCredentialID *domainaccess.ID
	RequestID             string
	ActivatedAt           time.Time
}

// ActiveSnapshot is the authoritative single-module view at one monotonic environment epoch.
type ActiveSnapshot struct {
	scope                 project.Scope
	epoch                 uint64
	origin                ActiveSnapshotOrigin
	binding               ModuleBinding
	activatedBy           string
	activatedCredentialID *domainaccess.ID
	requestID             string
	activatedAt           time.Time
}

// NewActiveSnapshot validates and constructs an immutable active snapshot.
func NewActiveSnapshot(material ActiveSnapshotMaterial) (ActiveSnapshot, error) {
	material.ActivatedBy = strings.TrimSpace(material.ActivatedBy)
	material.RequestID = strings.TrimSpace(material.RequestID)
	material.ActivatedAt = material.ActivatedAt.UTC()
	if material.Scope.Validate() != nil || material.Epoch == 0 || material.Epoch > maxReleaseEpoch ||
		!material.Origin.Valid() || material.Binding.Validate() != nil || material.ActivatedAt.IsZero() {
		return ActiveSnapshot{}, fmt.Errorf("active snapshot identity or state is invalid")
	}
	_, hasRelease := material.Binding.ReleaseID()
	var credentialID *domainaccess.ID
	if material.ActivatedCredentialID != nil {
		if !material.ActivatedCredentialID.Valid() {
			return ActiveSnapshot{}, fmt.Errorf("active snapshot credential provenance is invalid")
		}
		copied := *material.ActivatedCredentialID
		credentialID = &copied
	}
	switch material.Origin {
	case ActiveSnapshotOriginBootstrap:
		if hasRelease || material.ActivatedBy != bootstrapActor || credentialID != nil || material.RequestID != bootstrapActor {
			return ActiveSnapshot{}, fmt.Errorf("bootstrap active snapshot provenance is invalid")
		}
	case ActiveSnapshotOriginRelease:
		if !hasRelease || !provenancePattern.MatchString(material.ActivatedBy) || credentialID == nil ||
			!provenancePattern.MatchString(material.RequestID) {
			return ActiveSnapshot{}, fmt.Errorf("release active snapshot provenance is invalid")
		}
	}
	return ActiveSnapshot{
		scope: material.Scope, epoch: material.Epoch, origin: material.Origin,
		binding: material.Binding, activatedBy: material.ActivatedBy,
		activatedCredentialID: credentialID, requestID: material.RequestID,
		activatedAt: material.ActivatedAt,
	}, nil
}

// Validate rechecks the complete active snapshot and provenance.
func (snapshot ActiveSnapshot) Validate() error {
	_, err := NewActiveSnapshot(ActiveSnapshotMaterial{
		Scope: snapshot.scope, Epoch: snapshot.epoch, Origin: snapshot.origin,
		Binding: snapshot.binding, ActivatedBy: snapshot.activatedBy,
		ActivatedCredentialID: snapshot.activatedCredentialID,
		RequestID:             snapshot.requestID, ActivatedAt: snapshot.activatedAt,
	})
	return err
}

// Scope returns the exact Project and Environment active-state boundary.
func (snapshot ActiveSnapshot) Scope() project.Scope { return snapshot.scope }

// Epoch returns the positive environment-scoped monotonic activation epoch.
func (snapshot ActiveSnapshot) Epoch() uint64 { return snapshot.epoch }

// Origin returns bootstrap or owner-authorized Release provenance.
func (snapshot ActiveSnapshot) Origin() ActiveSnapshotOrigin { return snapshot.origin }

// Binding returns the immutable single-module binding.
func (snapshot ActiveSnapshot) Binding() ModuleBinding { return snapshot.binding }

// ModuleName returns the active module route identity.
func (snapshot ActiveSnapshot) ModuleName() string { return snapshot.binding.ModuleName() }

// ReleaseID returns the active Release identity, if this is not the bootstrap seed.
func (snapshot ActiveSnapshot) ReleaseID() (ID, bool) { return snapshot.binding.ReleaseID() }

// RuntimeRevision returns the active executable Revision.
func (snapshot ActiveSnapshot) RuntimeRevision() string { return snapshot.binding.RuntimeRevision() }

// RecordNamespaceRevision returns the active persistent Record namespace.
func (snapshot ActiveSnapshot) RecordNamespaceRevision() string {
	return snapshot.binding.RecordNamespaceRevision()
}

// DataSchemaFormat returns the active schema identity format.
func (snapshot ActiveSnapshot) DataSchemaFormat() int { return snapshot.binding.DataSchemaFormat() }

// DataSchemaFingerprint returns the active schema fingerprint.
func (snapshot ActiveSnapshot) DataSchemaFingerprint() string {
	return snapshot.binding.DataSchemaFingerprint()
}

// ActivatedBy returns the actor or system bootstrap provenance.
func (snapshot ActiveSnapshot) ActivatedBy() string { return snapshot.activatedBy }

// ActivatedCredentialID returns credential provenance for a Release activation.
func (snapshot ActiveSnapshot) ActivatedCredentialID() (domainaccess.ID, bool) {
	if snapshot.activatedCredentialID == nil {
		return domainaccess.ID{}, false
	}
	return *snapshot.activatedCredentialID, true
}

// RequestID returns the activation correlation identity.
func (snapshot ActiveSnapshot) RequestID() string { return snapshot.requestID }

// ActivatedAt returns the UTC activation timestamp.
func (snapshot ActiveSnapshot) ActivatedAt() time.Time { return snapshot.activatedAt }
