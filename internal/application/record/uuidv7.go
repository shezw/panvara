/*
   Panvara
   internal/application/record/uuidv7.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package record

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

// Clock makes timestamps deterministic in application tests.
type Clock interface {
	Now() time.Time
}

// IDGenerator creates a record identity for a supplied application timestamp.
type IDGenerator interface {
	New(time.Time) (ID, error)
}

// SystemClock reads wall-clock time.
type SystemClock struct{}

// Now returns current UTC time.
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

// UUIDv7Generator creates RFC 9562 UUIDv7 values using a cryptographic source.
type UUIDv7Generator struct {
	random io.Reader
}

// NewUUIDv7Generator constructs a generator with an explicit entropy source.
func NewUUIDv7Generator(random io.Reader) (*UUIDv7Generator, error) {
	if random == nil {
		return nil, fmt.Errorf("%w: nil UUID entropy source", ErrInvalidArgument)
	}
	return &UUIDv7Generator{random: random}, nil
}

// NewDefaultUUIDv7Generator uses crypto/rand.Reader.
func NewDefaultUUIDv7Generator() *UUIDv7Generator {
	return &UUIDv7Generator{random: rand.Reader}
}

// New returns a UUID whose leading 48 bits contain Unix epoch milliseconds.
func (generator *UUIDv7Generator) New(at time.Time) (ID, error) {
	if generator == nil || generator.random == nil {
		return ID{}, fmt.Errorf("%w: uninitialized UUID generator", ErrInvalidArgument)
	}
	milliseconds := at.UnixMilli()
	if milliseconds < 0 || uint64(milliseconds) > (1<<48)-1 {
		return ID{}, fmt.Errorf("%w: timestamp is outside UUIDv7 range", ErrInvalidArgument)
	}

	var raw [16]byte
	value := uint64(milliseconds)
	for index := 5; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	if _, err := io.ReadFull(generator.random, raw[6:]); err != nil {
		return ID{}, fmt.Errorf("generate UUIDv7 entropy: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80

	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	formatted := string(encoded[0:8]) + "-" + string(encoded[8:12]) + "-" +
		string(encoded[12:16]) + "-" + string(encoded[16:20]) + "-" + string(encoded[20:32])
	return ID{value: formatted}, nil
}
