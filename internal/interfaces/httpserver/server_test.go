/*
   Panvara
   internal/interfaces/httpserver/server_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shezw/panvara/internal/buildinfo"
)

func TestOperationalEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		ready      bool
		wantStatus int
		wantBody   string
	}{
		{name: "liveness", path: "/healthz", wantStatus: http.StatusOK, wantBody: "alive"},
		{name: "not ready", path: "/readyz", wantStatus: http.StatusServiceUnavailable, wantBody: "not_ready"},
		{name: "ready", path: "/readyz", ready: true, wantStatus: http.StatusOK, wantBody: "ready"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := New("127.0.0.1:0", staticStatus(test.ready), buildinfo.Current())
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["status"] != test.wantBody {
				t.Fatalf("body status = %q, want %q", body["status"], test.wantBody)
			}
			if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
				t.Fatalf("Content-Type = %q", contentType)
			}
		})
	}
}

func TestVersionEndpointReturnsIndependentAxes(t *testing.T) {
	t.Parallel()

	want := buildinfo.Current()
	server := New("127.0.0.1:0", staticStatus(true), want)
	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	var got buildinfo.Info
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("version = %+v, want %+v", got, want)
	}
}

func TestOperationalEndpointsRejectOtherMethods(t *testing.T) {
	t.Parallel()

	server := New("127.0.0.1:0", staticStatus(true), buildinfo.Current())
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func TestStartRejectsCanceledContextWithoutListening(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := New("127.0.0.1:0", staticStatus(true), buildinfo.Current())
	if err := server.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	if server.Addr() != "" {
		t.Fatalf("server unexpectedly listened on %q", server.Addr())
	}
}

type staticStatus bool

func (status staticStatus) Ready() bool {
	return bool(status)
}
