//go:build integration

/*
   Panvara
   tests/integration/access_helpers_test.go    2026-07-18
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	domainaccess "github.com/shezw/panvara/internal/domain/access"
	"github.com/shezw/panvara/internal/domain/project"
)

const (
	integrationEnvironmentA = "01981234-5678-7abc-8def-0123456789e0"
	integrationEnvironmentB = "01981234-5678-7abc-8def-0123456789e1"
)

// allowAuthorizer isolates persistence integration fixtures from access-store
// setup. Authorization denials are covered by application and Server E2E tests.
type allowAuthorizer struct{}

func (allowAuthorizer) Authorize(
	ctx context.Context,
	execution access.Execution,
	operation access.Operation,
) error {
	if ctx == nil {
		return access.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := execution.Validate(); err != nil {
		return err
	}
	if execution.Surface() != access.SurfaceAdmin || !operation.Valid() {
		return access.ErrForbidden
	}
	return nil
}

func integrationAdminExecution(
	t *testing.T,
	projectID project.ID,
	environmentText string,
	principalID string,
) access.Execution {
	t.Helper()
	environmentID, err := project.ParseEnvironmentID(environmentText)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := project.NewScope(projectID, environmentID)
	if err != nil {
		t.Fatal(err)
	}
	credentialID, err := domainaccess.ParseID("01981234-5678-7abc-8def-0123456789ef")
	if err != nil {
		t.Fatal(err)
	}
	const bearer = "integration-test-bootstrap-token-32-bytes"
	digest := sha256.Sum256([]byte(bearer))
	credential, err := domainaccess.NewCredential(domainaccess.CredentialMaterial{
		Scope: scope, ID: credentialID, PrincipalID: principalID,
		Label: "integration test credential", Hint: "sha256:" + hex.EncodeToString(digest[:6]),
		Status: domainaccess.CredentialStatusActive, IssuedBy: principalID,
		IssuedAt: time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := access.NewCredentialAuthenticator(integrationCredentialLookup{
		candidate: access.CredentialCandidate{Credential: credential, SecretDigest: digest},
	})
	if err != nil {
		t.Fatal(err)
	}
	authenticated, err := authenticator.Authenticate(context.Background(), scope, bearer)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := access.NewAdminExecution(scope, authenticated)
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

type integrationCredentialLookup struct {
	candidate access.CredentialCandidate
}

func (lookup integrationCredentialLookup) LookupCredential(
	context.Context,
	project.Scope,
	access.CredentialSelector,
) (access.CredentialCandidate, error) {
	return lookup.candidate, nil
}

var _ access.Authorizer = allowAuthorizer{}
