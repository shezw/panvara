/*
   Panvara
   internal/application/record/types_test.go    2026-07-14
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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/domain/project"
)

func TestScopeRequiresEveryIsolationBoundary(t *testing.T) {
	t.Parallel()
	projectID, err := project.ParseID("01981234-5678-7abc-8def-0123456789ab")
	if err != nil {
		t.Fatalf("ParseID() error = %v", err)
	}
	hash := "sha256:" + strings.Repeat("a", 64)

	scope, err := NewScope(projectID, "crm.leads", "lead", hash)
	if err != nil {
		t.Fatalf("NewScope() error = %v", err)
	}
	if scope.ModuleName != "crm.leads" || scope.ResourceName != "lead" || scope.RevisionHash != hash {
		t.Fatalf("NewScope() = %#v", scope)
	}

	_, err = NewScope(projectID, "crm.leads", "lead", "latest")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewScope() error = %v, want ErrInvalidArgument", err)
	}
}

func TestUUIDv7GeneratorEncodesTimestampVersionAndVariant(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.July, 14, 8, 9, 10, 123_000_000, time.UTC)
	generator, err := NewUUIDv7Generator(bytes.NewReader(bytes.Repeat([]byte{0xff}, 10)))
	if err != nil {
		t.Fatalf("NewUUIDv7Generator() error = %v", err)
	}
	id, err := generator.New(at)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !id.Valid() {
		t.Fatalf("New() id = %q, want valid UUIDv7", id.String())
	}

	raw, err := hex.DecodeString(strings.ReplaceAll(id.String(), "-", ""))
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	var timestampBytes [8]byte
	copy(timestampBytes[2:], raw[:6])
	if got := int64(binary.BigEndian.Uint64(timestampBytes[:])); got != at.UnixMilli() {
		t.Fatalf("timestamp = %d, want %d", got, at.UnixMilli())
	}
	if raw[6]>>4 != 7 {
		t.Fatalf("version = %d, want 7", raw[6]>>4)
	}
	if raw[8]>>6 != 2 {
		t.Fatalf("variant bits = %02b, want 10", raw[8]>>6)
	}
}

func TestNormalizeJSONObjectRejectsNestedDuplicateKeys(t *testing.T) {
	t.Parallel()
	_, err := normalizeValidatedData(ValidatedData{
		Data: []byte(`{"profile":{"email":"first@example.com","email":"second@example.com"}}`),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("normalizeValidatedData() error = %v, want ErrInvalidArgument", err)
	}
}
