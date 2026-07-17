/*
   Panvara
   internal/domain/appmodule/draft_test.go    2026-07-16
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
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

func TestDraftAllowsSemanticallyInvalidAndEmptyUTF8SourceButEnforcesEncodingAndBounds(t *testing.T) {
	t.Parallel()
	projectID, _ := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	id, _ := ParseDraftID("01981234-5678-7abc-8def-0123456789ac")
	baseline, _ := NewDraftBaseline("")
	at := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	value, err := NewDraft(DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: "notes", Baseline: baseline,
		SourceFormat: SourceFormatYAML, SourceHash: DraftSourceHash(nil), Source: nil,
		Generation: 1, CreatedBy: "owner", CreatedAt: at, UpdatedBy: "owner", UpdatedAt: at,
	})
	if err != nil || len(value.Source()) != 0 {
		t.Fatalf("NewDraft(empty) = %#v, %v", value, err)
	}
	invalidUTF8 := []byte{0xff}
	if _, err := NewDraftReplacement(SourceFormatYAML, invalidUTF8, "owner", at); err == nil {
		t.Fatal("NewDraftReplacement(invalid UTF-8) error = nil")
	}
	if _, err := NewDraftReplacement(SourceFormatYAML, []byte("name:\x00value"), "owner", at); err == nil {
		t.Fatal("NewDraftReplacement(NUL) error = nil")
	}
	oversized := bytes.Repeat([]byte("x"), (1<<20)+1)
	if _, err := NewDraftReplacement(SourceFormatYAML, oversized, "owner", at); err == nil {
		t.Fatal("NewDraftReplacement(oversized) error = nil")
	}
}

func TestDraftDefensivelyCopiesSourceAndRecognizesNoOp(t *testing.T) {
	t.Parallel()
	projectID, _ := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	id, _ := ParseDraftID("01981234-5678-7abc-8def-0123456789ac")
	baseline, _ := NewDraftBaseline("sha256:" + string(bytes.Repeat([]byte("a"), 64)))
	at := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	source := []byte("not yet valid")
	value, err := NewDraft(DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: "notes", Baseline: baseline,
		SourceFormat: SourceFormatYAML, SourceHash: DraftSourceHash(source), Source: source,
		Generation: 2, CreatedBy: "owner", CreatedAt: at, UpdatedBy: "owner", UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	source[0] = 'X'
	if string(value.Source()) != "not yet valid" {
		t.Fatalf("stored source = %q", value.Source())
	}
	replacement, err := NewDraftReplacement(SourceFormatYAML, []byte("not yet valid"), "owner", at.Add(time.Hour))
	if err != nil || !value.SameSource(replacement) {
		t.Fatalf("SameSource() = false, %v", err)
	}
}

func TestDraftGenerationFitsPostgreSQLBigint(t *testing.T) {
	t.Parallel()
	projectID, _ := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	id, _ := ParseDraftID("01981234-5678-7abc-8def-0123456789ac")
	baseline, _ := NewDraftBaseline("")
	at := time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)
	material := DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: "notes", Baseline: baseline,
		SourceFormat: SourceFormatYAML, SourceHash: DraftSourceHash(nil), Generation: MaxDraftGeneration,
		CreatedBy: "owner", CreatedAt: at, UpdatedBy: "owner", UpdatedAt: at,
	}
	if _, err := NewDraft(material); err != nil {
		t.Fatalf("NewDraft(MaxDraftGeneration) error = %v", err)
	}
	material.Generation = MaxDraftGeneration + 1
	if _, err := NewDraft(material); err == nil {
		t.Fatal("NewDraft(MaxDraftGeneration+1) error = nil")
	}
}
