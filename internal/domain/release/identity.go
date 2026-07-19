/*
   Panvara
   internal/domain/release/identity.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package release defines immutable environment-scoped module publication facts.
package release

import (
	"fmt"
	"regexp"
	"strings"
)

var uuidV7Pattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// ID is the bare, normalized UUIDv7 identity of one module release fact.
type ID struct{ value string }

// ParseID validates and normalizes a module release UUIDv7 identity.
func ParseID(value string) (ID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !uuidV7Pattern.MatchString(value) {
		return ID{}, fmt.Errorf("invalid UUIDv7 release id %q", value)
	}
	return ID{value: value}, nil
}

// String returns the normalized UUIDv7 identity.
func (id ID) String() string { return id.value }

// Valid reports whether the ID was constructed from a normalized UUIDv7 value.
func (id ID) Valid() bool { return uuidV7Pattern.MatchString(id.value) }
