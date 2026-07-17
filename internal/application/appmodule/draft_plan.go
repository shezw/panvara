/*
   Panvara
   internal/application/appmodule/draft_plan.go    2026-07-16
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
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

func buildPlanChanges(baseline, candidate moduleIR, baselineExists bool) []PlanChange {
	changes := make([]PlanChange, 0)
	if baselineExists {
		compareValue(&changes, "module.version_changed", "/version", "change", "low", "runtime revision metadata changes", false, baseline.Version, candidate.Version)
		compareValue(&changes, "module.labels_changed", "/labels", "change", "review", "localized module labels change", false, baseline.Labels, candidate.Labels)
		compareValue(&changes, "module.requires_changed", "/requires", "change", "review", "module dependency requirements change", false, baseline.Requires, candidate.Requires)
		compareValue(&changes, "module.provides_changed", "/provides", "change", "review", "provided capabilities change", false, baseline.Provides, candidate.Provides)
		compareValue(&changes, "module.conflicts_changed", "/conflicts", "change", "review", "module conflict declarations change", false, baseline.Conflicts, candidate.Conflicts)
	} else {
		changes = append(changes, newPlanChange(
			"module.baseline_absent", "/baseline", "change", "review",
			"no baseline supplied; compatibility and data migration requirements cannot be determined", false, nil, "none",
		))
	}
	baseResources := resourceIRMap(baseline.Resources)
	candidateResources := resourceIRMap(candidate.Resources)
	resourceNames := sortedUnionKeys(baseResources, candidateResources)
	for _, resourceName := range resourceNames {
		before, hadBefore := baseResources[resourceName]
		after, hasAfter := candidateResources[resourceName]
		path := "/resources/" + resourceName
		switch {
		case !hadBefore && hasAfter:
			migration := false
			risk := "review"
			impact := "no baseline supplied; existing resource data and migration requirements cannot be evaluated"
			if baselineExists {
				migration = true
				impact = "new resource storage must be provisioned"
			}
			changes = append(changes, newPlanChange("resource.added", path, "add", risk, impact, migration, nil, resourceName))
			continue
		case hadBefore && !hasAfter:
			changes = append(changes, newPlanChange("resource.removed", path, "remove", "destructive", "existing resource data can become inaccessible", true, resourceName, nil))
			continue
		}
		compareValue(&changes, "resource.labels_changed", path+"/labels", "change", "review", "localized resource labels change", false, before.Labels, after.Labels)
		compareValue(&changes, "resource.api_changed", path+"/api", "change", "review", "generated API access surface changes", false, before.API, after.API)
		compareValue(&changes, "resource.manager_changed", path+"/manager", "change", "review", "generated manager presentation changes", false, before.Manager, after.Manager)
		compareFields(&changes, path, before.Fields, after.Fields)
	}
	sort.Slice(changes, func(left, right int) bool {
		if changes[left].Path != changes[right].Path {
			return changes[left].Path < changes[right].Path
		}
		if changes[left].Code != changes[right].Code {
			return changes[left].Code < changes[right].Code
		}
		return changes[left].After < changes[right].After
	})
	return changes
}

func compareFields(changes *[]PlanChange, resourcePath string, beforeFields, afterFields []fieldIR) {
	beforeMap := fieldIRMap(beforeFields)
	afterMap := fieldIRMap(afterFields)
	for _, name := range sortedUnionKeys(beforeMap, afterMap) {
		before, hadBefore := beforeMap[name]
		after, hasAfter := afterMap[name]
		path := resourcePath + "/fields/" + name
		switch {
		case !hadBefore && hasAfter:
			risk := "low"
			impact := "new optional field requires storage evolution"
			if after.Required {
				risk = "review"
				impact = "new required field needs a backfill or default strategy"
			}
			*changes = append(*changes, newPlanChange("field.added", path, "add", risk, impact, after.Required, nil, after))
			continue
		case hadBefore && !hasAfter:
			*changes = append(*changes, newPlanChange("field.removed", path, "remove", "destructive", "stored field values can be lost or hidden", true, before, nil))
			continue
		}
		compareValue(changes, "field.labels_changed", path+"/labels", "change", "review", "localized field labels change", false, before.Labels, after.Labels)
		compareValue(changes, "field.kind_changed", path+"/kind", "change", "destructive", "stored values may not convert to the new type", true, before.Kind, after.Kind)
		if before.Required != after.Required {
			risk, impact := "low", "field becomes optional"
			if after.Required {
				risk, impact = "review", "existing records require a value before this constraint can hold"
			}
			*changes = append(*changes, newPlanChange("field.required_changed", path+"/required", "change", risk, impact, after.Required, before.Required, after.Required))
		}
		if before.Unique != after.Unique {
			risk, impact := "low", "unique enforcement is removed"
			if after.Unique {
				risk, impact = "review", "existing duplicates must be checked before enabling uniqueness"
			}
			*changes = append(*changes, newPlanChange("field.unique_changed", path+"/unique", "change", risk, impact, after.Unique, before.Unique, after.Unique))
		}
		compareValue(changes, "field.target_changed", path+"/target", "change", "destructive", "reference identity and integrity semantics change", true, before.Target, after.Target)
		if !reflect.DeepEqual(before.Options, after.Options) {
			risk, impact, migration := optionChangeRisk(before.Options, after.Options)
			*changes = append(*changes, newPlanChange("field.options_changed", path+"/options", "change", risk, impact, migration, before.Options, after.Options))
		}
		if !reflect.DeepEqual(before.Constraints, after.Constraints) {
			tightened := constraintsTightened(before.Constraints, after.Constraints)
			risk, impact := "low", "field constraints are relaxed"
			if tightened {
				risk, impact = "review", "existing values must be checked against tightened constraints"
			}
			*changes = append(*changes, newPlanChange("field.constraints_changed", path+"/constraints", "change", risk, impact, tightened, before.Constraints, after.Constraints))
		}
	}
}

func optionChangeRisk(before, after []string) (string, string, bool) {
	afterSet := make(map[string]struct{}, len(after))
	for _, value := range after {
		afterSet[value] = struct{}{}
	}
	for _, value := range before {
		if _, found := afterSet[value]; !found {
			return "destructive", "removed enum options can invalidate stored values", true
		}
	}
	if len(before) == len(after) {
		return "low", "enum option display order changes", false
	}
	return "low", "new enum options expand accepted values", false
}

func constraintsTightened(before, after constraintsIR) bool {
	return upperBoundTightened(before.MaxLength, after.MaxLength) ||
		upperBoundTightened(before.Precision, after.Precision) ||
		upperBoundTightened(before.Scale, after.Scale)
}

func upperBoundTightened(before, after *int) bool {
	return after != nil && (before == nil || *after < *before)
}

func summarizePlan(changes []PlanChange) (PlanRiskSummary, string, string) {
	var summary PlanRiskSummary
	hasMigration := false
	for _, change := range changes {
		hasMigration = hasMigration || change.RequiresMigration
		switch change.Risk {
		case "destructive":
			summary.Destructive++
		case "review":
			summary.Review++
		default:
			summary.Low++
		}
	}
	switch {
	case summary.Destructive > 0:
		return summary, "unsupported", "critical"
	case hasMigration:
		return summary, "migration_required", "high"
	case summary.Review > 0:
		return summary, "review_required", "medium"
	case len(changes) > 0:
		return summary, "compatible", "low"
	default:
		return summary, "compatible", "none"
	}
}

func decodeCanonicalModuleIR(value []byte) (moduleIR, error) {
	var result moduleIR
	if len(value) == 0 || len(value) > MaxDraftPlanBytes || !json.Valid(value) {
		return moduleIR{}, fmt.Errorf("canonical module IR is invalid")
	}
	if err := json.Unmarshal(value, &result); err != nil {
		return moduleIR{}, err
	}
	if result.FormatVersion != IRFormatVersion || result.Name == "" || result.Version == "" {
		return moduleIR{}, fmt.Errorf("canonical module IR identity is invalid")
	}
	encoded, err := json.Marshal(result)
	if err != nil || !bytes.Equal(encoded, value) {
		return moduleIR{}, fmt.Errorf("module IR is not in canonical form")
	}
	return result, nil
}

func compareValue(changes *[]PlanChange, code, path, kind, risk, impact string, migration bool, before, after any) {
	if !reflect.DeepEqual(before, after) {
		*changes = append(*changes, newPlanChange(code, path, kind, risk, impact, migration, before, after))
	}
}

func newPlanChange(code, path, kind, risk, impact string, migration bool, before, after any) PlanChange {
	return PlanChange{Code: code, Path: path, Kind: kind, Risk: risk, Impact: impact,
		RequiresMigration: migration, Before: planValue(before), After: planValue(after)}
}

func planValue(value any) string {
	if value == nil {
		return ""
	}
	encoded, _ := json.Marshal(value)
	if len(encoded) > 64<<10 {
		return `{"truncatedHash":"` + hashBytesForDraft(encoded) + `"}`
	}
	return string(encoded)
}

func resourceIRMap(values []resourceIR) map[string]resourceIR {
	result := make(map[string]resourceIR, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func fieldIRMap(values []fieldIR) map[string]fieldIR {
	result := make(map[string]fieldIR, len(values))
	for _, value := range values {
		result[value.Name] = value
	}
	return result
}

func sortedUnionKeys[T any](left, right map[string]T) []string {
	set := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		set[key] = struct{}{}
	}
	for key := range right {
		set[key] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
