// Package dbtest opens migrated throwaway databases for tests: SQLite in
// a temp dir by default, or — when SENTO_TEST_DB_URL points at postgres —
// an isolated schema per test, so the same suite doubles as the dialect
// conformance run (SENTO_TEST_DB_URL=postgres://… go test ./...).
package dbtest

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/db"
)

// Open returns a migrated test database and its dialect.
func Open(t *testing.T) (*sql.DB, string) {
	t.Helper()
	pgURL := os.Getenv("SENTO_TEST_DB_URL")
	if pgURL == "" {
		conn, dialect, err := db.Open("", filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn, dialect
	}

	schema := fmt.Sprintf("sento_test_%d_%d", os.Getpid(), rand.Uint64())
	admin, err := sql.Open("pgx", pgURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = admin.Close()
		t.Fatalf("creating test schema: %v", err)
	}

	sep := "?"
	if strings.Contains(pgURL, "?") {
		sep = "&"
	}
	scoped := pgURL + sep + "options=" + url.QueryEscape("-csearch_path="+schema)
	conn, dialect, err := db.Open(scoped, "")
	if err != nil {
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = admin.Close()
	})
	return conn, dialect
}
