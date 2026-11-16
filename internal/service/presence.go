package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/store"
)

// CheckFilePresence runs during scans and before acquisition, never ordinary ledger reads.
// A missing/unreadable series directory is a storage issue, not a missing book.
func CheckFilePresence(ctx context.Context, st *store.Store, seriesID int64) error {
	series, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return err
	}
	files, err := st.ListFiles(ctx, seriesID)
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
			root = siblingFolder(series, f.Path)
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
		if err = st.SetFilePresence(ctx, f.ID, state, detail); err != nil {
			return err
		}
	}
	return nil
}

// siblingFolder is the top folder holding path beside the series' own
// folders (an edition or version folder), so a book folder nested in it
// answers to it as one under the series folder does. A path elsewhere
// answers to its own folder.
func siblingFolder(sr *store.Series, path string) string {
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
