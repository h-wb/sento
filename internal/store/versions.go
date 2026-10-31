package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/h-wb/sento/internal/apperr"
	"strings"
)

type CollectedVersion struct {
	IncludeExtras bool   `json:"includeExtras"`
	ID            int64  `json:"id"`
	EditionID     int64  `json:"editionId"`
	Name          string `json:"name"`
	IsDefault     bool   `json:"isDefault"`
	Monitored     bool   `json:"monitored"`
	ProfileID     int64  `json:"profileId"`
	CutoffNum     int64  `json:"cutoffNum"`
	// RequiredGroup and AllowUnknown are the old copy restrictions, read
	// only by MigrateCopyRules, which turns them into profiles. Saving
	// never writes them.
	RequiredGroup  string `json:"-"`
	AllowUnknown   bool   `json:"-"`
	AcquireUnknown bool   `json:"acquireUnknown"`
}

const versionColumns = `id,edition_id,name,is_default,monitored,COALESCE(profile_id,0) AS profile_id,cutoff_num,required_group,allow_unknown,acquire_unknown,include_extras`

func (s *Store) ListVersions(ctx context.Context, seriesID int64) ([]CollectedVersion, error) {
	return list[CollectedVersion](ctx, s, `SELECT `+versionColumns+` FROM versions WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY edition_id,is_default DESC,id`, seriesID)
}
func (s *Store) GetVersion(ctx context.Context, id int64) (*CollectedVersion, error) {
	return get[CollectedVersion](ctx, s, `SELECT `+versionColumns+` FROM versions WHERE id=?`, id)
}
func (s *Store) SaveVersion(ctx context.Context, seriesID int64, t CollectedVersion) (int64, error) {
	c, err := s.GetEdition(ctx, t.EditionID)
	if err != nil {
		return 0, err
	}
	if c.SeriesID != seriesID {
		return 0, ErrNotFound
	}
	t.Name = strings.TrimSpace(t.Name)
	if t.CutoffNum < 0 {
		return 0, apperr.Invalidf("chapter cutoff cannot be negative")
	}
	if t.Name == "" {
		return 0, apperr.Invalidf("version name is required")
	}
	var prof any
	if t.ProfileID > 0 {
		if _, err = s.GetProfile(ctx, t.ProfileID); err != nil {
			return 0, err
		}
		prof = t.ProfileID
	}

	if t.ID == 0 {
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return 0, e
		}
		defer func() { _ = tx.Rollback() }()
		if e = s.lockEditions(ctx, tx, true, c.ID); e != nil {
			return 0, e
		}
		var id int64
		e = tx.QueryRowContext(ctx, s.q(`INSERT INTO versions(edition_id,name,monitored,profile_id,cutoff_num,acquire_unknown,include_extras) VALUES(?,?,?,?,?,?,?) RETURNING id`), t.EditionID, t.Name, t.Monitored, prof, t.CutoffNum, t.AcquireUnknown, t.IncludeExtras).Scan(&id)
		if e != nil {
			return 0, conflict(e, "a collected version already uses this name")
		}
		return id, tx.Commit()
	}

	old, err := s.GetVersion(ctx, t.ID)
	if err != nil {
		return 0, err
	}
	if old.EditionID != c.ID {
		return 0, ErrNotFound
	}
	pending, err := s.EditionPending(ctx, c.ID)
	if err != nil {
		return 0, err
	}
	if pending && (t.Name != old.Name || t.ProfileID != old.ProfileID || t.CutoffNum != old.CutoffNum || t.AcquireUnknown != old.AcquireUnknown || t.IncludeExtras != old.IncludeExtras) {
		return 0, apperr.Conflictf("finish pending downloads before changing acquisition versions")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	monitorOnly := t.Name == old.Name && t.ProfileID == old.ProfileID && t.CutoffNum == old.CutoffNum && t.AcquireUnknown == old.AcquireUnknown && t.IncludeExtras == old.IncludeExtras
	if err = s.lockEditions(ctx, tx, monitorOnly, c.ID); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE versions SET name=?,monitored=?,profile_id=?,cutoff_num=?,acquire_unknown=?,include_extras=? WHERE id=?`), t.Name, t.Monitored, prof, t.CutoffNum, t.AcquireUnknown, t.IncludeExtras, t.ID)
	if err == nil {
		err = tx.Commit()
	}

	return t.ID, err
}

// ProfileTaken reports whether another way of collecting the edition
// (not versionID) already uses the release profile (0: the series').
func (s *Store) ProfileTaken(ctx context.Context, editionID, versionID, profileID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM versions WHERE edition_id=? AND id<>? AND COALESCE(profile_id,0)=?`), editionID, versionID, profileID).Scan(&n)
	return n > 0, err
}

