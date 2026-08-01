/*
   Panvara
   cmd/panvara/main.go    2026-07-14
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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/shezw/panvara/internal/bootstrap/profile"
	"github.com/shezw/panvara/internal/buildinfo"
	"github.com/shezw/panvara/internal/interfaces/httpserver"
	"github.com/shezw/panvara/internal/runtime/kernel"
)

const shutdownTimeout = 10 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := execute(ctx, os.Args[1:], os.Stdout, os.Getenv); err != nil {
		slog.Error("panvara stopped", "error", err)
		os.Exit(1)
	}
}

func execute(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	getenv func(string) string,
) error {
	return executeWithFactories(
		ctx,
		args,
		stdout,
		getenv,
		func(address string, status httpserver.StatusSource, info buildinfo.Info, handler http.Handler) runtimeServer {
			return httpserver.NewWithHandler(address, status, info, handler)
		},
		buildServerApplication,
	)
}

type runtimeServer interface {
	kernel.Component
	Addr() string
	Errors() <-chan error
}

type serverFactory func(string, httpserver.StatusSource, buildinfo.Info, http.Handler) runtimeServer

func executeWithServerFactory(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	getenv func(string) string,
	newServer serverFactory,
) error {
	return executeWithFactories(ctx, args, stdout, getenv, newServer, buildServerApplication)
}

func executeWithFactories(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	getenv func(string) string,
	newServer serverFactory,
	buildApplication applicationBuilder,
) error {
	flags := flag.NewFlagSet("panvara", flag.ContinueOnError)
	flags.SetOutput(stdout)
	showVersion := flags.Bool("version", false, "print version metadata and exit")
	profileValue := flags.String("profile", envOr(getenv, "PANVARA_PROFILE", "lite"), "runtime profile")
	address := flags.String("http", envOr(getenv, "PANVARA_HTTP_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	databaseURL := flags.String("database-url", envOr(getenv, "PANVARA_DATABASE_URL", ""), "PostgreSQL connection URL")
	moduleSource := flags.String("module-source", envOr(getenv, "PANVARA_MODULE_SOURCE", ""), "AppModule YAML or JSON file")
	moduleFormat := flags.String("module-format", envOr(getenv, "PANVARA_MODULE_FORMAT", "auto"), "AppModule format: auto, json, or yaml")
	projectID := flags.String("project-id", envOr(getenv, "PANVARA_PROJECT_ID", ""), "single-project UUIDv7")
	projectKey := flags.String("project-key", envOr(getenv, "PANVARA_PROJECT_KEY", "default"), "single-project readable key")
	projectLocale := flags.String("project-locale", envOr(getenv, "PANVARA_PROJECT_LOCALE", "en-US"), "single-project BCP 47 locale")
	projectZone := flags.String("project-time-zone", envOr(getenv, "PANVARA_PROJECT_TIME_ZONE", "UTC"), "single-project IANA time zone")
	projectMoney := flags.String("project-currency", envOr(getenv, "PANVARA_PROJECT_CURRENCY", "USD"), "single-project ISO currency")
	environmentKey := flags.String("environment-key", envOr(getenv, "PANVARA_ENVIRONMENT_KEY", "default"), "single-project default environment key")
	adminToken := flags.String(
		"admin-token",
		envOr(getenv, "PANVARA_ADMIN_TOKEN", ""),
		"first-start bootstrap credential token (optional after initialization; prefer PANVARA_ADMIN_TOKEN because CLI arguments are process-visible)",
	)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %q", flags.Args())
	}

	info := buildinfo.Current()
	if *showVersion {
		return json.NewEncoder(stdout).Encode(info)
	}

	profileName, err := profile.Parse(*profileValue)
	if err != nil {
		return err
	}
	definition, err := profile.DefinitionFor(profileName)
	if err != nil {
		return err
	}
	if !definition.Runnable {
		return fmt.Errorf(
			"profile %q has no runnable composition in Panvara %s",
			profileName,
			info.Distribution,
		)
	}

	var application *applicationRuntime
	switch profileName {
	case profile.Lite:
	case profile.Server:
		if buildApplication == nil {
			return fmt.Errorf("server profile application builder is nil")
		}
		application, err = buildApplication(ctx, serverConfig{
			databaseURL: *databaseURL, moduleSource: *moduleSource, moduleFormat: *moduleFormat,
			projectID: *projectID, projectKey: *projectKey, projectLocale: *projectLocale,
			projectZone: *projectZone, projectMoney: *projectMoney,
			environmentKey: *environmentKey, adminToken: *adminToken,
		})
		if err != nil {
			return fmt.Errorf("assemble server profile: %w", err)
		}
		if application == nil || application.handler == nil || application.ready == nil {
			if application != nil {
				application.Close()
			}
			return fmt.Errorf("assemble server profile: application router is nil")
		}
		defer application.Close()
	default:
		return fmt.Errorf("profile %q is marked runnable without a composition", profileName)
	}

	core := kernel.New()
	var applicationHandler http.Handler
	var readiness httpserver.StatusSource = core
	if application != nil {
		applicationHandler = application.handler
		readiness = combinedReadiness{core: core, dependency: application.ready}
	}
	server := newServer(*address, readiness, info, applicationHandler)
	if err := core.Register(server); err != nil {
		return err
	}
	if err := core.Start(ctx); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "panvara %s profile=%s address=%s\n", info.Distribution, definition.Name, server.Addr())
	if application != nil {
		fmt.Fprintf(
			stdout, "module=%s revision=%s epoch=%d\n",
			application.module, application.revision, application.epoch,
		)
	}

	var runErr error
	select {
	case <-ctx.Done():
	case serveErr, ok := <-server.Errors():
		if !ok {
			runErr = fmt.Errorf("http server error channel closed unexpectedly")
		} else if serveErr != nil {
			runErr = fmt.Errorf("http server: %w", serveErr)
		}
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return errors.Join(runErr, core.Stop(stopCtx))
}

type combinedReadiness struct {
	core       httpserver.StatusSource
	dependency httpserver.StatusSource
}

func (readiness combinedReadiness) Ready() bool {
	return readiness.core != nil && readiness.core.Ready() &&
		readiness.dependency != nil && readiness.dependency.Ready()
}

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}
