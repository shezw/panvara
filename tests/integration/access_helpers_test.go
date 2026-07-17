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
	"testing"

	"github.com/shezw/panvara/internal/application/access"
	"github.com/shezw/panvara/internal/domain/actor"
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
	subject, err := actor.New(projectID.String(), principalID, nil)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := access.NewExecution(scope, subject, access.SurfaceAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

var _ access.Authorizer = allowAuthorizer{}
