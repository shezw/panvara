/*
   Panvara
   internal/spec/appmodule/v1alpha1/decode_fuzz_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package v1alpha1

import "testing"

func FuzzDecodeNeverPanics(fuzz *testing.F) {
	fuzz.Add([]byte(validJSONDocument), string(FormatJSON))
	fuzz.Add([]byte(validYAMLDocument), string(FormatYAML))
	fuzz.Add([]byte{0xff}, "yaml")
	fuzz.Fuzz(func(t *testing.T, source []byte, rawFormat string) {
		_ = t
		_, _ = Decode(source, Format(rawFormat))
	})
}
