/*
   Panvara
   internal/application/appmodule/draft_types.go    2026-07-16
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package appmodule

import (
	"context"
	"errors"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	// DraftValidationFormatVersion identifies the immutable validation snapshot contract.
	DraftValidationFormatVersion = 1
	// DraftPlanFormatVersion identifies the deterministic change-plan contract.
	DraftPlanFormatVersion = 1
	// MaxValidationIssues bounds diagnostic amplification from untrusted source.
	MaxValidationIssues = 100
	// MaxValidationIssuesBytes bounds persisted canonical issue JSON.
	MaxValidationIssuesBytes = 256 << 10
	// MaxDraftPlanBytes bounds persisted canonical plan JSON.
	MaxDraftPlanBytes = 16 << 20
)

var (
	// ErrDraftInvalid reports malformed draft input or a corrupt application result.
	ErrDraftInvalid = errors.New("invalid module draft request")
	// ErrDraftForbidden aliases the shared access denial for compatibility.
	ErrDraftForbidden = access.ErrForbidden
	// ErrDraftNotFound reports an absent project/module/draft identity.
	ErrDraftNotFound = errors.New("module draft not found")
	// ErrDraftConflict reports an optimistic-concurrency generation mismatch.
	ErrDraftConflict = errors.New("module draft generation conflict")
	// ErrDraftIdempotencyConflict reports reuse of a key for a different create intent.
	ErrDraftIdempotencyConflict = errors.New("module draft idempotency conflict")
	// ErrDraftCorrupt reports persisted state that fails identity verification.
	ErrDraftCorrupt = errors.New("module draft storage is corrupt")
	// ErrValidationNotFound reports an absent exact immutable validation snapshot.
	ErrValidationNotFound = errors.New("module draft validation not found")
	// ErrValidationInvalid reports an attempt to plan from a failed validation.
	ErrValidationInvalid = errors.New("module draft validation failed")
	// ErrPlanNotFound reports an absent exact immutable plan snapshot.
	ErrPlanNotFound = errors.New("module draft plan not found")
)

// DraftStore persists mutable drafts and immutable validation/plan snapshots.
type DraftStore interface {
	Create(context.Context, domain.Draft, string, string) (domain.Draft, bool, error)
	Get(context.Context, project.ID, string, domain.DraftID) (domain.Draft, error)
	Replace(context.Context, project.ID, string, domain.DraftID, uint64, domain.DraftReplacement) (domain.Draft, bool, error)
	SaveValidation(context.Context, DraftValidation) (DraftValidation, bool, error)
	GetValidation(context.Context, project.ID, string, domain.DraftID, string) (DraftValidation, error)
	SavePlan(context.Context, DraftPlan, uint64) (DraftPlan, bool, error)
	GetPlan(context.Context, project.ID, string, domain.DraftID, string) (DraftPlan, error)
}

// DraftRevisionReader returns a fully verified immutable baseline revision.
type DraftRevisionReader interface {
	Get(context.Context, access.Execution, string, string) (domain.Revision, error)
}

// DraftClock supplies deterministic workflow timestamps in tests.
type DraftClock interface{ Now() time.Time }

// DraftIDGenerator creates an opaque UUIDv7 draft identity.
type DraftIDGenerator interface {
	New(time.Time) (domain.DraftID, error)
}

// SystemDraftClock reads the host UTC clock.
type SystemDraftClock struct{}

// Now returns the current UTC timestamp.
func (SystemDraftClock) Now() time.Time { return time.Now().UTC() }

// CreateDraftInput contains author-controlled draft creation values.
type CreateDraftInput struct {
	BaselineRevision string
	Format           domain.SourceFormat
	Source           []byte
	IdempotencyKey   string
}

// ReplaceDraftInput contains a complete raw source replacement.
type ReplaceDraftInput struct {
	Format domain.SourceFormat
	Source []byte
}

// DraftSource is a generation-bound defensive source response.
type DraftSource struct {
	Format     domain.SourceFormat
	Hash       string
	Bytes      []byte
	Generation uint64
}

// ValidationIssue is a deterministic, machine-addressable diagnostic.
type ValidationIssue struct {
	Code     string `json:"code"`
	Stage    string `json:"stage"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// CandidateIdentity contains the compiled identities required by later planning.
type CandidateIdentity struct {
	RevisionHash          string `json:"revisionHash"`
	ModuleVersion         string `json:"moduleVersion"`
	DataSchemaFormat      int    `json:"dataSchemaFormat"`
	DataSchemaFingerprint string `json:"dataSchemaFingerprint"`
}

// DraftValidation is one immutable result bound to an exact draft generation.
type DraftValidation struct {
	ID               string
	ValidationHash   string
	FormatVersion    int
	ProjectID        project.ID
	ModuleName       string
	DraftID          domain.DraftID
	Generation       uint64
	BaselineRevision string
	SourceFormat     domain.SourceFormat
	SourceHash       string
	Valid            bool
	Issues           []ValidationIssue
	IssuesHash       string
	Candidate        *CandidateIdentity
	CandidateIR      []byte
	CreatedBy        string
	CreatedAt        time.Time
	Stale            bool
}

// PlanChange describes one deterministic semantic difference.
type PlanChange struct {
	Code              string `json:"code"`
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Risk              string `json:"risk"`
	Impact            string `json:"impact"`
	RequiresMigration bool   `json:"requiresMigration"`
	Before            string `json:"before,omitempty"`
	After             string `json:"after,omitempty"`
}

// PlanRiskSummary provides bounded counts for machines and user interfaces.
type PlanRiskSummary struct {
	Low         int `json:"low"`
	Review      int `json:"review"`
	Destructive int `json:"destructive"`
}

// DraftPlan is an immutable, deterministic analysis result; it executes nothing.
type DraftPlan struct {
	ID                            string
	PlanHash                      string
	FormatVersion                 int
	ProjectID                     project.ID
	ModuleName                    string
	DraftID                       domain.DraftID
	DraftGeneration               uint64
	ValidationID                  string
	BaselineRevision              string
	BaselineDataSchemaFormat      int
	BaselineDataSchemaFingerprint string
	SourceFormat                  domain.SourceFormat
	SourceHash                    string
	Candidate                     CandidateIdentity
	Changes                       []PlanChange
	RiskSummary                   PlanRiskSummary
	Outcome                       string
	Risk                          string
	DataSchemaChanged             bool
	RecordNamespaceChanged        bool
	MigrationExecutionSupported   bool
	CreatedBy                     string
	CreatedAt                     time.Time
	Stale                         bool
}
