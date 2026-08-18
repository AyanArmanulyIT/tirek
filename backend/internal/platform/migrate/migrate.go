// Package migrate applies SQL migration files in order, guarded by an advisory
// lock so concurrent deploys are safe. Applied versions are recorded in
// schema_migrations.
package migrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Apply runs all pending *.sql migrations from dir in lexicographic order,
// each inside its own transaction (see docs/architecture/database.md §10).
func Apply(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(424242)"); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(ctx, "SELECT pg_advisory_unlock(424242)") }()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)

	applied := map[string]bool{}
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	for _, f := range files {
		version := filepath.Base(f)
		if applied[version] {
			continue
		}
		sqlBytes, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %s failed: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		fmt.Println("applied:", strings.TrimSuffix(version, ".sql"))
	}
	return nil
}