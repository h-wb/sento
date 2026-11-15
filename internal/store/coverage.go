package store

import (
	"context"
	"database/sql"

	"github.com/h-wb/sento/internal/apperr"
)

type AssetCoverage struct {
	Evidence  string `json:"evidence"`
	FileID    int64  `json:"fileId"`
	VersionID int64  `json:"versionId"`
	BookID    int64  `json:"bookId"`
	Verified  bool   `json:"verified"`
	Pinned    bool   `json:"pinned"`
}

func (s *Store) Coverage(ctx context.Context, seriesID int64) ([]AssetCoverage, error) {
	return list[AssetCoverage](ctx, s, `SELECT c.file_id,t.id AS version_id,c.book_id,c.verified,COALESCE(a.pinned,FALSE) AS pinned,c.evidence
 FROM asset_contents c JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id
 JOIN files f ON f.id=c.file_id LEFT JOIN asset_coverage a ON a.file_id=c.file_id AND a.book_id=c.book_id AND a.version_id=t.id
 WHERE f.series_id=? AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id) AND COALESCE(a.selected,TRUE)=TRUE`, seriesID)
}

func (s *Store) PinCoverage(ctx context.Context, seriesID int64, a AssetCoverage) error {
	version, err := s.GetVersion(ctx, a.VersionID)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.lockEditions(ctx, tx, true, version.EditionID); err != nil {
			return err
		}
		if err := s.rejectTrashEdit(ctx, tx, a.FileID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,pinned)
 SELECT c.file_id,t.id,c.book_id,? FROM asset_contents c JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id JOIN files f ON f.id=c.file_id
 WHERE c.file_id=? AND t.id=? AND c.book_id=? AND f.series_id=?
 ON CONFLICT(file_id,version_id,book_id) DO UPDATE SET pinned=excluded.pinned`), a.Pinned, a.FileID, a.VersionID, a.BookID, seriesID)
		return err
	})
}

// Explicit review replaces a file's mappings atomically. It does not rewrite
// publication facts or infer equivalence between editions with equal numbers.
func (s *Store) MapFile(ctx context.Context, seriesID, fileID int64, slots []BookRef) error {
	var fileSeries int64
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT series_id FROM files WHERE id=?`), fileID).Scan(&fileSeries); err != nil {
		return err
	}
	if fileSeries != seriesID {
		return ErrNotFound
	}
	for _, sl := range slots {
		t, err := s.GetVersion(ctx, sl.VersionID)
		if err != nil {
			return err
		}
		w, err := s.GetBook(ctx, sl.BookID)
		if err != nil {
			return err
		}
		c, err := s.editionOf(ctx, seriesID, w.EditionID)
		if err != nil {
			return err
		}
		if t.EditionID != c.ID {
			return apperr.Invalidf("file mapping must belong to this series and version")
		}
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		cids := []int64{}
		for _, sl := range slots {
			cids = append(cids, sl.EditionID)
		}
		if err := s.lockEditions(ctx, tx, true, cids...); err != nil {
			return err
		}
		if err := s.rejectTrashEdit(ctx, tx, fileID); err != nil {
			return err
		}
		for _, sl := range slots {
			valid, err := s.bookInVersion(ctx, tx, seriesID, sl.BookID, sl.VersionID, sl.EditionID)
			if err != nil {
				return err
			}
			if !valid {
				return apperr.Invalidf("book mapping changed; reopen this file to review it")
			}
		}
		pins, err := s.filePins(ctx, tx, fileID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM asset_coverage WHERE file_id=?`), fileID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.q(`DELETE FROM asset_contents WHERE file_id=?`), fileID); err != nil {
			return err
		}
		for _, sl := range slots {
			if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,verified,evidence) VALUES(?,?,TRUE,'manual') ON CONFLICT DO NOTHING`), fileID, sl.BookID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,pinned,evidence) VALUES(?,?,?,? ,'manual') ON CONFLICT DO NOTHING`), fileID, sl.VersionID, sl.BookID, pins[BookKey(sl.VersionID, sl.BookID)]); err != nil {
				return err
			}
		}
		// Explicit mapping choices may exclude existing versions; future versions can still reuse content.
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,selected)
 SELECT c.file_id,t.id,c.book_id,FALSE FROM asset_contents c JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id
 WHERE c.file_id=? ON CONFLICT DO NOTHING`), fileID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=FALSE,unrequested=? WHERE id=?`), len(slots) == 0, fileID); err != nil {
			return err
		}
		return nil
	})
}

// BookOwner is one file currently holding a book: who owns it, whether the
// hold is verified and pinned, and the file's parsed facts for scoring.
type BookOwner struct {
	Evidence string     `json:"evidence" db:"evidence"`
	Path     string     `json:"path" db:"path"`
	Facts    AssetFacts `json:"facts" db:"facts_json"`
	Claims   AssetFacts `json:"claims" db:"claims_json"`
	FileID   int64      `json:"fileId" db:"file_id"`
	BookID   int64      `json:"bookId" db:"book_id"`
	Verified bool       `json:"verified" db:"verified"`
	Pinned   bool       `json:"pinned" db:"pinned"`
	Present  bool       `json:"present" db:"present"`
}

// PathTaken reports whether another file row already records path — the
// unique index on files.path makes a blind move into it fail late.
func (s *Store) PathTaken(ctx context.Context, path string, exceptFileID int64) (bool, error) {
	n, err := get[int](ctx, s, `SELECT COUNT(*) FROM files WHERE path=? AND id<>?`, path, exceptFileID)
	if err != nil {
		return false, err
	}
	return *n > 0, nil
}

// BookOwners lists the selected files for one collected version's book.
// Selection and pins in another version do not participate.
func (s *Store) BookOwners(ctx context.Context, versionID, bookID int64) ([]BookOwner, error) {
	return list[BookOwner](ctx, s, `
		SELECT c.file_id,c.book_id,c.verified,COALESCE(c.evidence,'observed') AS evidence,f.path,f.facts_json,f.claims_json,
		       f.presence='present' AS present,COALESCE(v.pinned,FALSE) AS pinned
		FROM asset_contents c
		JOIN books w ON w.id=c.book_id
		JOIN versions t ON t.edition_id=w.edition_id AND t.id=?
		JOIN files f ON f.id=c.file_id
		LEFT JOIN asset_coverage v ON v.file_id=c.file_id AND v.book_id=c.book_id AND v.version_id=t.id
		WHERE c.book_id=? AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id) AND COALESCE(v.selected,TRUE)=TRUE ORDER BY c.file_id`, versionID, bookID)
}

// DeselectBook gives up a file for one collected version, preserving its
// verified contents and selections in other versions.
func (s *Store) DeselectBook(ctx context.Context, fileID, versionID, bookID int64) error {
	return s.touch(ctx, `INSERT INTO asset_coverage(file_id,version_id,book_id,selected)
 SELECT c.file_id,t.id,c.book_id,FALSE FROM asset_contents c
 JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id
 WHERE c.file_id=? AND t.id=? AND c.book_id=?
 ON CONFLICT(file_id,version_id,book_id) DO UPDATE SET selected=FALSE`, fileID, versionID, bookID)
}

type AssetContent struct {
	FileID   int64  `json:"fileId"`
	BookID   int64  `json:"bookId"`
	Verified bool   `json:"verified"`
	Evidence string `json:"evidence"`
}

func (s *Store) Contents(ctx context.Context, seriesID int64) ([]AssetContent, error) {
	return list[AssetContent](ctx, s, `SELECT c.file_id,c.book_id,c.verified,c.evidence FROM asset_contents c JOIN files f ON f.id=c.file_id WHERE f.series_id=? AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id)`, seriesID)
}
