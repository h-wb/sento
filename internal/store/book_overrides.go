package store

import (
	"context"
	"database/sql"

	"github.com/h-wb/sento/internal/apperr"
)

// upsertBookOverride sets one book's monitored flag in one version.
func (s *Store) upsertBookOverride(ctx context.Context, e execer, versionID, bookID int64, monitored bool) error {
	_, err := e.ExecContext(ctx, s.q(`INSERT INTO book_overrides(version_id,book_id,monitored) VALUES(?,?,?) ON CONFLICT(version_id,book_id) DO UPDATE SET monitored=excluded.monitored`), versionID, bookID, monitored)
	return err
}

func (s *Store) SetBookMonitored(ctx context.Context, seriesID, versionID, bookID int64, monitored bool) error {
	t, err := s.GetVersion(ctx, versionID)
	if err != nil {
		return err
	}
	w, err := s.GetBook(ctx, bookID)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, seriesID, t.EditionID)
	if err != nil {
		return err
	}
	if w.EditionID != c.ID {
		return ErrNotFound
	}
	return s.upsertBookOverride(ctx, s.db, versionID, bookID, monitored)
}

func (s *Store) BulkMonitor(ctx context.Context, sid, tid int64, ids []int64, monitored bool) error {
	// 10,000 is the most books an edition can hold (inferred chapters cap).
	if len(ids) == 0 || len(ids) > 10000 {
		return apperr.Invalidf("select between 1 and 10,000 books")
	}
	t, err := s.GetVersion(ctx, tid)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, sid, t.EditionID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, true, c.ID); err != nil {
			return err
		}
		for _, id := range ids {
			var count int
			if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books WHERE id=? AND edition_id=?`), id, c.ID).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return apperr.Invalidf("book belongs to another edition")
			}
			if err := s.upsertBookOverride(ctx, tx, tid, id, monitored); err != nil {
				return err
			}
		}
		return nil
	})
}

type BookOverride struct {
	VersionID int64
	BookID    int64
	Monitored bool
}

func (s *Store) SeriesBookOverrides(ctx context.Context, seriesID int64) ([]BookOverride, error) {
	return list[BookOverride](ctx, s, `SELECT version_id,book_id,monitored FROM book_overrides WHERE version_id IN(SELECT t.id FROM versions t JOIN editions c ON c.id=t.edition_id WHERE c.series_id=?)`, seriesID)
}
