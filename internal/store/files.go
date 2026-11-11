package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/h-wb/sento/internal/apperr"
)

type File struct {
	ReviewAction  string     `json:"reviewAction,omitempty"`
	ContentBooks  []BookRef  `json:"contentBooks,omitempty" db:"-"` // intrinsic contents, without version selections
	Claims        AssetFacts `json:"claims" db:"claims_json"`
	Presence      string     `json:"presence"`
	PresenceError string     `json:"presenceError"`
	CheckedAt     string     `json:"checkedAt"`
	Facts         AssetFacts `json:"facts" db:"facts_json"`
	NeedsReview   bool       `json:"needsReview"`
	Hash          string     `json:"hash,omitempty"`
	HashStat      string     `json:"-"` // stat signature of the archive Hash was read from
	// ParserVersion is the parse.Version that derived the filename-sourced
	// facts; a scan re-reads the name when the parser has moved on.
	ParserVersion int64  `json:"-"`
	ID            int64  `json:"id"`
	SeriesID      int64  `json:"seriesId"`
	Path          string `json:"path"`
	Size          int64  `json:"size"`
	Provenance    string `json:"provenance"`
	Unrequested   bool   `json:"unrequested"`
	AddedAt       string `json:"addedAt"`
}

type BookLink struct {
	VersionID int64
	BookID    int64
	FileID    int64
	Ledger    string
	EditionID int64
	Number    int64
}

