/*
   Panvara
   internal/buildinfo/info.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package buildinfo exposes the independent version axes used by Panvara.
package buildinfo

import "runtime"

const (
	DistributionVersion = "0.1.0-alpha.2"
	CoreAPIVersion      = "core.panvara.dev/v1alpha1"
	ModuleSpecVersion   = "panvara.dev/v1alpha1"
	ProviderAPIVersion  = "provider.panvara.dev/v1alpha1"
	IRFormatVersion     = 1
)

// Commit and BuildDate are intended to be populated by release linker flags.
var (
	Commit    = "dev"
	BuildDate = "unknown"
)

// ProtocolStatus distinguishes an executable experimental contract from a
// version target that is reserved for planned work.
type ProtocolStatus string

const (
	Experimental ProtocolStatus = "experimental"
	Planned      ProtocolStatus = "planned"
)

// Protocol reports one string-versioned contract.
type Protocol struct {
	Version string         `json:"version"`
	Status  ProtocolStatus `json:"status"`
}

// Format reports one integer-versioned serialization contract.
type Format struct {
	Version int            `json:"version"`
	Status  ProtocolStatus `json:"status"`
}

// Info is returned by the CLI and the /version endpoint.
type Info struct {
	Distribution string   `json:"distribution"`
	CoreAPI      Protocol `json:"core_api"`
	ModuleSpec   Protocol `json:"module_spec"`
	ProviderAPI  Protocol `json:"provider_api"`
	IRFormat     Format   `json:"ir_format"`
	Commit       string   `json:"commit"`
	BuildDate    string   `json:"build_date"`
	GoVersion    string   `json:"go_version"`
}

// Current returns immutable build metadata for the running process.
func Current() Info {
	return Info{
		Distribution: DistributionVersion,
		CoreAPI:      Protocol{Version: CoreAPIVersion, Status: Experimental},
		ModuleSpec:   Protocol{Version: ModuleSpecVersion, Status: Experimental},
		ProviderAPI:  Protocol{Version: ProviderAPIVersion, Status: Planned},
		IRFormat:     Format{Version: IRFormatVersion, Status: Experimental},
		Commit:       Commit,
		BuildDate:    BuildDate,
		GoVersion:    runtime.Version(),
	}
}
