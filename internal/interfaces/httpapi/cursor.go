/*
   Panvara
   internal/interfaces/httpapi/cursor.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/shezw/panvara/internal/application/record"
)

const maxCursorLength = 512

var queryHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type listCursorEnvelope struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	QueryHash string    `json:"query_hash"`
}

func encodeListCursor(cursor record.ListCursor) string {
	payload, _ := json.Marshal(listCursorEnvelope{
		CreatedAt: cursor.CreatedAt.UTC(), ID: cursor.ID.String(), QueryHash: cursor.QueryHash,
	})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeListCursor(value string) (record.ListCursor, error) {
	if value == "" || len(value) > maxCursorLength {
		return record.ListCursor{}, fmt.Errorf("invalid cursor length")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return record.ListCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var envelope listCursorEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return record.ListCursor{}, fmt.Errorf("decode cursor JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return record.ListCursor{}, fmt.Errorf("cursor has trailing JSON")
	}
	id, err := record.ParseID(envelope.ID)
	if err != nil || envelope.CreatedAt.IsZero() || !queryHashPattern.MatchString(envelope.QueryHash) {
		return record.ListCursor{}, fmt.Errorf("cursor values are invalid")
	}
	return record.ListCursor{CreatedAt: envelope.CreatedAt.UTC(), ID: id, QueryHash: envelope.QueryHash}, nil
}
