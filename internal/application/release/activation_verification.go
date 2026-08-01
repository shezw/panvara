/*
   Panvara
   internal/application/release/activation_verification.go    2026-08-02
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
	"errors"
	"fmt"
	"time"

	"github.com/shezw/panvara/internal/application/access"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	domainmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
	domainrelease "github.com/shezw/panvara/internal/domain/release"
)

const activationTimestampPrecision = time.Microsecond

func verifyRequestedActive(
	value domainrelease.ActiveSnapshot,
	scope project.Scope,
	module string,
) error {
	if err := value.Validate(); err != nil || !sameScope(value.Scope(), scope) || value.ModuleName() != module {
		return fmt.Errorf("%w: active snapshot crossed its requested identity", ErrCorrupt)
	}
	return nil
}

func verifyRequestedRelease(
	value domainrelease.ModuleRelease,
	scope project.Scope,
	module string,
	id domainrelease.ID,
) error {
	if err := value.Validate(); err != nil || !sameScope(value.Scope(), scope) ||
		value.ModuleName() != module || value.ID().String() != id.String() {
		return fmt.Errorf("%w: activation release crossed its requested identity", ErrCorrupt)
	}
	return nil
}

func verifyActivationBaseline(
	current domainrelease.ActiveSnapshot,
	target domainrelease.ModuleRelease,
) (bool, error) {
	currentID, hasCurrentRelease := current.ReleaseID()
	replay := hasCurrentRelease && currentID.String() == target.ID().String()
	if replay {
		if current.RuntimeRevision() != target.CandidateRevision() ||
			current.DataSchemaFormat() != target.DataSchemaFormat() ||
			current.DataSchemaFingerprint() != target.DataSchemaFingerprint() {
			return false, fmt.Errorf("%w: active Release binding contradicts its immutable fact", ErrCorrupt)
		}
		return true, nil
	}
	if target.BaselineRevision() == "" || target.BaselineRevision() != current.RuntimeRevision() {
		return false, fmt.Errorf("%w: target Release does not extend the current runtime", ErrActivationConflict)
	}
	return false, nil
}

func verifyActivationRevisions(
	current domainrelease.ActiveSnapshot,
	target domainrelease.ModuleRelease,
	candidate domainmodule.Revision,
	namespace domainmodule.Revision,
	replay bool,
) error {
	projectID := current.Scope().ProjectID().String()
	if candidate.ProjectID().String() != projectID || candidate.ModuleName() != current.ModuleName() ||
		candidate.RevisionHash() != target.CandidateRevision() || candidate.SourceHash() != target.SourceHash() {
		return fmt.Errorf("%w: candidate Revision contradicts the target Release", ErrCorrupt)
	}
	candidateIdentity, found := candidate.DataSchemaIdentity(target.DataSchemaFormat())
	if !found || candidateIdentity.Fingerprint() != target.DataSchemaFingerprint() {
		return fmt.Errorf("%w: candidate Revision contradicts the Release data identity", ErrCorrupt)
	}
	if namespace.ProjectID().String() != projectID || namespace.ModuleName() != current.ModuleName() ||
		namespace.RevisionHash() != current.RecordNamespaceRevision() {
		return fmt.Errorf("%w: Record namespace Revision crossed the active binding", ErrCorrupt)
	}
	namespaceIdentity, found := namespace.DataSchemaIdentity(current.DataSchemaFormat())
	if !found || namespaceIdentity.Fingerprint() != current.DataSchemaFingerprint() {
		return fmt.Errorf("%w: Record namespace Revision contradicts the active data identity", ErrCorrupt)
	}
	if target.DataSchemaFormat() != current.DataSchemaFormat() ||
		target.DataSchemaFingerprint() != current.DataSchemaFingerprint() ||
		candidateIdentity.Format() != namespaceIdentity.Format() ||
		candidateIdentity.Fingerprint() != namespaceIdentity.Fingerprint() {
		return fmt.Errorf("%w: compatible activation requires an unchanged data identity", ErrNotActivatable)
	}
	if replay && candidate.RevisionHash() != current.RuntimeRevision() {
		return fmt.Errorf("%w: active replay runtime differs from its candidate", ErrCorrupt)
	}
	return nil
}

func verifyActivationResult(
	stored domainrelease.ActiveSnapshot,
	current domainrelease.ActiveSnapshot,
	target domainrelease.ModuleRelease,
	invocation access.Invocation,
	at time.Time,
	advanced bool,
	replay bool,
) error {
	if err := verifyRequestedActive(stored, current.Scope(), current.ModuleName()); err != nil {
		return err
	}
	if advanced {
		storedReleaseID, hasRelease := stored.ReleaseID()
		credentialID, hasCredential := stored.ActivatedCredentialID()
		execution := invocation.Execution()
		if replay || stored.Epoch() != current.Epoch()+1 || stored.Origin() != domainrelease.ActiveSnapshotOriginRelease ||
			!hasRelease || storedReleaseID.String() != target.ID().String() ||
			stored.RuntimeRevision() != target.CandidateRevision() ||
			stored.RecordNamespaceRevision() != current.RecordNamespaceRevision() ||
			stored.DataSchemaFormat() != target.DataSchemaFormat() ||
			stored.DataSchemaFingerprint() != target.DataSchemaFingerprint() ||
			stored.ActivatedBy() != execution.Actor().ActorID() || !hasCredential ||
			credentialID.String() != execution.CredentialID().String() ||
			stored.RequestID() != invocation.RequestID() || !stored.ActivatedAt().Equal(at) {
			return fmt.Errorf("%w: stored activation differs from the verified intent", ErrCorrupt)
		}
		return nil
	}
	if !replay || !sameActiveSnapshot(stored, current) {
		if replay || stored.Epoch() <= current.Epoch() || !activeSnapshotMatchesRelease(stored, current, target) {
			return fmt.Errorf("%w: store reported a non-advancing activation for another state", ErrCorrupt)
		}
	}
	return nil
}

func activeSnapshotMatchesRelease(
	stored domainrelease.ActiveSnapshot,
	previous domainrelease.ActiveSnapshot,
	target domainrelease.ModuleRelease,
) bool {
	storedReleaseID, hasRelease := stored.ReleaseID()
	return stored.Origin() == domainrelease.ActiveSnapshotOriginRelease && hasRelease &&
		storedReleaseID.String() == target.ID().String() &&
		stored.RuntimeRevision() == target.CandidateRevision() &&
		stored.RecordNamespaceRevision() == previous.RecordNamespaceRevision() &&
		stored.DataSchemaFormat() == target.DataSchemaFormat() &&
		stored.DataSchemaFingerprint() == target.DataSchemaFingerprint()
}

func sameActiveSnapshot(left, right domainrelease.ActiveSnapshot) bool {
	leftRelease, leftHasRelease := left.ReleaseID()
	rightRelease, rightHasRelease := right.ReleaseID()
	leftCredential, leftHasCredential := left.ActivatedCredentialID()
	rightCredential, rightHasCredential := right.ActivatedCredentialID()
	return sameScope(left.Scope(), right.Scope()) && left.Epoch() == right.Epoch() &&
		left.Origin() == right.Origin() && left.ModuleName() == right.ModuleName() &&
		leftHasRelease == rightHasRelease && (!leftHasRelease || leftRelease.String() == rightRelease.String()) &&
		left.RuntimeRevision() == right.RuntimeRevision() &&
		left.RecordNamespaceRevision() == right.RecordNamespaceRevision() &&
		left.DataSchemaFormat() == right.DataSchemaFormat() &&
		left.DataSchemaFingerprint() == right.DataSchemaFingerprint() &&
		left.ActivatedBy() == right.ActivatedBy() && leftHasCredential == rightHasCredential &&
		(!leftHasCredential || leftCredential.String() == rightCredential.String()) &&
		left.RequestID() == right.RequestID() && left.ActivatedAt().Equal(right.ActivatedAt())
}

func normalizeRevisionError(action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, stable := range []error{ErrInvalid, ErrNotFound, ErrCorrupt, ErrUnavailable} {
		if errors.Is(err, stable) {
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	switch {
	case errors.Is(err, moduleapp.ErrRevisionNotFound):
		return fmt.Errorf("%s: %w", action, ErrNotFound)
	case errors.Is(err, moduleapp.ErrRevisionInvalid), errors.Is(err, moduleapp.ErrRevisionCorrupt):
		return fmt.Errorf("%s: %w", action, ErrCorrupt)
	default:
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
	}
}

func normalizeRuntimeError(action string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	for _, stable := range []error{ErrInvalid, ErrNotActivatable, ErrActivationConflict, ErrCorrupt, ErrUnavailable} {
		if errors.Is(err, stable) {
			return fmt.Errorf("%s: %w", action, err)
		}
	}
	return fmt.Errorf("%w: %s: %v", ErrUnavailable, action, err)
}
