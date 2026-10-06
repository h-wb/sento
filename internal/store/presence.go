package store

import (
	"context"
	"os"
	"path/filepath"
)

// CheckFilePresence runs during scans and before acquisition, never ordinary ledger reads.
// A missing/unreadable series directory is a storage issue, not a missing book.
func (s *Store) CheckFilePresence(ctx context.Context, seriesID int64) error {
	series, err := s.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	files, err := s.ListFiles(ctx, seriesID)
	if err != nil {
		return err
	}
	roots := map[string]error{}
	for _, f := range files {
		root := series.Path
		if root == "" {
			root = filepath.Dir(f.Path)
		}
		rootErr, ok := roots[root]
		if !ok {
			_, rootErr = os.ReadDir(root)
			roots[root] = rootErr
		}
		state, detail := "present", ""
		info, e := os.Stat(f.Path)
		switch {
		case rootErr != nil:
			state, detail = "unavailable", rootErr.Error()
		case e == nil && info.IsDir():
			state, detail = "unavailable", "expected an archive, found a directory"
		case os.IsNotExist(e):
			state = "missing"
		case e != nil:
			state, detail = "unavailable", e.Error()
		}
		if _, err = s.db.ExecContext(ctx, s.q(`UPDATE files SET presence=?,presence_error=?,checked_at=? WHERE id=?`), state, detail, utcNow(), f.ID); err != nil {
			return err
		}
	}
	return nil
}

type AssetContent struct {
	FileID   int64  `json:"fileId"`
	WorkID   int64  `json:"workId"`
	Verified bool   `json:"verified"`
	Evidence string `json:"evidence"`
}

func (s *Store) Contents(ctx context.Context, seriesID int64) ([]AssetContent, error) {
	return list[AssetContent](ctx, s, `SELECT c.file_id,c.work_id,c.verified,c.evidence FROM asset_contents c JOIN files f ON f.id=c.file_id WHERE f.series_id=?`, seriesID)
}
func (s *Store) SeriesWorks(ctx context.Context, seriesID int64) ([]Work, error) {
	return list[Work](ctx, s, `SELECT `+workColumns+` FROM works WHERE collection_id IN(SELECT id FROM collections WHERE series_id=?) ORDER BY sequence_num,count_type,id`, seriesID)
}

type UnitOverride struct {
	TargetID  int64
	WorkID    int64
	Monitored bool
}

func (s *Store) SeriesUnitOverrides(ctx context.Context, seriesID int64) ([]UnitOverride, error) {
	return list[UnitOverride](ctx, s, `SELECT target_id,work_id,monitored FROM unit_overrides WHERE target_id IN(SELECT t.id FROM download_targets t JOIN collections c ON c.id=t.collection_id WHERE c.series_id=?)`, seriesID)
}
