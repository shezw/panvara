/*
   Panvara
   internal/application/appmodule/record_validator_fuzz_test.go    2026-07-14
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

func FuzzRecordDecodeAndValidationNeverPanic(fuzz *testing.F) {
	module := compiledTestModuleForFuzz(fuzz)
	fuzz.Add([]byte(`{"email":"a@example.com","stage":"new"}`))
	fuzz.Add([]byte(`{"email":null,"stage":"missing"}`))
	fuzz.Add([]byte{0xff})
	fuzz.Fuzz(func(t *testing.T, source []byte) {
		first, firstErr := DecodeRecordJSON(source)
		second, secondErr := DecodeRecordJSON(source)
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("DecodeRecordJSON() error stability differs: %v / %v", firstErr, secondErr)
		}
		if firstErr == nil {
			_, _ = module.ValidateCompleteRecord("lead", first)
			_, _ = module.ValidateCompleteRecord("lead", second)
		}
	})
}

func compiledTestModuleForFuzz(fuzz *testing.F) *CompiledModule {
	fuzz.Helper()
	module, err := NewCompiler().CompileDocument(testDocument())
	if err != nil {
		fuzz.Fatal(err)
	}
	return module
}
