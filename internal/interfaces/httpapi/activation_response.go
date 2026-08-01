/*
   Panvara
   internal/interfaces/httpapi/activation_response.go    2026-08-02
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

	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

type activeSnapshotResponse struct {
	Module                  string                             `json:"module"`
	ReleaseID               *string                            `json:"release_id"`
	RuntimeRevision         string                             `json:"runtime_revision"`
	RecordNamespaceRevision string                             `json:"record_namespace_revision"`
	DataSchemaIdentity      dataSchemaIdentityResponse         `json:"data_schema_identity"`
	Epoch                   uint64                             `json:"epoch"`
	Origin                  domainrelease.ActiveSnapshotOrigin `json:"origin"`
	ActivatedBy             string                             `json:"activated_by"`
	ActivatedCredentialID   *string                            `json:"activated_credential_id"`
	RequestID               string                             `json:"request_id"`
	ActivatedAt             time.Time                          `json:"activated_at"`
}

func makeActiveSnapshotResponse(value domainrelease.ActiveSnapshot) activeSnapshotResponse {
	var releaseID *string
	if id, found := value.ReleaseID(); found {
		encoded := id.String()
		releaseID = &encoded
	}
	var credentialID *string
	if id, found := value.ActivatedCredentialID(); found {
		encoded := id.String()
		credentialID = &encoded
	}
	return activeSnapshotResponse{
		Module: value.ModuleName(), ReleaseID: releaseID,
		RuntimeRevision:         value.RuntimeRevision(),
		RecordNamespaceRevision: value.RecordNamespaceRevision(),
		DataSchemaIdentity: dataSchemaIdentityResponse{
			Format: value.DataSchemaFormat(), Fingerprint: value.DataSchemaFingerprint(),
		},
		Epoch: value.Epoch(), Origin: value.Origin(), ActivatedBy: value.ActivatedBy(),
		ActivatedCredentialID: credentialID, RequestID: value.RequestID(),
		ActivatedAt: value.ActivatedAt().UTC(),
	}
}

func writeActiveSnapshot(writer http.ResponseWriter, status int, value domainrelease.ActiveSnapshot) {
	writer.Header().Set("ETag", fmt.Sprintf(`"release-epoch-%d"`, value.Epoch()))
	writePrivateJSON(writer, status, makeActiveSnapshotResponse(value))
}

func activeReleaseLocation(module string) string {
	return fmt.Sprintf("/api/admin/core/v1alpha1/modules/%s/active", module)
}
