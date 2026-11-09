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
		// chapter folder says nothing about the volumes, and a book in
		// an edition or version folder answers to that folder — the
		// series' own may be gone, nothing lives there anymore.
		root := series.Path
		if series.InChapterFolder(f.Path) {
			root = series.ChapterPath
		}
		if root == "" || !strings.HasPrefix(f.Path, root+string(filepath.Separator)) {
			root = series.siblingFolder(f.Path)
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

// siblingFolder is the top folder holding path beside the series' own
// folders (an edition or version folder), so a book folder nested in it
// answers to it as one under the series folder does. A path elsewhere
// answers to its own folder.
func (sr *Series) siblingFolder(path string) string {
	for _, own := range []string{sr.Path, sr.ChapterPath} {
		if own == "" {
			continue
		}
		parent := filepath.Dir(own)
		rel, err := filepath.Rel(parent, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if top, rest, nested := strings.Cut(rel, string(filepath.Separator)); nested && rest != "" {
			return filepath.Join(parent, top)
		}
	}
	return filepath.Dir(path)
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
func (s *Store) SeriesBooks(ctx context.Context, seriesID int64) ([]Book, error) {
	return list[Book](ctx, s, `SELECT `+bookColumns+` FROM books WHERE edition_id IN(SELECT id FROM editions WHERE series_id=?) ORDER BY sequence_num,count_type,id`, seriesID)
}

type BookOverride struct {
	VersionID int64
	BookID    int64
	Monitored bool
}

func (s *Store) SeriesBookOverrides(ctx context.Context, seriesID int64) ([]BookOverride, error) {
	return list[BookOverride](ctx, s, `SELECT version_id,book_id,monitored FROM book_overrides WHERE version_id IN(SELECT t.id FROM versions t JOIN editions c ON c.id=t.edition_id WHERE c.series_id=?)`, seriesID)
}
