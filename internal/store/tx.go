package store

import (
	"context"
	"database/sql"
)

// inTx runs fn in one transaction: it rolls back on error or panic and
// commits otherwise, returning fn's error or the commit error.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
