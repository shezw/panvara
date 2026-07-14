/*
   Panvara
   internal/infrastructure/postgres/store.go    2026-07-14
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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shezw/panvara/internal/application/record"
)

var _ record.Store = (*Store)(nil)

// Store implements atomic flex_record/flex_unique/flex_reference persistence.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a PostgreSQL record adapter.
func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("construct PostgreSQL record store: nil pool")
	}
	return &Store{pool: pool}, nil
}

// Create inserts a record, its unique values, and its references atomically.
func (store *Store) Create(ctx context.Context, command record.CreateCommand) (record.Record, error) {
	if err := validateCreate(command); err != nil {
		return record.Record{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return record.Record{}, fmt.Errorf("begin PostgreSQL create record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		INSERT INTO flex_record (
			project_id, module_name, resource_name, record_id, revision_hash,
			record_version, data, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 1, $6::jsonb, $7, $7)
		RETURNING record_id::text, record_version, data, created_at, updated_at, deleted_at
	`,
		command.Scope.ProjectID.String(), command.Scope.ModuleName, command.Scope.ResourceName,
		command.ID.String(), command.Scope.RevisionHash, string(command.Data), command.At.UTC(),
	)
	created, err := scanRecord(row, command.Scope)
	if err != nil {
		return record.Record{}, translateWriteError("insert", err)
	}
	if err := replaceIndexes(ctx, tx, command.Scope, command.ID, command.Uniques, command.References); err != nil {
		return record.Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return record.Record{}, translateWriteError("commit create", err)
	}
	return created, nil
}

// Get returns one live record in the exact project/module/resource/revision scope.
func (store *Store) Get(ctx context.Context, scope record.Scope, id record.ID) (record.Record, error) {
	if err := validateIdentity(scope, id); err != nil {
		return record.Record{}, err
	}
	row := store.pool.QueryRow(ctx, `
		SELECT record_id::text, record_version, data, created_at, updated_at, deleted_at
		FROM flex_record
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND record_id = $5
		  AND deleted_at IS NULL
	`, scope.ProjectID.String(), scope.ModuleName, scope.ResourceName, scope.RevisionHash, id.String())
	value, err := scanRecord(row, scope)
	if errors.Is(err, pgx.ErrNoRows) {
		return record.Record{}, record.ErrNotFound
	}
	if err != nil {
		return record.Record{}, fmt.Errorf("query PostgreSQL record: %w", err)
	}
	return value, nil
}

// List scans a bounded live-record page using the stable creation tuple.
func (store *Store) List(
	ctx context.Context,
	scope record.Scope,
	options record.ListOptions,
) (record.ListResult, error) {
	if err := scope.Validate(); err != nil {
		return record.ListResult{}, err
	}
	if options.Limit <= 0 || options.Limit > record.MaxListLimit {
		return record.ListResult{}, fmt.Errorf("%w: invalid PostgreSQL list limit", record.ErrInvalidArgument)
	}
	if options.Cursor != nil && (options.Cursor.CreatedAt.IsZero() || !options.Cursor.ID.Valid()) {
		return record.ListResult{}, fmt.Errorf("%w: invalid PostgreSQL list cursor", record.ErrInvalidArgument)
	}

	query, arguments, err := buildListQuery(scope, options)
	if err != nil {
		return record.ListResult{}, err
	}
	rows, err := store.pool.Query(ctx, query, arguments...)
	if err != nil {
		return record.ListResult{}, fmt.Errorf("list PostgreSQL records: %w", err)
	}
	defer rows.Close()

	values := make([]record.Record, 0, options.Limit+1)
	for rows.Next() {
		value, err := scanRecord(rows, scope)
		if err != nil {
			return record.ListResult{}, fmt.Errorf("scan PostgreSQL record page: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return record.ListResult{}, fmt.Errorf("iterate PostgreSQL record page: %w", err)
	}

	result := record.ListResult{Records: values}
	if len(values) > options.Limit {
		result.Records = values[:options.Limit]
		last := result.Records[len(result.Records)-1]
		result.Next = &record.ListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return result, nil
}

func buildListQuery(scope record.Scope, options record.ListOptions) (string, []any, error) {
	if len(options.Filters) > record.MaxListFilters {
		return "", nil, fmt.Errorf("%w: too many PostgreSQL list filters", record.ErrInvalidArgument)
	}
	arguments := []any{
		scope.ProjectID.String(), scope.ModuleName, scope.ResourceName, scope.RevisionHash,
	}
	var query strings.Builder
	query.WriteString(`
		SELECT record_id::text, record_version, data, created_at, updated_at, deleted_at
		FROM flex_record
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND deleted_at IS NULL`)
	seen := make(map[string]struct{}, len(options.Filters))
	for _, filter := range options.Filters {
		if filter.Field == "" || len(filter.Field) > 128 || len(filter.Value) > record.MaxListFilterValueBytes {
			return "", nil, fmt.Errorf("%w: invalid PostgreSQL list filter", record.ErrInvalidArgument)
		}
		if _, exists := seen[filter.Field]; exists {
			return "", nil, fmt.Errorf("%w: duplicate PostgreSQL list filter", record.ErrInvalidArgument)
		}
		seen[filter.Field] = struct{}{}
		arguments = append(arguments, filter.Field, filter.Value)
		fieldPosition := len(arguments) - 1
		valuePosition := len(arguments)
		query.WriteString(fmt.Sprintf("\n\t\t  AND data ->> $%d = $%d", fieldPosition, valuePosition))
	}
	if options.Cursor != nil {
		arguments = append(arguments, options.Cursor.CreatedAt.UTC(), options.Cursor.ID.String())
		createdPosition := len(arguments) - 1
		idPosition := len(arguments)
		query.WriteString(fmt.Sprintf(
			"\n\t\t  AND (created_at, record_id) > ($%d, $%d)",
			createdPosition,
			idPosition,
		))
	}
	arguments = append(arguments, options.Limit+1)
	query.WriteString(fmt.Sprintf("\n\t\tORDER BY created_at, record_id\n\t\tLIMIT $%d", len(arguments)))
	return query.String(), arguments, nil
}

// Update replaces data and derived indexes after locking and checking version.
func (store *Store) Update(ctx context.Context, command record.UpdateCommand) (record.Record, error) {
	if err := validateUpdate(command); err != nil {
		return record.Record{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return record.Record{}, fmt.Errorf("begin PostgreSQL update record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAndCheckVersion(ctx, tx, command.Scope, command.ID, command.ExpectedVersion); err != nil {
		return record.Record{}, err
	}
	if err := removeIndexes(ctx, tx, command.Scope, command.ID); err != nil {
		return record.Record{}, err
	}
	if err := replaceIndexes(ctx, tx, command.Scope, command.ID, command.Uniques, command.References); err != nil {
		return record.Record{}, err
	}
	row := tx.QueryRow(ctx, `
		UPDATE flex_record
		SET data = $6::jsonb,
		    record_version = record_version + 1,
		    updated_at = $7
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND record_id = $5
		  AND deleted_at IS NULL
		RETURNING record_id::text, record_version, data, created_at, updated_at, deleted_at
	`,
		command.Scope.ProjectID.String(), command.Scope.ModuleName, command.Scope.ResourceName,
		command.Scope.RevisionHash, command.ID.String(), string(command.Data), command.At.UTC(),
	)
	updated, err := scanRecord(row, command.Scope)
	if err != nil {
		return record.Record{}, translateWriteError("update", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return record.Record{}, translateWriteError("commit update", err)
	}
	return updated, nil
}

// Delete rejects live inbound references, then soft-deletes and releases the
// record's unique values and outbound references in one transaction.
func (store *Store) Delete(ctx context.Context, command record.DeleteCommand) (record.Record, error) {
	if err := validateDelete(command); err != nil {
		return record.Record{}, err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return record.Record{}, fmt.Errorf("begin PostgreSQL delete record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockAndCheckVersion(ctx, tx, command.Scope, command.ID, command.ExpectedVersion); err != nil {
		return record.Record{}, err
	}
	var referenced bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM flex_reference AS relation
			JOIN flex_record AS source
			  ON source.project_id = relation.project_id
			 AND source.module_name = relation.module_name
			 AND source.revision_hash = relation.revision_hash
			 AND source.resource_name = relation.source_resource_name
			 AND source.record_id = relation.source_record_id
			WHERE relation.project_id = $1
			  AND relation.module_name = $2
			  AND relation.revision_hash = $3
			  AND relation.target_resource_name = $4
			  AND relation.target_record_id = $5
			  AND source.deleted_at IS NULL
			  AND NOT (
			      relation.source_resource_name = $4
			      AND relation.source_record_id = $5
			  )
		)
	`,
		command.Scope.ProjectID.String(), command.Scope.ModuleName,
		command.Scope.RevisionHash, command.Scope.ResourceName, command.ID.String(),
	).Scan(&referenced)
	if err != nil {
		return record.Record{}, fmt.Errorf("check PostgreSQL record references: %w", err)
	}
	if referenced {
		return record.Record{}, record.ErrReferenced
	}
	if err := removeIndexes(ctx, tx, command.Scope, command.ID); err != nil {
		return record.Record{}, err
	}
	row := tx.QueryRow(ctx, `
		UPDATE flex_record
		SET record_version = record_version + 1,
		    updated_at = $6,
		    deleted_at = $6
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND record_id = $5
		  AND deleted_at IS NULL
		RETURNING record_id::text, record_version, data, created_at, updated_at, deleted_at
	`,
		command.Scope.ProjectID.String(), command.Scope.ModuleName, command.Scope.ResourceName,
		command.Scope.RevisionHash, command.ID.String(), command.At.UTC(),
	)
	deleted, err := scanRecord(row, command.Scope)
	if err != nil {
		return record.Record{}, translateWriteError("soft-delete", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return record.Record{}, translateWriteError("commit delete", err)
	}
	return deleted, nil
}

func lockAndCheckVersion(
	ctx context.Context,
	tx pgx.Tx,
	scope record.Scope,
	id record.ID,
	expected uint64,
) error {
	var current int64
	err := tx.QueryRow(ctx, `
		SELECT record_version
		FROM flex_record
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND record_id = $5
		  AND deleted_at IS NULL
		FOR UPDATE
	`, scope.ProjectID.String(), scope.ModuleName, scope.ResourceName, scope.RevisionHash, id.String()).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return record.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock PostgreSQL record: %w", err)
	}
	if uint64(current) != expected {
		return record.ErrVersionConflict
	}
	return nil
}

func replaceIndexes(
	ctx context.Context,
	tx pgx.Tx,
	scope record.Scope,
	id record.ID,
	uniques []record.UniqueValue,
	references []record.Reference,
) error {
	references = append([]record.Reference(nil), references...)
	sort.Slice(references, func(left, right int) bool {
		if references[left].TargetResource == references[right].TargetResource {
			return references[left].TargetID.String() < references[right].TargetID.String()
		}
		return references[left].TargetResource < references[right].TargetResource
	})
	for _, reference := range references {
		var exists int
		err := tx.QueryRow(ctx, `
			SELECT 1
			FROM flex_record
			WHERE project_id = $1
			  AND module_name = $2
			  AND resource_name = $3
			  AND revision_hash = $4
			  AND record_id = $5
			  AND deleted_at IS NULL
			FOR KEY SHARE
		`,
			scope.ProjectID.String(), scope.ModuleName,
			reference.TargetResource, scope.RevisionHash, reference.TargetID.String(),
		).Scan(&exists)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: reference target does not exist", record.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("lock PostgreSQL reference target: %w", err)
		}
	}

	for _, unique := range uniques {
		_, err := tx.Exec(ctx, `
			INSERT INTO flex_unique (
				project_id, module_name, resource_name, revision_hash,
				record_id, field_name, canonical_value
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			scope.ProjectID.String(), scope.ModuleName, scope.ResourceName,
			scope.RevisionHash, id.String(), unique.Field, unique.CanonicalValue,
		)
		if err != nil {
			return translateWriteError("insert unique value", err)
		}
	}
	for _, reference := range references {
		_, err := tx.Exec(ctx, `
			INSERT INTO flex_reference (
				project_id, module_name, revision_hash, source_resource_name,
				source_record_id, field_name, target_resource_name, target_record_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`,
			scope.ProjectID.String(), scope.ModuleName, scope.RevisionHash,
			scope.ResourceName, id.String(),
			reference.Field, reference.TargetResource, reference.TargetID.String(),
		)
		if err != nil {
			return translateWriteError("insert reference", err)
		}
	}
	return nil
}

func removeIndexes(ctx context.Context, tx pgx.Tx, scope record.Scope, id record.ID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM flex_reference
		WHERE project_id = $1
		  AND module_name = $2
		  AND revision_hash = $3
		  AND source_resource_name = $4
		  AND source_record_id = $5
	`, scope.ProjectID.String(), scope.ModuleName, scope.RevisionHash, scope.ResourceName, id.String()); err != nil {
		return fmt.Errorf("delete PostgreSQL outbound references: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM flex_unique
		WHERE project_id = $1
		  AND module_name = $2
		  AND resource_name = $3
		  AND revision_hash = $4
		  AND record_id = $5
	`, scope.ProjectID.String(), scope.ModuleName, scope.ResourceName, scope.RevisionHash, id.String()); err != nil {
		return fmt.Errorf("delete PostgreSQL unique values: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanRecord(row scanner, scope record.Scope) (record.Record, error) {
	var (
		idText    string
		version   int64
		data      []byte
		createdAt time.Time
		updatedAt time.Time
		deletedAt pgtype.Timestamptz
	)
	if err := row.Scan(&idText, &version, &data, &createdAt, &updatedAt, &deletedAt); err != nil {
		return record.Record{}, err
	}
	id, err := record.ParseID(idText)
	if err != nil {
		return record.Record{}, fmt.Errorf("parse PostgreSQL record id: %w", err)
	}
	if version <= 0 || !json.Valid(data) {
		return record.Record{}, fmt.Errorf("PostgreSQL record contains invalid persisted data")
	}
	value := record.Record{
		Scope: scope, ID: id, Version: uint64(version), Data: json.RawMessage(append([]byte(nil), data...)),
		CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC(),
	}
	if deletedAt.Valid {
		at := deletedAt.Time.UTC()
		value.DeletedAt = &at
	}
	return value, nil
}

func validateCreate(command record.CreateCommand) error {
	if err := validateIdentity(command.Scope, command.ID); err != nil {
		return err
	}
	if command.At.IsZero() || !json.Valid(command.Data) {
		return fmt.Errorf("%w: invalid PostgreSQL create payload", record.ErrInvalidArgument)
	}
	return nil
}

func validateUpdate(command record.UpdateCommand) error {
	if err := validateMutation(command.Scope, command.ID, command.ExpectedVersion, command.At); err != nil {
		return err
	}
	if !json.Valid(command.Data) {
		return fmt.Errorf("%w: invalid PostgreSQL update payload", record.ErrInvalidArgument)
	}
	return nil
}

func validateDelete(command record.DeleteCommand) error {
	return validateMutation(command.Scope, command.ID, command.ExpectedVersion, command.At)
}

func validateIdentity(scope record.Scope, id record.ID) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if !id.Valid() {
		return fmt.Errorf("%w: invalid PostgreSQL record id", record.ErrInvalidArgument)
	}
	return nil
}

func validateMutation(scope record.Scope, id record.ID, expected uint64, at time.Time) error {
	if err := validateIdentity(scope, id); err != nil {
		return err
	}
	if expected == 0 || at.IsZero() {
		return fmt.Errorf("%w: invalid PostgreSQL mutation version or timestamp", record.ErrInvalidArgument)
	}
	return nil
}

func translateWriteError(operation string, err error) error {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) {
		return fmt.Errorf("%s PostgreSQL record: %w", operation, err)
	}
	switch postgresError.Code {
	case "23505":
		switch postgresError.ConstraintName {
		case "flex_record_pkey":
			return fmt.Errorf("%s PostgreSQL record: %w", operation, record.ErrAlreadyExists)
		case "flex_unique_scope_value_key", "flex_unique_pkey":
			return fmt.Errorf("%s PostgreSQL record: %w", operation, record.ErrUniqueConflict)
		default:
			return fmt.Errorf("%s PostgreSQL record: %w", operation, record.ErrAlreadyExists)
		}
	case "23503":
		return fmt.Errorf("%s PostgreSQL record: %w", operation, record.ErrNotFound)
	case "22001", "22P02", "23514":
		return fmt.Errorf("%s PostgreSQL record: %w", operation, record.ErrInvalidArgument)
	default:
		return fmt.Errorf("%s PostgreSQL record: %w", operation, err)
	}
}
