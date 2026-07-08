package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// migrationsLockID is an arbitrary constant key for a Postgres session-level
// advisory lock. It serializes RunMigrations across instances that start
// concurrently (Render's free tier has no zero-downtime deploys, so an old
// and new instance can briefly run at once) so they can't race on
// schema_migrations -- one instance applying a migration while another's
// conflicting DDL/rollback leaves the tracking table out of sync with the
// actual schema.
const migrationsLockID = 8743019284

func RunMigrations(ctx context.Context, database *DB, migrationsDir string) error {
	conn, err := database.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	// Session-scoped: held on this one connection until explicitly released
	// below (or the connection closes), blocking concurrent instances until
	// this one finishes.
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationsLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", migrationsLockID)

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, filename := range files {
		var applied bool
		if err := conn.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)", filename).
			Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", filename, err)
		}
		if applied {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsDir, filename))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", filename, err)
		}
		if _, err := conn.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}
		if _, err := conn.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)", filename); err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}
		fmt.Printf("applied migration: %s\n", filename)
	}
	return nil
}
