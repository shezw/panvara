/*
   Panvara
   internal/domain/appmodule/version_test.go    2026-07-14
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

import "testing"

func TestSatisfiesVersionRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    string
		constraint string
		want       bool
	}{
		{name: "wildcard", version: "99.0.0", constraint: "*", want: true},
		{name: "exact ignores build", version: "1.2.3+build.9", constraint: "1.2.3", want: true},
		{name: "caret major", version: "1.9.0", constraint: "^1.2.3", want: true},
		{name: "caret major upper", version: "2.0.0", constraint: "^1.2.3", want: false},
		{name: "caret zero minor", version: "0.2.9", constraint: "^0.2.3", want: true},
		{name: "comparison set", version: "1.5.0", constraint: ">=1.0.0 <2.0.0", want: true},
		{name: "prerelease lower", version: "1.0.0-alpha.2", constraint: ">=1.0.0-alpha.1 <1.0.0", want: true},
		{name: "arbitrary large identifier", version: "999999999999999999999.0.0", constraint: ">1.0.0", want: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := SatisfiesVersionRange(test.version, test.constraint)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("SatisfiesVersionRange(%q, %q) = %t, want %t", test.version, test.constraint, got, test.want)
			}
		})
	}
}

func TestValidateVersionRangeRejectsAmbiguousSyntax(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", "latest", "1.2", ">=1.0.0  <2.0.0", "~1.2.3", " 1.2.3", ">=1.0.0 || <2.0.0"} {
		if err := ValidateVersionRange(value); err == nil {
			t.Fatalf("ValidateVersionRange(%q) unexpectedly succeeded", value)
		}
	}
}
