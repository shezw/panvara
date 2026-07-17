/*
   Panvara
   internal/application/appmodule/draft_snapshot.go    2026-07-16
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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	domain "github.com/shezw/panvara/internal/domain/appmodule"
)

func makeValidationIssue(err error) ValidationIssue {
	message := truncateUTF8(strings.TrimSpace(err.Error()), 4096)
	stage, code := "compile", "appmodule_invalid"
	lower := strings.ToLower(message)
	if strings.Contains(lower, "decode") || strings.Contains(lower, "yaml") || strings.Contains(lower, "json") ||
		strings.Contains(lower, "unknown field") || strings.Contains(lower, "document") || strings.Contains(lower, "empty") {
		stage, code = "decode", "source_invalid"
	}
	return ValidationIssue{Code: code, Stage: stage, Path: "/source", Message: message, Severity: "error"}
}

func sortValidationIssues(issues []ValidationIssue) {
	sort.Slice(issues, func(left, right int) bool { return validationIssueLess(issues[left], issues[right]) })
}

func validationIssueLess(left, right ValidationIssue) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Stage != right.Stage {
		return left.Stage < right.Stage
	}
	if left.Code != right.Code {
		return left.Code < right.Code
	}
	return left.Message < right.Message
}

// ValidateDraftValidation verifies bounds and deterministic snapshot identity.
func ValidateDraftValidation(value DraftValidation) error {
	issuesJSON, err := json.Marshal(value.Issues)
	if err != nil || len(value.Issues) > MaxValidationIssues || len(issuesJSON) > MaxValidationIssuesBytes {
		return fmt.Errorf("%w: validation issues exceed bounds", ErrDraftCorrupt)
	}
	if value.FormatVersion != DraftValidationFormatVersion || !value.ProjectID.Valid() || !domain.ValidModuleName(value.ModuleName) ||
		!value.DraftID.Valid() || !validDraftGeneration(value.Generation) || !domain.ValidContentHash(value.SourceHash) ||
		(value.SourceFormat != domain.SourceFormatJSON && value.SourceFormat != domain.SourceFormatYAML) ||
		(value.BaselineRevision != "" && !domain.ValidContentHash(value.BaselineRevision)) || value.CreatedBy == "" || value.CreatedAt.IsZero() {
		return fmt.Errorf("%w: validation metadata is invalid", ErrDraftCorrupt)
	}
	if value.IssuesHash != hashCanonical(value.Issues) {
		return fmt.Errorf("%w: validation issue hash mismatch", ErrDraftCorrupt)
	}
	for index, issue := range value.Issues {
		if issue.Code == "" || len(issue.Code) > 128 || issue.Stage == "" || len(issue.Stage) > 64 ||
			issue.Path == "" || len(issue.Path) > 512 || issue.Message == "" || len(issue.Message) > 4096 ||
			issue.Severity == "" || len(issue.Severity) > 32 {
			return fmt.Errorf("%w: validation issue is invalid", ErrDraftCorrupt)
		}
		if index > 0 && validationIssueLess(issue, value.Issues[index-1]) {
			return fmt.Errorf("%w: validation issues are not sorted", ErrDraftCorrupt)
		}
	}
	if value.Valid {
		if len(value.Issues) != 0 || value.Candidate == nil || len(value.CandidateIR) == 0 ||
			!domain.ValidContentHash(value.Candidate.RevisionHash) || value.Candidate.RevisionHash != hashBytesForDraft(value.CandidateIR) ||
			value.Candidate.DataSchemaFormat != DataSchemaFormatVersion || !domain.ValidContentHash(value.Candidate.DataSchemaFingerprint) {
			return fmt.Errorf("%w: valid validation candidate is invalid", ErrDraftCorrupt)
		}
		identity, err := decodeCanonicalModuleIR(value.CandidateIR)
		if err != nil || identity.Name != value.ModuleName || identity.Version != value.Candidate.ModuleVersion {
			return fmt.Errorf("%w: candidate IR identity mismatch", ErrDraftCorrupt)
		}
		fingerprint, err := dataSchemaFingerprintFromModuleIR(identity)
		if err != nil || fingerprint != value.Candidate.DataSchemaFingerprint {
			return fmt.Errorf("%w: candidate data schema identity mismatch", ErrDraftCorrupt)
		}
	} else if len(value.Issues) == 0 || value.Candidate != nil || len(value.CandidateIR) != 0 {
		return fmt.Errorf("%w: invalid validation result is malformed", ErrDraftCorrupt)
	}
	want := validationIdentity(value)
	if value.ID != want || value.ValidationHash != want {
		return fmt.Errorf("%w: validation identity mismatch", ErrDraftCorrupt)
	}
	return nil
}

// ValidateDraftPlan verifies plan bounds, invariants, and deterministic identity.
func ValidateDraftPlan(value DraftPlan) error {
	encoded, err := json.Marshal(value.Changes)
	if err != nil || len(value.Changes) > 65536 || len(encoded) > MaxDraftPlanBytes || value.FormatVersion != DraftPlanFormatVersion ||
		!value.ProjectID.Valid() || !domain.ValidModuleName(value.ModuleName) || !value.DraftID.Valid() ||
		!validDraftGeneration(value.DraftGeneration) || !domain.ValidContentHash(value.ValidationID) || !domain.ValidContentHash(value.SourceHash) ||
		(value.SourceFormat != domain.SourceFormatJSON && value.SourceFormat != domain.SourceFormatYAML) ||
		!domain.ValidContentHash(value.Candidate.RevisionHash) || !domain.ValidContentHash(value.Candidate.DataSchemaFingerprint) ||
		value.Candidate.DataSchemaFormat != DataSchemaFormatVersion || value.MigrationExecutionSupported || value.CreatedBy == "" || value.CreatedAt.IsZero() {
		return fmt.Errorf("%w: plan metadata is invalid", ErrDraftCorrupt)
	}
	if value.BaselineRevision == "" {
		if value.BaselineDataSchemaFormat != 0 || value.BaselineDataSchemaFingerprint != "" {
			return fmt.Errorf("%w: none baseline has data identity", ErrDraftCorrupt)
		}
	} else if !domain.ValidContentHash(value.BaselineRevision) || value.BaselineDataSchemaFormat != DataSchemaFormatVersion ||
		!domain.ValidContentHash(value.BaselineDataSchemaFingerprint) {
		return fmt.Errorf("%w: plan baseline identity is invalid", ErrDraftCorrupt)
	}
	for index, change := range value.Changes {
		if change.Code == "" || len(change.Code) > 128 || change.Path == "" || len(change.Path) > 512 ||
			!validPlanChangeKind(change.Kind) || !validPlanChangeRisk(change.Risk) ||
			change.Impact == "" || len(change.Impact) > 2048 || len(change.Before) > 64<<10 || len(change.After) > 64<<10 {
			return fmt.Errorf("%w: plan change is invalid", ErrDraftCorrupt)
		}
		if index > 0 {
			previous := value.Changes[index-1]
			if previous.Path > change.Path || (previous.Path == change.Path && previous.Code > change.Code) {
				return fmt.Errorf("%w: plan changes are not sorted", ErrDraftCorrupt)
			}
		}
	}
	summary, outcome, risk := summarizePlan(value.Changes)
	if summary != value.RiskSummary || outcome != value.Outcome || risk != value.Risk ||
		value.DataSchemaChanged != (value.BaselineRevision == "" || value.BaselineDataSchemaFingerprint != value.Candidate.DataSchemaFingerprint) ||
		value.RecordNamespaceChanged != (value.BaselineRevision == "" || value.BaselineRevision != value.Candidate.RevisionHash) {
		return fmt.Errorf("%w: plan classification is inconsistent", ErrDraftCorrupt)
	}
	wantHash := planHashIdentity(value)
	if value.PlanHash != wantHash {
		return fmt.Errorf("%w: semantic plan hash mismatch", ErrDraftCorrupt)
	}
	wantID := planSnapshotIdentity(value)
	if value.ID != wantID {
		return fmt.Errorf("%w: plan snapshot identity mismatch", ErrDraftCorrupt)
	}
	return nil
}

func validPlanChangeKind(value string) bool {
	switch value {
	case "add", "remove", "change":
		return true
	default:
		return false
	}
}

func validPlanChangeRisk(value string) bool {
	switch value {
	case "low", "review", "destructive":
		return true
	default:
		return false
	}
}

func validationIdentity(value DraftValidation) string {
	candidateIRHash := ""
	if len(value.CandidateIR) > 0 {
		candidateIRHash = hashBytesForDraft(value.CandidateIR)
	}
	return hashCanonical(struct {
		Format                 int `json:"formatVersion"`
		Project, Module, Draft string
		Generation             uint64
		Baseline               string
		SourceFormat           domain.SourceFormat `json:"sourceFormat"`
		SourceHash             string              `json:"sourceHash"`
		Valid                  bool                `json:"valid"`
		Issues                 []ValidationIssue   `json:"issues"`
		Candidate              *CandidateIdentity  `json:"candidate"`
		CandidateIRHash        string              `json:"candidateIRHash"`
	}{value.FormatVersion, value.ProjectID.String(), value.ModuleName, value.DraftID.String(), value.Generation,
		value.BaselineRevision, value.SourceFormat, value.SourceHash, value.Valid, value.Issues, value.Candidate, candidateIRHash})
}

func planHashIdentity(value DraftPlan) string {
	return hashCanonical(struct {
		Format                                            int `json:"formatVersion"`
		Module, Baseline                                  string
		BaselineFormat                                    int
		BaselineFingerprint                               string
		Candidate                                         CandidateIdentity
		Changes                                           []PlanChange
		Summary                                           PlanRiskSummary
		Outcome, Risk                                     string
		DataChanged, NamespaceChanged, ExecutionSupported bool
	}{value.FormatVersion, value.ModuleName, value.BaselineRevision,
		value.BaselineDataSchemaFormat, value.BaselineDataSchemaFingerprint,
		value.Candidate, value.Changes, value.RiskSummary, value.Outcome, value.Risk,
		value.DataSchemaChanged, value.RecordNamespaceChanged, value.MigrationExecutionSupported})
}

func planSnapshotIdentity(value DraftPlan) string {
	return hashCanonical(struct {
		Format       int `json:"formatVersion"`
		Project      string
		Module       string
		Draft        string
		Generation   uint64
		ValidationID string
		SourceFormat domain.SourceFormat
		SourceHash   string
		PlanHash     string
	}{
		value.FormatVersion, value.ProjectID.String(), value.ModuleName, value.DraftID.String(),
		value.DraftGeneration, value.ValidationID, value.SourceFormat, value.SourceHash, value.PlanHash,
	})
}

func hashCanonical(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal internal deterministic identity: %v", err))
	}
	return hashBytesForDraft(encoded)
}

func hashBytesForDraft(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func cloneValidation(value DraftValidation) DraftValidation {
	if value.Issues != nil {
		value.Issues = append([]ValidationIssue{}, value.Issues...)
	}
	value.CandidateIR = append([]byte(nil), value.CandidateIR...)
	if value.Candidate != nil {
		copy := *value.Candidate
		value.Candidate = &copy
	}
	return value
}

func clonePlan(value DraftPlan) DraftPlan {
	if value.Changes != nil {
		value.Changes = append([]PlanChange{}, value.Changes...)
	}
	return value
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	result := value[:maximum]
	for !utf8.ValidString(result) {
		result = result[:len(result)-1]
	}
	return result
}

func dataSchemaFingerprintFromModuleIR(value moduleIR) (string, error) {
	projection := dataSchemaIR{
		FormatVersion: DataSchemaFormatVersion,
		Module:        value.Name,
		Resources:     make([]dataResourceIR, 0, len(value.Resources)),
	}
	for _, resource := range value.Resources {
		projected := dataResourceIR{Name: resource.Name, Fields: make([]dataFieldIR, 0, len(resource.Fields))}
		for _, field := range resource.Fields {
			options := append([]string(nil), field.Options...)
			sort.Strings(options)
			if options == nil {
				options = []string{}
			}
			projected.Fields = append(projected.Fields, dataFieldIR{
				Name: field.Name, Kind: field.Kind, Required: field.Required, Unique: field.Unique,
				Target: field.Target, Options: options, Constraints: field.Constraints,
			})
		}
		projection.Resources = append(projection.Resources, projected)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return fingerprintDataSchema(encoded), nil
}
