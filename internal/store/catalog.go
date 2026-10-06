package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/metadata"
)

// A Work is a stable collection membership. External work references may be
// shared across collections; numbers and labels never identify its bytes.
func (s *Store) upsertWork(ctx context.Context, tx *sql.Tx, cid int64, w metadata.Work) error {
	var id int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM works WHERE collection_id=? AND (mb_work_id=? OR id IN(SELECT local_id FROM external_refs WHERE provider='mangabaka' AND kind='work' AND external_id=?)) ORDER BY id LIMIT 1`), cid, w.ProviderID, w.ProviderID).Scan(&id)
	if w.ProviderID == "" {
		err = sql.ErrNoRows
	}
	known := w.NumberKnown || w.SequenceNum != 0
	if !known {
		_, e := strconv.ParseFloat(w.SequenceStr, 64)
		known = e == nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, s.q(`INSERT INTO works(collection_id,mb_work_id,sequence_num,sequence_string,count_type,release_date,isbn,pages,raw_json,number_known,revision,sync_state) VALUES(?,?,?,?,?,?,?,?,?,?,?,'linked') RETURNING id`), cid, w.ProviderID, int64(w.SequenceNum), w.SequenceStr, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, string(w.RawJSON), known, w.Revision).Scan(&id)
	} else if err == nil {
		var overrides string
		if err = tx.QueryRowContext(ctx, s.q(`SELECT overrides_json FROM works WHERE id=?`), id).Scan(&overrides); err != nil {
			return err
		}
		local := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(overrides), &local)
		fields := map[string]any{"sequence_num": int64(w.SequenceNum), "sequence_string": w.SequenceStr, "count_type": w.CountType, "release_date": w.ReleaseDate, "isbn": w.ISBN, "pages": w.Pages, "number_known": known, "revision": w.Revision}
		for k, v := range fields {
			if _, ok := local[k]; !ok {
				if _, err = tx.ExecContext(ctx, s.q(`UPDATE works SET `+k+`=? WHERE id=?`), v, id); err != nil {
					return err
				}
			}
		}
		_, err = tx.ExecContext(ctx, s.q(`UPDATE works SET mb_work_id=?,raw_json=?,sync_state=CASE WHEN sync_state='retired' THEN 'retired' ELSE 'linked' END WHERE id=?`), w.ProviderID, string(w.RawJSON), id)
	}
	if err != nil {
		return err
	}
	if w.ProviderID != "" {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO external_refs(provider,kind,external_id,local_id,last_seen) VALUES('mangabaka','work',?,?,?) ON CONFLICT(provider,kind,external_id,local_id) DO UPDATE SET last_seen=excluded.last_seen`), w.ProviderID, id, utcNow())
	}
	return err
}

