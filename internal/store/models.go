package store

import (
	"context"
	"database/sql"
	"strconv"
)

// EnsureModels creates default targets and explicitly inferred chapter books. It never adopts local metadata or touches bytes.
func (s *Store) EnsureModels(ctx context.Context, seriesID int64) error {
	series, err := s.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	cols, err := s.ListEditions(ctx, seriesID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		for _, c := range cols {
			if !c.Custom && c.SyncState == "local" {
				if _, err := tx.ExecContext(ctx, s.q(`UPDATE editions SET sync_state='linked' WHERE id=?`), c.ID); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO versions(edition_id,name,is_default,monitored,cutoff_num) SELECT ?,'Default',TRUE,?,? WHERE NOT EXISTS(SELECT 1 FROM versions WHERE edition_id=? AND is_default=TRUE) ON CONFLICT DO NOTHING`), c.ID, c.Monitored, c.CutoffNum, c.ID); err != nil {
				return err
			}
			// Metadata's chapter count is a compatibility bound, not an observation.
			// Explicit chapter memberships always remain, including fractions/extras.
			if c.Type == "chapter" && c.NumberingSource == "series_count" && series.TotalChapters <= 10000 {
				var bound int
				if err := tx.QueryRowContext(ctx, s.q(`SELECT generated_bound FROM editions WHERE id=?`), c.ID).Scan(&bound); err != nil {
					return err
				}
				for n := bound + 1; n <= series.TotalChapters; n++ {
					if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO books(edition_id,sequence_num,sequence_string,count_type,sync_state) SELECT ?,?,?,'main','inferred' WHERE NOT EXISTS(SELECT 1 FROM books WHERE edition_id=? AND sequence_num=? AND count_type='main')`), c.ID, n*100, strconv.Itoa(n), c.ID, n*100); err != nil {
						return err
					}
				}
				if _, err := tx.ExecContext(ctx, s.q(`UPDATE editions SET generated_bound=? WHERE id=?`), max(bound, series.TotalChapters), c.ID); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
