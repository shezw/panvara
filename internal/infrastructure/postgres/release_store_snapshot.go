/*
   Panvara
   internal/infrastructure/postgres/release_store_snapshot.go    2026-07-19
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	moduleapp "github.com/shezw/panvara/internal/application/appmodule"
	releaseapp "github.com/shezw/panvara/internal/application/release"
	domainappmodule "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

// GetPublishSnapshot resolves exactly one immutable Plan and its bound
// Validation and Draft. Publish still locks and rechecks the mutable Draft head
// in its write transaction; this read only supplies inputs for recompilation.
func (store *ReleaseStore) GetPublishSnapshot(
	ctx context.Context,
	projectID project.ID,
	module string,
	planID string,
) (domainappmodule.Draft, moduleapp.DraftValidation, moduleapp.DraftPlan, error) {
	if ctx == nil || !projectID.Valid() || !domainappmodule.ValidModuleName(module) ||
		!domainappmodule.ValidContentHash(planID) {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, releaseapp.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, err
	}
	plan, err := scanPlan(store.pool.QueryRow(ctx, `
		SELECT `+planColumns+`
		FROM panvara_module_draft_plan
		WHERE project_id = $1 AND module_name = $2 AND plan_id = $3
	`, projectID.String(), module, planID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, snapshotReadError("plan", err)
	}
	validation, err := scanValidation(store.pool.QueryRow(ctx, `
		SELECT `+validationColumns+`
		FROM panvara_module_draft_validation
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
		  AND validation_id = $4
	`, projectID.String(), module, plan.DraftID.String(), plan.ValidationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, snapshotReadError("validation", err)
	}
	draft, err := scanDraft(store.pool.QueryRow(ctx, `
		SELECT `+draftColumns+`
		FROM panvara_module_draft
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
	`, projectID.String(), module, plan.DraftID.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, releaseapp.ErrNotFound
	}
	if err != nil {
		return domainappmodule.Draft{}, moduleapp.DraftValidation{}, moduleapp.DraftPlan{}, snapshotReadError("draft", err)
	}
	validation.Stale = draft.Generation() != validation.Generation ||
		draft.SourceFormat() != validation.SourceFormat || draft.SourceHash() != validation.SourceHash
	plan.Stale = draft.Generation() != plan.DraftGeneration ||
		draft.SourceFormat() != plan.SourceFormat || draft.SourceHash() != plan.SourceHash
	return draft, validation, plan, nil
}

func snapshotReadError(kind string, err error) error {
	if errors.Is(err, moduleapp.ErrDraftCorrupt) {
		return fmt.Errorf("%w: read PostgreSQL publish %s: %v", releaseapp.ErrCorrupt, kind, err)
	}
	return fmt.Errorf("read PostgreSQL publish %s: %w", kind, err)
}
