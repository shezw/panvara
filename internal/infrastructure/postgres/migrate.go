/*
   Panvara
   internal/infrastructure/postgres/migrate.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

// Package postgres adapts Panvara application ports to PostgreSQL.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shezw/panvara/db/migrations"
)

const migrationLockID int64 = 0x50616e76617261

// Migrate applies each embedded migration once under a transaction-scoped
// advisory lock. A changed checksum fails closed instead of rewriting history.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("migrate PostgreSQL: nil pool")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin PostgreSQL migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("lock PostgreSQL migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS panvara_schema_migration (
			version text PRIMARY KEY,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create PostgreSQL migration ledger: %w", err)
	}

	files := migrations.Files()
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read embedded PostgreSQL migrations: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		script, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return fmt.Errorf("read PostgreSQL migration %q: %w", entry.Name(), err)
		}
		checksumBytes := sha256.Sum256(script)
		checksum := hex.EncodeToString(checksumBytes[:])

		var storedChecksum string
		err = tx.QueryRow(ctx,
			`SELECT checksum FROM panvara_schema_migration WHERE version = $1`,
			entry.Name(),
		).Scan(&storedChecksum)
		switch {
		case err == nil:
			if storedChecksum != checksum {
				return fmt.Errorf("PostgreSQL migration %q checksum changed", entry.Name())
			}
			continue
		case err != pgx.ErrNoRows:
			return fmt.Errorf("read PostgreSQL migration %q state: %w", entry.Name(), err)
		}

		if _, err := tx.Exec(ctx, string(script), pgx.QueryExecModeSimpleProtocol); err != nil {
			return fmt.Errorf("apply PostgreSQL migration %q: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO panvara_schema_migration (version, checksum) VALUES ($1, $2)`,
			entry.Name(),
			checksum,
		); err != nil {
			return fmt.Errorf("record PostgreSQL migration %q: %w", entry.Name(), err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit PostgreSQL migrations: %w", err)
	}
	return nil
}