// AddFile records a library file and its book links. Re-adding a known
// path updates the row in place (paths are unique) and adds any new links,
// so an import retried after a partial failure converges instead of
// tripping over what the first attempt already recorded.
func (s *Store) AddFile(ctx context.Context, f File, parse any, links []BookLink) (int64, error) {
	if err := s.EnsureModels(ctx, f.SeriesID); err != nil {
		return 0, err
	}
	parseJSON, _ := json.Marshal(parse)
	factsJSON, _ := json.Marshal(f.Facts)
	claimsJSON, _ := json.Marshal(f.Claims)
	presence := "present"
	if info, e := os.Stat(f.Path); e != nil || info.IsDir() {
		presence = "unknown"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	cids := []int64{}
	for _, l := range links {
		cids = append(cids, l.EditionID)
	}
	for _, b := range f.ContentBooks {
		cids = append(cids, b.EditionID)
	}
	if err = s.lockEditions(ctx, tx, true, cids...); err != nil {
		return 0, err
	}
	var held int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM file_trash t JOIN files f ON f.id=t.file_id WHERE f.path=?`), f.Path).Scan(&held); err != nil {
		return 0, err
	}
	if held > 0 {
		return 0, apperr.Conflictf("restore or empty this path's trash entry before importing it again")
	}
	var oldID int64
	var oldHash, oldFacts, oldReview string
	oldErr := tx.QueryRowContext(ctx, s.q(`SELECT id,COALESCE(hash,''),facts_json,review_action FROM files WHERE path=? AND series_id=?`), f.Path, f.SeriesID).Scan(&oldID, &oldHash, &oldFacts, &oldReview)
	if oldErr != nil && !errors.Is(oldErr, sql.ErrNoRows) {
		return 0, oldErr
	}
	if oldErr == nil {
		if err = s.rejectTrashEdit(ctx, tx, oldID); err != nil {
			return 0, err
		}
	}
	if oldErr == nil && f.Hash != "" && oldHash == f.Hash {
		var manual int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM asset_contents WHERE file_id=? AND evidence='manual'`), oldID).Scan(&manual); err != nil {
			return 0, err
		}
		if manual > 0 || oldReview != "" {
			// A retry or scan must not undo a person's verified contents or corrections.
			_, err = tx.ExecContext(ctx, s.q(`UPDATE files SET presence=?,presence_error='',checked_at=?,parser_version=? WHERE id=?`), presence, utcNow(), f.ParserVersion, oldID)
			if err != nil {
				return 0, err
			}
			return oldID, tx.Commit()
		}
		var previous AssetFacts
		if err = json.Unmarshal([]byte(oldFacts), &previous); err != nil {
			return 0, err
		}
		f.Facts = f.Facts.KeepingManual(previous)
		factsJSON, _ = json.Marshal(f.Facts)
	}

	var fileID int64
	if err := tx.QueryRowContext(ctx, s.q(`
		INSERT INTO files (series_id, path, size, parse_json, provenance, unrequested,needs_review,facts_json,claims_json,hash,hash_stat,parser_version,presence,checked_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			series_id = excluded.series_id,
			size = excluded.size,
			parser_version = excluded.parser_version,
			parse_json = excluded.parse_json,
			provenance = excluded.provenance,
			unrequested = excluded.unrequested,
			needs_review = excluded.needs_review,
 facts_json=excluded.facts_json,claims_json=excluded.claims_json,hash=excluded.hash,hash_stat=excluded.hash_stat,presence=excluded.presence,checked_at=excluded.checked_at,review_action=''
		WHERE files.series_id = excluded.series_id
		RETURNING id`),
		f.SeriesID, f.Path, f.Size, string(parseJSON), f.Provenance, f.Unrequested, f.NeedsReview, string(factsJSON), string(claimsJSON), f.Hash, f.HashStat, f.ParserVersion, presence, utcNow()).Scan(&fileID); err != nil {
		return 0, err
	}
	for _, l := range links {
		var valid int
		if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN versions t ON t.edition_id=w.edition_id JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND t.id=? AND c.id=? AND c.series_id=?`), l.BookID, l.VersionID, l.EditionID, f.SeriesID).Scan(&valid); err != nil {
			return 0, err
		}
		if valid != 1 {
			return 0, apperr.Invalidf("file mapping does not belong to this edition/version")
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,evidence) VALUES(?,?,?,?) ON CONFLICT DO NOTHING`), fileID, l.VersionID, l.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,evidence) VALUES(?,?,?) ON CONFLICT DO NOTHING`), fileID, l.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}

	}

	for _, b := range f.ContentBooks {
		var valid int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND c.id=? AND c.series_id=?`), b.BookID, b.EditionID, f.SeriesID).Scan(&valid); err != nil {
			return 0, err
		}
		if valid != 1 {
			return 0, apperr.Invalidf("file contents do not belong to this series and edition")
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,evidence) VALUES(?,?,?) ON CONFLICT DO NOTHING`), fileID, b.BookID, orDefault(f.Provenance, "observed")); err != nil {
			return 0, err
		}
	}
	return fileID, tx.Commit()
}

func (s *Store) ListFiles(ctx context.Context, seriesID int64) ([]File, error) {
	files, err := list[File](ctx, s, `
		SELECT `+fileColumns+`
		FROM files WHERE series_id = ? AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=files.id) ORDER BY path`, seriesID)
	if err != nil {
		return nil, err
	}
	refs, err := list[fileBook](ctx, s, `SELECT c.file_id,c.book_id,b.edition_id,e.type AS ledger,b.sequence_num AS number FROM asset_contents c JOIN books b ON b.id=c.book_id JOIN editions e ON e.id=b.edition_id WHERE e.series_id=? AND c.verified=TRUE ORDER BY c.file_id,c.book_id`, seriesID)
	if err != nil {
		return nil, err
	}
	byFile := map[int64][]BookRef{}
	for _, r := range refs {
		byFile[r.FileID] = append(byFile[r.FileID], BookRef{BookID: r.BookID, EditionID: r.EditionID, Ledger: r.Ledger, Number: r.Number})
	}
	for i := range files {
		files[i].ContentBooks = byFile[files[i].ID]
	}
	return files, nil
}

type fileBook struct {
	FileID, BookID, EditionID, Number int64
	Ledger                            string
}

const fileColumns = `id,series_id,path,size,provenance,unrequested,added_at,needs_review,COALESCE(hash,'') AS hash,hash_stat,parser_version,facts_json,claims_json,presence,presence_error,checked_at,review_action`

// RecordedFiles includes trash so rejected files remain accounted for in retries
// and download settlement, without counting them as library ownership.
func (s *Store) RecordedFiles(ctx context.Context, seriesID int64) ([]File, error) {
	return list[File](ctx, s, `SELECT `+fileColumns+` FROM files WHERE (?=0 OR series_id=?) ORDER BY id`, seriesID, seriesID)
}

// ForgetFile drops what is recorded about a file whose bytes were replaced
// on disk: its contents, selections and pins described the old bytes. The
// file itself is untouched; the caller records it afresh.
func (s *Store) ForgetFile(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, s.q(`DELETE FROM files WHERE id = ?`), id)
	return err
}

// RewriteFileFacts replaces a file's recorded facts after a parser-version
// re-read of its name — even when a person reviewed it or corrected facts —
// keeping what a person corrected (language, group, flags), the review
// action, hash, links and presence untouched.
func (s *Store) RewriteFileFacts(ctx context.Context, id int64, facts AssetFacts, parserVersion int64) error {
	var old string
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT facts_json FROM files WHERE id=?`), id).Scan(&old); err != nil {
		return err
	}
	var previous AssetFacts
	if err := json.Unmarshal([]byte(old), &previous); err != nil {
		return err
	}
	raw, err := json.Marshal(facts.KeepingManual(previous))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.q(`UPDATE files SET facts_json=?,parser_version=? WHERE id=?`), string(raw), parserVersion, id)
	return err
}

