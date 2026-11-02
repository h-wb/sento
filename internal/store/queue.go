package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/h-wb/sento/internal/apperr"
	"sort"
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
	// CanTryAnother excludes jobs that already delivered files for review.
	CanTryAnother    bool    `json:"canTryAnother" db:"-"`
	DownloadProgress float64 `json:"downloadProgress"`
	ProgressAt       string  `json:"progressAt"`
	ObservedAt       string  `json:"observedAt"`
	// SeriesTitle is filled by the Activity listing only.
	SeriesTitle   string `json:"seriesTitle,omitempty" db:"series_title"`
	DownloadState string `json:"downloadState"`
	Stalled       bool   `json:"stalled"`
	RetryCount    int    `json:"retryCount"`
	RetryAt       string `json:"retryAt"`
	ProviderID    string `json:"providerId,omitempty"`
	ReleaseKey    string `json:"releaseKey,omitempty"`
	// DownloadClient is the id of the client the download was sent to,
	// when the provider has several ("" = unrecorded or not applicable).
	DownloadClient string   `json:"downloadClient,omitempty" db:"download_client"`
	Snapshot       JSONText `json:"snapshot" db:"snapshot_json" tstype:"any"`
	ID             int64    `json:"id"`
	SeriesID       int64    `json:"seriesId"`
	Protocol       string   `json:"protocol"` // torrent|source
	ExternalID     string   `json:"externalId"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Books          JSONText `json:"books" db:"books_json" tstype:"BookRef[]"`
	Error          string   `json:"error,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt,omitempty"`
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
	VersionID int64  `json:"versionId,omitempty"`
	BookID    int64  `json:"bookId,omitempty"`
	Ledger    string `json:"ledger"`
	EditionID int64  `json:"editionId"`
	Number    int64  `json:"number"`
}

func (s *Store) AddQueueItem(ctx context.Context, q QueueItem) (int64, error) {
	return s.insert(ctx, `
		INSERT INTO queue_items (series_id, protocol, external_id, title, status, books_json)
		VALUES (?,?,?,?,?,?) RETURNING id`,
		q.SeriesID, q.Protocol, q.ExternalID, q.Title, orDefault(q.Status, QueueQueued),
		string(q.Books))
}

func (s *Store) UpdateQueueItem(ctx context.Context, id int64, status, errMsg string) error {
	return s.touch(ctx, `
		UPDATE queue_items SET status = ?, error = ?, updated_at = ? WHERE id = ? AND status NOT IN('cancelled','imported')`,
		status, errMsg, utcNow(), id)
}

const queueColumns = `id, series_id, protocol, external_id, title, status, books_json,
	             COALESCE(error,'') AS error, created_at,provider_id,release_key,snapshot_json,
	             COALESCE(updated_at,'') AS updated_at,retry_count,retry_at,download_client,
	             download_progress,progress_at,observed_at,download_state,stalled`

// ListQueueItems is the Activity listing, newest first, with each job's
// series title: every job still in progress or waiting on a person, and
// unless activeOnly the 200 most recent finished ones. It is a page for
// people, not a work list (see ListRunnableQueue).
func (s *Store) ListQueueItems(ctx context.Context, activeOnly bool) ([]QueueItem, error) {
	const columns = `SELECT ` + queueColumns + `, COALESCE((SELECT title FROM series WHERE series.id=queue_items.series_id),'') AS series_title FROM queue_items`
	items, err := list[QueueItem](ctx, s, columns+` WHERE status NOT IN ('imported','cancelled') ORDER BY created_at DESC, id DESC`)
	if err != nil || activeOnly {
		return items, err
	}
	finished, err := list[QueueItem](ctx, s, columns+` WHERE status IN ('imported','cancelled') ORDER BY created_at DESC, id DESC LIMIT 200`)
	return append(items, finished...), err
}

// PruneFinishedQueue deletes terminal jobs (imported/cancelled/failed)
// beyond the newest keep, so the table does not grow without bound; the
// Activity page already shows only the newest 200 finished ones.
func (s *Store) PruneFinishedQueue(ctx context.Context, keep int) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.q(`
		DELETE FROM queue_items WHERE status IN ('imported','cancelled','failed')
		AND id NOT IN (
			SELECT id FROM queue_items WHERE status IN ('imported','cancelled','failed')
			ORDER BY created_at DESC, id DESC LIMIT ?
		)`), keep)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// DownloadShared reports whether a job of another series holds the same
// download (one pack can serve two series).
func (s *Store) DownloadShared(ctx context.Context, seriesID int64, protocol, externalID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM queue_items WHERE protocol=? AND external_id=? AND series_id<>?`), protocol, externalID, seriesID).Scan(&n)
	return n > 0, err
}

// SeriesQueueItems is every job of one series, whatever its state.
func (s *Store) SeriesQueueItems(ctx context.Context, seriesID int64) ([]QueueItem, error) {
	return list[QueueItem](ctx, s, `SELECT `+queueColumns+` FROM queue_items WHERE series_id = ? ORDER BY id`, seriesID)
}

