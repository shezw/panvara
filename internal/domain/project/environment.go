/*
   Panvara
   internal/domain/project/environment.go    2026-07-18
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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// EnvironmentID is a stable UUIDv7 deployment-environment identity.
type EnvironmentID struct {
	value string
}

// ParseEnvironmentID validates and normalizes a UUIDv7 environment identity.
func ParseEnvironmentID(value string) (EnvironmentID, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !uuidPattern.MatchString(value) {
		return EnvironmentID{}, fmt.Errorf("invalid UUIDv7 environment id %q", value)
	}
	return EnvironmentID{value: value}, nil
}

// String returns the normalized UUIDv7 string.
func (id EnvironmentID) String() string {
	return id.value
}

// Valid reports whether the ID was parsed or generated as a UUIDv7 value.
func (id EnvironmentID) Valid() bool {
	return uuidPattern.MatchString(id.value)
}

// EnvironmentIDClock supplies deterministic timestamps to an ID generator.
type EnvironmentIDClock interface {
	Now() time.Time
}

// EnvironmentIDGenerator creates UUIDv7 environment identities.
type EnvironmentIDGenerator struct {
	entropy io.Reader
	clock   EnvironmentIDClock
}

// NewEnvironmentIDGenerator constructs a generator with injectable entropy
// and time sources.
func NewEnvironmentIDGenerator(
	entropy io.Reader,
	clock EnvironmentIDClock,
) (*EnvironmentIDGenerator, error) {
	if entropy == nil {
		return nil, fmt.Errorf("environment id entropy source is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("environment id clock is required")
	}
	return &EnvironmentIDGenerator{entropy: entropy, clock: clock}, nil
}

// NewDefaultEnvironmentIDGenerator uses cryptographic entropy and wall-clock
// UTC time.
func NewDefaultEnvironmentIDGenerator() *EnvironmentIDGenerator {
	return &EnvironmentIDGenerator{entropy: rand.Reader, clock: systemEnvironmentIDClock{}}
}

// New returns a UUIDv7 environment identity whose leading 48 bits contain the
// current Unix epoch milliseconds.
func (generator *EnvironmentIDGenerator) New() (EnvironmentID, error) {
	if generator == nil || generator.entropy == nil || generator.clock == nil {
		return EnvironmentID{}, fmt.Errorf("environment id generator is not initialized")
	}
	milliseconds := generator.clock.Now().UnixMilli()
	if milliseconds < 0 || uint64(milliseconds) > (1<<48)-1 {
		return EnvironmentID{}, fmt.Errorf("environment id timestamp is outside UUIDv7 range")
	}

	var raw [16]byte
	value := uint64(milliseconds)
	for index := 5; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	if _, err := io.ReadFull(generator.entropy, raw[6:]); err != nil {
		return EnvironmentID{}, fmt.Errorf("generate environment UUIDv7 entropy: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	formatted := string(encoded[0:8]) + "-" + string(encoded[8:12]) + "-" +
		string(encoded[12:16]) + "-" + string(encoded[16:20]) + "-" + string(encoded[20:32])
	return EnvironmentID{value: formatted}, nil
}

type systemEnvironmentIDClock struct{}

func (systemEnvironmentIDClock) Now() time.Time {
	return time.Now().UTC()
}
