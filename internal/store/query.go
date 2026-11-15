package store

import (
	"context"
	"database/sql"

	"github.com/georgysavva/scany/v2/sqlscan"
)

// querier is what sqlscan needs: *sql.DB and *sql.Tx both satisfy it.
type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// execer is the exec side of the helpers: *sql.DB and *sql.Tx both satisfy it.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// listQ runs a query on q and scans every row into T. Columns map to
// snake-cased field names or `db` tags (scany); an empty result is a nil slice.
func listQ[T any](ctx context.Context, q querier, query string, args ...any) ([]T, error) {
	var out []T
	if err := sqlscan.Select(ctx, q, &out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}

// getQ scans exactly one row on q into T; no row maps to ErrNotFound.
func getQ[T any](ctx context.Context, q querier, query string, args ...any) (*T, error) {
	var v T
	err := sqlscan.Get(ctx, q, &v, query, args...)
	if sqlscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// list runs a query and scans every row into T. Columns map to snake-cased
// field names or `db` tags (scany); an empty result is a nil slice.
func list[T any](ctx context.Context, s *Store, query string, args ...any) ([]T, error) {
	return listQ[T](ctx, s.db, s.q(query), args...)
}

// get scans exactly one row into T; no row maps to ErrNotFound.
func get[T any](ctx context.Context, s *Store, query string, args ...any) (*T, error) {
	return getQ[T](ctx, s.db, s.q(query), args...)
}

// touch runs an update/delete that must affect at least one row; affecting
// none maps to ErrNotFound.
func (s *Store) touch(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, s.q(query), args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
