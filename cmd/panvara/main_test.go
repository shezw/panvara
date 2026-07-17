/*
   Panvara
   cmd/panvara/main_test.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/shezw/panvara/internal/buildinfo"
	"github.com/shezw/panvara/internal/interfaces/httpserver"
	"github.com/shezw/panvara/internal/runtime/kernel"
)

func TestExecuteVersion(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := execute(context.Background(), []string{"--version"}, &output, emptyEnvironment); err != nil {
		t.Fatal(err)
	}
	var info buildinfo.Info
	if err := json.NewDecoder(&output).Decode(&info); err != nil {
		t.Fatal(err)
	}
	if info.Distribution != buildinfo.DistributionVersion {
		t.Fatalf("distribution = %q, want %q", info.Distribution, buildinfo.DistributionVersion)
	}
}

func TestExecuteRejectsUnknownProfile(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := execute(context.Background(), []string{"--profile=unknown"}, &output, emptyEnvironment)
	if err == nil || !strings.Contains(err.Error(), "unknown profile") {
		t.Fatalf("execute() error = %v, want unknown profile", err)
	}
}

func TestExecuteRejectsProfileWithoutRunnableComposition(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := execute(context.Background(), []string{"--profile=site"}, &output, emptyEnvironment)
	if err == nil || !strings.Contains(err.Error(), "has no runnable composition") {
		t.Fatalf("execute() error = %v, want planned profile error", err)
	}
}

func TestExecuteHelpIsSuccessful(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := execute(context.Background(), []string{"--help"}, &output, emptyEnvironment); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if !strings.Contains(output.String(), "Usage of panvara") {
		t.Fatalf("help output = %q", output.String())
	}
}

func TestExecuteRejectsPositionalArguments(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := execute(context.Background(), []string{"serve"}, &output, emptyEnvironment)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("execute() error = %v, want positional argument error", err)
	}
}

func TestExecuteBootsAndStopsLiteProfile(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	server := &fakeRuntimeServer{errors: make(chan error)}
	err := executeWithServerFactory(
		ctx,
		[]string{"--profile=lite", "--http=127.0.0.1:0"},
		&output,
		emptyEnvironment,
		func(string, httpserver.StatusSource, buildinfo.Info, http.Handler) runtimeServer {
			return server
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "profile=lite") {
		t.Fatalf("output = %q", output.String())
	}
	if !server.started || !server.stopped {
		t.Fatalf("server lifecycle: started=%v stopped=%v", server.started, server.stopped)
	}
}

func TestExecuteStopsCoreAfterRuntimeFailure(t *testing.T) {
	t.Parallel()

	runtimeErr := errors.New("listener failed")
	server := &fakeRuntimeServer{errors: make(chan error, 1)}
	server.errors <- runtimeErr
	var output bytes.Buffer
	err := executeWithServerFactory(
		context.Background(),
		[]string{"--profile=lite"},
		&output,
		emptyEnvironment,
		func(string, httpserver.StatusSource, buildinfo.Info, http.Handler) runtimeServer {
			return server
		},
	)
	if !errors.Is(err, runtimeErr) {
		t.Fatalf("execute() error = %v, want runtime failure", err)
	}
	if !server.stopped {
		t.Fatal("runtime failure did not stop the Core")
	}
}

func TestExecuteServerProfileAssemblesApplicationRouter(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := &fakeRuntimeServer{errors: make(chan error)}
	applicationClosed := false
	router := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	var output bytes.Buffer
	err := executeWithFactories(
		ctx,
		[]string{
			"--profile=server",
			"--database-url=postgres://redacted",
			"--module-source=crm.yaml",
			"--project-id=018f7e93-7b2c-7abc-8def-1234567890ab",
			"--admin-token=panvara-test-token-32-bytes-minimum",
		},
		&output,
		emptyEnvironment,
		func(_ string, _ httpserver.StatusSource, _ buildinfo.Info, handler http.Handler) runtimeServer {
			if handler == nil {
				t.Fatal("server received a nil application handler")
			}
			return server
		},
		func(_ context.Context, config serverConfig) (*applicationRuntime, error) {
			if config.databaseURL != "postgres://redacted" || config.moduleSource != "crm.yaml" ||
				config.projectID != "018f7e93-7b2c-7abc-8def-1234567890ab" ||
				config.environmentKey != "default" {
				t.Fatalf("server config = %+v", config)
			}
			return &applicationRuntime{
				handler: router, module: "crm", revision: testRevisionHash, ready: readinessStatus(true),
				close: func() { applicationClosed = true },
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !applicationClosed || !server.started || !server.stopped {
		t.Fatalf("closed/started/stopped = %v/%v/%v", applicationClosed, server.started, server.stopped)
	}
	if !strings.Contains(output.String(), "profile=server") || !strings.Contains(output.String(), "module=crm") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestCombinedReadinessRequiresCoreAndDatabase(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		core       bool
		dependency bool
		want       bool
	}{
		{core: true, dependency: true, want: true},
		{core: true, dependency: false},
		{core: false, dependency: true},
	} {
		got := (combinedReadiness{
			core: readinessStatus(test.core), dependency: readinessStatus(test.dependency),
		}).Ready()
		if got != test.want {
			t.Fatalf("combined readiness %v/%v = %v, want %v", test.core, test.dependency, got, test.want)
		}
	}
}

func TestExecuteServerProfileRequiresCompleteComposition(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := execute(context.Background(), []string{"--profile=server"}, &output, emptyEnvironment)
	if err == nil || !strings.Contains(err.Error(), "server profile requires") {
		t.Fatalf("execute() error = %v", err)
	}
	if strings.Contains(err.Error(), "postgres://") {
		t.Fatalf("error leaked a database URL: %v", err)
	}
}

func TestParseModuleFormat(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value string
		path  string
		want  string
	}{
		{value: "json", path: "model.unknown", want: "json"},
		{value: "yaml", path: "model.unknown", want: "yaml"},
		{value: "auto", path: "model.yml", want: "yaml"},
		{path: "model.json", want: "json"},
	} {
		format, err := parseModuleFormat(test.value, test.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(format) != test.want {
			t.Fatalf("parseModuleFormat(%q, %q) = %q", test.value, test.path, format)
		}
	}
	if _, err := parseModuleFormat("auto", "model.txt"); err == nil {
		t.Fatal("parseModuleFormat accepted an unknown extension")
	}
}

func emptyEnvironment(string) string {
	return ""
}

type fakeRuntimeServer struct {
	errors  chan error
	started bool
	stopped bool
}

func (server *fakeRuntimeServer) Name() string {
	return "fake.http"
}

func (server *fakeRuntimeServer) Start(context.Context) error {
	server.started = true
	return nil
}

func (server *fakeRuntimeServer) Stop(context.Context) error {
	server.stopped = true
	return nil
}

func (server *fakeRuntimeServer) Addr() string {
	return "127.0.0.1:0"
}

func (server *fakeRuntimeServer) Errors() <-chan error {
	return server.errors
}

var _ kernel.Component = (*fakeRuntimeServer)(nil)

const testRevisionHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type readinessStatus bool

func (status readinessStatus) Ready() bool { return bool(status) }
