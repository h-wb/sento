// Package db opens the database (SQLite or PostgreSQL) and applies the
// embedded migrations of the matching dialect.
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Dialect names the SQL flavor a connection speaks. The store needs it for
// the few things the portable subset can't express (placeholder style,
// constraint-violation codes).
const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

//go:embed migrations/sqlite/*.sql migrations/postgres/*.sql
var migrationsFS embed.FS

// Open connects to PostgreSQL when url is set, otherwise to SQLite at
// path, and migrates to the latest schema version. The returned dialect
// feeds store.New.
func Open(url, path string) (*sql.DB, string, error) {
	if url != "" {
		conn, err := openPostgres(url)
		return conn, DialectPostgres, err
	}
	conn, err := openSQLite(path)
	return conn, DialectSQLite, err
}

// openSQLite creates the parent directory if needed and opens the file
// with WAL and foreign keys enabled.
func openSQLite(path string) (*sql.DB, error) {
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

	if err := migrate(conn, DialectSQLite); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}
	return conn, nil
}

func openPostgres(url string) (*sql.DB, error) {
	conn, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}
	if err := migrate(conn, DialectPostgres); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}
	return conn, nil
}

// migrate applies the dialect's pending migration files in version order.
// Version numbers mean the same schema state in both dialects: postgres
// starts at a consolidated baseline carrying the version current at its
// introduction, and later migrations exist in both directories.
func migrate(conn *sql.DB, dialect string) error {
	if _, err := conn.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT ''
		)`,
	); err != nil {
		return err
	}

	var current int
	if err := conn.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&current); err != nil {
		return err
	}

	dir := "migrations/" + dialect
	entries, err := migrationsFS.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	record := `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`
	if dialect == DialectPostgres {
		record = `INSERT INTO schema_migrations (version, applied_at) VALUES ($1, $2)`
	}

	for _, entry := range entries {
		var version int
		if _, err := fmt.Sscanf(entry.Name(), "%d_", &version); err != nil {
			return fmt.Errorf("migration %q: name must start with a version number", entry.Name())
		}
		if version <= current {
			continue
		}

		body, err := migrationsFS.ReadFile(dir + "/" + entry.Name())
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
		if _, err := tx.Exec(record, version, time.Now().UTC().Format("2006-01-02 15:04:05")); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
