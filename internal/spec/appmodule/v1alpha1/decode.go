/*
   Panvara
   internal/spec/appmodule/v1alpha1/decode.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const defaultMaxDocumentBytes int64 = 1 << 20

var (
	// ErrDocumentTooLarge reports an authoring document above the decoder limit.
	ErrDocumentTooLarge = errors.New("appmodule document exceeds size limit")
	// ErrUnsupportedFormat reports an unknown source encoding.
	ErrUnsupportedFormat = errors.New("unsupported appmodule document format")
)

// Format selects the explicit source document encoding.
type Format string

const (
	// FormatJSON decodes strict JSON.
	FormatJSON Format = "json"
	// FormatYAML decodes strict YAML.
	FormatYAML Format = "yaml"
)

// Decode parses one bounded AppModule document and rejects unknown fields.
func Decode(source []byte, format Format) (Document, error) {
	return DecodeReader(bytes.NewReader(source), format)
}

// DecodeReader parses exactly one bounded AppModule document. JSON trailing
// values and YAML multi-document streams are rejected to avoid hidden input.
func DecodeReader(reader io.Reader, format Format) (Document, error) {
	if reader == nil {
		return Document{}, errors.New("decode appmodule: nil reader")
	}

	source, err := io.ReadAll(io.LimitReader(reader, defaultMaxDocumentBytes+1))
	if err != nil {
		return Document{}, fmt.Errorf("read appmodule document: %w", err)
	}
	if int64(len(source)) > defaultMaxDocumentBytes {
		return Document{}, ErrDocumentTooLarge
	}
	if !utf8.Valid(source) {
		return Document{}, errors.New("decode appmodule: source is not valid UTF-8")
	}

	switch format {
	case FormatJSON:
		return decodeJSON(source)
	case FormatYAML:
		return decodeYAML(source)
	default:
		return Document{}, fmt.Errorf("%w %q", ErrUnsupportedFormat, format)
	}
}

func decodeJSON(source []byte) (Document, error) {
	if err := validateJSONStructure(source); err != nil {
		return Document{}, fmt.Errorf("decode appmodule JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()

	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode appmodule JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("decode appmodule JSON: multiple values are not allowed")
		}
		return Document{}, fmt.Errorf("decode appmodule JSON trailing value: %w", err)
	}
	return document, nil
}

func decodeYAML(source []byte) (Document, error) {
	if err := validateYAMLStructure(source); err != nil {
		return Document{}, fmt.Errorf("decode appmodule YAML: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(source))
	decoder.KnownFields(true)

	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, fmt.Errorf("decode appmodule YAML: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Document{}, errors.New("decode appmodule YAML: multiple documents are not allowed")
		}
		return Document{}, fmt.Errorf("decode appmodule YAML trailing document: %w", err)
	}
	return document, nil
}
