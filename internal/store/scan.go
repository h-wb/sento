package store

import (
	"context"

	"github.com/georgysavva/scany/v2/sqlscan"
)

// list runs a query and scans every row into T. Columns map to snake-cased
// field names or `db` tags (scany); an empty result is a nil slice.
func list[T any](ctx context.Context, s *Store, query string, args ...any) ([]T, error) {
	var out []T
	if err := sqlscan.Select(ctx, s.db, &out, query, args...); err != nil {
		return nil, err
	}
	return out, nil
}

// get scans exactly one row into T; no row maps to ErrNotFound.
func get[T any](ctx context.Context, s *Store, query string, args ...any) (*T, error) {
	var v T
	err := sqlscan.Get(ctx, s.db, &v, query, args...)
	if sqlscan.NotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// touch runs an update/delete that must affect at least one row; affecting
// none maps to ErrNotFound.
func (s *Store) touch(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
