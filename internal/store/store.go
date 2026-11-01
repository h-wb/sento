// Package store is the persistence layer, SQLite or Postgres: hand-written SQL, scany
// for row mapping (columns match snake-cased field names or `db` tags).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/h-wb/sento/internal/apperr"
)

// ErrNotFound is the apperr sentinel: services and handlers match it with
// errors.Is and the HTTP layer maps it to 404.
var ErrNotFound = apperr.ErrNotFound

// conflict maps a uniqueness violation to apperr.ErrConflict with a
// caller-supplied message; other errors pass through.
func conflict(err error, msg string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
		return apperr.Conflictf("%s", msg)
	}
	if strings.Contains(err.Error(), "UNIQUE constraint") { // modernc/sqlite
		return apperr.Conflictf("%s", msg)
	}
	return err
}

// Store speaks the portable subset of SQL shared by SQLite and PostgreSQL;
// queries are written with ? placeholders and rebound per dialect (q).
type Store struct {
	db *sql.DB
	pg bool
}

// New wraps a connection opened by internal/db; dialect is one of its
// Dialect constants.
func New(db *sql.DB, dialect string) *Store {
	return &Store{db: db, pg: dialect == "postgres"}
}

// q rebinds ? placeholders to PostgreSQL's $N form. Queries never contain
// a literal '?' (no SQL string literal here uses one), so the rewrite is a
// plain scan.
func (s *Store) q(query string) string {
	if !s.pg || !strings.ContainsRune(query, '?') {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// utcNow renders the canonical stored timestamp: UTC, second precision,
// the same format both dialects' column defaults produce.
func utcNow() string { return time.Now().UTC().Format("2006-01-02 15:04:05") }

// insert runs an INSERT ... RETURNING id and scans the new row id (the
// portable replacement for LastInsertId, which pgx does not implement).
func (s *Store) insert(ctx context.Context, query string, args ...any) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, s.q(query), args...).Scan(&id)
	return id, err
}

// --- helpers ---

func sortTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	for _, art := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(t, art) {
			return strings.TrimSpace(t[len(art):])
		}
	}
	return t
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
