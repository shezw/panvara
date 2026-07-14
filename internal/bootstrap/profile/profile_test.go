/*
   Panvara
   internal/bootstrap/profile/profile_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package profile

import "testing"

func TestParseSupportedProfiles(t *testing.T) {
	t.Parallel()

	for _, definition := range All() {
		definition := definition
		t.Run(string(definition.Name), func(t *testing.T) {
			t.Parallel()
			name, err := Parse(" " + string(definition.Name) + " ")
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if name != definition.Name {
				t.Fatalf("Parse() = %q, want %q", name, definition.Name)
			}
		})
	}
}

func TestParseRejectsUnknownProfile(t *testing.T) {
	t.Parallel()
	if _, err := Parse("enterprise-everything"); err == nil {
		t.Fatal("Parse() accepted an unknown profile")
	}
}

func TestDefinitionForReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	first, err := DefinitionFor(Commerce)
	if err != nil {
		t.Fatal(err)
	}
	first.Features[0] = "mutated"

	second, err := DefinitionFor(Commerce)
	if err != nil {
		t.Fatal(err)
	}
	if second.Features[0] == "mutated" {
		t.Fatal("profile definition leaked mutable package state")
	}
}

func TestOnlyLiteIsImplementedInAlphaOne(t *testing.T) {
	t.Parallel()

	for _, definition := range All() {
		want := definition.Name == Lite
		if definition.Implemented != want {
			t.Fatalf("profile %q implemented = %v, want %v", definition.Name, definition.Implemented, want)
		}
	}
}
