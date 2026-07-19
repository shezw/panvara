/*
   Panvara
   internal/domain/release/module_release_test.go    2026-07-19
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

	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

func TestModuleReleasePreservesEnvironmentScopedPublishFacts(t *testing.T) {
	t.Parallel()
	material := validModuleReleaseMaterial(t)
	value, err := NewModuleRelease(material)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.ID().String() != material.ID.String() || value.ModuleName() != "notes" ||
		value.Scope().EnvironmentID().String() != material.Scope.EnvironmentID().String() ||
		value.DraftID().String() != material.DraftID.String() || value.DraftGeneration() != 3 ||
		value.ValidationID() != material.ValidationID || value.PlanID() != material.PlanID ||
		value.PlanHash() != material.PlanHash || value.BaselineRevision() != material.BaselineRevision ||
		value.CandidateRevision() != material.CandidateRevision || value.DataSchemaFormat() != 1 ||
		value.DataSchemaFingerprint() != material.DataSchemaFingerprint || value.SourceHash() != material.SourceHash ||
		value.Outcome() != OutcomeMigrationRequired || value.Risk() != "high" ||
		value.PublishedBy() != "svc:publisher" ||
		value.PublishedCredentialID().String() != material.PublishedCredentialID.String() ||
		value.RequestID() != "request-1" || !value.PublishedAt().Equal(material.PublishedAt) {
		t.Fatalf("ModuleRelease getters did not preserve material: %#v", value)
	}
}

func TestModuleReleaseRejectsInvalidFacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		change func(*ModuleReleaseMaterial)
	}{
		{name: "release id", change: func(value *ModuleReleaseMaterial) { value.ID = ID{} }},
		{name: "scope", change: func(value *ModuleReleaseMaterial) { value.Scope = project.Scope{} }},
		{name: "module", change: func(value *ModuleReleaseMaterial) { value.ModuleName = "INVALID" }},
		{name: "draft id", change: func(value *ModuleReleaseMaterial) { value.DraftID = appmodule.DraftID{} }},
		{name: "generation", change: func(value *ModuleReleaseMaterial) { value.DraftGeneration = 0 }},
		{name: "validation", change: func(value *ModuleReleaseMaterial) { value.ValidationID = "invalid" }},
		{name: "plan", change: func(value *ModuleReleaseMaterial) { value.PlanID = "invalid" }},
		{name: "plan hash", change: func(value *ModuleReleaseMaterial) { value.PlanHash = "invalid" }},
		{name: "baseline", change: func(value *ModuleReleaseMaterial) { value.BaselineRevision = "invalid" }},
		{name: "candidate", change: func(value *ModuleReleaseMaterial) { value.CandidateRevision = "invalid" }},
		{name: "schema format", change: func(value *ModuleReleaseMaterial) { value.DataSchemaFormat = 0 }},
		{name: "schema fingerprint", change: func(value *ModuleReleaseMaterial) { value.DataSchemaFingerprint = "invalid" }},
		{name: "source", change: func(value *ModuleReleaseMaterial) { value.SourceHash = "invalid" }},
		{name: "outcome", change: func(value *ModuleReleaseMaterial) { value.Outcome = Outcome("unsupported") }},
		{name: "risk", change: func(value *ModuleReleaseMaterial) { value.Risk = "critical" }},
		{name: "classification mismatch", change: func(value *ModuleReleaseMaterial) { value.Outcome = OutcomeCompatible }},
		{name: "actor", change: func(value *ModuleReleaseMaterial) { value.PublishedBy = "bad actor" }},
		{name: "credential", change: func(value *ModuleReleaseMaterial) { value.PublishedCredentialID = domainaccess.ID{} }},
		{name: "request", change: func(value *ModuleReleaseMaterial) { value.RequestID = "bad request" }},
		{name: "time", change: func(value *ModuleReleaseMaterial) { value.PublishedAt = time.Time{} }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			material := validModuleReleaseMaterial(t)
			test.change(&material)
			if _, err := NewModuleRelease(material); err == nil {
				t.Fatal("NewModuleRelease() error = nil")
			}
		})
	}
}

func TestModuleReleasePublishIntentExcludesIdentityAndProvenance(t *testing.T) {
	t.Parallel()
	leftMaterial := validModuleReleaseMaterial(t)
	rightMaterial := validModuleReleaseMaterial(t)
	rightMaterial.ID = mustReleaseID(t, "019f5c36-b399-7c52-9325-ec59f95c8fae")
	rightMaterial.PublishedBy = "other-owner"
	rightMaterial.PublishedCredentialID = mustAccessID(t, "019f5c36-b398-7c52-9325-ec59f95c8fae")
	rightMaterial.RequestID = "request-2"
	rightMaterial.PublishedAt = rightMaterial.PublishedAt.Add(time.Hour)
	left, err := NewModuleRelease(leftMaterial)
	if err != nil {
		t.Fatal(err)
	}
	right, err := NewModuleRelease(rightMaterial)
	if err != nil {
		t.Fatal(err)
	}
	if !left.SamePublishIntent(right) {
		t.Fatal("equivalent publication intent was coupled to release provenance")
	}
	rightMaterial.CandidateRevision = "sha256:" + strings.Repeat("9", 64)
	different, err := NewModuleRelease(rightMaterial)
	if err != nil {
		t.Fatal(err)
	}
	if left.SamePublishIntent(different) {
		t.Fatal("different candidate revision matched publication intent")
	}
}

func TestReleaseIDAndOutcomeValidation(t *testing.T) {
	t.Parallel()
	id, err := ParseID(" 019F5C36-B322-7C52-9325-EC59F95C8FAE ")
	if err != nil || id.String() != "019f5c36-b322-7c52-9325-ec59f95c8fae" {
		t.Fatalf("ParseID() = %q, %v", id.String(), err)
	}
	if _, err := ParseID("019f5c36-b322-6c52-9325-ec59f95c8fae"); err == nil {
		t.Fatal("ParseID() accepted non-UUIDv7")
	}
	for _, outcome := range []Outcome{OutcomeCompatible, OutcomeReviewRequired, OutcomeMigrationRequired} {
		if !outcome.Valid() {
			t.Fatalf("Outcome %q is invalid", outcome)
		}
	}
	if Outcome("unsupported").Valid() {
		t.Fatal("unsupported outcome is valid")
	}
}

func validModuleReleaseMaterial(t *testing.T) ModuleReleaseMaterial {
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
	draftID, err := appmodule.ParseDraftID("019f5c36-b324-7c52-9325-ec59f95c8fae")
	if err != nil {
		t.Fatal(err)
	}
	return ModuleReleaseMaterial{
		ID: mustReleaseID(t, "019f5c36-b325-7c52-9325-ec59f95c8fae"), Scope: scope,
		ModuleName: "notes", DraftID: draftID, DraftGeneration: 3,
		ValidationID: "sha256:" + strings.Repeat("1", 64), PlanID: "sha256:" + strings.Repeat("2", 64),
		PlanHash: "sha256:" + strings.Repeat("3", 64), BaselineRevision: "sha256:" + strings.Repeat("4", 64),
		CandidateRevision: "sha256:" + strings.Repeat("5", 64), DataSchemaFormat: 1,
		DataSchemaFingerprint: "sha256:" + strings.Repeat("6", 64), SourceHash: "sha256:" + strings.Repeat("7", 64),
		Outcome: OutcomeMigrationRequired, Risk: "high", PublishedBy: "svc:publisher",
		PublishedCredentialID: mustAccessID(t, "019f5c36-b326-7c52-9325-ec59f95c8fae"),
		RequestID:             "request-1", PublishedAt: time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC),
	}
}

func mustReleaseID(t *testing.T, value string) ID {
	t.Helper()
	id, err := ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustAccessID(t *testing.T, value string) domainaccess.ID {
	t.Helper()
	id, err := domainaccess.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
