/*
   Panvara
   internal/application/release/uuidv7.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package release

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

// UUIDv7Generator produces release identities from injected cryptographic entropy.
type UUIDv7Generator struct{ entropy io.Reader }

// NewUUIDv7Generator constructs a release identity generator.
func NewUUIDv7Generator(entropy io.Reader) (*UUIDv7Generator, error) {
	if entropy == nil {
		return nil, fmt.Errorf("%w: release id entropy is required", ErrInvalid)
	}
	return &UUIDv7Generator{entropy: entropy}, nil
}

// NewDefaultUUIDv7Generator uses crypto/rand entropy.
func NewDefaultUUIDv7Generator() *UUIDv7Generator {
	return &UUIDv7Generator{entropy: rand.Reader}
}

// New returns a bare lowercase UUIDv7 identity bound to at's Unix milliseconds.
func (generator *UUIDv7Generator) New(at time.Time) (domainrelease.ID, error) {
	if generator == nil || generator.entropy == nil {
		return domainrelease.ID{}, fmt.Errorf("%w: release id generator is not initialized", ErrInvalid)
	}
	milliseconds := at.UnixMilli()
	if at.IsZero() || milliseconds < 0 || uint64(milliseconds) > (1<<48)-1 {
		return domainrelease.ID{}, fmt.Errorf("%w: release id timestamp is outside UUIDv7 range", ErrInvalid)
	}
	var raw [16]byte
	value := uint64(milliseconds)
	for index := 5; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	if _, err := io.ReadFull(generator.entropy, raw[6:]); err != nil {
		return domainrelease.ID{}, fmt.Errorf("%w: generate release UUIDv7 entropy: %v", ErrUnavailable, err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	formatted := string(encoded[0:8]) + "-" + string(encoded[8:12]) + "-" +
		string(encoded[12:16]) + "-" + string(encoded[16:20]) + "-" + string(encoded[20:32])
	id, err := domainrelease.ParseID(formatted)
	if err != nil {
		return domainrelease.ID{}, fmt.Errorf("%w: generated release identity: %v", ErrCorrupt, err)
	}
	return id, nil
}
