/*
   Panvara
   internal/interfaces/httpserver/server.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package httpserver exposes Core operational endpoints over net/http.
package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shezw/panvara/internal/buildinfo"
)

const (
	componentName            = "interfaces.http"
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 30 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultMaxHeaderBytes    = 1 << 20
)

// StatusSource supplies Core readiness without coupling this interface to a
// concrete application implementation.
type StatusSource interface {
	Ready() bool
}

// Server is both an HTTP adapter and a Kernel lifecycle component.
type Server struct {
	mu       sync.RWMutex
	address  string
	status   StatusSource
	info     buildinfo.Info
	handler  http.Handler
	server   *http.Server
	listener net.Listener
	errors   chan error
}

// New creates an operational server. The listener is opened by Start.
func New(address string, status StatusSource, info buildinfo.Info) *Server {
	return NewWithHandler(address, status, info, nil)
}

// NewWithHandler creates a server that combines operational routes with an
// externally assembled application router. The listener is opened by Start.
func NewWithHandler(
	address string,
	status StatusSource,
	info buildinfo.Info,
	handler http.Handler,
) *Server {
	return &Server{
		address: address,
		status:  status,
		info:    info,
		handler: handler,
		errors:  make(chan error, 1),
	}
}

// Name implements kernel.Component.
func (server *Server) Name() string {
	return componentName
}

// Handler returns the testable HTTP routing surface.
func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /readyz", server.handleReady)
	mux.HandleFunc("GET /version", server.handleVersion)
	if server.handler != nil {
		mux.Handle("/", server.handler)
	}
	return mux
}

// Start opens the configured listener and serves in the background.
func (server *Server) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start http server: %w", err)
	}
	if strings.TrimSpace(server.address) == "" {
		return fmt.Errorf("http listen address is empty")
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.listener != nil {
		return fmt.Errorf("http server is already started")
	}
	listener, err := net.Listen("tcp", server.address)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", server.address, err)
	}
	httpServer := server.newHTTPServer()
	server.listener = listener
	server.server = httpServer

	go func() {
		if serveErr := httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			select {
			case server.errors <- serveErr:
			default:
			}
		}
	}()
	return nil
}

func (server *Server) newHTTPServer() *http.Server {
	return &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		ReadTimeout:       defaultReadTimeout,
		WriteTimeout:      defaultWriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
	}
}

// Stop gracefully shuts down the HTTP listener.
func (server *Server) Stop(ctx context.Context) error {
	server.mu.RLock()
	httpServer := server.server
	server.mu.RUnlock()
	if httpServer == nil {
		return nil
	}
	if err := httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}
	return nil
}

// Addr returns the bound address after Start.
func (server *Server) Addr() string {
	server.mu.RLock()
	defer server.mu.RUnlock()
	if server.listener == nil {
		return ""
	}
	return server.listener.Addr().String()
}

// Errors reports asynchronous serving failures.
func (server *Server) Errors() <-chan error {
	return server.errors
}

func (server *Server) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "alive"})
}

func (server *Server) handleReady(writer http.ResponseWriter, _ *http.Request) {
	if server.status == nil || !server.status.Ready() {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (server *Server) handleVersion(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, server.info)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
