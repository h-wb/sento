package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/h-wb/sento/internal/apperr"
	"strings"
	"time"
)

// JSONText is raw JSON that scans from a TEXT column (json.RawMessage
// itself has no sql.Scanner).
type JSONText []byte

func (j *JSONText) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case string:
		*j = JSONText(v)
	case []byte:
		*j = append(JSONText(nil), v...)
	default:
		return fmt.Errorf("cannot scan %T into JSONText", src)
	}
	return nil
}

func (j JSONText) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

type QueueItem struct {
	RetryCount int      `json:"retryCount"`
	RetryAt    string   `json:"retryAt"`
	ProviderID string   `json:"providerId,omitempty"`
	ReleaseKey string   `json:"releaseKey,omitempty"`
	Snapshot   JSONText `json:"snapshot" db:"snapshot_json" tstype:"any"`
	ID         int64    `json:"id"`
	SeriesID   int64    `json:"seriesId"`
	Protocol   string   `json:"protocol"` // torrent|source
	ExternalID string   `json:"externalId"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Books      JSONText `json:"books" db:"expected_slots_json" tstype:"BookRef[]"`
	Error      string   `json:"error,omitempty"`
	CreatedAt  string   `json:"createdAt"`
	UpdatedAt  string   `json:"updatedAt,omitempty"`
	// Progress is the live download fraction (0–1) for in-flight items,
	// filled in by the queue handler from the provider. Runtime-only: it is
	// neither stored nor scanned from the database.
	Progress *float64 `json:"progress,omitempty" db:"-"`
}

const (
	QueueQueued          = "queued"
	QueueDownloading     = "downloading"
	QueueImporting       = "importing"
	QueueImported        = "imported"
	QueueFailed          = "failed"
	QueueNeedsReview     = "needs_review"
	QueueRetryWait       = "retry_wait"
	QueueDispatchUnknown = "dispatch_unknown"
	QueueCancelled       = "cancelled"
)

// BookRef records explicit version and book coverage for a pending grab.
type BookRef struct {
	VersionID    int64  `json:"versionId,omitempty"`
	BookID       int64  `json:"bookId,omitempty"`
	Ledger       string `json:"ledger"`
	CollectionID int64  `json:"collectionId"`
	Number       int64  `json:"number"`
}

// UnmarshalJSON also accepts "workId" and "targetId", the names these
// fields had in queue items, decisions and snapshots written before they
// were renamed.
func (b *BookRef) UnmarshalJSON(data []byte) error {
	type plain BookRef
	aux := struct {
		*plain
		LegacyBook    int64 `json:"workId"`
		LegacyVersion int64 `json:"targetId"`
	}{plain: (*plain)(b)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if b.BookID == 0 {
		b.BookID = aux.LegacyBook
	}
	if b.VersionID == 0 {
		b.VersionID = aux.LegacyVersion
	}
	return nil
}

func (s *Store) AddQueueItem(ctx context.Context, q QueueItem) (int64, error) {
	return s.insert(ctx, `
		INSERT INTO queue_items (series_id, protocol, external_id, title, status, expected_slots_json)
		VALUES (?,?,?,?,?,?) RETURNING id`,
		q.SeriesID, q.Protocol, q.ExternalID, q.Title, orDefault(q.Status, QueueQueued),
		string(q.Books))
}

func (s *Store) UpdateQueueItem(ctx context.Context, id int64, status, errMsg string) error {
	return s.touch(ctx, `
		UPDATE queue_items SET status = ?, error = ?, updated_at = ? WHERE id = ? AND status NOT IN('cancelled','imported')`,
		status, errMsg, utcNow(), id)
}

func (s *Store) ListQueueItems(ctx context.Context, activeOnly bool) ([]QueueItem, error) {
	q := `SELECT id, series_id, protocol, external_id, title, status, expected_slots_json,
	             COALESCE(error,'') AS error, created_at,provider_id,release_key,snapshot_json,
	             COALESCE(updated_at,'') AS updated_at,retry_count,retry_at
	      FROM queue_items`
	if activeOnly {
		q += ` WHERE status IN ('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')`
	}
	q += ` ORDER BY created_at DESC LIMIT 200`
	return list[QueueItem](ctx, s, q)
}

// SetSeriesChapterPath records the separate chapter folder ("" = chapters
// share the series folder).
func (s *Store) SetSeriesChapterPath(ctx context.Context, seriesID int64, path string) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE series SET chapter_path = ? WHERE id = ?`), path, seriesID)
	return err
}

