/*
   Panvara
   internal/application/release/activation_types.go    2026-08-02
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
	"context"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

// ActivationStore owns the authoritative environment active pointer and epoch.
// ActivateCompatible must reauthorize mutation inside the same transaction as
// its baseline CAS, snapshot append, pointer switch, and successful audit.
type ActivationStore interface {
	GetActive(context.Context, project.Scope, string) (domainrelease.ActiveSnapshot, error)
	ActivateCompatible(
		context.Context,
		access.MutationContext,
		ActivateCompatibleCommand,
	) (domainrelease.ActiveSnapshot, bool, error)
}

// ReleaseReader resolves one exact immutable Release in its complete scope.
type ReleaseReader interface {
	Get(context.Context, project.Scope, string, domainrelease.ID) (domainrelease.ModuleRelease, error)
}

// RevisionReader resolves one exact immutable project/module/Revision fact.
type RevisionReader interface {
	Get(context.Context, project.ID, string, string) (domainmodule.Revision, error)
}

// ActivateCompatibleCommand carries verified facts into the atomic storage CAS.
// The repository must independently re-read and verify every referenced fact.
type ActivateCompatibleCommand struct {
	Expected    domainrelease.ActiveSnapshot
	Release     domainrelease.ModuleRelease
	Candidate   domainmodule.Revision
	ActivatedAt time.Time
}

// PreparedRuntime is an inert candidate that has completed all fallible runtime preparation.
type PreparedRuntime interface {
	RuntimeRevision() string
	RecordNamespaceRevision() string
}

// Runtime prepares a candidate before the database switch and installs the
// committed authoritative snapshot through an in-process atomic swap.
type Runtime interface {
	Prepare(context.Context, domainmodule.Revision, string) (PreparedRuntime, error)
	Install(domainrelease.ActiveSnapshot, PreparedRuntime) error
}