func (s *Store) DeleteVersion(ctx context.Context, seriesID, id int64) error {
	t, err := s.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.GetEdition(ctx, t.EditionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	// Any way an edition is collected can be removed but the last: that
	// one is switched off instead, and the edition stays in the library.
	var count int
	if err = s.db.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM versions WHERE edition_id=?`), c.ID).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return apperr.Invalidf("an edition is always collected one way; stop collecting it instead")
	}
	pending, err := s.EditionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("edition has pending downloads")
	}
	// Version deletion drops references, never assets or files.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, c.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM versions WHERE id=?`), id); err != nil {
		return err
	}
	// The edition's remaining oldest way of collecting it becomes its
	// default (it carries the edition's catalog-link and scan defaults).
	if t.IsDefault {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE versions SET is_default=TRUE WHERE id=(SELECT MIN(id) FROM versions WHERE edition_id=?)`), c.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type AssetFacts struct {
	Sources     map[string]string `json:"sources,omitempty"`
	Title       string            `json:"title,omitempty"`
	Site        string            `json:"site,omitempty"`
	Protocol    string            `json:"protocol,omitempty"`
	Flags       map[string]bool   `json:"flags,omitempty"`
	FactsKnown  bool              `json:"factsKnown"`
	Language    string            `json:"language,omitempty"`
	Group       string            `json:"group,omitempty"`
	Format      string            `json:"format,omitempty"`
	FormatScore int               `json:"formatScore"`
	Resolution  int               `json:"resolution"`
	Revision    int               `json:"revision"`
	Digital     bool              `json:"digital"`
	ProfileID   int64             `json:"profileId,omitempty"`
	ReleaseKey  string            `json:"releaseKey,omitempty"`
}
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
 WHERE f.series_id=? AND COALESCE(a.selected,TRUE)=TRUE`, seriesID)
}
func (s *Store) PinCoverage(ctx context.Context, seriesID int64, a AssetCoverage) error {
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,pinned)
 SELECT c.file_id,t.id,c.book_id,? FROM asset_contents c JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id JOIN files f ON f.id=c.file_id
 WHERE c.file_id=? AND t.id=? AND c.book_id=? AND f.series_id=?
 ON CONFLICT(file_id,version_id,book_id) DO UPDATE SET pinned=excluded.pinned`), a.Pinned, a.FileID, a.VersionID, a.BookID, seriesID)
	return err
}
func (s *Store) FileFacts(ctx context.Context, id int64) (AssetFacts, error) {
	var f AssetFacts
	raw, err := get[string](ctx, s, `SELECT facts_json FROM files WHERE id=?`, id)
	if err == nil {
		err = json.Unmarshal([]byte(*raw), &f)
	}
	return f, err
}
func (s *Store) SetAssetFacts(ctx context.Context, id int64, f AssetFacts, hash string) error {
	raw, err := json.Marshal(f)
	if err != nil {
		return err
	}
	return s.touch(ctx, `UPDATE files SET facts_json=?,hash=? WHERE id=?`, string(raw), hash, id)
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
		c, err := s.GetEdition(ctx, w.EditionID)
		if err != nil {
			return err
		}
		if t.EditionID != c.ID || c.SeriesID != seriesID {
			return apperr.Invalidf("file mapping must belong to this series and version")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	cids := []int64{}
	for _, sl := range slots {
		cids = append(cids, sl.EditionID)
	}
	if err = s.lockEditions(ctx, tx, true, cids...); err != nil {
		return err
	}
	for _, sl := range slots {
		var valid int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books w JOIN versions t ON t.edition_id=w.edition_id JOIN editions c ON c.id=w.edition_id WHERE w.id=? AND t.id=? AND c.id=? AND c.series_id=?`), sl.BookID, sl.VersionID, sl.EditionID, seriesID).Scan(&valid); err != nil {
			return err
		}
		if valid != 1 {
			return apperr.Invalidf("book mapping changed; reopen this file to review it")
		}
	}
	pins := map[string]bool{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT version_id,book_id,pinned FROM asset_coverage WHERE file_id=?`), fileID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var tid, wid int64
		var pinned bool
		if err = rows.Scan(&tid, &wid, &pinned); err != nil {
			_ = rows.Close()
			return err
		}
		pins[BookKey(tid, wid)] = pinned
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_coverage WHERE file_id=?`), fileID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM asset_contents WHERE file_id=?`), fileID); err != nil {
		return err
	}
	for _, sl := range slots {
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,book_id,verified,evidence) VALUES(?,?,TRUE,'manual') ON CONFLICT DO NOTHING`), fileID, sl.BookID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,pinned,evidence) VALUES(?,?,?,? ,'manual') ON CONFLICT DO NOTHING`), fileID, sl.VersionID, sl.BookID, pins[BookKey(sl.VersionID, sl.BookID)]); err != nil {
			return err
		}
	}
	// Explicit mapping choices may exclude existing versions; future versions can still reuse content.
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,version_id,book_id,selected)
 SELECT c.file_id,t.id,c.book_id,FALSE FROM asset_contents c JOIN books w ON w.id=c.book_id JOIN versions t ON t.edition_id=w.edition_id
 WHERE c.file_id=? ON CONFLICT DO NOTHING`), fileID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=FALSE,unrequested=? WHERE id=?`), len(slots) == 0, fileID); err != nil {
		return err
	}
	return tx.Commit()
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
	c, err := s.GetEdition(ctx, t.EditionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID || w.EditionID != c.ID {
		return ErrNotFound
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO book_overrides(version_id,book_id,monitored) VALUES(?,?,?) ON CONFLICT(version_id,book_id) DO UPDATE SET monitored=excluded.monitored`), versionID, bookID, monitored)
	return err
}
func (s *Store) BookOverrides(ctx context.Context, versionID int64) (map[int64]bool, error) {
	rows, err := list[struct {
		BookID    int64
		Monitored bool
	}](ctx, s, `SELECT book_id,monitored FROM book_overrides WHERE version_id=?`, versionID)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, r := range rows {
		out[r.BookID] = r.Monitored
	}
	return out, nil
}