func (s *Store) SetSeriesPath(ctx context.Context, seriesID int64, path string) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE series SET path = ? WHERE id = ?`), path, seriesID)
	return err
}

// SeriesTitles returns the primary title plus every alias from the cached
// provider payload — anchoring and source binding match releases named in
// any language (Japscan lists French titles, raws use kana, …).
func (s *Store) SeriesTitles(ctx context.Context, seriesID int64) ([]string, error) {
	row, err := get[struct {
		Title   string
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT title, COALESCE(raw_json,'') AS raw_json FROM series WHERE id = ?`, seriesID)
	if err != nil {
		return nil, err
	}
	titles := []string{row.Title}
	add := func(t string) {
		t = strings.TrimSpace(t)
		if t == "" {
			return
		}
		for _, ex := range titles {
			if strings.EqualFold(ex, t) {
				return
			}
		}
		titles = append(titles, t)
	}
	var raw struct {
		NativeTitle    string `json:"native_title"`
		RomanizedTitle string `json:"romanized_title"`
		Titles         []struct {
			Title string `json:"title"`
		} `json:"titles"`
		Secondary map[string][]struct {
			Title string `json:"title"`
		} `json:"secondary_titles"`
	}
	if json.Unmarshal([]byte(row.RawJSON), &raw) == nil {
		add(raw.NativeTitle)
		add(raw.RomanizedTitle)
		for _, t := range raw.Titles {
			add(t.Title)
		}
		for _, list := range raw.Secondary {
			for _, t := range list {
				add(t.Title)
			}
		}
	}
	return titles, nil
}

