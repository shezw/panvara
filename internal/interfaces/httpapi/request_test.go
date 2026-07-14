/*
   Panvara
   internal/interfaces/httpapi/request_test.go    2026-07-14
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadJSONBodyContract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		contentType string
		body        []byte
		wantStatus  int
	}{
		{name: "json", contentType: "application/json; charset=utf-8", body: []byte(`{"name":"Ada"}`)},
		{name: "missing type", body: []byte(`{}`), wantStatus: http.StatusUnsupportedMediaType},
		{name: "wrong type", contentType: "text/plain", body: []byte(`{}`), wantStatus: http.StatusUnsupportedMediaType},
		{name: "empty", contentType: "application/json", body: []byte(" \n"), wantStatus: http.StatusBadRequest},
		{name: "too large", contentType: "application/json", body: bytes.Repeat([]byte("x"), int(maxJSONBodyBytes)+1), wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			body, problem := readJSONBody(response, request)
			if test.wantStatus == 0 {
				if problem != nil || string(body) != string(test.body) {
					t.Fatalf("body = %q, problem = %+v", body, problem)
				}
				return
			}
			if problem == nil || problem.status != test.wantStatus {
				t.Fatalf("problem = %+v, want status %d", problem, test.wantStatus)
			}
		})
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	t.Parallel()
	handler := withRequestID(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeError(writer, request, http.StatusBadRequest, "bad_request", "bad request", nil)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, "client-request-42")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if got := response.Header().Get(requestIDHeader); got != "client-request-42" {
		t.Fatalf("X-Request-ID = %q", got)
	}
	if !strings.Contains(response.Body.String(), `"request_id":"client-request-42"`) {
		t.Fatalf("body = %q", response.Body.String())
	}
}
