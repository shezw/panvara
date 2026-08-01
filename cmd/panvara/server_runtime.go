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
	"github.com/shezw/panvara/internal/application/access"
	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	"github.com/shezw/panvara/internal/domain/actor"
	"github.com/shezw/panvara/internal/domain/project"
	"github.com/shezw/panvara/internal/infrastructure/postgres"
	"github.com/shezw/panvara/internal/interfaces/httpapi"
	"github.com/shezw/panvara/internal/interfaces/httpserver"
	"github.com/shezw/panvara/internal/runtime/modelruntime"
)

const maxModuleSourceBytes int64 = 1 << 20
const bootstrapAdminPrincipal = "bootstrap-admin"

type applicationRuntime struct {
	handler  http.Handler
	module   string
	revision string
	epoch    uint64
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
	projectAccessStore, err := postgres.NewProjectAccessStore(pool)
	if err != nil {
		return nil, err
	}
	executionScope, err := projectAccessStore.EnsureBootstrapScope(
		ctx, projectContext, config.environmentKey, bootstrapAdminPrincipal,
	)
	if err != nil {
		return nil, fmt.Errorf("ensure bootstrap project access scope: %w", err)
	}
	accessStore, err := postgres.NewAccessAdminStore(pool)
	if err != nil {
		return nil, err
	}
	bootstrapCredentials, err := access.NewDefaultBootstrapCredentialRegistrar(accessStore)
	if err != nil {
		return nil, fmt.Errorf("construct bootstrap credential registrar: %w", err)
	}
	if _, err := bootstrapCredentials.Register(
		ctx, executionScope, bootstrapAdminPrincipal, config.adminToken,
	); err != nil {
		return nil, fmt.Errorf("ensure bootstrap administrator credential: %w", err)
	}
	authenticator, err := access.NewCredentialAuthenticator(accessStore)
	if err != nil {
		return nil, fmt.Errorf("construct credential authenticator: %w", err)
	}
	auth, err := httpapi.NewCredentialAdminAuth(executionScope, authenticator)
	if err != nil {
		return nil, fmt.Errorf("construct administrator authentication: %w", err)
	}
	authorizer, err := access.NewPolicy(projectAccessStore)
	if err != nil {
		return nil, fmt.Errorf("construct application access policy: %w", err)
	}
	accessAdministration, err := access.NewDefaultAdministration(
		accessStore, authorizer, accessStore,
	)
	if err != nil {
		return nil, fmt.Errorf("construct access administration: %w", err)
	}
	revisionStore, err := postgres.NewRevisionStore(pool)
	if err != nil {
		return nil, err
	}
	revisions, err := appmodule.NewRevisionRegistry(
		revisionStore, authorizer, appmodule.SystemRevisionClock{},
	)
	if err != nil {
		return nil, err
	}
	if _, _, err := revisions.RegisterBootstrap(
		ctx, projectContext.ID(), module, source, format,
	); err != nil {
		return nil, fmt.Errorf("register bootstrap AppModule revision: %w", err)
	}
	draftStore, err := postgres.NewDraftStore(pool)
	if err != nil {
		return nil, err
	}
	drafts, err := appmodule.NewDraftWorkflow(
		draftStore, revisions, authorizer, appmodule.SystemDraftClock{},
		appmodule.NewDefaultDraftUUIDv7Generator(),
	)
	if err != nil {
		return nil, err
	}
	releaseStore, err := postgres.NewReleaseStore(pool)
	if err != nil {
		return nil, err
	}
	releases, err := composeReleasePublisher(releaseStore, authorizer, accessStore)
	if err != nil {
		return nil, err
	}
	recordStore, err := postgres.NewStore(pool)
	if err != nil {
		return nil, err
	}
	active, _, err := releaseStore.EnsureBootstrap(
		ctx, executionScope, module.Name(), module.RevisionHash(), time.Now().UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("ensure bootstrap active Runtime Snapshot: %w", err)
	}
	activeRevision, err := revisionStore.Get(
		ctx, projectContext.ID(), active.ModuleName(), active.RuntimeRevision(),
	)
	if err != nil {
		return nil, fmt.Errorf("load active Runtime Revision: %w", err)
	}
	var activations httpapi.ReleaseActivationService
	modelRuntime, err := modelruntime.New(modelruntime.BuilderFunc(func(
		buildContext context.Context,
		activeModule *appmodule.CompiledModule,
		recordNamespace string,
	) (http.Handler, error) {
		if err := buildContext.Err(); err != nil {
			return nil, err
		}
		if activations == nil {
			return nil, fmt.Errorf("release activation service is not composed")
		}
		validator, err := record.NewCompiledModuleValidatorForNamespace(activeModule, recordNamespace)
		if err != nil {
			return nil, err
		}
		records, err := record.NewDefaultService(recordStore, validator, authorizer)
		if err != nil {
			return nil, err
		}
		return httpapi.New(httpapi.Config{
			Project: projectContext, Scope: executionScope,
			PublicActor: publicActor, Module: activeModule,
			RecordNamespaceRevision: recordNamespace, Records: records,
			Revisions: revisions, Drafts: drafts, Releases: releases, Activations: activations,
			AdminAuth: auth, AccessAdministration: accessAdministration,
		})
	}))
	if err != nil {
		return nil, err
	}
	activator, err := releaseapp.NewDefaultActivator(
		releaseStore, releaseStore, revisionStore, modelRuntime, authorizer, accessStore,
	)
	if err != nil {
		return nil, fmt.Errorf("construct module release activator: %w", err)
	}
	activations = activator
	prepared, err := modelRuntime.Prepare(ctx, activeRevision, active.RecordNamespaceRevision())
	if err != nil {
		return nil, fmt.Errorf("prepare active Runtime Snapshot: %w", err)
	}
	if err := modelRuntime.Install(active, prepared); err != nil {
		return nil, fmt.Errorf("install active Runtime Snapshot: %w", err)
	}
	success = true
	return &applicationRuntime{
		handler: modelRuntime, module: active.ModuleName(), revision: active.RuntimeRevision(), epoch: active.Epoch(),
		ready: &runtimeReadiness{
			database: &databaseReadiness{pool: pool, timeout: 500 * time.Millisecond},
			runtime:  modelRuntime,
		},
		close: pool.Close,
	}, nil
}

func composeReleasePublisher(
	store *postgres.ReleaseStore,
	authorizer access.Authorizer,
	denied access.DeniedAuditor,
) (*releaseapp.Publisher, error) {
	publisher, err := releaseapp.NewDefaultPublisher(store, store, authorizer, denied)
	if err != nil {
		return nil, fmt.Errorf("construct module release publisher: %w", err)
	}
	return publisher, nil
}

type databaseReadiness struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

type runtimeReadiness struct {
	database httpserver.StatusSource
	runtime  *modelruntime.Runtime
}

func (readiness *runtimeReadiness) Ready() bool {
	return readiness != nil && readiness.database != nil && readiness.database.Ready() &&
		readiness.runtime != nil && readiness.runtime.Ready()
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
