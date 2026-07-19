/*
   Panvara
   internal/application/release/types.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package release publishes verified Draft/Validation/Plan chains as immutable facts.
package release

import (
	"context"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	"github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

// PublishInput is the complete caller-selected publication intent.
type PublishInput struct {
	PlanID         string
	IdempotencyKey string
}

// SnapshotReader resolves the exact Draft, Validation, and Plan chain from one
// project/module/plan identity. It must not infer a current or latest plan.
type SnapshotReader interface {
	GetPublishSnapshot(
		context.Context,
		project.ID,
		string,
		string,
	) (appmodule.Draft, moduleapp.DraftValidation, moduleapp.DraftPlan, error)
}

// ReleaseStore atomically registers the Revision, ModuleRelease, idempotency
// result, and successful audit after transaction-local access reauthorization.
type ReleaseStore interface {
	// ResolveReplay checks an existing idempotency key before snapshot reads. If
	// the key is unbound but the scoped plan was already published, it atomically
	// records the key alias and successful audit before returning that Release.
	ResolveReplay(
		context.Context,
		access.MutationContext,
		string,
		string,
		string,
		string,
		time.Time,
	) (domainrelease.ModuleRelease, bool, error)
	Publish(
		context.Context,
		access.MutationContext,
		appmodule.Revision,
		domainrelease.ModuleRelease,
		string,
		string,
	) (domainrelease.ModuleRelease, bool, error)
	Get(context.Context, project.Scope, string, domainrelease.ID) (domainrelease.ModuleRelease, error)
}

// Clock supplies deterministic publication timestamps.
type Clock interface{ Now() time.Time }

// ReleaseIDGenerator creates bare UUIDv7 release identities at a supplied time.
type ReleaseIDGenerator interface {
	New(time.Time) (domainrelease.ID, error)
}

// SystemClock reads the current UTC wall clock.
type SystemClock struct{}

// Now returns the current UTC wall-clock time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }
