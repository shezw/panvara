/*
   Panvara
   internal/application/release/verification.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package release

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
	spec "github.com/shezw/panvara/internal/spec/appmodule/v1alpha1"
)

func verifySnapshotChain(
	projectID string,
	module string,
	planID string,
	draft domainmodule.Draft,
	validation moduleapp.DraftValidation,
	plan moduleapp.DraftPlan,
) error {
	if err := moduleapp.ValidateDraftValidation(validation); err != nil {
		return fmt.Errorf("%w: invalid validation snapshot: %v", ErrCorrupt, err)
	}
	if err := moduleapp.ValidateDraftPlan(plan); err != nil {
		return fmt.Errorf("%w: invalid plan snapshot: %v", ErrCorrupt, err)
	}
	if draft.ProjectID().String() != projectID || validation.ProjectID.String() != projectID ||
		plan.ProjectID.String() != projectID || draft.ModuleName() != module || validation.ModuleName != module ||
		plan.ModuleName != module || plan.ID != planID || validation.ID != plan.ValidationID ||
		validation.DraftID.String() != plan.DraftID.String() || validation.Generation != plan.DraftGeneration ||
		validation.BaselineRevision != plan.BaselineRevision || validation.SourceFormat != plan.SourceFormat ||
		validation.SourceHash != plan.SourceHash || validation.Candidate == nil ||
		*validation.Candidate != plan.Candidate {
		return fmt.Errorf("%w: publish snapshot chain identities differ", ErrCorrupt)
	}
	if draft.ID().String() != plan.DraftID.String() ||
		draft.Baseline().RevisionHash() != plan.BaselineRevision {
		return fmt.Errorf("%w: draft identity differs from immutable plan chain", ErrCorrupt)
	}
	if validation.Stale || plan.Stale || draft.Generation() != plan.DraftGeneration ||
		draft.SourceFormat() != plan.SourceFormat || draft.SourceHash() != plan.SourceHash {
		return ErrStale
	}
	if !validation.Valid {
		return ErrNotPublishable
	}
	return nil
}

func verifyCompiledCandidate(
	module string,
	compiled *moduleapp.CompiledModule,
	validation moduleapp.DraftValidation,
	plan moduleapp.DraftPlan,
) error {
	if compiled == nil || validation.Candidate == nil || compiled.Name() != module ||
		compiled.Version() != validation.Candidate.ModuleVersion ||
		compiled.RevisionHash() != validation.Candidate.RevisionHash ||
		compiled.DataSchemaFormat() != validation.Candidate.DataSchemaFormat ||
		compiled.DataSchemaFingerprint() != validation.Candidate.DataSchemaFingerprint ||
		validation.Candidate.RevisionHash != plan.Candidate.RevisionHash ||
		validation.Candidate.ModuleVersion != plan.Candidate.ModuleVersion ||
		validation.Candidate.DataSchemaFormat != plan.Candidate.DataSchemaFormat ||
		validation.Candidate.DataSchemaFingerprint != plan.Candidate.DataSchemaFingerprint ||
		!bytes.Equal(compiled.CanonicalIR(), validation.CandidateIR) {
		return fmt.Errorf("%w: source compile differs from planned candidate", ErrCorrupt)
	}
	return nil
}

func publishableOutcome(value string) (domainrelease.Outcome, error) {
	outcome := domainrelease.Outcome(strings.TrimSpace(value))
	if !outcome.Valid() {
		return "", fmt.Errorf("%w: plan outcome %q", ErrNotPublishable, value)
	}
	return outcome, nil
}

func verifyStoredRelease(stored, proposed domainrelease.ModuleRelease, created bool) error {
	if err := stored.Validate(); err != nil || !stored.SamePublishIntent(proposed) ||
		(created && stored.ID().String() != proposed.ID().String()) {
		return fmt.Errorf("%w: stored release differs from verified publish intent", ErrCorrupt)
	}
	return nil
}

func verifyReplayedRelease(
	stored domainrelease.ModuleRelease,
	scope project.Scope,
	module string,
	planID string,
) error {
	if err := stored.Validate(); err != nil || !sameScope(stored.Scope(), scope) ||
		stored.ModuleName() != module || stored.PlanID() != planID {
		return fmt.Errorf("%w: replayed release differs from requested publish intent", ErrCorrupt)
	}
	return nil
}

func publishIntentHash(invocation access.Invocation, module, planID string) string {
	scope := invocation.Execution().Scope()
	encoded, err := json.Marshal(struct {
		Format      int    `json:"formatVersion"`
		Project     string `json:"project"`
		Environment string `json:"environment"`
		Module      string `json:"module"`
		PlanID      string `json:"planId"`
	}{1, scope.ProjectID().String(), scope.EnvironmentID().String(), module, planID})
	if err != nil {
		panic(fmt.Sprintf("marshal internal publish intent: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func specFormat(format domainmodule.SourceFormat) spec.Format {
	if format == domainmodule.SourceFormatJSON {
		return spec.FormatJSON
	}
	return spec.FormatYAML
}

func sameScope(left, right project.Scope) bool {
	return left.ProjectID().String() == right.ProjectID().String() &&
		left.EnvironmentID().String() == right.EnvironmentID().String()
}
