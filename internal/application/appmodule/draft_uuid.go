/*
   Panvara
   internal/application/appmodule/draft_uuid.go    2026-07-16
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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

// DraftUUIDv7Generator creates RFC 9562 UUIDv7 draft identities.
type DraftUUIDv7Generator struct{ random io.Reader }

// NewDraftUUIDv7Generator constructs a generator with explicit entropy.
func NewDraftUUIDv7Generator(random io.Reader) (*DraftUUIDv7Generator, error) {
	if random == nil {
		return nil, fmt.Errorf("%w: nil draft UUID entropy", ErrDraftInvalid)
	}
	return &DraftUUIDv7Generator{random: random}, nil
}

// NewDefaultDraftUUIDv7Generator uses crypto/rand.Reader.
func NewDefaultDraftUUIDv7Generator() *DraftUUIDv7Generator {
	return &DraftUUIDv7Generator{random: rand.Reader}
}

// New creates a UUIDv7 whose first 48 bits contain Unix epoch milliseconds.
func (generator *DraftUUIDv7Generator) New(at time.Time) (domain.DraftID, error) {
	if generator == nil || generator.random == nil {
		return domain.DraftID{}, fmt.Errorf("%w: uninitialized draft UUID generator", ErrDraftInvalid)
	}
	milliseconds := at.UnixMilli()
	if milliseconds < 0 || uint64(milliseconds) > (1<<48)-1 {
		return domain.DraftID{}, fmt.Errorf("%w: timestamp outside UUIDv7 range", ErrDraftInvalid)
	}
	var raw [16]byte
	value := uint64(milliseconds)
	for index := 5; index >= 0; index-- {
		raw[index] = byte(value)
		value >>= 8
	}
	if _, err := io.ReadFull(generator.random, raw[6:]); err != nil {
		return domain.DraftID{}, fmt.Errorf("generate draft UUIDv7 entropy: %w", err)
	}
	raw[6] = (raw[6] & 0x0f) | 0x70
	raw[8] = (raw[8] & 0x3f) | 0x80
	encoded := make([]byte, 32)
	hex.Encode(encoded, raw[:])
	formatted := string(encoded[0:8]) + "-" + string(encoded[8:12]) + "-" +
		string(encoded[12:16]) + "-" + string(encoded[16:20]) + "-" + string(encoded[20:32])
	id, err := domain.ParseDraftID(formatted)
	if err != nil {
		return domain.DraftID{}, fmt.Errorf("%w: generated invalid draft UUID: %v", ErrDraftCorrupt, err)
	}
	return id, nil
}
