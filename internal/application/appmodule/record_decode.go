/*
   Panvara
   internal/application/appmodule/record_decode.go    2026-07-14
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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	maxRecordBytes = 256 << 10
	maxRecordDepth = 16
	maxRecordNodes = 10_000
)

// DecodeRecordJSON decodes one strict, bounded JSON object with json.Number
// values. Duplicate keys, trailing values, invalid UTF-8, and deep payloads are
// rejected before model validation.
func DecodeRecordJSON(source []byte) (map[string]any, error) {
	if len(source) > maxRecordBytes {
		return nil, fmt.Errorf("record JSON exceeds %d bytes", maxRecordBytes)
	}
	if !utf8.Valid(source) {
		return nil, errors.New("record JSON is not valid UTF-8")
	}
	if err := validateRecordJSONStructure(source); err != nil {
		return nil, fmt.Errorf("decode record JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decode record JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode record JSON: multiple values are not allowed")
		}
		return nil, fmt.Errorf("decode record JSON trailing value: %w", err)
	}
	return result, nil
}

func validateRecordJSONStructure(source []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("record payload must be an object")
	}
	nodes := 0
	if err := consumeRecordObject(decoder, 1, &nodes); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple values are not allowed")
		}
		return err
	}
	return nil
}

func consumeRecordObject(decoder *json.Decoder, depth int, nodes *int) error {
	if depth > maxRecordDepth {
		return fmt.Errorf("record exceeds maximum depth %d", maxRecordDepth)
	}
	seen := map[string]struct{}{}
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("record object has a non-string key")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("record object has duplicate key %q", key)
		}
		seen[key] = struct{}{}
		valueToken, err := decoder.Token()
		if err != nil {
			return err
		}
		if err := consumeRecordValue(decoder, valueToken, depth+1, nodes); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return errors.New("record object is not closed")
	}
	return nil
}

func consumeRecordValue(decoder *json.Decoder, token json.Token, depth int, nodes *int) error {
	*nodes++
	if *nodes > maxRecordNodes {
		return fmt.Errorf("record has more than %d nodes", maxRecordNodes)
	}
	if depth > maxRecordDepth {
		return fmt.Errorf("record exceeds maximum depth %d", maxRecordDepth)
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeRecordObject(decoder, depth, nodes)
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := consumeRecordValue(decoder, valueToken, depth+1, nodes); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("record array is not closed")
		}
		return nil
	default:
		return errors.New("record contains an unexpected delimiter")
	}
}
