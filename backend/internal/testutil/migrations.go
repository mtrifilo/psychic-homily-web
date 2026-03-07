package testutil

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// RunAllMigrations reads all *.up.sql files from migrationDir, sorts them
// by filename (which gives numeric order), and executes them against db.
// It automatically strips CREATE INDEX CONCURRENTLY (not allowed inside
// transactions) so tests don't need to special-case migration 27.
func RunAllMigrations(t *testing.T, db *sql.DB, migrationDir string) {
	t.Helper()

	pattern := filepath.Join(migrationDir, "*.up.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("failed to glob migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no migration files found in %s", migrationDir)
	}

	sort.Strings(files)

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("failed to read migration %s: %v", filepath.Base(f), err)
		}

		// CONCURRENTLY is not allowed inside transactions (used by testcontainers)
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		_, err = db.Exec(migrationSQL)
		if err != nil {
			t.Fatalf("failed to run migration %s: %v", filepath.Base(f), err)
		}
	}
}
