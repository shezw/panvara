/*
   Panvara
   internal/domain/release/active_snapshot_test.go    2026-08-02
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
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

func TestActiveSnapshotSeparatesRuntimeAndRecordNamespace(t *testing.T) {
	t.Parallel()
	material := validReleaseActiveSnapshotMaterial(t)
	snapshot, err := NewActiveSnapshot(material)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	releaseID, hasRelease := snapshot.ReleaseID()
	credentialID, hasCredential := snapshot.ActivatedCredentialID()
	if snapshot.Epoch() != 2 || snapshot.Origin() != ActiveSnapshotOriginRelease ||
		snapshot.ModuleName() != "notes" || !hasRelease || releaseID.String() != material.Binding.releaseID.String() ||
		snapshot.RuntimeRevision() == snapshot.RecordNamespaceRevision() ||
		snapshot.DataSchemaFormat() != 1 || snapshot.DataSchemaFingerprint() != schemaHash("3") ||
		snapshot.ActivatedBy() != "owner-1" || !hasCredential ||
		credentialID.String() != material.ActivatedCredentialID.String() ||
		snapshot.RequestID() != "request-2" || !snapshot.ActivatedAt().Equal(material.ActivatedAt.UTC()) {
		t.Fatalf("active snapshot getters did not preserve material: %#v", snapshot)
	}
}

func TestBootstrapActiveSnapshotHasSystemProvenanceAndNoRelease(t *testing.T) {
	t.Parallel()
	material := validReleaseActiveSnapshotMaterial(t)
	material.Epoch = 1
	material.Origin = ActiveSnapshotOriginBootstrap
	material.Binding = mustModuleBinding(t, nil, schemaHash("1"), schemaHash("1"), schemaHash("3"))
	material.ActivatedBy = bootstrapActor
	material.ActivatedCredentialID = nil
	material.RequestID = bootstrapActor
	snapshot, err := NewActiveSnapshot(material)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := snapshot.ReleaseID(); found {
		t.Fatal("bootstrap snapshot unexpectedly has a Release")
	}
	if _, found := snapshot.ActivatedCredentialID(); found {
		t.Fatal("bootstrap snapshot unexpectedly has credential provenance")
	}
}

func TestActiveSnapshotRejectsInvalidStateAndProvenance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*ActiveSnapshotMaterial)
	}{
		{name: "scope", change: func(value *ActiveSnapshotMaterial) { value.Scope = project.Scope{} }},
		{name: "epoch zero", change: func(value *ActiveSnapshotMaterial) { value.Epoch = 0 }},
		{name: "epoch overflow", change: func(value *ActiveSnapshotMaterial) { value.Epoch = 1 << 63 }},
		{name: "origin", change: func(value *ActiveSnapshotMaterial) { value.Origin = "unknown" }},
		{name: "binding", change: func(value *ActiveSnapshotMaterial) { value.Binding = ModuleBinding{} }},
		{name: "release absent", change: func(value *ActiveSnapshotMaterial) {
			value.Binding = mustModuleBinding(t, nil, schemaHash("1"), schemaHash("2"), schemaHash("3"))
		}},
		{name: "actor", change: func(value *ActiveSnapshotMaterial) { value.ActivatedBy = "bad actor" }},
		{name: "credential", change: func(value *ActiveSnapshotMaterial) { value.ActivatedCredentialID = nil }},
		{name: "request", change: func(value *ActiveSnapshotMaterial) { value.RequestID = "bad request" }},
		{name: "time", change: func(value *ActiveSnapshotMaterial) { value.ActivatedAt = time.Time{} }},
		{name: "bootstrap release", change: func(value *ActiveSnapshotMaterial) {
			value.Origin = ActiveSnapshotOriginBootstrap
			value.ActivatedBy = bootstrapActor
			value.ActivatedCredentialID = nil
			value.RequestID = bootstrapActor
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			material := validReleaseActiveSnapshotMaterial(t)
			test.change(&material)
			if _, err := NewActiveSnapshot(material); err == nil {
				t.Fatal("NewActiveSnapshot() error = nil")
			}
		})
	}
}

func TestModuleBindingRejectsInvalidIdentitiesAndCopiesRelease(t *testing.T) {
	t.Parallel()
	releaseID := mustReleaseID(t, "019f5c36-b325-7c52-9325-ec59f95c8fae")
	binding := mustModuleBinding(t, &releaseID, schemaHash("1"), schemaHash("2"), schemaHash("3"))
	releaseID = mustReleaseID(t, "019f5c36-b399-7c52-9325-ec59f95c8fae")
	stored, found := binding.ReleaseID()
	if !found || stored.String() != "019f5c36-b325-7c52-9325-ec59f95c8fae" {
		t.Fatalf("binding ReleaseID() = %q, %v", stored.String(), found)
	}
	invalid := []ModuleBindingMaterial{
		{ModuleName: "INVALID", RuntimeRevision: schemaHash("1"), RecordNamespaceRevision: schemaHash("2"), DataSchemaFormat: 1, DataSchemaFingerprint: schemaHash("3")},
		{ModuleName: "notes", RuntimeRevision: "bad", RecordNamespaceRevision: schemaHash("2"), DataSchemaFormat: 1, DataSchemaFingerprint: schemaHash("3")},
		{ModuleName: "notes", RuntimeRevision: schemaHash("1"), RecordNamespaceRevision: "bad", DataSchemaFormat: 1, DataSchemaFingerprint: schemaHash("3")},
		{ModuleName: "notes", RuntimeRevision: schemaHash("1"), RecordNamespaceRevision: schemaHash("2"), DataSchemaFormat: 0, DataSchemaFingerprint: schemaHash("3")},
		{ModuleName: "notes", RuntimeRevision: schemaHash("1"), RecordNamespaceRevision: schemaHash("2"), DataSchemaFormat: 1, DataSchemaFingerprint: "bad"},
	}
	for index, material := range invalid {
		if _, err := NewModuleBinding(material); err == nil {
			t.Fatalf("invalid binding %d was accepted", index)
		}
	}
}

func validReleaseActiveSnapshotMaterial(t *testing.T) ActiveSnapshotMaterial {
	t.Helper()
	releaseID := mustReleaseID(t, "019f5c36-b325-7c52-9325-ec59f95c8fae")
	credentialID := mustAccessID(t, "019f5c36-b326-7c52-9325-ec59f95c8fae")
	return ActiveSnapshotMaterial{
		Scope: validActiveScope(t), Epoch: 2, Origin: ActiveSnapshotOriginRelease,
		Binding:     mustModuleBinding(t, &releaseID, schemaHash("1"), schemaHash("2"), schemaHash("3")),
		ActivatedBy: "owner-1", ActivatedCredentialID: &credentialID,
		RequestID: "request-2", ActivatedAt: time.Date(2026, 8, 2, 1, 2, 3, 4, time.FixedZone("offset", 8*60*60)),
	}
}

func validActiveScope(t *testing.T) project.Scope {
	t.Helper()
	projectID, err := project.ParseID("019f5c36-b322-7c52-9325-ec59f95c8fae")
	if err != nil {
		t.Fatal(err)
	}
	environmentID, err := project.ParseEnvironmentID("019f5c36-b323-7c52-9325-ec59f95c8fae")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func mustModuleBinding(
	t *testing.T,
	releaseID *ID,
	runtimeRevision string,
	recordNamespace string,
	fingerprint string,
) ModuleBinding {
	t.Helper()
	binding, err := NewModuleBinding(ModuleBindingMaterial{
		ModuleName: "notes", ReleaseID: releaseID, RuntimeRevision: runtimeRevision,
		RecordNamespaceRevision: recordNamespace, DataSchemaFormat: 1,
		DataSchemaFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func schemaHash(character string) string { return "sha256:" + strings.Repeat(character, 64) }