// ListRunnableQueue is the worker's list: every job it can advance, oldest
// first. Jobs waiting on a person (needs_review, dispatch_unknown) are left
// out, so no number of them can keep a runnable job from being seen.
func (s *Store) ListRunnableQueue(ctx context.Context) ([]QueueItem, error) {
	return list[QueueItem](ctx, s, `SELECT `+queueColumns+` FROM queue_items
		WHERE status IN ('queued','downloading','importing','retry_wait') ORDER BY created_at, id`)
}

// GetQueueItem reads one job by id, however old.
func (s *Store) GetQueueItem(ctx context.Context, id int64) (*QueueItem, error) {
	return get[QueueItem](ctx, s, `SELECT `+queueColumns+` FROM queue_items WHERE id = ?`, id)
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

// seriesNames is a series' primary title and the names MangaBaka gives it.
type seriesNames struct {
	NativeTitle    string `json:"native_title"`
	RomanizedTitle string `json:"romanized_title"`
	Titles         []struct {
		Title string `json:"title"`
	} `json:"titles"`
	Secondary map[string][]struct {
		Title string `json:"title"`
	} `json:"secondary_titles"`
}

func (s *Store) seriesNames(ctx context.Context, seriesID int64) (string, seriesNames, error) {
	row, err := get[struct {
		Title   string
		RawJSON string `db:"raw_json"`
	}](ctx, s, `SELECT title, COALESCE(raw_json,'') AS raw_json FROM series WHERE id = ?`, seriesID)
	if err != nil {
		return "", seriesNames{}, err
	}
	var raw seriesNames
	_ = json.Unmarshal([]byte(row.RawJSON), &raw)
	return row.Title, raw, nil
}

// titleList collects titles once each, in order, ignoring case.
type titleList []string

func (l *titleList) add(t string) {
	t = strings.TrimSpace(t)
	if t == "" {
		return
	}
	for _, ex := range *l {
		if strings.EqualFold(ex, t) {
			return
		}
	}
	*l = append(*l, t)
}

// SeriesTitles returns the primary title plus every alias from the cached
// provider payload — anchoring and source binding match releases named in
// any language (Japscan lists French titles, raws use kana, …).
func (s *Store) SeriesTitles(ctx context.Context, seriesID int64, preferredLanguage ...string) ([]string, error) {
	title, raw, err := s.seriesNames(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	titles := titleList{title}
	for _, lang := range preferredLanguage {
		for _, t := range raw.Secondary[lang] {
			titles.add(t.Title)
		}
	}
	titles.add(raw.NativeTitle)
	titles.add(raw.RomanizedTitle)
	for _, t := range raw.Titles {
		titles.add(t.Title)
	}
	languages := make([]string, 0, len(raw.Secondary))
	for lang := range raw.Secondary {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	for _, lang := range languages {
		for _, t := range raw.Secondary[lang] {
			titles.add(t.Title)
		}
	}
	return titles, nil
}

// QueryTitles are the names worth searching an indexer by for editions in
// the given languages: the primary title, the titles in those languages,
// and the romanized one. Other languages' aliases ("budak getah") only
// find other languages' releases.
func (s *Store) QueryTitles(ctx context.Context, seriesID int64, languages ...string) ([]string, error) {
	title, raw, err := s.seriesNames(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	titles := titleList{title}
	for _, lang := range languages {
		for _, t := range raw.Secondary[lang] {
			titles.add(t.Title)
		}
	}
	titles.add(raw.RomanizedTitle)
	return titles, nil
}

// PendingBookStates retains the phase of each reservation. File review can
// hold an unfinished pack without hiding books it already imported. An
// overlapping transfer takes precedence over review-only work.
func (s *Store) PendingBookStates(ctx context.Context, seriesID int64) (map[string]string, error) {
	rows, err := list[struct {
		Books  string `db:"books_json"`
		Status string
	}](ctx, s, `SELECT books_json,status FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, row := range rows {
		var slots []BookRef
		if err = json.Unmarshal([]byte(row.Books), &slots); err != nil {
			return nil, err
		}
		for _, sl := range slots {
			if sl.VersionID > 0 && sl.BookID > 0 {
				key := BookKey(sl.VersionID, sl.BookID)
				if out[key] == "" || out[key] == QueueNeedsReview {
					out[key] = row.Status
				}
			}
		}
	}
	return out, nil
}

func (s *Store) PendingBooks(ctx context.Context, seriesID int64) (map[string]bool, error) {
	states, err := s.PendingBookStates(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for key := range states {
		out[key] = true
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
		c, e := s.GetEdition(ctx, sl.EditionID)
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
		if w.EditionID != c.ID || t.EditionID != c.ID {
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
		editionIDs = append(editionIDs, sl.EditionID)
	}
	if err = s.lockEditions(ctx, tx, true, editionIDs...); err != nil {
		return 0, false, err
	}
	for _, sl := range slots {
		var n int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN versions t ON t.edition_id=w.edition_id WHERE w.id=? AND t.id=? AND w.edition_id=? AND w.sync_state<>'retired'`), sl.BookID, sl.VersionID, sl.EditionID).Scan(&n); err != nil {
			return 0, false, err
		}
		if n != 1 {
			return 0, false, apperr.Conflictf("edition changed while preparing the download; search again")
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM book_reservations WHERE queue_id IN(SELECT id FROM queue_items WHERE status IN('failed','imported','cancelled'))`); err != nil {
		return 0, false, err
	}
	var id int64
	var raw, snap string
	err = sql.ErrNoRows
	if q.ReleaseKey != "" {
		query := `SELECT id,books_json,snapshot_json FROM queue_items WHERE series_id=? AND provider_id=? AND release_key=? AND status IN('queued','downloading') ORDER BY id LIMIT 1`
		if s.pg {
			query += " FOR UPDATE"
		}
		err = tx.QueryRowContext(ctx, s.q(query), q.SeriesID, q.ProviderID, q.ReleaseKey).Scan(&id, &raw, &snap)
	}
	created := errors.Is(err, sql.ErrNoRows)
	var plan []BookRef
	switch {
	case created:
		body, _ := json.Marshal(slots)
		if err = tx.QueryRowContext(ctx, s.q(`INSERT INTO queue_items(series_id,protocol,external_id,title,status,books_json,provider_id,release_key,snapshot_json,download_client) VALUES(?,?,'',?,'queued',?,?,?,?,?) RETURNING id`), q.SeriesID, q.Protocol, q.Title, string(body), q.ProviderID, q.ReleaseKey, string(q.Snapshot), q.DownloadClient).Scan(&id); err != nil {
			return 0, false, conflict(err, "release already has an active job")
		}
	case err != nil:
		return 0, false, err
	default:
		if err = json.Unmarshal([]byte(raw), &plan); err != nil {
			return 0, false, err
		}
	}
	for _, sl := range slots {
		exists := false
		for _, old := range plan {
			if old.VersionID == sl.VersionID && old.BookID == sl.BookID && old.EditionID == sl.EditionID && old.Number == sl.Number {
				exists = true
			}
		}
		if exists {
			continue
		}
		if sl.VersionID > 0 && sl.BookID > 0 {
			if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO book_reservations(version_id,book_id,queue_id) VALUES(?,?,?)`), sl.VersionID, sl.BookID, id); err != nil {
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
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE queue_items SET books_json=?,snapshot_json=? WHERE id=?`), string(body), string(saved), id); err != nil {
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
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,series_id,protocol,external_id,title,status,books_json,provider_id,release_key,snapshot_json,created_at,download_client FROM queue_items WHERE id=?`), id).Scan(&q.ID, &q.SeriesID, &q.Protocol, &q.ExternalID, &q.Title, &q.Status, &q.Books, &q.ProviderID, &q.ReleaseKey, &q.Snapshot, &q.CreatedAt, &q.DownloadClient)
	if err != nil {
		return nil, err
	}
	return &q, tx.Commit()
}

func (s *Store) DeferQueue(ctx context.Context, q QueueItem, reason string) error {
	return s.DeferQueueUntil(ctx, q, reason, time.Time{})
}

// DeferQueueUntil respects both the queue's backoff and the source's
// Retry-After deadline. The latter may exceed the usual backoff cap.
func (s *Store) DeferQueueUntil(ctx context.Context, q QueueItem, reason string, notBefore time.Time) error {
	delay := 30 * time.Second * time.Duration(1<<min(q.RetryCount, 6))
	at := time.Now().UTC().Add(delay)
	if notBefore.After(at) {
		at = notBefore.UTC()
	}
	// Stored timestamps have second precision; round up, never retry early.
	at = at.Add(time.Second - time.Nanosecond).Truncate(time.Second)
	return s.touch(ctx, `UPDATE queue_items SET status='retry_wait',error=?,retry_count=retry_count+1,retry_at=?,updated_at=?,progress_at='',stalled=FALSE WHERE id=? AND status NOT IN('cancelled','imported')`, reason, at.Format("2006-01-02 15:04:05"), utcNow(), q.ID)
}
func (s *Store) ResumeQueue(ctx context.Context, id int64, remoteID string) error {
	if remoteID != "" {
		return s.touch(ctx, `UPDATE queue_items SET status='queued',external_id=?,error='',retry_at='',retry_count=0 WHERE id=? AND status IN('needs_review','retry_wait','dispatch_unknown')`, remoteID, id)
	}
	return s.touch(ctx, `UPDATE queue_items SET status='queued',error='',retry_at='',retry_count=0 WHERE id=? AND status IN('needs_review','retry_wait','dispatch_unknown')`, id)
}

// SettleReviewedQueue finishes a job held for review whose files have all
// been reviewed: it is imported, and its reservations are released so the
// books it did not deliver are wanted again. False when the job was no
// longer waiting.
func (s *Store) SettleReviewedQueue(ctx context.Context, id int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.q(`UPDATE queue_items SET status='imported',error='',updated_at=? WHERE id=? AND status='needs_review'`), utcNow(), id)
	if err != nil {
		return false, err
	}
	if n, err := result.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM book_reservations WHERE queue_id=?`), id); err != nil {
		return false, err
	}
	return true, tx.Commit()
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
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM book_reservations WHERE queue_id=?`), id); err != nil {
		return err
	}
	return tx.Commit()
}
