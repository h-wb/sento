package store

import (
	"context"
	"database/sql"
	"sort"

	"github.com/h-wb/sento/internal/apperr"
)

// All operations that change edition identity and all new reservations use the
// same parent lock. SQLite's write transaction provides the equivalent ordering.
func (s *Store) lockEditions(ctx context.Context, tx *sql.Tx, allowPending bool, ids ...int64) error {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if i > 0 && ids[i-1] == id {
			continue
		}
		sid, err := s.lockRow(ctx, tx, "editions", "series_id", id)
		if err != nil {
			return err
		}
		if allowPending {
			continue
		}
		rows, err := s.pendingRefs(ctx, tx, sid)
		if err != nil {
			return err
		}
		pending := false
		for _, row := range rows {
			slots, err := row.bookRefs()
			if err != nil {
				return err
			}
			for _, sl := range slots {
				if sl.EditionID == id {
					pending = true
				}
			}
		}
		if pending {
			return apperr.Conflictf("finish or cancel this edition's pending downloads first")
		}
	}
	return nil
}

// lockRow takes the dialect's write lock on one row and returns the selected
// column; table and column are fixed call-site strings, never caller input.
func (s *Store) lockRow(ctx context.Context, tx *sql.Tx, table, column string, id int64) (int64, error) {
	query := `SELECT ` + column + ` FROM ` + table + ` WHERE id=?`
	if s.pg {
		query += ` FOR UPDATE`
	} else if _, err := tx.ExecContext(ctx, s.q(`UPDATE `+table+` SET id=id WHERE id=?`), id); err != nil {
		return 0, err
	}
	var v int64
	if err := tx.QueryRowContext(ctx, s.q(query), id).Scan(&v); err != nil {
		return 0, err
	}
	return v, nil
}
