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
	"io"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

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

func TestServerCombinesExternalHandlerWithoutShadowingOperations(t *testing.T) {
	t.Parallel()

	external := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/example" {
			http.NotFound(writer, request)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
	})
	server := NewWithHandler("127.0.0.1:0", staticStatus(true), buildinfo.Current(), external)

	for _, test := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/healthz", wantStatus: http.StatusOK},
		{path: "/api/example", wantStatus: http.StatusAccepted},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != test.wantStatus {
			t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, test.wantStatus)
		}
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

func TestHTTPServerHasBoundedResourceDefaults(t *testing.T) {
	t.Parallel()
	configured := New("127.0.0.1:0", staticStatus(true), buildinfo.Current()).newHTTPServer()
	if configured.ReadHeaderTimeout != defaultReadHeaderTimeout ||
		configured.ReadTimeout != defaultReadTimeout ||
		configured.WriteTimeout != defaultWriteTimeout ||
		configured.IdleTimeout != defaultIdleTimeout ||
		configured.MaxHeaderBytes != defaultMaxHeaderBytes {
		t.Fatalf(
			"HTTP limits = read-header:%s read:%s write:%s idle:%s headers:%d",
			configured.ReadHeaderTimeout,
			configured.ReadTimeout,
			configured.WriteTimeout,
			configured.IdleTimeout,
			configured.MaxHeaderBytes,
		)
	}
}

func TestServerRealListenerStartServeStop(t *testing.T) {
	t.Parallel()

	server := New("127.0.0.1:0", staticStatus(true), buildinfo.Current())
	if err := server.Start(context.Background()); err != nil {
		if errors.Is(err, syscall.EPERM) {
			t.Skipf("environment forbids loopback listeners: %v", err)
		}
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + server.Addr() + "/healthz")
	if err != nil {
		t.Fatalf("GET real listener: %v", err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	response, err = client.Get("http://" + server.Addr() + "/healthz")
	if err == nil {
		_ = response.Body.Close()
		t.Fatal("listener still accepted requests after Stop")
	}
}

type staticStatus bool

func (status staticStatus) Ready() bool {
	return bool(status)
}
