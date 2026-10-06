package store

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/h-wb/sento/internal/apperr"
	"regexp"
	"strings"
)

type DownloadTarget struct {
	IncludeExtras  bool   `json:"includeExtras"`
	ID             int64  `json:"id"`
	CollectionID   int64  `json:"collectionId"`
	Name           string `json:"name"`
	IsDefault      bool   `json:"isDefault"`
	Monitored      bool   `json:"monitored"`
	ProfileID      int64  `json:"profileId"`
	CutoffNum      int64  `json:"cutoffNum"`
	RequiredGroup  string `json:"requiredGroup"`
	AllowUnknown   bool   `json:"allowUnknown"`
	AcquireUnknown bool   `json:"acquireUnknown"`
}

const targetColumns = `id,collection_id,name,is_default,monitored,COALESCE(profile_id,0) AS profile_id,cutoff_num,required_group,allow_unknown,acquire_unknown,include_extras`

func (s *Store) ListTargets(ctx context.Context, seriesID int64) ([]DownloadTarget, error) {
	return list[DownloadTarget](ctx, s, `SELECT `+targetColumns+` FROM download_targets WHERE collection_id IN(SELECT id FROM collections WHERE series_id=?) ORDER BY collection_id,is_default DESC,id`, seriesID)
}
func (s *Store) GetTarget(ctx context.Context, id int64) (*DownloadTarget, error) {
	return get[DownloadTarget](ctx, s, `SELECT `+targetColumns+` FROM download_targets WHERE id=?`, id)
}
func (s *Store) SaveTarget(ctx context.Context, seriesID int64, t DownloadTarget) (int64, error) {
	c, err := s.GetCollection(ctx, t.CollectionID)
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
	if t.RequiredGroup != "" {
		if _, err = regexp.Compile("(?i)" + t.RequiredGroup); err != nil {
			return 0, apperr.Invalidf("invalid group expression: %v", err)
		}
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
		e = tx.QueryRowContext(ctx, s.q(`INSERT INTO download_targets(collection_id,name,monitored,profile_id,cutoff_num,required_group,allow_unknown,acquire_unknown,include_extras) VALUES(?,?,?,?,?,?,?,?,?) RETURNING id`), t.CollectionID, t.Name, t.Monitored, prof, t.CutoffNum, t.RequiredGroup, t.AllowUnknown, t.AcquireUnknown, t.IncludeExtras).Scan(&id)
		if e != nil {
			return 0, conflict(e, "a collected version already uses this name")
		}
		return id, tx.Commit()
	}

	old, err := s.GetTarget(ctx, t.ID)
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
	if pending && (t.Name != old.Name || t.ProfileID != old.ProfileID || t.CutoffNum != old.CutoffNum || t.RequiredGroup != old.RequiredGroup || t.AllowUnknown != old.AllowUnknown || t.AcquireUnknown != old.AcquireUnknown || t.IncludeExtras != old.IncludeExtras) {
		return 0, apperr.Conflictf("finish pending downloads before changing acquisition versions")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	monitorOnly := t.Name == old.Name && t.ProfileID == old.ProfileID && t.CutoffNum == old.CutoffNum && t.RequiredGroup == old.RequiredGroup && t.AllowUnknown == old.AllowUnknown && t.AcquireUnknown == old.AcquireUnknown && t.IncludeExtras == old.IncludeExtras
	if err = s.lockEditions(ctx, tx, monitorOnly, c.ID); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, s.q(`UPDATE download_targets SET name=?,monitored=?,profile_id=?,cutoff_num=?,required_group=?,allow_unknown=?,acquire_unknown=?,include_extras=? WHERE id=?`), t.Name, t.Monitored, prof, t.CutoffNum, t.RequiredGroup, t.AllowUnknown, t.AcquireUnknown, t.IncludeExtras, t.ID)
	if err == nil {
		err = tx.Commit()
	}

	return t.ID, err
}
func (s *Store) DeleteTarget(ctx context.Context, seriesID, id int64) error {
	t, err := s.GetTarget(ctx, id)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, t.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID {
		return ErrNotFound
	}
	if t.IsDefault {
		return apperr.Invalidf("the default version cannot be deleted; pause it instead")
	}
	pending, err := s.CollectionPending(ctx, c.ID)
	if err != nil {
		return err
	}
	if pending {
		return apperr.Conflictf("collection has pending downloads")
	}
	// Target deletion drops references, never assets or files.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = s.lockEditions(ctx, tx, false, c.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM download_targets WHERE id=?`), id); err != nil {
		return err
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
	Evidence string `json:"evidence"`
	FileID   int64  `json:"fileId"`
	TargetID int64  `json:"targetId"`
	WorkID   int64  `json:"workId"`
	Verified bool   `json:"verified"`
	Pinned   bool   `json:"pinned"`
}

