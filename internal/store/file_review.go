package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
)

type reviewQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// ReviewRevision identifies the facts a review decision depends on. Stable
// ordering includes deselected mappings and pins, not just accepted owners.
func (s *Store) ReviewRevision(ctx context.Context, sid int64) (string, error) {
	return s.reviewRevision(ctx, s.db, sid)
}

func (s *Store) reviewRevision(ctx context.Context, db reviewQuerier, sid int64) (string, error) {
	h := sha256.New()
	queries := []string{
		`SELECT * FROM series WHERE id=? ORDER BY id`,
		`SELECT * FROM editions WHERE series_id=? ORDER BY id`,
		`SELECT * FROM versions WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY id`,
		`SELECT * FROM books WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY id`,
		`SELECT id,series_id,path,size,hash,facts_json,claims_json,presence,needs_review,unrequested,review_action FROM files WHERE series_id=? ORDER BY id`,
		`SELECT c.* FROM asset_contents c JOIN files f ON f.id=c.file_id WHERE f.series_id=? ORDER BY c.file_id,c.book_id`,
		`SELECT a.* FROM asset_coverage a JOIN files f ON f.id=a.file_id WHERE f.series_id=? ORDER BY a.file_id,a.version_id,a.book_id`,
		`SELECT t.* FROM file_trash t JOIN files f ON f.id=t.file_id WHERE f.series_id=? ORDER BY t.file_id`,
		// A download's progress through the queue is not review evidence; its
		// files, books and requested replacements are.
		`SELECT id,books_json,snapshot_json,release_key,external_id FROM queue_items WHERE series_id=? ORDER BY id`,
		`SELECT * FROM profiles ORDER BY id`,
		`SELECT * FROM custom_formats ORDER BY id`,
		// Scheduler timestamps and connection bookkeeping do not change a
		// review decision. Only inherited policy and file placement do.
		`SELECT * FROM settings WHERE key IN('default_profile_id','media') ORDER BY key`,
	}
	// A metadata refresh stamps the series even when nothing it read changed.
	skip := map[string]bool{"refreshed_at": true}
	for _, query := range queries {
		if err := json.NewEncoder(h).Encode(query); err != nil {
			return "", err
		}
		var args []any
		if strings.ContainsRune(query, '?') {
			args = []any{sid}
		}
		rows, err := db.QueryContext(ctx, s.q(query), args...)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			return "", err
		}
		for rows.Next() {
			values := make([]any, len(columns))
			ptrs := make([]any, len(columns))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err = rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return "", err
			}
			kept := values[:0]
			for i, v := range values {
				if !skip[columns[i]] {
					kept = append(kept, v)
				}
			}
			if err = json.NewEncoder(h).Encode(kept); err != nil {
				_ = rows.Close()
				return "", err
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FileReviewWrite records content confirmation independently of selection.
// Replacements name the exact former owners seen in the validated preview.
type FileReviewWrite struct {
	FileID       int64
	Facts        AssetFacts
	Hash         string
	Reset        bool
	Contents     []BookRef
	Selected     []BookRef
	Replacements []AssetCoverage
	Disposition  string
}

// validate checks external evidence after acquiring the decision locks and
// before changing any rows. It must not access the store while its tx is open.
func (s *Store) CommitFileReviews(ctx context.Context, sid int64, revision string, writes []FileReviewWrite, validate func() error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Inherited policy lives on the series itself. Lock it before editions
	// (the same order as catalog refresh) so direct profile/path updates cannot
	// change the decision snapshot between its revision check and commit.
	query := `SELECT id FROM series WHERE id=?`
	if s.pg {
		query += ` FOR UPDATE`
	} else if _, err = tx.ExecContext(ctx, s.q(`UPDATE series SET id=id WHERE id=?`), sid); err != nil {
		return err
	}
	var lockedID int64
	if err = tx.QueryRowContext(ctx, s.q(query), sid).Scan(&lockedID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM editions WHERE series_id=? ORDER BY id`), sid)
	if err != nil {
		return err
	}
	var eids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		eids = append(eids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if err = s.lockEditions(ctx, tx, true, eids...); err != nil {
		return err
	}
	if s.pg {
		// Configuration writers do not take edition locks. Keep the profile,
		// format and default-setting snapshot stable until selection commits.
		if _, err = tx.ExecContext(ctx, `LOCK TABLE profiles,custom_formats,settings IN SHARE MODE`); err != nil {
			return err
		}
	}
	fresh, err := s.reviewRevision(ctx, tx, sid)
	if err != nil {
		return err
	}
	if revision != fresh {
		return apperr.Conflictf("review changed; refresh the preview before applying choices")
	}
	// Check protection against the original selections across the whole batch.
	// An earlier write must not deselect an owner to make a later trash safe.
	for _, w := range writes {
		if w.Disposition == "trash" {
			uses, e := s.reviewFileUses(ctx, tx, w.FileID)
			if e != nil {
				return e
			}
			if uses > 0 {
				return apperr.Conflictf("this archive is currently selected; keep it instead of moving it to trash")
			}
		}
	}
	if validate != nil {
		if err = validate(); err != nil {
			return err
		}
	}
	for _, w := range writes {
		if err = s.writeFileReview(ctx, tx, sid, w); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) writeFileReview(ctx context.Context, tx *sql.Tx, sid int64, w FileReviewWrite) error {
	var hash string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(hash,'') FROM files WHERE id=? AND series_id=?`), w.FileID, sid).Scan(&hash); err != nil {
		return err
	}
	if hash != "" && hash != w.Hash {
		return apperr.Conflictf("archive contents changed; rescan the file before reviewing it")
	}
	if err := s.rejectTrashEdit(ctx, tx, w.FileID); err != nil {
		return err
	}
	pins := map[string]bool{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT version_id,book_id FROM asset_coverage WHERE file_id=? AND pinned=TRUE`), w.FileID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var vid, bid int64
		if err = rows.Scan(&vid, &bid); err != nil {
			_ = rows.Close()
			return err
		}
		pins[BookKey(vid, bid)] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if w.Reset {
		for key := range pins {
			if !slices.ContainsFunc(w.Selected, func(b BookRef) bool { return BookKey(b.VersionID, b.BookID) == key }) {
				return apperr.Conflictf("unpin the file before removing its selections")
			}
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_coverage WHERE file_id=?`), w.FileID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_contents WHERE file_id=?`), w.FileID); err != nil {
			return err
		}
	}
	for _, b := range w.Contents {
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,verified,evidence) VALUES(?,?,TRUE,'manual') ON CONFLICT(file_id,book_id) DO UPDATE SET verified=TRUE,evidence='manual'`), w.FileID, b.BookID); err != nil {
			return err
		}
		// Explicitly record non-selection in every existing version. Otherwise
		// the implicit selection rule would adopt an unused duplicate.
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,selected,evidence)
 SELECT ?,id,?,FALSE,'manual' FROM versions WHERE edition_id=? ON CONFLICT DO NOTHING`), w.FileID, b.BookID, b.EditionID); err != nil {
			return err
		}
	}
	for _, b := range w.Contents {
		selected := slices.ContainsFunc(w.Selected, func(v BookRef) bool { return v.VersionID == b.VersionID && v.BookID == b.BookID })
		if pins[BookKey(b.VersionID, b.BookID)] && !selected {
			return apperr.Conflictf("unpin the file before removing its selections")
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE asset_coverage SET selected=?,pinned=?,evidence='manual' WHERE file_id=? AND version_id=? AND book_id=?`), selected, pins[BookKey(b.VersionID, b.BookID)], w.FileID, b.VersionID, b.BookID); err != nil {
			return err
		}
	}
	for _, a := range w.Replacements {
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,selected)
 VALUES(?,?,?,FALSE) ON CONFLICT(file_id,version_id,book_id) DO UPDATE SET selected=FALSE WHERE asset_coverage.pinned=FALSE`), a.FileID, a.VersionID, a.BookID); err != nil {
			return err
		}
	}
	if len(w.Contents) == 0 && !w.Reset {
		if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_contents WHERE file_id=? AND verified=FALSE`), w.FileID); err != nil {
			return err
		}
	}
	uses, err := s.reviewFileUses(ctx, tx, w.FileID)
	if err != nil {
		return err
	}
	if w.Disposition == "trash" {
		if uses > 0 || len(pins) > 0 {
			return apperr.Conflictf("this archive is still selected or pinned; keep the shared file")
		}
	}
	raw, err := json.Marshal(w.Facts)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE files SET facts_json=?,hash=?,needs_review=FALSE,unrequested=?,review_action=? WHERE id=? AND series_id=?`), string(raw), w.Hash, uses == 0, w.Disposition, w.FileID, sid)
	return err
}

func (s *Store) reviewFileUses(ctx context.Context, tx *sql.Tx, fid int64) (int, error) {
	var uses int
	err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM asset_contents c JOIN books b ON b.id=c.book_id JOIN versions v ON v.edition_id=b.edition_id
 LEFT JOIN asset_coverage a ON a.file_id=c.file_id AND a.version_id=v.id AND a.book_id=c.book_id
 WHERE c.file_id=? AND (COALESCE(a.selected,TRUE)=TRUE OR COALESCE(a.pinned,FALSE)=TRUE)`), fid).Scan(&uses)
	return uses, err
}

func (s *Store) PendingReviewTrash(ctx context.Context, afterID int64) ([]File, error) {
	return list[File](ctx, s, `SELECT `+fileColumns+` FROM files WHERE review_action='trash' AND needs_review=FALSE AND id>? ORDER BY id LIMIT 500`, afterID)
}

func (s *Store) CompleteReviewTrash(ctx context.Context, fid int64) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE files SET review_action='trashed' WHERE id=? AND review_action='trash' AND EXISTS(SELECT 1 FROM file_trash WHERE file_id=files.id)`), fid)
	return err
}

// The caller has verified that no archive exists at the journal's destination.
// Reopen the changed original for review without renewing consent for its bytes.
func (s *Store) ReopenReviewTrash(ctx context.Context, fid int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM file_trash WHERE file_id=?`), fid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=TRUE WHERE id=? AND review_action='trash'`), fid); err != nil {
		return err
	}
	return tx.Commit()
}