func (f *AssetFacts) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), f)
	case []byte:
		return json.Unmarshal(v, f)
	case nil:
		*f = AssetFacts{}
		return nil
	default:
		return fmt.Errorf("cannot scan asset facts from %T", src)
	}
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
	c, err := s.GetEdition(ctx, t.EditionID)
	if err != nil {
		return err
	}
	if c.SeriesID != sid {
		return ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, true, c.ID); err != nil {
		return err
	}
	for _, id := range ids {
		var count int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM books WHERE id=? AND edition_id=?`), id, c.ID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return apperr.Invalidf("book belongs to another edition")
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO book_overrides(version_id,book_id,monitored) VALUES(?,?,?) ON CONFLICT(version_id,book_id) DO UPDATE SET monitored=excluded.monitored`), tid, id, monitored); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type AssetFactPatch struct {
	Language *string `json:"language,omitempty"`
	Group    *string `json:"group,omitempty"`
}

func (s *Store) CorrectFileFacts(ctx context.Context, sid, fid int64, p AssetFactPatch) error {
	f, err := get[File](ctx, s, `SELECT id,series_id,facts_json FROM files WHERE id=?`, fid)
	if err != nil {
		return err
	}
	if f.SeriesID != sid {
		return ErrNotFound
	}
	facts := f.Facts
	if facts.Sources == nil {
		facts.Sources = map[string]string{}
	}
	if p.Language != nil {
		facts.Language = strings.TrimSpace(*p.Language)
		facts.Sources["language"] = "manual"
	}
	if p.Group != nil {
		facts.Group = strings.TrimSpace(*p.Group)
		facts.Sources["group"] = "manual"
	}
	raw, err := json.Marshal(facts)
	if err != nil {
		return err
	}
	return s.touch(ctx, `UPDATE files SET facts_json=? WHERE id=?`, string(raw), fid)
}
