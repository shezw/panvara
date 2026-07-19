/*
   Panvara
   internal/interfaces/httpapi/release_request.go    2026-07-19
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
	"strings"

	"github.com/shezw/panvara/internal/domain/appmodule"
)

type publishReleaseRequest struct {
	PlanID string `json:"plan_id"`
}

func decodePublishReleaseRequest(
	writer http.ResponseWriter,
	request *http.Request,
) (publishReleaseRequest, *requestProblem) {
	body, problem := readJSONBody(writer, request)
	if problem != nil {
		return publishReleaseRequest{}, problem
	}
	if !hasExactReleaseObjectKey(body) {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	if err := rejectDuplicateJSONFields(body); err != nil {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 1 {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	if _, ok := fields["plan_id"]; !ok {
		return publishReleaseRequest{}, invalidReleaseBody()
	}

	var value publishReleaseRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	if value.PlanID != strings.TrimSpace(value.PlanID) || !appmodule.ValidContentHash(value.PlanID) {
		return publishReleaseRequest{}, invalidReleaseBody()
	}
	return value, nil
}

// hasExactReleaseObjectKey validates the raw wire spelling before encoding/json
// can normalize Unicode escapes. Publish accepts one literal ASCII key only:
// "plan_id". The remaining object structure is validated by the JSON decoders.
func hasExactReleaseObjectKey(source []byte) bool {
	index := skipReleaseJSONSpace(source, 0)
	if index >= len(source) || source[index] != '{' {
		return false
	}
	index = skipReleaseJSONSpace(source, index+1)
	const key = `"plan_id"`
	if !bytes.HasPrefix(source[index:], []byte(key)) {
		return false
	}
	index = skipReleaseJSONSpace(source, index+len(key))
	return index < len(source) && source[index] == ':'
}

func skipReleaseJSONSpace(source []byte, index int) int {
	for index < len(source) {
		switch source[index] {
		case ' ', '\t', '\r', '\n':
			index++
		default:
			return index
		}
	}
	return index
}

func parseReleaseIdempotencyKey(request *http.Request) (string, *requestProblem) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) == 0 {
		return "", &requestProblem{
			status: http.StatusBadRequest, code: "idempotency_key_required",
			message: "Idempotency-Key is required to publish a release",
		}
	}
	if len(values) != 1 || !idempotencyKeyPattern.MatchString(values[0]) {
		return "", &requestProblem{
			status: http.StatusBadRequest, code: "invalid_idempotency_key",
			message: "Idempotency-Key must contain 1..128 safe identifier characters",
		}
	}
	return values[0], nil
}

func rejectReleaseQuery(target *url.URL) *requestProblem {
	if target == nil || target.ForceQuery || target.RawQuery != "" {
		return &requestProblem{
			status: http.StatusBadRequest, code: "invalid_query",
			message: "release endpoints do not accept query parameters",
		}
	}
	return nil
}

func requireReleaseEmptyBody(writer http.ResponseWriter, request *http.Request) *requestProblem {
	if request.ContentLength > 0 {
		return unexpectedReleaseBody()
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1)
	body, err := io.ReadAll(request.Body)
	if err != nil || len(body) != 0 {
		return unexpectedReleaseBody()
	}
	return nil
}

func invalidReleaseBody() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_release_request",
		message: "request body must be one strict JSON object containing only plan_id",
	}
}

func unexpectedReleaseBody() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "unexpected_request_body",
		message: "release lookup does not accept a request body",
	}
}
