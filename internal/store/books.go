package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/apperr"
)

func (s *Store) GetBook(ctx context.Context, id int64) (*Book, error) {
	return get[Book](ctx, s, `SELECT `+bookColumns+` FROM books WHERE id=?`, id)
}

// SeriesBooksByNumber returns every book across the series' editions in one
// query, ordered by number, count type and id, for passes that touch them
// all (organize planning over many files).
func (s *Store) SeriesBooksByNumber(ctx context.Context, seriesID int64) ([]Book, error) {
	return list[Book](ctx, s, `SELECT `+bookColumns+` FROM books WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY sequence_num,count_type,id`, seriesID)
}

// BookRawPayloads returns the raw catalog document of each of the series'
// books, keyed by book id, in one query.
func (s *Store) BookRawPayloads(ctx context.Context, seriesID int64) (map[int64]string, error) {
	rows, err := list[struct {
		ID      int64  `db:"id"`
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT id, COALESCE(raw_json,'') AS raw_json FROM books WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?)`, seriesID)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.RawJSON
	}
	return out, nil
}

const bookColumns = `id,edition_id,COALESCE(provider_id,'') AS provider_id,sequence_num,COALESCE(sequence_string,'') AS sequence_string,count_type,COALESCE(release_date,'') AS release_date,COALESCE(isbn,'') AS isbn,COALESCE(pages,0) AS pages,number_known,revision,sync_state,overrides_json,(SELECT COUNT(*) FROM works x WHERE x.book_id=books.id AND x.sync_state='linked') AS copies`

func (s *Store) SaveBook(ctx context.Context, seriesID int64, w Book) (int64, error) {
	c, err := s.editionOf(ctx, seriesID, w.EditionID)
	if err != nil {
		return 0, err
	}
	w.SequenceStr = strings.TrimSpace(w.SequenceStr)
	if w.SequenceNum < 0 {
		return 0, apperr.Invalidf("book number cannot be negative")
	}
	if w.ReleaseDate != "" {
		if _, err := time.Parse("2006-01-02", w.ReleaseDate); err != nil {
			return 0, apperr.Invalidf("publication date must be YYYY-MM-DD")
		}
	}
	if w.SequenceStr == "" {
		return 0, apperr.Invalidf("book label is required")
	}
	if w.CountType != "main" && w.CountType != "extra" && w.CountType != "other" {
		return 0, apperr.Invalidf("count type must be main, extra or other")
	}
	var id int64
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, false, c.ID); err != nil {
			return err
		}
		if w.ID == 0 {
			return tx.QueryRowContext(ctx, s.q(`INSERT INTO books(edition_id,sequence_num,sequence_string,count_type,release_date,isbn,pages,number_known,revision) VALUES(?,?,?,?,?,?,?,?,?) RETURNING id`), w.EditionID, w.SequenceNum, w.SequenceStr, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, w.NumberKnown, w.Revision).Scan(&id)
		}
		old, err := getQ[Book](ctx, s, tx, `SELECT `+bookColumns+` FROM books WHERE id=? AND edition_id=?`, w.ID, c.ID)
		if err != nil {
			return err
		}
		var previous string
		if err := tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM books WHERE id=?`), w.ID).Scan(&previous); err != nil {
			return err
		}
		local := map[string]any{}
		if err := json.Unmarshal([]byte(previous), &local); err != nil {
			return err
		}
		before := map[string]any{"sequence_num": old.SequenceNum, "sequence_string": old.SequenceStr, "count_type": old.CountType, "release_date": old.ReleaseDate, "isbn": old.ISBN, "pages": old.Pages, "number_known": old.NumberKnown, "revision": old.Revision}
		after := map[string]any{"sequence_num": w.SequenceNum, "sequence_string": w.SequenceStr, "count_type": w.CountType, "release_date": w.ReleaseDate, "isbn": w.ISBN, "pages": w.Pages, "number_known": w.NumberKnown, "revision": w.Revision}
		for k, v := range after {
			if v != before[k] {
				local[k] = v
			}
		}
		overrides, _ := json.Marshal(local)
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE books SET sequence_num=?,sequence_string=?,count_type=?,release_date=?,isbn=?,pages=?,number_known=?,revision=?,overrides_json=? WHERE id=?`), w.SequenceNum, w.SequenceStr, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, w.NumberKnown, w.Revision, string(overrides), w.ID); err != nil {
			return err
		}
		id = w.ID
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) DeleteBook(ctx context.Context, seriesID, id int64) error {
	w, err := s.GetBook(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, seriesID, w.EditionID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, false, c.ID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM asset_contents WHERE book_id=?`), id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return apperr.Conflictf("unlink owned files before deleting this book")
		}
		var err error
		if w.ProviderID != "" {
			_, err = tx.ExecContext(ctx, s.q(`UPDATE books SET sync_state='retired' WHERE id=? AND edition_id=?`), id, c.ID)
		} else {
			_, err = tx.ExecContext(ctx, s.q(`DELETE FROM books WHERE id=? AND edition_id=?`), id, c.ID)
		}
		return err
	})
}

