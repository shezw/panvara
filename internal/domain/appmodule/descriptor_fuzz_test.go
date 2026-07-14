/*
   Panvara
   internal/domain/appmodule/descriptor_fuzz_test.go    2026-07-14
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

func FuzzDescriptorValidateNeverPanics(f *testing.F) {
	f.Add("crm.leads", "1.0.0", "lead", "email", "email")
	f.Add("", "", "", "", "")

	f.Fuzz(func(t *testing.T, module, version, resource, field, kind string) {
		descriptor := Descriptor{
			Name:    module,
			Version: version,
			Resources: []Resource{{
				Name: resource,
				Fields: []Field{{
					Name: field,
					Kind: FieldKind(kind),
				}},
			}},
		}
		_ = descriptor.Validate()
	})
}
