/*
   Panvara
   internal/interfaces/httpapi/access_request.go    2026-07-19
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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

var accessPrincipalIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

const (
	maxAccessJSONDepth = 16
	maxAccessJSONNodes = 10_000
)

type createPrincipalRequest struct {
	DisplayName string `json:"display_name"`
}

type issueCredentialRequest struct {
	Label string `json:"label"`
}

func decodeStrictAccessJSON(
	writer http.ResponseWriter,
	request *http.Request,
	destination any,
	exactFields ...string,
) *requestProblem {
	body, problem := readJSONBody(writer, request)
	if problem != nil {
		return problem
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return invalidAccessBody()
	}
	if err := rejectUnexpectedAccessFields(body, exactFields); err != nil {
		return invalidAccessBody()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return invalidAccessBody()
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return invalidAccessBody()
	}
	return nil
}

// rejectUnexpectedAccessFields keeps the wire contract stricter than
// encoding/json's case-insensitive Unicode field matching. Only the exact
// ASCII JSON names declared by an Access endpoint may reach struct decoding.
func rejectUnexpectedAccessFields(source []byte, exactFields []string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(source, &object); err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(exactFields))
	for _, field := range exactFields {
		allowed[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowed[field]; !ok {
			return errors.New("unsupported access JSON field")
		}
	}
	return nil
}

func rejectDuplicateJSONFields(source []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return errors.New("access request must be a JSON object")
	}
	nodes := 0
	if err := consumeUniqueJSONObject(decoder, 1, &nodes); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func consumeUniqueJSONObject(decoder *json.Decoder, depth int, nodes *int) error {
	if depth > maxAccessJSONDepth {
		return errors.New("access JSON exceeds maximum depth")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("JSON object key is not a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return errors.New("duplicate JSON object key")
		}
		seen[key] = struct{}{}
		valueToken, err := decoder.Token()
		if err != nil {
			return err
		}
		*nodes++
		if *nodes > maxAccessJSONNodes {
			return errors.New("access JSON exceeds maximum nodes")
		}
		if err := consumeUniqueJSONValue(decoder, valueToken, depth+1, nodes); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return err
	}
	if closing != json.Delim('}') {
		return errors.New("JSON object is not closed")
	}
	return nil
}

func consumeUniqueJSONValue(
	decoder *json.Decoder,
	token json.Token,
	depth int,
	nodes *int,
) error {
	if depth > maxAccessJSONDepth {
		return errors.New("access JSON exceeds maximum depth")
	}
	delimiter, nested := token.(json.Delim)
	if !nested {
		return nil
	}
	switch delimiter {
	case '{':
		return consumeUniqueJSONObject(decoder, depth, nodes)
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			*nodes++
			if *nodes > maxAccessJSONNodes {
				return errors.New("access JSON exceeds maximum nodes")
			}
			if err := consumeUniqueJSONValue(decoder, valueToken, depth+1, nodes); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
		return nil
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func rejectAccessQuery(target *url.URL) *requestProblem {
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil || len(query) != 0 {
		return &requestProblem{
			status: http.StatusBadRequest, code: "invalid_query",
			message: "this access endpoint does not accept query parameters",
		}
	}
	return nil
}

func parseAccessPrincipalPath(request *http.Request) (string, *requestProblem) {
	value := request.PathValue("principal")
	if !accessPrincipalIDPattern.MatchString(value) {
		return "", &requestProblem{
			status: http.StatusBadRequest, code: "invalid_access_request",
			message: "principal id is invalid",
		}
	}
	return value, nil
}

func invalidAccessBody() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_access_request",
		message: "request body must be one strict JSON object with only supported fields",
	}
}
