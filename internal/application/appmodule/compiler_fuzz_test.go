/*
   Panvara
   internal/application/appmodule/compiler_fuzz_test.go    2026-07-14
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

	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func FuzzCompileIsDeterministicAndNeverPanics(fuzz *testing.F) {
	fuzz.Add([]byte(`{"apiVersion":"panvara.dev/v1alpha1","kind":"AppModule","metadata":{"name":"crm","version":"1.0.0"},"spec":{}}`))
	fuzz.Add([]byte{0xff})
	fuzz.Fuzz(func(t *testing.T, source []byte) {
		compiler := NewCompiler()
		first, firstErr := compiler.Compile(source, spec.FormatJSON)
		second, secondErr := compiler.Compile(source, spec.FormatJSON)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("Compile() error stability differs: %v / %v", firstErr, secondErr)
		}
		if firstErr == nil && (first.RevisionHash() != second.RevisionHash() ||
			!bytes.Equal(first.CanonicalIR(), second.CanonicalIR())) {
			t.Fatal("Compile() is not deterministic")
		}
	})
}
