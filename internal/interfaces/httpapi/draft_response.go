/*
   Panvara
   internal/interfaces/httpapi/draft_response.go    2026-07-16
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"fmt"
	"net/http"
	"time"

	application "github.com/shezw/panvara/internal/application/appmodule"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

type draftResponse struct {
	DraftID          string              `json:"draft_id"`
	Module           string              `json:"module"`
	BaselineRevision *string             `json:"baseline_revision"`
	DraftVersion     uint64              `json:"draft_version"`
	SourceFormat     domain.SourceFormat `json:"source_format"`
	SourceHash       string              `json:"source_hash"`
	CreatedBy        string              `json:"created_by"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedBy        string              `json:"updated_by"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

type workflowEffectsResponse struct {
	PlanRecorded       bool `json:"plan_recorded,omitempty"`
	RevisionRegistered bool `json:"revision_registered"`
	Published          bool `json:"published"`
	Activated          bool `json:"activated"`
	RecordsMigrated    bool `json:"records_migrated"`
	RuntimeChanged     bool `json:"runtime_changed"`
}

type validationResponse struct {
	ValidationID       string                        `json:"validation_id"`
	ValidationFormat   int                           `json:"validation_format"`
	ValidationHash     string                        `json:"validation_hash"`
	DraftID            string                        `json:"draft_id"`
	DraftVersion       uint64                        `json:"draft_version"`
	BaselineRevision   *string                       `json:"baseline_revision"`
	SourceFormat       domain.SourceFormat           `json:"source_format"`
	SourceHash         string                        `json:"source_hash"`
	Valid              bool                          `json:"valid"`
	CandidateRevision  *string                       `json:"candidate_revision"`
	DataSchemaIdentity []dataSchemaIdentityResponse  `json:"data_schema_identities"`
	Violations         []application.ValidationIssue `json:"violations"`
	Warnings           []application.ValidationIssue `json:"warnings"`
	Stale              bool                          `json:"stale"`
	CreatedAt          time.Time                     `json:"created_at"`
	Effects            workflowEffectsResponse       `json:"effects"`
}

type planSummaryResponse struct {
	Classification string                      `json:"classification"`
	Risk           string                      `json:"risk"`
	ChangeCount    int                         `json:"change_count"`
	RiskCounts     application.PlanRiskSummary `json:"risk_counts"`
}

type planChangeResponse struct {
	Code              string `json:"code"`
	Path              string `json:"path"`
	Kind              string `json:"kind"`
	Risk              string `json:"risk"`
	Impact            string `json:"impact"`
	RequiresMigration bool   `json:"requires_migration"`
	Before            string `json:"before,omitempty"`
	After             string `json:"after,omitempty"`
}

type planBlockerResponse struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

type planResponse struct {
	PlanID                        string                        `json:"plan_id"`
	PlanFormat                    int                           `json:"plan_format"`
	PlanHash                      string                        `json:"plan_hash"`
	ValidationID                  string                        `json:"validation_id"`
	DraftID                       string                        `json:"draft_id"`
	DraftVersion                  uint64                        `json:"draft_version"`
	SourceFormat                  domain.SourceFormat           `json:"source_format"`
	SourceHash                    string                        `json:"source_hash"`
	BaselineRevision              *string                       `json:"baseline_revision"`
	CandidateRevision             string                        `json:"candidate_revision"`
	BaselineDataSchemaIdentities  []dataSchemaIdentityResponse  `json:"baseline_data_schema_identities"`
	CandidateDataSchemaIdentities []dataSchemaIdentityResponse  `json:"candidate_data_schema_identities"`
	Summary                       planSummaryResponse           `json:"summary"`
	Changes                       []planChangeResponse          `json:"changes"`
	Warnings                      []application.ValidationIssue `json:"warnings"`
	Blockers                      []planBlockerResponse         `json:"blockers"`
	DataSchemaChanged             bool                          `json:"data_schema_changed"`
	RecordNamespaceChanged        bool                          `json:"record_namespace_changed"`
	MigrationExecutionSupported   bool                          `json:"migration_execution_supported"`
	Stale                         bool                          `json:"stale"`
	CreatedAt                     time.Time                     `json:"created_at"`
	Effects                       workflowEffectsResponse       `json:"effects"`
}

func writeDraftMetadata(writer http.ResponseWriter, status int, value domain.Draft) {
	writer.Header().Set("ETag", formatVersionETag(value.Generation()))
	writePrivateJSON(writer, status, makeDraftResponse(value))
}

func writePrivateJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Cache-Control", "private, no-store")
	writeJSON(writer, status, value)
}

func makeDraftResponse(value domain.Draft) draftResponse {
	return draftResponse{
		DraftID: value.ID().String(), Module: value.ModuleName(), BaselineRevision: optionalRevision(value.Baseline().RevisionHash()),
		DraftVersion: value.Generation(), SourceFormat: value.SourceFormat(), SourceHash: value.SourceHash(),
		CreatedBy: value.CreatedBy(), CreatedAt: value.CreatedAt().UTC(), UpdatedBy: value.UpdatedBy(), UpdatedAt: value.UpdatedAt().UTC(),
	}
}

func makeValidationResponse(value application.DraftValidation) validationResponse {
	identities := make([]dataSchemaIdentityResponse, 0, 1)
	var candidate *string
	if value.Candidate != nil {
		candidate = optionalRevision(value.Candidate.RevisionHash)
		identities = append(identities, dataSchemaIdentityResponse{
			Format: value.Candidate.DataSchemaFormat, Fingerprint: value.Candidate.DataSchemaFingerprint,
		})
	}
	issues := append([]application.ValidationIssue(nil), value.Issues...)
	if issues == nil {
		issues = []application.ValidationIssue{}
	}
	return validationResponse{
		ValidationID: value.ID, ValidationFormat: value.FormatVersion, ValidationHash: value.ValidationHash,
		DraftID: value.DraftID.String(), DraftVersion: value.Generation,
		BaselineRevision: optionalRevision(value.BaselineRevision), SourceFormat: value.SourceFormat, SourceHash: value.SourceHash,
		Valid: value.Valid, CandidateRevision: candidate, DataSchemaIdentity: identities,
		Violations: issues, Warnings: []application.ValidationIssue{}, Stale: value.Stale,
		CreatedAt: value.CreatedAt.UTC(), Effects: workflowEffectsResponse{},
	}
}

func makePlanResponse(value application.DraftPlan) planResponse {
	baselineIdentities := make([]dataSchemaIdentityResponse, 0, 1)
	if value.BaselineDataSchemaFormat > 0 && value.BaselineDataSchemaFingerprint != "" {
		baselineIdentities = append(baselineIdentities, dataSchemaIdentityResponse{
			Format: value.BaselineDataSchemaFormat, Fingerprint: value.BaselineDataSchemaFingerprint,
		})
	}
	candidateIdentities := []dataSchemaIdentityResponse{{
		Format: value.Candidate.DataSchemaFormat, Fingerprint: value.Candidate.DataSchemaFingerprint,
	}}
	changes := make([]planChangeResponse, 0, len(value.Changes))
	blockers := make([]planBlockerResponse, 0)
	for _, change := range value.Changes {
		changes = append(changes, planChangeResponse{
			Code: change.Code, Path: change.Path, Kind: change.Kind, Risk: change.Risk, Impact: change.Impact,
			RequiresMigration: change.RequiresMigration, Before: change.Before, After: change.After,
		})
		if change.Kind == "unsupported" || change.Risk == "destructive" || change.Risk == "critical" {
			blockers = append(blockers, planBlockerResponse{
				Code: change.Code, Path: change.Path, Message: "this change is not executable in alpha.3b",
			})
		}
	}
	return planResponse{
		PlanID: value.ID, PlanFormat: value.FormatVersion, PlanHash: value.PlanHash,
		ValidationID: value.ValidationID, DraftID: value.DraftID.String(), DraftVersion: value.DraftGeneration,
		SourceFormat: value.SourceFormat, SourceHash: value.SourceHash,
		BaselineRevision: optionalRevision(value.BaselineRevision), CandidateRevision: value.Candidate.RevisionHash,
		BaselineDataSchemaIdentities: baselineIdentities, CandidateDataSchemaIdentities: candidateIdentities,
		Summary: planSummaryResponse{
			Classification: value.Outcome, Risk: value.Risk, ChangeCount: len(changes), RiskCounts: value.RiskSummary,
		},
		Changes: changes, Warnings: []application.ValidationIssue{}, Blockers: blockers,
		DataSchemaChanged: value.DataSchemaChanged, RecordNamespaceChanged: value.RecordNamespaceChanged,
		MigrationExecutionSupported: value.MigrationExecutionSupported, Stale: value.Stale,
		CreatedAt: value.CreatedAt.UTC(), Effects: workflowEffectsResponse{PlanRecorded: true},
	}
}

func setDraftSourceContentType(writer http.ResponseWriter, format domain.SourceFormat) bool {
	switch format {
	case domain.SourceFormatJSON:
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		return true
	case domain.SourceFormatYAML:
		writer.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		return true
	}
	return false
}

func optionalRevision(value string) *string {
	if value == "" {
		return nil
	}
	result := value
	return &result
}

func draftLocation(module, draft string) string {
	return fmt.Sprintf("/api/admin/core/v1alpha1/modules/%s/drafts/%s", module, draft)
}

func validationLocation(module, draft, validation string) string {
	return fmt.Sprintf("%s/validations/%s", draftLocation(module, draft), validation)
}

func planLocation(module, draft, plan string) string {
	return fmt.Sprintf("%s/plans/%s", draftLocation(module, draft), plan)
}

func statusCreated(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}