// RestoreReleaseLink gives a file back the download it came in: the release
// key (and site) on its facts and the release's claims. It only fills what
// is missing: a file that still names a release on either is left alone.
func (s *Store) RestoreReleaseLink(ctx context.Context, id int64, claims AssetFacts) error {
	var rawFacts, rawClaims string
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT facts_json,claims_json FROM files WHERE id=?`), id).Scan(&rawFacts, &rawClaims); err != nil {
		return err
	}
	var facts, old AssetFacts
	if err := json.Unmarshal([]byte(rawFacts), &facts); err != nil {
		return err
	}
	_ = json.Unmarshal([]byte(rawClaims), &old)
	if facts.ReleaseKey != "" || old.ReleaseKey != "" {
		return nil
	}
	facts.ReleaseKey = claims.ReleaseKey
	if facts.Site == "" {
		facts.Site = claims.Site
	}
	newFacts, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	newClaims, err := json.Marshal(claims)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.q(`UPDATE files SET facts_json=?,claims_json=? WHERE id=?`), string(newFacts), string(newClaims), id)
	return err
}

// SetFileSize records a file's size after its bytes changed without its
// pages changing (retagged metadata).
func (s *Store) SetFileSize(ctx context.Context, id, size int64) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE files SET size = ? WHERE id = ?`), size, id)
	return err
}

// SetFileHashStat records the stat signature taken before a fingerprint
// matched the file's recorded hash. A changed hash leaves the row alone.
func (s *Store) SetFileHashStat(ctx context.Context, id int64, hash, stat string) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE files SET hash_stat = ? WHERE id = ? AND hash = ?`), stat, id, hash)
	return err
}

// DeleteFile removes a file row (links cascade) and returns its path so
// the caller can remove it from disk.
func (s *Store) DeleteFile(ctx context.Context, id int64) (string, error) {
	pinned, err := get[int](ctx, s, `SELECT COUNT(*) FROM asset_coverage WHERE file_id=? AND pinned=TRUE`, id)
	if err != nil {
		return "", err
	}
	if *pinned > 0 {
		return "", apperr.Conflictf("unpin retained copies before deleting this file")
	}
	path, err := get[string](ctx, s, `SELECT path FROM files WHERE id = ?`, id)
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, s.q(`DELETE FROM files WHERE id = ?`), id); err != nil {
		return "", err
	}
	return *path, nil
}

// LowestOwnedChapters returns, per chapter edition of a series, the lowest
// numbered chapter a verified file holds.
func (s *Store) LowestOwnedChapters(ctx context.Context, seriesID int64) (map[int64]int64, error) {
	rows, err := list[struct {
		EditionID int64
		Lowest    int64
	}](ctx, s, `
		SELECT w.edition_id, MIN(w.sequence_num) AS lowest
		FROM asset_contents a JOIN books w ON w.id=a.book_id JOIN editions c ON c.id=w.edition_id JOIN files f ON f.id=a.file_id
		WHERE f.series_id=? AND NOT EXISTS(SELECT 1 FROM file_trash WHERE file_id=f.id) AND c.type='chapter' AND a.verified=TRUE AND f.needs_review=FALSE AND w.sequence_num>0
		GROUP BY w.edition_id`, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for _, r := range rows {
		out[r.EditionID] = r.Lowest
	}
	return out, nil
}

// BookKey identifies one book of one collected version: the book of
// wanting, reserving and owning.
func BookKey(versionID, bookID int64) string {
	return fmt.Sprintf("%d/%d", versionID, bookID)
}
