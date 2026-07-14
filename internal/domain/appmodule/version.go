/*
   Panvara
   internal/domain/appmodule/version.go    2026-07-14
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
	"fmt"
	"strings"
)

const maxVersionRangeBytes = 512

type semanticVersion struct {
	major      string
	minor      string
	patch      string
	prerelease []string
}

type versionPredicate struct {
	operator string
	version  semanticVersion
}

type parsedVersionRange struct {
	wildcard   bool
	caret      *semanticVersion
	predicates []versionPredicate
}

// ValidateVersionRange validates Panvara's deterministic v1alpha1 SemVer
// range subset: *, an exact version, ^version, or space-separated comparison
// predicates such as ">=1.2.0 <2.0.0".
func ValidateVersionRange(value string) error {
	_, err := parseVersionRange(value)
	return err
}

// SatisfiesVersionRange reports whether a semantic version matches a valid
// Panvara v1alpha1 range.
func SatisfiesVersionRange(version, constraint string) (bool, error) {
	candidate, err := parseSemanticVersion(version)
	if err != nil {
		return false, fmt.Errorf("invalid semantic version %q", version)
	}
	rangeValue, err := parseVersionRange(constraint)
	if err != nil {
		return false, err
	}
	if rangeValue.wildcard {
		return true, nil
	}
	if rangeValue.caret != nil {
		lower := *rangeValue.caret
		if compareSemanticVersion(candidate, lower) < 0 {
			return false, nil
		}
		upper := caretUpperBound(lower)
		return compareSemanticVersion(candidate, upper) < 0, nil
	}
	for _, predicate := range rangeValue.predicates {
		comparison := compareSemanticVersion(candidate, predicate.version)
		matches := false
		switch predicate.operator {
		case "=":
			matches = comparison == 0
		case ">":
			matches = comparison > 0
		case ">=":
			matches = comparison >= 0
		case "<":
			matches = comparison < 0
		case "<=":
			matches = comparison <= 0
		}
		if !matches {
			return false, nil
		}
	}
	return true, nil
}

func parseVersionRange(value string) (parsedVersionRange, error) {
	if value == "" || len(value) > maxVersionRangeBytes || value != strings.TrimSpace(value) {
		return parsedVersionRange{}, fmt.Errorf("invalid semantic version range %q", value)
	}
	if value == "*" {
		return parsedVersionRange{wildcard: true}, nil
	}
	if strings.HasPrefix(value, "^") {
		version, err := parseSemanticVersion(strings.TrimPrefix(value, "^"))
		if err != nil {
			return parsedVersionRange{}, fmt.Errorf("invalid semantic version range %q", value)
		}
		return parsedVersionRange{caret: &version}, nil
	}
	if version, err := parseSemanticVersion(value); err == nil {
		return parsedVersionRange{predicates: []versionPredicate{{operator: "=", version: version}}}, nil
	}

	parts := strings.Fields(value)
	if len(parts) == 0 || strings.Join(parts, " ") != value {
		return parsedVersionRange{}, fmt.Errorf("invalid semantic version range %q", value)
	}
	predicates := make([]versionPredicate, 0, len(parts))
	for _, part := range parts {
		operator := ""
		for _, candidate := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(part, candidate) {
				operator = candidate
				break
			}
		}
		if operator == "" {
			return parsedVersionRange{}, fmt.Errorf("invalid semantic version range predicate %q", part)
		}
		version, err := parseSemanticVersion(strings.TrimPrefix(part, operator))
		if err != nil {
			return parsedVersionRange{}, fmt.Errorf("invalid semantic version range predicate %q", part)
		}
		predicates = append(predicates, versionPredicate{operator: operator, version: version})
	}
	return parsedVersionRange{predicates: predicates}, nil
}

func parseSemanticVersion(value string) (semanticVersion, error) {
	if !semverPattern.MatchString(value) {
		return semanticVersion{}, fmt.Errorf("invalid semantic version %q", value)
	}
	withoutBuild, _, _ := strings.Cut(value, "+")
	core, prerelease, hasPrerelease := strings.Cut(withoutBuild, "-")
	parts := strings.Split(core, ".")
	result := semanticVersion{major: parts[0], minor: parts[1], patch: parts[2]}
	if hasPrerelease {
		result.prerelease = strings.Split(prerelease, ".")
	}
	return result, nil
}

func compareSemanticVersion(left, right semanticVersion) int {
	for _, pair := range [][2]string{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if comparison := compareNumericIdentifier(pair[0], pair[1]); comparison != 0 {
			return comparison
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return 0
	}
	if len(left.prerelease) == 0 {
		return 1
	}
	if len(right.prerelease) == 0 {
		return -1
	}
	limit := min(len(left.prerelease), len(right.prerelease))
	for index := 0; index < limit; index++ {
		comparison := comparePrereleaseIdentifier(left.prerelease[index], right.prerelease[index])
		if comparison != 0 {
			return comparison
		}
	}
	return compareInt(len(left.prerelease), len(right.prerelease))
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric := isNumeric(left)
	rightNumeric := isNumeric(right)
	switch {
	case leftNumeric && rightNumeric:
		return compareNumericIdentifier(left, right)
	case leftNumeric:
		return -1
	case rightNumeric:
		return 1
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareNumericIdentifier(left, right string) int {
	if len(left) != len(right) {
		return compareInt(len(left), len(right))
	}
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func isNumeric(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func caretUpperBound(version semanticVersion) semanticVersion {
	switch {
	case version.major != "0":
		return semanticVersion{major: incrementNumericIdentifier(version.major), minor: "0", patch: "0"}
	case version.minor != "0":
		return semanticVersion{major: "0", minor: incrementNumericIdentifier(version.minor), patch: "0"}
	default:
		return semanticVersion{major: "0", minor: "0", patch: incrementNumericIdentifier(version.patch)}
	}
}

func incrementNumericIdentifier(value string) string {
	bytes := []byte(value)
	for index := len(bytes) - 1; index >= 0; index-- {
		if bytes[index] < '9' {
			bytes[index]++
			return string(bytes)
		}
		bytes[index] = '0'
	}
	return "1" + string(bytes)
}
