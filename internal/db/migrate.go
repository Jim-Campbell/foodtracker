package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RunMigrations(ctx context.Context, database *DB, migrationsDir string) error {
	_, err := database.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	if err != nil {
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
		if err := database.pool.QueryRow(ctx,
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
		if _, err := database.pool.Exec(ctx, string(content)); err != nil {
			return fmt.Errorf("execute migration %s: %w", filename, err)
		}
		if _, err := database.pool.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)", filename); err != nil {
			return fmt.Errorf("record migration %s: %w", filename, err)
		}
		fmt.Printf("applied migration: %s\n", filename)
	}
	return nil
}
