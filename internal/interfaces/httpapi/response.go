/*
   Panvara
   internal/interfaces/httpapi/response.go    2026-07-14
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
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

const (
	requestIDHeader  = "X-Request-ID"
	maxRequestIDSize = 128
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
var fallbackRequestSequence atomic.Uint64

type requestIDContextKey struct{}

// ErrorEnvelope is the stable error response shared by every Panvara HTTP API.
type ErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Details   any    `json:"details,omitempty"`
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := normalizedRequestID(request.Header.Get(requestIDHeader))
		writer.Header().Set(requestIDHeader, requestID)
		ctx := context.WithValue(request.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func requestID(request *http.Request) string {
	if request != nil {
		if value, ok := request.Context().Value(requestIDContextKey{}).(string); ok && value != "" {
			return value
		}
		if value := normalizedRequestID(request.Header.Get(requestIDHeader)); value != "" {
			return value
		}
	}
	return newRequestID()
}

func normalizedRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRequestIDSize || !requestIDPattern.MatchString(value) {
		return newRequestID()
	}
	return value
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return "req_" + hex.EncodeToString(bytes)
	}
	sequence := fallbackRequestSequence.Add(1)
	return fmt.Sprintf("req_%x_%x", time.Now().UnixNano(), sequence)
}

func writeError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
	details any,
) {
	writeJSON(writer, status, ErrorEnvelope{
		Code: code, Message: message, RequestID: requestID(request), Details: details,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
