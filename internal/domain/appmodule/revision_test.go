/*
   Panvara
   internal/domain/appmodule/revision_test.go    2026-07-15
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
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

func TestRevisionIsImmutableAndValidatesContentIdentity(t *testing.T) {
	t.Parallel()
	material := validRevisionMaterial(t)
	revision, err := NewRevision(material)
	if err != nil {
		t.Fatal(err)
	}
	material.Source[0] = '!'
	material.CanonicalIR[0] = '!'
	source := revision.Source()
	source[0] = '?'
	if revision.Source()[0] == '?' || revision.CanonicalIR()[0] == '!' {
		t.Fatal("Revision leaked mutable artifact bytes")
	}
	if revision.ProjectID().String() != "01981234-5678-7abc-8def-0123456789ab" ||
		revision.ModuleName() != "crm.leads" || revision.Origin() != RevisionOriginBootstrap {
		t.Fatalf("Revision identity = %#v", revision)
	}
	identities := revision.DataSchemaIdentities()
	identities[0] = DataSchemaIdentity{}
	if got := revision.DataSchemaIdentities(); len(got) != 2 || got[0].Format() != 1 || got[1].Format() != 2 {
		t.Fatalf("DataSchemaIdentities() = %#v, want formats 1,2", got)
	}
	summary := revision.Summary()
	if summary.ProjectID().String() != revision.ProjectID().String() ||
		summary.RevisionHash() != revision.RevisionHash() || len(summary.DataSchemaIdentities()) != 2 {
		t.Fatalf("Summary() = %#v", summary)
	}
}

func TestRevisionRejectsHashAndCanonicalIdentityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*RevisionMaterial)
	}{
		{name: "source hash", change: func(value *RevisionMaterial) { value.SourceHash = "sha256:" + strings.Repeat("a", 64) }},
		{name: "revision hash", change: func(value *RevisionMaterial) { value.RevisionHash = "sha256:" + strings.Repeat("b", 64) }},
		{name: "module identity", change: func(value *RevisionMaterial) { value.ModuleName = "crm.other" }},
		{name: "duplicate schema format", change: func(value *RevisionMaterial) {
			value.DataSchemaIdentities = append(value.DataSchemaIdentities, value.DataSchemaIdentities[0])
		}},
		{name: "provenance", change: func(value *RevisionMaterial) { value.RegisteredBy = "project-owner" }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			material := validRevisionMaterial(t)
			test.change(&material)
			if _, err := NewRevision(material); err == nil {
				t.Fatal("NewRevision() error = nil")
			}
		})
	}
}

func TestRevisionArtifactEqualityRetainsSourceProvenance(t *testing.T) {
	t.Parallel()
	leftMaterial := validRevisionMaterial(t)
	rightMaterial := validRevisionMaterial(t)
	rightMaterial.Source = append([]byte("# equivalent authoring source\n"), rightMaterial.Source...)
	rightMaterial.SourceHash = hashBytes(rightMaterial.Source)
	left, err := NewRevision(leftMaterial)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewRevision(rightMaterial)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(left.Source(), right.Source()) || !left.SameArtifacts(right) {
		t.Fatal("equivalent artifacts were coupled to source provenance")
	}
}

func validRevisionMaterial(t *testing.T) RevisionMaterial {
	t.Helper()
	projectID, err := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("apiVersion: panvara.dev/v1alpha1\nkind: AppModule\n")
	canonical := []byte(`{"formatVersion":1,"specVersion":"panvara.dev/v1alpha1","name":"crm.leads","version":"1.0.0"}`)
	formatOne, err := NewDataSchemaIdentity(1, "sha256:"+strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	formatTwo, err := NewDataSchemaIdentity(2, "sha256:"+strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	return RevisionMaterial{
		ProjectID: projectID, ModuleName: "crm.leads", ModuleVersion: "1.0.0",
		RevisionHash: hashBytes(canonical), DataSchemaIdentities: []DataSchemaIdentity{formatTwo, formatOne},
		SpecVersion: "panvara.dev/v1alpha1", IRFormat: 1,
		SourceFormat: SourceFormatYAML, SourceHash: hashBytes(source), Source: source,
		CanonicalIR: canonical, OpenAPI: []byte(`{"openapi":"3.1.0"}`),
		ManagerSchema: []byte(`{"schema":"manager"}`), Origin: RevisionOriginBootstrap,
		RegisteredBy: "system:bootstrap", RegisteredAt: time.Date(2026, 7, 15, 1, 2, 3, 0, time.UTC),
	}
}
