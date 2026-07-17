/*
   Panvara
   internal/bootstrap/profile/profile.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package profile defines deployment compositions without coupling Core to
// infrastructure implementations.
package profile

import (
	"fmt"
	"sort"
	"strings"
)

// Name identifies a supported deployment composition.
type Name string

const (
	Lite        Name = "lite"
	Server      Name = "server"
	Manager     Name = "manager"
	Site        Name = "site"
	Commerce    Name = "commerce"
	Distributed Name = "distributed"
)

// Role is an independently deployable process responsibility.
type Role string

const (
	RoleAllInOne Role = "all-in-one"
	RoleServer   Role = "server"
	RoleManager  Role = "manager"
	RoleWorker   Role = "worker"
)

// Feature is a composable business capability.
type Feature string

const (
	FeatureAppModule    Feature = "appmodule"
	FeatureSite         Feature = "site"
	FeatureAssets       Feature = "assets"
	FeatureCommerce     Feature = "commerce"
	FeaturePayments     Feature = "payments"
	FeatureCoordination Feature = "coordination"
)

// Definition describes modules and external services for a profile.
type Definition struct {
	Name             Name
	Description      string
	Roles            []Role
	Features         []Feature
	RequiredServices []string
	OptionalServices []string
	// Runnable only reports whether the distribution has an executable
	// composition for this profile. It does not describe feature maturity.
	Runnable bool
}

var definitions = map[Name]Definition{
	Lite: {
		Name:        Lite,
		Description: "single-process Core and HTTP with no external service",
		Roles:       []Role{RoleAllInOne},
		Features:    []Feature{FeatureAppModule},
		Runnable:    true,
	},
	Server: {
		Name:             Server,
		Description:      "API server with durable application data",
		Roles:            []Role{RoleServer},
		Features:         []Feature{FeatureAppModule},
		RequiredServices: []string{"postgres"},
		Runnable:         true,
	},
	Manager: {
		Name:             Manager,
		Description:      "server and management control-plane roles",
		Roles:            []Role{RoleServer, RoleManager},
		Features:         []Feature{FeatureAppModule},
		RequiredServices: []string{"postgres"},
	},
	Site: {
		Name:             Site,
		Description:      "headless website and asset capabilities",
		Roles:            []Role{RoleServer},
		Features:         []Feature{FeatureAppModule, FeatureSite, FeatureAssets},
		RequiredServices: []string{"postgres"},
		OptionalServices: []string{"object-storage"},
	},
	Commerce: {
		Name:             Commerce,
		Description:      "headless commerce and payment capabilities",
		Roles:            []Role{RoleServer},
		Features:         []Feature{FeatureAppModule, FeatureCommerce, FeaturePayments},
		RequiredServices: []string{"postgres"},
		OptionalServices: []string{"valkey"},
	},
	Distributed: {
		Name:             Distributed,
		Description:      "separately deployable server and worker roles",
		Roles:            []Role{RoleServer, RoleWorker},
		Features:         []Feature{FeatureAppModule, FeatureCoordination},
		RequiredServices: []string{"postgres"},
		OptionalServices: []string{"nats", "valkey", "object-storage"},
	},
}

// Parse validates a profile name.
func Parse(value string) (Name, error) {
	name := Name(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := definitions[name]; !ok {
		return "", fmt.Errorf("unknown profile %q", value)
	}
	return name, nil
}

// DefinitionFor returns a defensive copy of a profile definition.
func DefinitionFor(name Name) (Definition, error) {
	definition, ok := definitions[name]
	if !ok {
		return Definition{}, fmt.Errorf("unknown profile %q", name)
	}
	definition.Roles = append([]Role(nil), definition.Roles...)
	definition.Features = append([]Feature(nil), definition.Features...)
	definition.RequiredServices = append([]string(nil), definition.RequiredServices...)
	definition.OptionalServices = append([]string(nil), definition.OptionalServices...)
	return definition, nil
}

// All returns every supported profile in stable name order.
func All() []Definition {
	result := make([]Definition, 0, len(definitions))
	for name := range definitions {
		definition, _ := DefinitionFor(name)
		result = append(result, definition)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}
