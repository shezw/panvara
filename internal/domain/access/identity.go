/*
   Panvara
   internal/domain/access/identity.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package access defines project-local principals, credentials, and owner grants.
package access

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var uuidV7Pattern = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

// ID is a stable UUIDv7 identity for an access-domain fact.
type ID struct {
	value string
}

// ParseID validates and normalizes a UUIDv7 access identity.
func ParseID(value string) (ID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !uuidV7Pattern.MatchString(value) {
		return ID{}, fmt.Errorf("invalid UUIDv7 access id %q", value)
	}
	return ID{value: value}, nil
}

// String returns the normalized UUIDv7 string.
func (id ID) String() string { return id.value }

// Valid reports whether the ID is a normalized UUIDv7 value.
func (id ID) Valid() bool { return uuidV7Pattern.MatchString(id.value) }

// IDClock supplies deterministic timestamps to an access ID generator.
type IDClock interface {
	Now() time.Time
}

// IDGenerator creates UUIDv7 access-domain identities.
type IDGenerator struct {
	entropy io.Reader
	clock   IDClock
}

// NewIDGenerator constructs a UUIDv7 generator with injectable dependencies.
func NewIDGenerator(entropy io.Reader, clock IDClock) (*IDGenerator, error) {
	if entropy == nil {
		return nil, fmt.Errorf("access id entropy source is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("access id clock is required")
	}
	return &IDGenerator{entropy: entropy, clock: clock}, nil
}

// NewDefaultIDGenerator uses cryptographic entropy and the wall clock.
func NewDefaultIDGenerator() *IDGenerator {
	return &IDGenerator{entropy: rand.Reader, clock: systemIDClock{}}
}

// New returns a UUIDv7 access identity.
func (generator *IDGenerator) New() (ID, error) {
	if generator == nil || generator.entropy == nil || generator.clock == nil {
		return ID{}, fmt.Errorf("access id generator is not initialized")
	}
	milliseconds := generator.clock.Now().UnixMilli()
	if milliseconds < 0 || uint64(milliseconds) > (1<<48)-1 {
		return ID{}, fmt.Errorf("access id timestamp is outside UUIDv7 range")
	}

	var raw [16]byte
	value := uint64(milliseconds)
	for index := 5; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	if _, err := io.ReadFull(generator.entropy, raw[6:]); err != nil {
		return ID{}, fmt.Errorf("generate access UUIDv7 entropy: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	formatted := string(encoded[0:8]) + "-" + string(encoded[8:12]) + "-" +
		string(encoded[12:16]) + "-" + string(encoded[16:20]) + "-" + string(encoded[20:32])
	return ID{value: formatted}, nil
}

type systemIDClock struct{}

func (systemIDClock) Now() time.Time { return time.Now().UTC() }