// EnsureModels creates default targets and explicitly inferred chapter units. It never adopts local metadata or touches bytes.
func (s *Store) EnsureModels(ctx context.Context, seriesID int64) error {
	series, err := s.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	cols, err := s.ListCollections(ctx, seriesID)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range cols {
		if c.EditionLayout == "" || c.ContentVariant == "" {
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET edition_layout=?,content_variant=? WHERE id=?`), orDefault(c.EditionLayout, metadata.EditionLayout(c.EditionName)), orDefault(c.ContentVariant, metadata.ContentVariant(c.EditionName)), c.ID); err != nil {
				return err
			}
		}
		if !c.Custom && c.SyncState == "local" {
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET sync_state='linked' WHERE id=?`), c.ID); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO download_targets(collection_id,name,is_default,monitored,cutoff_num) SELECT ?,'Default',TRUE,?,? WHERE NOT EXISTS(SELECT 1 FROM download_targets WHERE collection_id=? AND is_default=TRUE) ON CONFLICT DO NOTHING`), c.ID, c.Monitored, c.CutoffNum, c.ID); err != nil {
			return err
		}
		// Metadata's chapter count is a compatibility bound, not an observation.
		// Explicit chapter memberships always remain, including fractions/extras.
		if c.Type == "chapter" && c.NumberingSource == "series_count" && series.TotalChapters <= 10000 {
			var bound int
			if err = tx.QueryRowContext(ctx, s.q(`SELECT generated_bound FROM collections WHERE id=?`), c.ID).Scan(&bound); err != nil {
				return err
			}
			for n := bound + 1; n <= series.TotalChapters; n++ {
				if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO works(collection_id,sequence_num,sequence_string,count_type,sync_state) SELECT ?,?,?,'main','inferred' WHERE NOT EXISTS(SELECT 1 FROM works WHERE collection_id=? AND sequence_num=? AND count_type='main')`), c.ID, n*100, strconv.Itoa(n), c.ID, n*100); err != nil {
					return err
				}
			}
			if _, err = tx.ExecContext(ctx, s.q(`UPDATE collections SET generated_bound=? WHERE id=?`), max(bound, series.TotalChapters), c.ID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (s *Store) GetWork(ctx context.Context, id int64) (*Work, error) {
	return get[Work](ctx, s, `SELECT `+workColumns+` FROM works WHERE id=?`, id)
}

const workColumns = `id,collection_id,COALESCE(mb_work_id,'') AS provider_id,sequence_num,COALESCE(sequence_string,'') AS sequence_string,count_type,COALESCE(release_date,'') AS release_date,COALESCE(isbn,'') AS isbn,COALESCE(pages,0) AS pages,number_known,revision,sync_state`

func (s *Store) SaveWork(ctx context.Context, seriesID int64, w Work) (int64, error) {
	c, err := s.GetCollection(ctx, w.CollectionID)
	if err != nil {
		return 0, err
	}
	if c.SeriesID != seriesID {
		return 0, ErrNotFound
	}
	w.SequenceStr = strings.TrimSpace(w.SequenceStr)
	if w.SequenceNum < 0 {
		return 0, apperr.Invalidf("unit number cannot be negative")
	}
	if w.ReleaseDate != "" {
		if _, err := time.Parse("2006-01-02", w.ReleaseDate); err != nil {
			return 0, apperr.Invalidf("publication date must be YYYY-MM-DD")
		}
	}
	if w.SequenceStr == "" {
		return 0, apperr.Invalidf("unit label is required")
	}
	if w.CountType != "main" && w.CountType != "extra" && w.CountType != "other" {
		return 0, apperr.Invalidf("count type must be main, extra or other")
	}
	if w.ID == 0 {
		return s.insert(ctx, `INSERT INTO works(collection_id,sequence_num,sequence_string,count_type,release_date,isbn,pages,number_known,revision) VALUES(?,?,?,?,?,?,?,?,?) RETURNING id`, w.CollectionID, w.SequenceNum, w.SequenceStr, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, w.NumberKnown, w.Revision)
	}
	old, err := s.GetWork(ctx, w.ID)
	if err != nil {
		return 0, err
	}
	if old.CollectionID != c.ID {
		return 0, ErrNotFound
	}
	pending, err := s.CollectionPending(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	if pending {
		return 0, apperr.Conflictf("finish or cancel pending downloads before changing units")
	}
	previous, err := get[string](ctx, s, `SELECT overrides_json FROM works WHERE id=?`, w.ID)
	if err != nil {
		return 0, err
	}
	local := map[string]any{}
	if err = json.Unmarshal([]byte(*previous), &local); err != nil {
		return 0, err
	}
	before := map[string]any{"sequence_num": old.SequenceNum, "sequence_string": old.SequenceStr, "count_type": old.CountType, "release_date": old.ReleaseDate, "isbn": old.ISBN, "pages": old.Pages, "number_known": old.NumberKnown, "revision": old.Revision}
	after := map[string]any{"sequence_num": w.SequenceNum, "sequence_string": w.SequenceStr, "count_type": w.CountType, "release_date": w.ReleaseDate, "isbn": w.ISBN, "pages": w.Pages, "number_known": w.NumberKnown, "revision": w.Revision}
	for k, v := range after {
		if v != before[k] {
			local[k] = v
		}
	}
	overrides, _ := json.Marshal(local)
	err = s.touch(ctx, `UPDATE works SET sequence_num=?,sequence_string=?,count_type=?,release_date=?,isbn=?,pages=?,number_known=?,revision=?,overrides_json=? WHERE id=?`, w.SequenceNum, w.SequenceStr, w.CountType, w.ReleaseDate, w.ISBN, w.Pages, w.NumberKnown, w.Revision, string(overrides), w.ID)
	return w.ID, err
}
func (s *Store) DeleteWork(ctx context.Context, seriesID, id int64) error {
	w, err := s.GetWork(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, w.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	pending, err := s.CollectionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("collection has pending downloads")
	}
	n, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_coverage WHERE work_id=?`, id)
	if err != nil {
		return err
	}
	if *n > 0 {
		return apperr.Conflictf("unlink owned files before deleting this unit")
	}
	if w.ProviderID != "" {
		return s.touch(ctx, `UPDATE works SET sync_state='retired' WHERE id=?`, id)
	}
	return s.touch(ctx, `DELETE FROM works WHERE id=?`, id)
}
func (s *Store) CollectionPending(ctx context.Context, cid int64) (bool, error) {
	c, err := s.GetCollection(ctx, cid)
	if err != nil {
		return false, err
	}
	raws, err := list[string](ctx, s, `SELECT expected_slots_json FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing')`, c.SeriesID)
	if err != nil {
		return false, err
	}
	for _, raw := range raws {
		var slots []ExpectedSlot
		if err = json.Unmarshal([]byte(raw), &slots); err != nil {
			return false, fmt.Errorf("invalid pending download mapping: %w", err)
		}
		for _, sl := range slots {
			if sl.CollectionID == cid {
				return true, nil
			}
		}
	}
	return false, nil
}

type UnitObservation struct {
	ID           int64  `json:"id"`
	CollectionID int64  `json:"collectionId"`
	Source       string `json:"source"`
	ExternalKey  string `json:"externalKey"`
	Label        string `json:"label"`
	Number       int64  `json:"number"`
	State        string `json:"state"`
}

func (s *Store) ObserveUnit(ctx context.Context, seriesID int64, o UnitObservation) error {
	c, err := s.GetCollection(ctx, o.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if o.Number < 0 {
		return apperr.Invalidf("observation number cannot be negative")
	}
	if o.Source == "" || o.ExternalKey == "" || o.Label == "" {
		return apperr.Invalidf("source, external key and label are required")
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO unit_observations(collection_id,source,external_key,label,number) VALUES(?,?,?,?,?) ON CONFLICT(collection_id,source,external_key) DO NOTHING`), o.CollectionID, o.Source, o.ExternalKey, o.Label, o.Number)
	return err
}
func (s *Store) ListObservations(ctx context.Context, seriesID int64) ([]UnitObservation, error) {
	return list[UnitObservation](ctx, s, `SELECT id,collection_id,source,external_key,label,number,state FROM unit_observations WHERE collection_id IN(SELECT id FROM collections WHERE series_id=?) ORDER BY id DESC LIMIT 500`, seriesID)
}
func (s *Store) ResolveObservation(ctx context.Context, seriesID, id int64, accept bool) error {
	o, err := get[UnitObservation](ctx, s, `SELECT id,collection_id,source,external_key,label,number,state FROM unit_observations WHERE id=?`, id)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, o.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if o.State != "pending" {
		return apperr.Conflictf("observation already reviewed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	state := "rejected"
	if accept {
		state = "accepted"
	}
	res, err := tx.ExecContext(ctx, s.q(`UPDATE unit_observations SET state=? WHERE id=? AND state='pending'`), state, id)
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
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO works(collection_id,sequence_num,sequence_string,count_type,number_known) SELECT ?,?,?,'main',TRUE WHERE NOT EXISTS(SELECT 1 FROM works WHERE collection_id=? AND sequence_num=? AND count_type='main')`), c.ID, o.Number, o.Label, c.ID, o.Number)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type WorkRelation struct {
	FromID   int64  `json:"fromId"`
	ToID     int64  `json:"toId"`
	Relation string `json:"relation"`
}

func (s *Store) SetRelation(ctx context.Context, seriesID int64, r WorkRelation) error {
	if r.FromID == r.ToID {
		return apperr.Invalidf("choose two different units")
	}
	for _, id := range []int64{r.FromID, r.ToID} {
		w, err := s.GetWork(ctx, id)
		if err != nil {
			return err
		}
		c, err := s.GetCollection(ctx, w.CollectionID)
		if err != nil {
			return err
		}
		if c.SeriesID != seriesID {
			return ErrNotFound
		}
	}
	if r.Relation != "contains" && r.Relation != "equivalent" && r.Relation != "revision" {
		return apperr.Invalidf("invalid relationship")
	}
	if r.Relation != "equivalent" {
		n, err := get[int](ctx, s, `WITH RECURSIVE reachable(id) AS (SELECT to_id FROM work_relations WHERE from_id=? AND relation=? UNION SELECT rel.to_id FROM work_relations rel JOIN reachable reach ON rel.from_id=reach.id WHERE rel.relation=?) SELECT COUNT(*) FROM reachable WHERE id=?`, r.ToID, r.Relation, r.Relation, r.FromID)
		if err != nil {
			return err
		}
		if *n > 0 {
			return apperr.Invalidf("relationship would create a cycle")
		}
	}
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO work_relations(from_id,to_id,relation) VALUES(?,?,?) ON CONFLICT DO NOTHING`), r.FromID, r.ToID, r.Relation)
	return err
}
func (s *Store) Relations(ctx context.Context, seriesID int64) ([]WorkRelation, error) {
	return list[WorkRelation](ctx, s, `SELECT from_id,to_id,relation FROM work_relations WHERE from_id IN(SELECT w.id FROM works w JOIN collections c ON c.id=w.collection_id WHERE c.series_id=?)`, seriesID)
}

func (s *Store) SetBindingEdition(ctx context.Context, seriesID, bindingID, cid int64) error {
	if cid > 0 {
		c, err := s.GetCollection(ctx, cid)
		if err != nil {
			return err
		}
		if c.SeriesID != seriesID {
			return ErrNotFound
		}
	}
	return s.touch(ctx, `UPDATE source_bindings SET collection_id=? WHERE id=? AND series_id=?`, cid, bindingID, seriesID)
}

func (s *Store) DeleteRelation(ctx context.Context, seriesID int64, r WorkRelation) error {
	for _, id := range []int64{r.FromID, r.ToID} {
		w, e := s.GetWork(ctx, id)
		if e != nil {
			return e
		}
		c, e := s.GetCollection(ctx, w.CollectionID)
		if e != nil {
			return e
		}
		if c.SeriesID != seriesID {
			return ErrNotFound
		}
	}
	return s.touch(ctx, `DELETE FROM work_relations WHERE from_id=? AND to_id=? AND relation=?`, r.FromID, r.ToID, r.Relation)
}
func (s *Store) ResetWork(ctx context.Context, seriesID, id int64) error {
	w, err := s.GetWork(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, w.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if w.ProviderID == "" {
		return apperr.Invalidf("unit has no catalog link")
	}
	pending, err := s.CollectionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("finish pending downloads before resetting metadata")
	}
	return s.touch(ctx, `UPDATE works SET overrides_json='{}',sync_state='linked' WHERE id=?`, id)
}