func (s *Store) EditionPending(ctx context.Context, cid int64) (bool, error) {
	c, err := s.GetEdition(ctx, cid)
	if err != nil {
		return false, err
	}
	pending, err := s.pendingEditions(ctx, s.db, c.SeriesID)
	if err != nil {
		return false, err
	}
	return pending[cid], nil
}

type BookObservation struct {
	ID          int64  `json:"id"`
	EditionID   int64  `json:"editionId"`
	Source      string `json:"source"`
	ExternalKey string `json:"externalKey"`
	Label       string `json:"label"`
	Number      int64  `json:"number"`
	State       string `json:"state"`
}

func (s *Store) ObserveBook(ctx context.Context, seriesID int64, o BookObservation) error {
	if _, err := s.editionOf(ctx, seriesID, o.EditionID); err != nil {
		return err
	}
	if o.Number < 0 {
		return apperr.Invalidf("observation number cannot be negative")
	}
	if o.Source == "" || o.ExternalKey == "" || o.Label == "" {
		return apperr.Invalidf("source, external key and label are required")
	}
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO book_observations(edition_id,source,external_key,label,number) VALUES(?,?,?,?,?) ON CONFLICT(edition_id,source,external_key) DO NOTHING`), o.EditionID, o.Source, o.ExternalKey, o.Label, o.Number)
	return err
}

func (s *Store) ListObservations(ctx context.Context, seriesID int64) ([]BookObservation, error) {
	return list[BookObservation](ctx, s, `SELECT id,edition_id,source,external_key,label,number,state FROM book_observations WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY id DESC LIMIT 500`, seriesID)
}

func (s *Store) ResolveObservation(ctx context.Context, seriesID, id int64, accept bool) error {
	o, err := get[BookObservation](ctx, s, `SELECT id,edition_id,source,external_key,label,number,state FROM book_observations WHERE id=?`, id)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, seriesID, o.EditionID)
	if err != nil {
		return err
	}
	if o.State != "pending" {
		return apperr.Conflictf("observation already reviewed")
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		state := "rejected"
		if accept {
			state = "accepted"
		}
		res, err := tx.ExecContext(ctx, s.q(`UPDATE book_observations SET state=? WHERE id=? AND state='pending'`), state, id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return apperr.Conflictf("observation already reviewed")
		}
		if accept {
			if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO books(edition_id,sequence_num,sequence_string,count_type,number_known) SELECT ?,?,?,'main',TRUE WHERE NOT EXISTS(SELECT 1 FROM books WHERE edition_id=? AND sequence_num=? AND count_type='main')`), c.ID, o.Number, o.Label, c.ID, o.Number); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ResetBook(ctx context.Context, seriesID, id int64) error {
	w, err := s.GetBook(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.editionOf(ctx, seriesID, w.EditionID)
	if err != nil {
		return err
	}
	if w.ProviderID == "" {
		return apperr.Invalidf("book has no catalog link")
	}
	pending, err := s.EditionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("finish pending downloads before resetting metadata")
	}
	return s.touch(ctx, `UPDATE books SET overrides_json='{}',sync_state='linked' WHERE id=?`, id)
}

func (s *Store) AcceptTrustedObservation(ctx context.Context, sid int64, o BookObservation) error {
	id, err := get[int64](ctx, s, `SELECT id FROM book_observations WHERE edition_id=? AND source=? AND external_key=? AND state='pending'`, o.EditionID, o.Source, o.ExternalKey)
	if err == ErrNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	return s.ResolveObservation(ctx, sid, *id, true)
}
