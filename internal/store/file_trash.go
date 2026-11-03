package store

import (
	"context"
	"database/sql"

	"github.com/h-wb/sento/internal/apperr"
)

// TrashedFile retains the original file identity and mappings until expiry.
// Path is its original library path, reserved until restore or permanent deletion.
type TrashedFile struct {
	FileID     int64  `json:"fileId"`
	SeriesID   int64  `json:"seriesId"`
	Path       string `json:"path"`
	TrashPath  string `json:"trashPath"`
	Size       int64  `json:"size"`
	Digest     string `json:"-"`
	TrashedAt  string `json:"trashedAt"`
	PurgeAfter string `json:"purgeAfter"`
}

const trashColumns = `f.id AS file_id,f.series_id,f.path,f.size,t.trash_path,t.digest,t.trashed_at,t.purge_after`

func (s *Store) ListFileTrash(ctx context.Context, sid int64) ([]TrashedFile, error) {
	return list[TrashedFile](ctx, s, `SELECT `+trashColumns+` FROM file_trash t JOIN files f ON f.id=t.file_id WHERE (?=0 OR f.series_id=?) ORDER BY t.trashed_at DESC,f.id`, sid, sid)
}
func (s *Store) GetFileTrash(ctx context.Context, fid int64) (*TrashedFile, error) {
	return get[TrashedFile](ctx, s, `SELECT `+trashColumns+` FROM file_trash t JOIN files f ON f.id=t.file_id WHERE f.id=?`, fid)
}

// BeginFileTrash journals the move before touching disk. The same edition
// locks as mapping/monitoring make stale previews safe to reject.
func (s *Store) BeginFileTrash(ctx context.Context, sid, fid int64, path, trashPath, digest, at, expiry string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM editions WHERE series_id=? ORDER BY id`), sid)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if err = s.lockEditions(ctx, tx, true, ids...); err != nil {
		return err
	}
	var facts, claims AssetFacts
	if err = tx.QueryRowContext(ctx, s.q(`SELECT facts_json,claims_json FROM files WHERE id=? AND series_id=?`), fid, sid).Scan(&facts, &claims); err != nil {
		return err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM queue_items WHERE series_id=? AND status IN('queued','downloading','importing','needs_review','retry_wait','dispatch_unknown')
 AND ((release_key<>'' AND release_key IN(?,?)) OR (external_id<>'' AND external_id IN(?,?)))
 AND NOT(status='needs_review' AND EXISTS(SELECT 1 FROM files WHERE id=? AND review_action='trash' AND needs_review=FALSE))`), sid, facts.ReleaseKey, claims.ReleaseKey, facts.ReleaseKey, claims.ReleaseKey, fid).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return apperr.Conflictf("finish this file's download before cleanup")
	}
	var valid int
	err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM files f WHERE f.id=? AND f.series_id=? AND f.path=? AND f.needs_review=FALSE
 AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id)
 AND NOT EXISTS(SELECT 1 FROM asset_coverage WHERE file_id=f.id AND pinned=TRUE)
 AND NOT EXISTS(SELECT 1 FROM asset_contents c WHERE c.file_id=f.id AND c.verified=FALSE)
 AND NOT EXISTS(SELECT 1 FROM asset_contents c JOIN books b ON b.id=c.book_id JOIN versions v ON v.edition_id=b.edition_id
 LEFT JOIN asset_coverage a ON a.file_id=c.file_id AND a.book_id=c.book_id AND a.version_id=v.id
 WHERE c.file_id=f.id AND (v.monitored=TRUE OR f.review_action='trash') AND COALESCE(a.selected,TRUE)=TRUE)`), fid, sid, path).Scan(&valid)
	if err != nil {
		return err
	}
	if valid != 1 {
		return apperr.Conflictf("file changed or is protected; refresh the cleanup preview")
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO file_trash(file_id,trash_path,digest,trashed_at,purge_after) VALUES(?,?,?,?,?)`), fid, trashPath, digest, at, expiry)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RestoreFileRecord(ctx context.Context, fid int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM file_trash WHERE file_id=?`), fid); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE files SET presence='present',presence_error='',checked_at=?,review_action=CASE WHEN review_action='trash' THEN 'keep' ELSE review_action END WHERE id=?`), utcNow(), fid); err != nil {
		return err
	}
	return tx.Commit()
}

// Aborting an unsuccessful move keeps a review's recorded trash intent, so
// the queue pass can retry it. An explicit restore cancels that intent.
func (s *Store) AbortFileTrash(ctx context.Context, fid int64) error {
	return s.touch(ctx, `DELETE FROM file_trash WHERE file_id=?`, fid)
}

// RejectTrashEdit runs inside the caller's edition-locked transaction.
func (s *Store) rejectTrashEdit(ctx context.Context, tx *sql.Tx, fid int64) error {
	var n int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM file_trash WHERE file_id=?`), fid).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return apperr.Conflictf("restore the file from trash before editing it")
	}
	return nil
}

func (s *Store) PinnedFiles(ctx context.Context, sid int64) ([]int64, error) {
	return list[int64](ctx, s, `SELECT DISTINCT a.file_id FROM asset_coverage a JOIN files f ON f.id=a.file_id WHERE f.series_id=? AND a.pinned=TRUE`, sid)
}
