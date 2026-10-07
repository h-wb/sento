package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/h-wb/sento/internal/apperr"
	"sort"
)

// All operations that change edition identity and all new reservations use the
// same parent lock. SQLite's write transaction provides the equivalent ordering.
func (s *Store) lockEditions(ctx context.Context, tx *sql.Tx, allowPending bool, ids ...int64) error {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, id := range ids {
		if i > 0 && ids[i-1] == id {
			continue
		}
		var sid int64
		query := `SELECT series_id FROM collections WHERE id=?`
		if s.pg {
			query += ` FOR UPDATE`
		} else {
			if _, err := tx.ExecContext(ctx, s.q(`UPDATE collections SET id=id WHERE id=?`), id); err != nil {
				return err
			}
		}
		if err := tx.QueryRowContext(ctx, s.q(query), id).Scan(&sid); err != nil {
			return err
		}
		if allowPending {
			continue
		}
		rows, err := tx.QueryContext(ctx, s.q(`SELECT expected_slots_json FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')`), sid)
		if err != nil {
			return err
		}
		pending := false
		for rows.Next() {
			var raw string
			if err = rows.Scan(&raw); err != nil {
				_ = rows.Close()
				return err
			}
			var slots []BookRef
			if err = json.Unmarshal([]byte(raw), &slots); err != nil {
				_ = rows.Close()
				return err
			}
			for _, sl := range slots {
				if sl.EditionID == id {
					pending = true
				}
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if pending {
			return apperr.Conflictf("finish or cancel this edition's pending downloads first")
		}
	}
	return nil
}
