// Package db opens the SQLite database and applies embedded migrations.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open creates the parent directory if needed, opens the database with WAL
// and foreign keys enabled, and migrates it to the latest schema version.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite serializes writes; a single connection avoids
	// SQLITE_BUSY surprises until we need finer tuning.
	conn.SetMaxOpenConns(1)

	if err := migrate(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	); err != nil {
		return err
	}

	var current int
	if err := conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return fmt.Errorf("migration %q: name must start with a version number", entry.Name())
		}
		if version <= current {
			continue
		}

		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}

		tx, err := conn.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("applying %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
