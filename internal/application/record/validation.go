/*
   Panvara
   internal/application/record/validation.go    2026-07-14
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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

const (
	maxRecordBytes       = 256 << 10
	maxCanonicalValueLen = 512
)

func normalizeValidatedData(value ValidatedData) (ValidatedData, error) {
	data, err := normalizeJSONObject(value.Data)
	if err != nil {
		return ValidatedData{}, err
	}

	uniques := append([]UniqueValue(nil), value.Uniques...)
	uniqueFields := make(map[string]struct{}, len(uniques))
	for _, unique := range uniques {
		if !resourceNamePattern.MatchString(unique.Field) {
			return ValidatedData{}, fmt.Errorf("%w: invalid unique field %q", ErrInvalidArgument, unique.Field)
		}
		if len(unique.CanonicalValue) > maxCanonicalValueLen {
			return ValidatedData{}, fmt.Errorf(
				"%w: canonical value for %q exceeds %d bytes",
				ErrInvalidArgument,
				unique.Field,
				maxCanonicalValueLen,
			)
		}
		if _, exists := uniqueFields[unique.Field]; exists {
			return ValidatedData{}, fmt.Errorf("%w: duplicate unique field %q", ErrInvalidArgument, unique.Field)
		}
		uniqueFields[unique.Field] = struct{}{}
	}
	sort.Slice(uniques, func(left, right int) bool { return uniques[left].Field < uniques[right].Field })

	references := append([]Reference(nil), value.References...)
	referenceFields := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if !resourceNamePattern.MatchString(reference.Field) {
			return ValidatedData{}, fmt.Errorf("%w: invalid reference field %q", ErrInvalidArgument, reference.Field)
		}
		if !resourceNamePattern.MatchString(reference.TargetResource) {
			return ValidatedData{}, fmt.Errorf(
				"%w: invalid reference target resource %q",
				ErrInvalidArgument,
				reference.TargetResource,
			)
		}
		if !reference.TargetID.Valid() {
			return ValidatedData{}, fmt.Errorf("%w: invalid reference target id", ErrInvalidArgument)
		}
		if _, exists := referenceFields[reference.Field]; exists {
			return ValidatedData{}, fmt.Errorf("%w: duplicate reference field %q", ErrInvalidArgument, reference.Field)
		}
		referenceFields[reference.Field] = struct{}{}
	}
	sort.Slice(references, func(left, right int) bool { return references[left].Field < references[right].Field })

	return ValidatedData{Data: data, Uniques: uniques, References: references}, nil
}

func normalizeJSONObject(source json.RawMessage) (json.RawMessage, error) {
	if len(source) == 0 || len(source) > maxRecordBytes {
		return nil, fmt.Errorf("%w: record body must contain 1..%d bytes", ErrInvalidArgument, maxRecordBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: decode record JSON: %v", ErrInvalidArgument, err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '{' {
		return nil, fmt.Errorf("%w: record body must be a JSON object", ErrInvalidArgument)
	}
	if err := consumeJSONObject(decoder); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%w: record body has trailing JSON", ErrInvalidArgument)
		}
		return nil, fmt.Errorf("%w: decode trailing JSON: %v", ErrInvalidArgument, err)
	}

	buffer := bytes.NewBuffer(make([]byte, 0, len(source)))
	if err := json.Compact(buffer, source); err != nil {
		return nil, fmt.Errorf("%w: compact record JSON: %v", ErrInvalidArgument, err)
	}
	return json.RawMessage(append([]byte(nil), buffer.Bytes()...)), nil
}

func mergeTopLevelPatch(current, patch json.RawMessage) (json.RawMessage, error) {
	current, err := normalizeJSONObject(current)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid persisted record before patch: %v", ErrInvalidArgument, err)
	}
	patch, err = normalizeJSONObject(patch)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid top-level record patch: %v", ErrInvalidArgument, err)
	}

	var target map[string]json.RawMessage
	if err := json.Unmarshal(current, &target); err != nil {
		return nil, fmt.Errorf("%w: decode persisted record before patch: %v", ErrInvalidArgument, err)
	}
	var changes map[string]json.RawMessage
	if err := json.Unmarshal(patch, &changes); err != nil {
		return nil, fmt.Errorf("%w: decode top-level record patch: %v", ErrInvalidArgument, err)
	}
	for field, value := range changes {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%w: patch field %q cannot be null", ErrInvalidArgument, field)
		}
		target[field] = append(json.RawMessage(nil), value...)
	}
	merged, err := json.Marshal(target)
	if err != nil {
		return nil, fmt.Errorf("%w: encode merged record: %v", ErrInvalidArgument, err)
	}
	return json.RawMessage(merged), nil
}

func consumeJSONObject(decoder *json.Decoder) error {
	fields := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: decode record field: %v", ErrInvalidArgument, err)
		}
		name, ok := token.(string)
		if !ok {
			return fmt.Errorf("%w: record object key is not a string", ErrInvalidArgument)
		}
		if _, exists := fields[name]; exists {
			return fmt.Errorf("%w: duplicate JSON field %q", ErrInvalidArgument, name)
		}
		fields[name] = struct{}{}
		if err := consumeJSONValue(decoder); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("%w: close record object: %v", ErrInvalidArgument, err)
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: decode record value: %v", ErrInvalidArgument, err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		return consumeJSONObject(decoder)
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return fmt.Errorf("%w: close record array: %v", ErrInvalidArgument, err)
		}
		return nil
	default:
		return fmt.Errorf("%w: unexpected JSON delimiter %q", ErrInvalidArgument, delimiter)
	}
}
