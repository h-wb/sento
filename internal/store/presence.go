package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
		// A file answers to the folder it lives under: an unreachable
		// chapter folder says nothing about the volumes.
		root := series.Path
		if series.InChapterFolder(f.Path) {
			root = series.ChapterPath
		}
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

// InChapterFolder reports whether path lies in the series' separate
// chapter folder. False when chapters share the series folder.
func (sr *Series) InChapterFolder(path string) bool {
	return sr.ChapterPath != "" && strings.HasPrefix(path, sr.ChapterPath+string(filepath.Separator))
}

type AssetContent struct {
	FileID   int64  `json:"fileId"`
	BookID   int64  `json:"bookId" db:"work_id"`
	Verified bool   `json:"verified"`
	Evidence string `json:"evidence"`
}

func (s *Store) Contents(ctx context.Context, seriesID int64) ([]AssetContent, error) {
	return list[AssetContent](ctx, s, `SELECT c.file_id,c.work_id,c.verified,c.evidence FROM asset_contents c JOIN files f ON f.id=c.file_id WHERE f.series_id=?`, seriesID)
}
func (s *Store) SeriesBooks(ctx context.Context, seriesID int64) ([]Book, error) {
	return list[Book](ctx, s, `SELECT `+workColumns+` FROM works WHERE collection_id IN(SELECT id FROM collections WHERE series_id=?) ORDER BY sequence_num,count_type,id`, seriesID)
}

type BookOverride struct {
	VersionID int64 `db:"target_id"`
	BookID    int64 `db:"work_id"`
	Monitored bool
}

func (s *Store) SeriesBookOverrides(ctx context.Context, seriesID int64) ([]BookOverride, error) {
	return list[BookOverride](ctx, s, `SELECT target_id,work_id,monitored FROM unit_overrides WHERE target_id IN(SELECT t.id FROM download_targets t JOIN collections c ON c.id=t.collection_id WHERE c.series_id=?)`, seriesID)
}
