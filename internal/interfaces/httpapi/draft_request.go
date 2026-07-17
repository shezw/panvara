/*
   Panvara
   internal/interfaces/httpapi/draft_request.go    2026-07-16
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
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type createPlanRequest struct {
	ValidationID string `json:"validation_id"`
}

func parseDraftBaseline(target *url.URL) (string, *requestProblem) {
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil || len(query) != 1 {
		return "", invalidDraftBaseline()
	}
	values, found := query["baseline_revision"]
	if !found || len(values) != 1 || values[0] != strings.TrimSpace(values[0]) {
		return "", invalidDraftBaseline()
	}
	value := values[0]
	if value == "none" {
		return "", nil
	}
	if !domain.ValidContentHash(value) {
		return "", invalidDraftBaseline()
	}
	return value, nil
}

func invalidDraftBaseline() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_baseline_revision",
		message: "exactly one baseline_revision query parameter containing none or a revision hash is required",
	}
}

func parseIdempotencyKey(request *http.Request) (string, *requestProblem) {
	values := request.Header.Values("Idempotency-Key")
	if len(values) == 0 {
		return "", &requestProblem{
			status: http.StatusBadRequest, code: "idempotency_key_required",
			message: "Idempotency-Key is required to create a draft",
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

func decodePlanRequest(body []byte) (createPlanRequest, *requestProblem) {
	var value createPlanRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || strings.TrimSpace(value.ValidationID) == "" {
		return createPlanRequest{}, invalidPlanBody()
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return createPlanRequest{}, invalidPlanBody()
	}
	return value, nil
}

func invalidPlanBody() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_plan_request",
		message: "request body must contain only one non-empty validation_id",
	}
}

func requireEmptyBody(writer http.ResponseWriter, request *http.Request) *requestProblem {
	if request.ContentLength > 0 {
		return unexpectedRequestBody()
	}
	request.Body = http.MaxBytesReader(writer, request.Body, 1)
	body, err := io.ReadAll(request.Body)
	if err != nil || len(body) != 0 {
		return unexpectedRequestBody()
	}
	return nil
}

func unexpectedRequestBody() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "unexpected_request_body",
		message: "validation requests do not accept a request body",
	}
}
