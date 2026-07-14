/*
   Panvara
   internal/buildinfo/info_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package buildinfo

import (
	"testing"

	"github.com/shezw/panvara/internal/domain/appmodule"
)

func TestCurrentIncludesEveryVersionAxis(t *testing.T) {
	t.Parallel()

	info := Current()
	if info.Distribution == "" ||
		info.CoreAPI.Version == "" ||
		info.ModuleSpec.Version == "" ||
		info.ProviderAPI.Version == "" {
		t.Fatalf("version axes must not be empty: %+v", info)
	}
	if info.IRFormat.Version < 1 {
		t.Fatalf("IR format must be positive: %d", info.IRFormat.Version)
	}
	if info.GoVersion == "" {
		t.Fatal("Go version must be reported")
	}
	if info.ProviderAPI.Status != Planned || info.IRFormat.Status != Planned {
		t.Fatalf("unimplemented contracts must be reported as planned: %+v", info)
	}
	if info.ModuleSpec.Version != appmodule.APIVersion {
		t.Fatalf(
			"reported module spec %q differs from validator %q",
			info.ModuleSpec.Version,
			appmodule.APIVersion,
		)
	}
}