func (s *Store) Coverage(ctx context.Context, seriesID int64) ([]AssetCoverage, error) {
	return list[AssetCoverage](ctx, s, `SELECT c.file_id,t.id AS target_id,c.work_id,c.verified,COALESCE(a.pinned,FALSE) AS pinned,c.evidence
 FROM asset_contents c JOIN works w ON w.id=c.work_id JOIN download_targets t ON t.collection_id=w.collection_id
 JOIN files f ON f.id=c.file_id LEFT JOIN asset_coverage a ON a.file_id=c.file_id AND a.work_id=c.work_id AND a.target_id=t.id
 WHERE f.series_id=? AND COALESCE(a.selected,TRUE)=TRUE`, seriesID)
}
func (s *Store) PinCoverage(ctx context.Context, seriesID int64, a AssetCoverage) error {
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,target_id,work_id,pinned)
 SELECT c.file_id,t.id,c.work_id,? FROM asset_contents c JOIN works w ON w.id=c.work_id JOIN download_targets t ON t.collection_id=w.collection_id JOIN files f ON f.id=c.file_id
 WHERE c.file_id=? AND t.id=? AND c.work_id=? AND f.series_id=?
 ON CONFLICT(file_id,target_id,work_id) DO UPDATE SET pinned=excluded.pinned`), a.Pinned, a.FileID, a.TargetID, a.WorkID, seriesID)
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
func (s *Store) MapFile(ctx context.Context, seriesID, fileID int64, slots []ExpectedSlot) error {
	var fileSeries int64
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT series_id FROM files WHERE id=?`), fileID).Scan(&fileSeries); err != nil {
		return err
	}
	if fileSeries != seriesID {
		return ErrNotFound
	}
	for _, sl := range slots {
		t, err := s.GetTarget(ctx, sl.TargetID)
		if err != nil {
			return err
		}
		w, err := s.GetWork(ctx, sl.WorkID)
		if err != nil {
			return err
		}
		c, err := s.GetCollection(ctx, w.CollectionID)
		if err != nil {
			return err
		}
		if t.CollectionID != c.ID || c.SeriesID != seriesID {
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
		cids = append(cids, sl.CollectionID)
	}
	if err = s.lockEditions(ctx, tx, true, cids...); err != nil {
		return err
	}
	for _, sl := range slots {
		var valid int
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM works w JOIN download_targets t ON t.collection_id=w.collection_id JOIN collections c ON c.id=w.collection_id WHERE w.id=? AND t.id=? AND c.id=? AND c.series_id=?`), sl.WorkID, sl.TargetID, sl.CollectionID, seriesID).Scan(&valid); err != nil {
			return err
		}
		if valid != 1 {
			return apperr.Invalidf("book mapping changed; reopen this file to review it")
		}
	}
	pins := map[string]bool{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT target_id,work_id,pinned FROM asset_coverage WHERE file_id=?`), fileID)
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
		pins[SlotKey("unit", tid, wid)] = pinned
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
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_contents(file_id,work_id,verified,evidence) VALUES(?,?,TRUE,'manual') ON CONFLICT DO NOTHING`), fileID, sl.WorkID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,target_id,work_id,pinned,evidence) VALUES(?,?,?,? ,'manual') ON CONFLICT DO NOTHING`), fileID, sl.TargetID, sl.WorkID, pins[SlotKey("unit", sl.TargetID, sl.WorkID)]); err != nil {
			return err
		}
	}
	// Explicit mapping choices may exclude existing versions; future versions can still reuse content.
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO asset_coverage(file_id,target_id,work_id,selected)
 SELECT c.file_id,t.id,c.work_id,FALSE FROM asset_contents c JOIN works w ON w.id=c.work_id JOIN download_targets t ON t.collection_id=w.collection_id
 WHERE c.file_id=? ON CONFLICT DO NOTHING`), fileID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE files SET needs_review=FALSE,unrequested=? WHERE id=?`), len(slots) == 0, fileID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SetUnitMonitored(ctx context.Context, seriesID, targetID, workID int64, monitored bool) error {
	t, err := s.GetTarget(ctx, targetID)
	if err != nil {
		return err
	}
	w, err := s.GetWork(ctx, workID)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, t.CollectionID)
	if err != nil {
		return err
	}
	if c.SeriesID != seriesID || w.CollectionID != c.ID {
		return ErrNotFound
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO unit_overrides(target_id,work_id,monitored) VALUES(?,?,?) ON CONFLICT(target_id,work_id) DO UPDATE SET monitored=excluded.monitored`), targetID, workID, monitored)
	return err
}
func (s *Store) UnitOverrides(ctx context.Context, targetID int64) (map[int64]bool, error) {
	rows, err := list[struct {
		WorkID    int64
		Monitored bool
	}](ctx, s, `SELECT work_id,monitored FROM unit_overrides WHERE target_id=?`, targetID)
	if err != nil {
		return nil, err
	}
	out := map[int64]bool{}
	for _, r := range rows {
		out[r.WorkID] = r.Monitored
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
	t, err := s.GetTarget(ctx, tid)
	if err != nil {
		return err
	}
	c, err := s.GetCollection(ctx, t.CollectionID)
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
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM works WHERE id=? AND collection_id=?`), id, c.ID).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return apperr.Invalidf("book belongs to another edition")
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO unit_overrides(target_id,work_id,monitored) VALUES(?,?,?) ON CONFLICT(target_id,work_id) DO UPDATE SET monitored=excluded.monitored`), tid, id, monitored); err != nil {
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
