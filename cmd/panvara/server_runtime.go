/*
   Panvara
   cmd/panvara/server_runtime.go    2026-07-14
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
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
	"github.com/shezw/panvara/internal/infrastructure/postgres"
	"github.com/shezw/panvara/internal/interfaces/httpapi"
	"github.com/shezw/panvara/internal/interfaces/httpserver"
)

const maxModuleSourceBytes int64 = 1 << 20

type applicationRuntime struct {
	handler  http.Handler
	module   string
	revision string
	ready    httpserver.StatusSource
	close    func()
}

func (runtime *applicationRuntime) Close() {
	if runtime != nil && runtime.close != nil {
		runtime.close()
	}
}

type applicationBuilder func(context.Context, serverConfig) (*applicationRuntime, error)

func buildServerApplication(ctx context.Context, config serverConfig) (*applicationRuntime, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	format, err := parseModuleFormat(config.moduleFormat, config.moduleSource)
	if err != nil {
		return nil, err
	}
	source, err := readModuleSource(config.moduleSource)
	if err != nil {
		return nil, fmt.Errorf("read AppModule source %q: %w", config.moduleSource, err)
	}
	compiled, err := appmodule.NewCompiler().Compile(source, format)
	if err != nil {
		return nil, fmt.Errorf("compile AppModule source %q: %w", config.moduleSource, err)
	}
	catalog, err := appmodule.NewCatalog(compiled)
	if err != nil {
		return nil, fmt.Errorf("register AppModule catalog: %w", err)
	}
	module, found := catalog.Lookup(compiled.Name())
	if !found {
		return nil, fmt.Errorf("register AppModule catalog: compiled module is missing after registration")
	}

	projectContext, err := project.NewContext(
		config.projectID, config.projectKey, config.projectLocale, config.projectZone, config.projectMoney,
	)
	if err != nil {
		return nil, fmt.Errorf("construct project context: %w", err)
	}
	publicActor, err := actor.NewAnonymous(projectContext.ID().String())
	if err != nil {
		return nil, fmt.Errorf("construct public actor context: %w", err)
	}
	adminActor, err := actor.New(
		projectContext.ID().String(), "bootstrap-admin", []string{"project.owner"},
	)
	if err != nil {
		return nil, fmt.Errorf("construct bootstrap administrator context: %w", err)
	}
	auth, err := httpapi.NewBootstrapAdminAuth(config.adminToken, adminActor)
	if err != nil {
		return nil, fmt.Errorf("construct bootstrap administrator authentication: %w", err)
	}

	pool, err := postgres.Open(ctx, config.databaseURL)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			pool.Close()
		}
	}()
	if err := postgres.Migrate(ctx, pool); err != nil {
		return nil, err
	}
	store, err := postgres.NewStore(pool)
	if err != nil {
		return nil, err
	}
	validator, err := record.NewCompiledModuleValidator(module)
	if err != nil {
		return nil, err
	}
	records, err := record.NewDefaultService(store, validator)
	if err != nil {
		return nil, err
	}
	router, err := httpapi.New(httpapi.Config{
		Project: projectContext, PublicActor: publicActor, Module: module, Records: records, AdminAuth: auth,
	})
	if err != nil {
		return nil, err
	}
	success = true
	return &applicationRuntime{
		handler: router, module: module.Name(), revision: module.RevisionHash(),
		ready: &databaseReadiness{pool: pool, timeout: 500 * time.Millisecond}, close: pool.Close,
	}, nil
}

type databaseReadiness struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func (readiness *databaseReadiness) Ready() bool {
	if readiness == nil || readiness.pool == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), readiness.timeout)
	defer cancel()
	return readiness.pool.Ping(ctx) == nil
}

func readModuleSource(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open AppModule source %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat AppModule source %q: %w", path, err)
	}
	if info.Size() > maxModuleSourceBytes {
		return nil, fmt.Errorf("AppModule source %q exceeds %d bytes", path, maxModuleSourceBytes)
	}
	source, err := io.ReadAll(io.LimitReader(file, maxModuleSourceBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read AppModule source %q: %w", path, err)
	}
	if int64(len(source)) > maxModuleSourceBytes {
		return nil, fmt.Errorf("AppModule source %q exceeds %d bytes", path, maxModuleSourceBytes)
	}
	return source, nil
}
