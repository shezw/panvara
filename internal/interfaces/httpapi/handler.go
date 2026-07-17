/*
   Panvara
   internal/interfaces/httpapi/handler.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package httpapi adapts Panvara application use cases to the versioned HTTP
// API. It owns transport validation, authentication, and error mapping only.
package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shezw/panvara/internal/application/access"
	appmodule "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/application/record"
	"github.com/shezw/panvara/internal/domain/actor"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	coreModulePath  = "/api/core/v1alpha1/modules/{module}/{artifact}"
	publicDataPath  = "/api/public/v1alpha1/{module}/{resource}"
	adminDataPath   = "/api/admin/v1alpha1/{module}/{resource}"
	adminRecordPath = "/api/admin/v1alpha1/{module}/{resource}/{id}"
)

// Module exposes immutable artifacts needed by the HTTP adapter.
type Module interface {
	Name() string
	RevisionHash() string
	Descriptor() domainmodule.Descriptor
	ManagerUISchema() []byte
	OpenAPI() []byte
}

// RecordService is the model-driven CRUD use-case boundary consumed by HTTP.
type RecordService interface {
	Create(context.Context, access.Execution, record.Scope, json.RawMessage) (record.Record, error)
	Get(context.Context, access.Execution, record.Scope, record.ID) (record.Record, error)
	List(context.Context, access.Execution, record.Scope, record.ListOptions) (record.ListResult, error)
	Update(context.Context, access.Execution, record.Scope, record.ID, uint64, json.RawMessage) (record.Record, error)
	Delete(context.Context, access.Execution, record.Scope, record.ID, uint64) (record.Record, error)
}

// Config contains the complete single-project composition for one API router.
// Actor contexts are explicit even though alpha.2 only ships anonymous public
// access and one bootstrap project owner.
type Config struct {
	Project     project.Context
	Scope       project.Scope
	PublicActor actor.Context
	Module      Module
	Records     RecordService
	Revisions   RevisionRegistryService
	Drafts      DraftWorkflowService
	AdminAuth   *BootstrapAdminAuth
}

// Handler exposes generated schema and record APIs for one compiled module.
type Handler struct {
	project        project.Context
	executionScope project.Scope
	publicActor    actor.Context
	adminActor     actor.Context
	module         Module
	records        RecordService
	revisions      RevisionRegistryService
	drafts         DraftWorkflowService
	resources      map[string]resourcePolicy
	router         http.Handler
}

type resourcePolicy struct {
	public map[domainmodule.Operation]struct{}
	admin  map[domainmodule.Operation]struct{}
}

// New validates a complete API composition and constructs an immutable router.
func New(config Config) (*Handler, error) {
	if !config.Project.ID().Valid() {
		return nil, fmt.Errorf("http API project context is invalid")
	}
	if err := validateActors(config); err != nil {
		return nil, err
	}
	if err := config.Scope.Validate(); err != nil ||
		config.Scope.ProjectID().String() != config.Project.ID().String() {
		return nil, fmt.Errorf("http API project/environment scope is invalid")
	}
	if config.Module == nil {
		return nil, fmt.Errorf("http API module is nil")
	}
	if config.Records == nil {
		return nil, fmt.Errorf("http API record service is nil")
	}
	if config.AdminAuth == nil {
		return nil, fmt.Errorf("http API administrator authentication is nil")
	}

	handler := &Handler{
		project: config.Project, executionScope: config.Scope,
		publicActor: config.PublicActor, adminActor: config.AdminAuth.actor,
		module: config.Module, records: config.Records, revisions: config.Revisions, drafts: config.Drafts,
		resources: makeResourcePolicies(config.Module.Descriptor()),
	}
	mux := http.NewServeMux()
	mux.HandleFunc(coreModulePath, handler.handleModuleArtifact)
	mux.Handle(publicDataPath, handler.withIdentity(record.SurfacePublic, http.HandlerFunc(handler.handlePublicCollection)))
	adminCollection := handler.withIdentity(record.SurfaceAdmin, http.HandlerFunc(handler.handleAdminCollection))
	adminRecord := handler.withIdentity(record.SurfaceAdmin, http.HandlerFunc(handler.handleAdminRecord))
	mux.Handle(adminDataPath, config.AdminAuth.Middleware(adminCollection))
	mux.Handle(adminRecordPath, config.AdminAuth.Middleware(adminRecord))
	if config.Revisions != nil {
		revisionCollection := handler.withIdentity(record.SurfaceAdmin, http.HandlerFunc(handler.handleRevisionCollection))
		revisionItem := handler.withIdentity(record.SurfaceAdmin, http.HandlerFunc(handler.handleRevisionItem))
		revisionSource := handler.withIdentity(record.SurfaceAdmin, http.HandlerFunc(handler.handleRevisionSource))
		mux.Handle(adminRevisionCollectionPath, config.AdminAuth.Middleware(revisionCollection))
		mux.Handle(adminRevisionItemPath, config.AdminAuth.Middleware(revisionItem))
		mux.Handle(adminRevisionSourcePath, config.AdminAuth.Middleware(revisionSource))
	}
	if config.Drafts != nil {
		handler.registerDraftRoutes(mux, config.AdminAuth)
	}
	mux.HandleFunc("/", handler.handleNotFound)
	handler.router = withRequestID(mux)
	return handler, nil
}

// ServeHTTP implements http.Handler.
func (handler *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	handler.router.ServeHTTP(writer, request)
}

func validateActors(config Config) error {
	projectID := config.Project.ID().String()
	if !config.PublicActor.Valid() || !config.PublicActor.Anonymous() {
		return fmt.Errorf("http API public actor must be a valid anonymous actor")
	}
	if config.PublicActor.ProjectID().String() != projectID {
		return fmt.Errorf("http API public actor belongs to another project")
	}
	if config.AdminAuth == nil {
		return fmt.Errorf("http API administrator authentication is nil")
	}
	adminActor := config.AdminAuth.actor
	if !adminActor.Valid() || adminActor.Anonymous() {
		return fmt.Errorf("http API administrator actor must be authenticated")
	}
	if adminActor.ProjectID().String() != projectID {
		return fmt.Errorf("http API administrator actor belongs to another project")
	}
	return nil
}

func makeResourcePolicies(descriptor domainmodule.Descriptor) map[string]resourcePolicy {
	result := make(map[string]resourcePolicy, len(descriptor.Resources))
	for _, resource := range descriptor.Resources {
		result[resource.Name] = resourcePolicy{
			public: operationSet(resource.API.Public.Operations),
			admin:  operationSet(resource.API.Admin.Operations),
		}
	}
	return result
}

func operationSet(operations []domainmodule.Operation) map[domainmodule.Operation]struct{} {
	result := make(map[domainmodule.Operation]struct{}, len(operations))
	for _, operation := range operations {
		result[operation] = struct{}{}
	}
	return result
}

func (handler *Handler) operationAllowed(resourceName string, surface record.Surface, operation domainmodule.Operation) bool {
	policy, exists := handler.resources[resourceName]
	if !exists {
		return false
	}
	var operations map[domainmodule.Operation]struct{}
	switch surface {
	case record.SurfacePublic:
		operations = policy.public
	case record.SurfaceAdmin:
		operations = policy.admin
	default:
		return false
	}
	_, exists = operations[operation]
	return exists
}

func (handler *Handler) scope(request *http.Request) (record.Scope, error) {
	return record.NewScope(
		handler.project.ID(), request.PathValue("module"), request.PathValue("resource"), handler.module.RevisionHash(),
	)
}

func (handler *Handler) handleModuleArtifact(writer http.ResponseWriter, request *http.Request) {
	if !requireMethod(writer, request, http.MethodGet) {
		return
	}
	if request.PathValue("module") != handler.module.Name() {
		writeError(writer, request, http.StatusNotFound, "not_found", "module not found", nil)
		return
	}
	var artifact []byte
	switch request.PathValue("artifact") {
	case "openapi.json":
		artifact = handler.module.OpenAPI()
	case "ui-schema.json":
		artifact = handler.module.ManagerUISchema()
	default:
		writeError(writer, request, http.StatusNotFound, "not_found", "module artifact not found", nil)
		return
	}
	etag := formatArtifactETag(artifact)
	writer.Header().Set("ETag", etag)
	writer.Header().Set("Cache-Control", "no-cache")
	if request.Header.Get("If-None-Match") == etag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(artifact)
}

func (handler *Handler) handleNotFound(writer http.ResponseWriter, request *http.Request) {
	writeError(writer, request, http.StatusNotFound, "not_found", "route not found", nil)
}

func requireMethod(writer http.ResponseWriter, request *http.Request, methods ...string) bool {
	for _, method := range methods {
		if request.Method == method {
			return true
		}
	}
	writer.Header().Set("Allow", joinMethods(methods))
	writeError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	return false
}

func joinMethods(methods []string) string {
	result := ""
	for index, method := range methods {
		if index > 0 {
			result += ", "
		}
		result += method
	}
	return result
}

var _ Module = (*appmodule.CompiledModule)(nil)
var _ RecordService = (*record.Service)(nil)
