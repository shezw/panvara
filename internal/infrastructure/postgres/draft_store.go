/*
   Panvara
   internal/infrastructure/postgres/draft_store.go    2026-07-16
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
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	application "github.com/shezw/panvara/internal/application/appmodule"
	domain "github.com/shezw/panvara/internal/domain/appmodule"
	"github.com/shezw/panvara/internal/domain/project"
)

var _ application.DraftStore = (*DraftStore)(nil)

// DraftStore persists mutable draft heads and immutable workflow snapshots.
type DraftStore struct{ pool *pgxpool.Pool }

// NewDraftStore constructs the PostgreSQL draft workflow adapter.
func NewDraftStore(pool *pgxpool.Pool) (*DraftStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL draft store: nil pool")
	}
	return &DraftStore{pool: pool}, nil
}

// Create inserts one draft or replays the first result for the same intent.
func (store *DraftStore) Create(ctx context.Context, value domain.Draft, idempotencyKey, intentHash string) (domain.Draft, bool, error) {
	if idempotencyKey == "" || !domain.ValidContentHash(intentHash) {
		return domain.Draft{}, false, fmt.Errorf("%w: invalid PostgreSQL draft create metadata", application.ErrDraftInvalid)
	}
	row := store.pool.QueryRow(ctx, `
		INSERT INTO panvara_module_draft (
			project_id, module_name, draft_id, baseline_revision_hash,
			source_format, source_hash, source_bytes, generation,
			idempotency_key, create_intent_hash, created_by, created_at, updated_by, updated_at
		) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT DO NOTHING
		RETURNING `+draftColumns, draftArguments(value, idempotencyKey, intentHash)...)
	created, err := scanDraft(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Draft{}, false, fmt.Errorf("insert PostgreSQL module draft: %w", err)
	}
	var storedIntent string
	existing, err := scanDraftWithTail(store.pool.QueryRow(ctx, `
		SELECT `+draftColumns+`, create_intent_hash
		FROM panvara_module_draft
		WHERE project_id = $1 AND module_name = $2 AND idempotency_key = $3
	`, value.ProjectID().String(), value.ModuleName(), idempotencyKey), &storedIntent)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Draft{}, false, fmt.Errorf("%w: draft identity collided outside idempotency scope", application.ErrDraftConflict)
	}
	if err != nil {
		return domain.Draft{}, false, fmt.Errorf("read PostgreSQL idempotent draft: %w", err)
	}
	if storedIntent != intentHash {
		return domain.Draft{}, false, application.ErrDraftIdempotencyConflict
	}
	return existing, false, nil
}

// Get returns one exact project/module/draft head.
func (store *DraftStore) Get(ctx context.Context, projectID project.ID, module string, id domain.DraftID) (domain.Draft, error) {
	value, err := scanDraft(store.pool.QueryRow(ctx, `
		SELECT `+draftColumns+`
		FROM panvara_module_draft
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3
	`, projectID.String(), module, id.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Draft{}, application.ErrDraftNotFound
	}
	if err != nil {
		return domain.Draft{}, fmt.Errorf("get PostgreSQL module draft: %w", err)
	}
	return value, nil
}

// Replace performs one source CAS; identical bytes preserve generation and audit metadata.
func (store *DraftStore) Replace(ctx context.Context, projectID project.ID, module string, id domain.DraftID, expected uint64, replacement domain.DraftReplacement) (domain.Draft, bool, error) {
	if expected == 0 || expected > domain.MaxDraftGeneration {
		return domain.Draft{}, false, application.ErrDraftInvalid
	}
	row := store.pool.QueryRow(ctx, `
		UPDATE panvara_module_draft
		SET source_format = $5,
		    source_hash = $6,
		    source_bytes = $7,
		    generation = CASE
		        WHEN source_format = $5 AND source_hash = $6 AND source_bytes = $7 THEN generation
		        ELSE generation + 1
		    END,
		    updated_by = CASE
		        WHEN source_format = $5 AND source_hash = $6 AND source_bytes = $7 THEN updated_by
		        ELSE $8
		    END,
		    updated_at = CASE
		        WHEN source_format = $5 AND source_hash = $6 AND source_bytes = $7 THEN updated_at
		        ELSE $9
		    END
		WHERE project_id = $1 AND module_name = $2 AND draft_id = $3 AND generation = $4
		  AND (
		      generation < 9223372036854775807
		      OR (source_format = $5 AND source_hash = $6 AND source_bytes = $7)
		  )
		RETURNING `+draftColumns+`, generation <> $4
	`, projectID.String(), module, id.String(), expected, string(replacement.Format()), replacement.SourceHash(),
		replacement.Source(), replacement.ActorID(), replacement.At())
	var changed bool
	value, err := scanDraftWithTail(row, &changed)
	if err == nil {
		return value, changed, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Draft{}, false, fmt.Errorf("replace PostgreSQL module draft: %w", err)
	}
	if _, getErr := store.Get(ctx, projectID, module, id); errors.Is(getErr, application.ErrDraftNotFound) {
		return domain.Draft{}, false, application.ErrDraftNotFound
	} else if getErr != nil {
		return domain.Draft{}, false, getErr
	}
	return domain.Draft{}, false, application.ErrDraftConflict
}

const draftColumns = `
	project_id::text, module_name, draft_id::text, baseline_revision_hash,
	source_format, source_hash, source_bytes, generation,
	created_by, created_at, updated_by, updated_at
`

func draftArguments(value domain.Draft, idempotencyKey, intentHash string) []any {
	return []any{
		value.ProjectID().String(), value.ModuleName(), value.ID().String(), value.Baseline().RevisionHash(),
		string(value.SourceFormat()), value.SourceHash(), value.Source(), value.Generation(),
		idempotencyKey, intentHash, value.CreatedBy(), value.CreatedAt(), value.UpdatedBy(), value.UpdatedAt(),
	}
}

type draftScanner interface{ Scan(...any) error }

func scanDraft(row draftScanner) (domain.Draft, error) { return scanDraftWithTail(row) }

func scanDraftWithTail(row draftScanner, tail ...any) (domain.Draft, error) {
	var projectText, module, idText string
	var baseline *string
	var sourceFormat, sourceHash, createdBy, updatedBy string
	var source []byte
	var generation uint64
	var createdAt, updatedAt time.Time
	destinations := []any{&projectText, &module, &idText, &baseline, &sourceFormat, &sourceHash, &source,
		&generation, &createdBy, &createdAt, &updatedBy, &updatedAt}
	destinations = append(destinations, tail...)
	if err := row.Scan(destinations...); err != nil {
		return domain.Draft{}, err
	}
	projectID, err := project.ParseID(projectText)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("%w: parse persisted draft project: %v", application.ErrDraftCorrupt, err)
	}
	id, err := domain.ParseDraftID(idText)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("%w: parse persisted draft id: %v", application.ErrDraftCorrupt, err)
	}
	baselineText := ""
	if baseline != nil {
		baselineText = *baseline
	}
	baselineValue, err := domain.NewDraftBaseline(baselineText)
	if err != nil {
		return domain.Draft{}, fmt.Errorf("%w: parse persisted draft baseline: %v", application.ErrDraftCorrupt, err)
	}
	value, err := domain.NewDraft(domain.DraftMaterial{
		ID: id, ProjectID: projectID, ModuleName: module, Baseline: baselineValue,
		SourceFormat: domain.SourceFormat(sourceFormat), SourceHash: sourceHash, Source: source, Generation: generation,
		CreatedBy: createdBy, CreatedAt: createdAt, UpdatedBy: updatedBy, UpdatedAt: updatedAt,
	})
	if err != nil {
		return domain.Draft{}, fmt.Errorf("%w: validate persisted draft: %v", application.ErrDraftCorrupt, err)
	}
	return value, nil
}