func (s *Store) PendingBooks(ctx context.Context, seriesID int64) (map[string]bool, error) {
	raws, err := list[string](ctx, s, `SELECT expected_slots_json FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, raw := range raws {
		var slots []BookRef
		if err = json.Unmarshal([]byte(raw), &slots); err != nil {
			return nil, err
		}
		for _, sl := range slots {
			if sl.VersionID > 0 && sl.BookID > 0 {
				out[BookKey(sl.VersionID, sl.BookID)] = true
			}
		}
	}
	return out, nil
}

// ReserveQueue claims every version/book before contacting a downloader. Failed
// jobs release claims on the next reservation; completed jobs retain history.
func (s *Store) ReserveQueue(ctx context.Context, q QueueItem, slots []BookRef) (int64, bool, error) {
	if len(slots) == 0 {
		return 0, false, apperr.Invalidf("release has no verified book coverage")
	}
	if err := s.EnsureModels(ctx, q.SeriesID); err != nil {
		return 0, false, err
	}
	if !json.Valid(q.Snapshot) {
		q.Snapshot = JSONText(`{}`)
	}
	for i := range slots {
		sl := &slots[i]
		c, e := s.GetCollection(ctx, sl.CollectionID)
		if e != nil {
			return 0, false, e
		}
		if c.SeriesID != q.SeriesID || c.Type != sl.Ledger {
			return 0, false, apperr.Invalidf("invalid book edition")
		}
		w, e := s.GetBook(ctx, sl.BookID)
		if e != nil {
			return 0, false, e
		}
		t, e := s.GetVersion(ctx, sl.VersionID)
		if e != nil {
			return 0, false, e
		}
		if w.CollectionID != c.ID || t.CollectionID != c.ID {
			return 0, false, apperr.Invalidf("book/version belongs to another edition")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback() }()
	editionIDs := []int64{}
	for _, sl := range slots {
		editionIDs = append(editionIDs, sl.CollectionID)
	}
	if err = s.lockEditions(ctx, tx, true, editionIDs...); err != nil {
		return 0, false, err
	}
	for _, sl := range slots {
		var n int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM works w JOIN download_targets t ON t.collection_id=w.collection_id WHERE w.id=? AND t.id=? AND w.collection_id=? AND w.sync_state<>'retired'`), sl.BookID, sl.VersionID, sl.CollectionID).Scan(&n); err != nil {
			return 0, false, err
		}
		if n != 1 {
			return 0, false, apperr.Conflictf("edition changed while preparing the download; search again")
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM unit_reservations WHERE queue_id IN(SELECT id FROM queue_items WHERE status IN('failed','imported','cancelled'))`); err != nil {
		return 0, false, err
	}
	var id int64
	var raw, snap string
	err = sql.ErrNoRows
	if q.ReleaseKey != "" {
		query := `SELECT id,expected_slots_json,snapshot_json FROM queue_items WHERE series_id=? AND provider_id=? AND release_key=? AND status IN('queued','downloading') ORDER BY id LIMIT 1`
		if s.pg {
			query += " FOR UPDATE"
		}
		err = tx.QueryRowContext(ctx, s.q(query), q.SeriesID, q.ProviderID, q.ReleaseKey).Scan(&id, &raw, &snap)
	}
	created := errors.Is(err, sql.ErrNoRows)
	var plan []BookRef
	if created {
		body, _ := json.Marshal(slots)
		if err = tx.QueryRowContext(ctx, s.q(`INSERT INTO queue_items(series_id,protocol,external_id,title,status,expected_slots_json,provider_id,release_key,snapshot_json) VALUES(?,?,'',?,'queued',?,?,?,?) RETURNING id`), q.SeriesID, q.Protocol, q.Title, string(body), q.ProviderID, q.ReleaseKey, string(q.Snapshot)).Scan(&id); err != nil {
			return 0, false, conflict(err, "release already has an active job")
		}
	} else if err != nil {
		return 0, false, err
	} else {
		if err = json.Unmarshal([]byte(raw), &plan); err != nil {
			return 0, false, err
		}
	}
	for _, sl := range slots {
		exists := false
		for _, old := range plan {
			if old.VersionID == sl.VersionID && old.BookID == sl.BookID && old.CollectionID == sl.CollectionID && old.Number == sl.Number {
				exists = true
			}
		}
		if exists {
			continue
		}
		if sl.VersionID > 0 && sl.BookID > 0 {
			if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO unit_reservations(target_id,work_id,queue_id) VALUES(?,?,?)`), sl.VersionID, sl.BookID, id); err != nil {
				return 0, false, conflict(err, "book already has a pending download")
			}
		}
		plan = append(plan, sl)
	}
	if !created {
		body, _ := json.Marshal(plan)
		snapshot := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(snap), &snapshot)
		var extra []json.RawMessage
		_ = json.Unmarshal(snapshot["extraEvaluations"], &extra)
		extra = append(extra, json.RawMessage(q.Snapshot))
		snapshot["extraEvaluations"], _ = json.Marshal(extra)
		saved, _ := json.Marshal(snapshot)
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE queue_items SET expected_slots_json=?,snapshot_json=? WHERE id=?`), string(body), string(saved), id); err != nil {
			return 0, false, err
		}
	}
	return id, created, tx.Commit()
}
func (s *Store) SetQueueExternalID(ctx context.Context, id int64, key string) error {
	return s.touch(ctx, `UPDATE queue_items SET external_id=?,status='queued',retry_count=0,retry_at='' WHERE id=? AND status NOT IN('cancelled','imported')`, key, id)
}

// BeginImport freezes the plan and reads it after acquiring the row's write
// lock. A concurrent version attachment either finishes first or is refused.
func (s *Store) BeginImport(ctx context.Context, id int64) (*QueueItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE queue_items SET status='importing',updated_at=? WHERE id=? AND status IN('queued','downloading','importing','retry_wait')`), utcNow(), id)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, apperr.Conflictf("download is no longer active")
	}
	var q QueueItem
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,series_id,protocol,external_id,title,status,expected_slots_json,provider_id,release_key,snapshot_json,created_at FROM queue_items WHERE id=?`), id).Scan(&q.ID, &q.SeriesID, &q.Protocol, &q.ExternalID, &q.Title, &q.Status, &q.Books, &q.ProviderID, &q.ReleaseKey, &q.Snapshot, &q.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &q, tx.Commit()
}

func (s *Store) DeferQueue(ctx context.Context, q QueueItem, reason string) error {
	delay := 30 * time.Second * time.Duration(1<<min(q.RetryCount, 6))
	return s.touch(ctx, `UPDATE queue_items SET status='retry_wait',error=?,retry_count=retry_count+1,retry_at=?,updated_at=? WHERE id=? AND status NOT IN('cancelled','imported')`, reason, time.Now().UTC().Add(delay).Format("2006-01-02 15:04:05"), utcNow(), q.ID)
}
func (s *Store) ResumeQueue(ctx context.Context, id int64, remoteID string) error {
	if remoteID != "" {
		return s.touch(ctx, `UPDATE queue_items SET status='queued',external_id=?,error='',retry_at='',retry_count=0 WHERE id=? AND status IN('needs_review','retry_wait','dispatch_unknown')`, remoteID, id)
	}
	return s.touch(ctx, `UPDATE queue_items SET status='queued',error='',retry_at='',retry_count=0 WHERE id=? AND status IN('needs_review','retry_wait','dispatch_unknown')`, id)
}
func (s *Store) CancelQueue(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE queue_items SET status='cancelled',updated_at=? WHERE id=? AND status NOT IN('imported','importing')`), utcNow(), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperr.Conflictf("wait for the current import to finish before cancelling")
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM unit_reservations WHERE queue_id=?`), id); err != nil {
		return err
	}
	return tx.Commit()
}
