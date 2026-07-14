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
	return executeWithServerFactory(
		ctx,
		args,
		stdout,
		getenv,
		func(address string, status httpserver.StatusSource, info buildinfo.Info) runtimeServer {
			return httpserver.New(address, status, info)
		},
	)
}

type runtimeServer interface {
	kernel.Component
	Addr() string
	Errors() <-chan error
}

type serverFactory func(string, httpserver.StatusSource, buildinfo.Info) runtimeServer

func executeWithServerFactory(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	getenv func(string) string,
	newServer serverFactory,
) error {
	flags := flag.NewFlagSet("panvara", flag.ContinueOnError)
	flags.SetOutput(stdout)
	showVersion := flags.Bool("version", false, "print version metadata and exit")
	profileValue := flags.String("profile", envOr(getenv, "PANVARA_PROFILE", "lite"), "runtime profile")
	address := flags.String("http", envOr(getenv, "PANVARA_HTTP_ADDR", "127.0.0.1:8080"), "HTTP listen address")
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
	if !definition.Implemented {
		return fmt.Errorf(
			"profile %q is planned but not implemented in Panvara %s",
			profileName,
			info.Distribution,
		)
	}

	core := kernel.New()
	server := newServer(*address, core, info)
	if err := core.Register(server); err != nil {
		return err
	}
	if err := core.Start(ctx); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "panvara %s profile=%s address=%s\n", info.Distribution, definition.Name, server.Addr())

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

func envOr(getenv func(string) string, key, fallback string) string {
	if value := getenv(key); value != "" {
		return value
	}
	return fallback
}
