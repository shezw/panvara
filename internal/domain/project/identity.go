/*
   Panvara
   internal/domain/project/identity.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package project

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	uuidPattern = regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	keyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

// ID is a stable UUIDv7 project identity.
type ID struct {
	value string
}

// ParseID validates and normalizes a UUIDv7 project identity.
func ParseID(value string) (ID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !uuidPattern.MatchString(value) {
		return ID{}, fmt.Errorf("invalid UUIDv7 project id %q", value)
	}
	return ID{value: value}, nil
}

// String returns the normalized UUIDv7 string.
func (id ID) String() string {
	return id.value
}

// Valid reports whether the ID was created by ParseID.
func (id ID) Valid() bool {
	return uuidPattern.MatchString(id.value)
}

// Key is a readable project routing key that may change independently of ID.
type Key struct {
	value string
}

// ParseKey validates and normalizes a readable project key.
func ParseKey(value string) (Key, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !keyPattern.MatchString(value) {
		return Key{}, fmt.Errorf("invalid project key %q", value)
	}
	return Key{value: value}, nil
}

// String returns the normalized project key.
func (key Key) String() string {
	return key.value
}

// Valid reports whether the key was created by ParseKey.
func (key Key) Valid() bool {
	return keyPattern.MatchString(key.value)
}
