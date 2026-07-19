/*
   Panvara
   internal/interfaces/httpapi/release_response.go    2026-07-19
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
	"time"

	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

type releaseEffectsResponse struct {
	RevisionRegistered  bool `json:"revision_registered"`
	Published           bool `json:"published"`
	Activated           bool `json:"activated"`
	RecordsMigrated     bool `json:"records_migrated"`
	RuntimeChanged      bool `json:"runtime_changed"`
	ActivationSupported bool `json:"activation_supported"`
}

type releaseResponse struct {
	ReleaseID             string                     `json:"release_id"`
	Module                string                     `json:"module"`
	DraftID               string                     `json:"draft_id"`
	DraftGeneration       uint64                     `json:"draft_generation"`
	ValidationID          string                     `json:"validation_id"`
	PlanID                string                     `json:"plan_id"`
	PlanHash              string                     `json:"plan_hash"`
	BaselineRevision      *string                    `json:"baseline_revision"`
	CandidateRevision     string                     `json:"candidate_revision"`
	DataSchemaIdentity    dataSchemaIdentityResponse `json:"data_schema_identity"`
	SourceHash            string                     `json:"source_hash"`
	Outcome               domainrelease.Outcome      `json:"outcome"`
	Risk                  string                     `json:"risk"`
	PublishedBy           string                     `json:"published_by"`
	PublishedCredentialID string                     `json:"published_credential_id"`
	RequestID             string                     `json:"request_id"`
	PublishedAt           time.Time                  `json:"published_at"`
	Effects               releaseEffectsResponse     `json:"effects"`
}

func makeReleaseResponse(value domainrelease.ModuleRelease) releaseResponse {
	return releaseResponse{
		ReleaseID: value.ID().String(), Module: value.ModuleName(),
		DraftID: value.DraftID().String(), DraftGeneration: value.DraftGeneration(),
		ValidationID: value.ValidationID(), PlanID: value.PlanID(), PlanHash: value.PlanHash(),
		BaselineRevision: optionalRevision(value.BaselineRevision()), CandidateRevision: value.CandidateRevision(),
		DataSchemaIdentity: dataSchemaIdentityResponse{
			Format: value.DataSchemaFormat(), Fingerprint: value.DataSchemaFingerprint(),
		},
		SourceHash: value.SourceHash(), Outcome: value.Outcome(), Risk: value.Risk(),
		PublishedBy: value.PublishedBy(), PublishedCredentialID: value.PublishedCredentialID().String(),
		RequestID: value.RequestID(), PublishedAt: value.PublishedAt().UTC(),
		Effects: releaseEffectsResponse{RevisionRegistered: true, Published: true},
	}
}

func releaseLocation(module, releaseID string) string {
	return fmt.Sprintf("/api/admin/core/v1alpha1/modules/%s/releases/%s", module, releaseID)
}
