package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// FileChange is what organising would do to one library file: move it to
// where the current naming and layout settings put it, and bring its
// embedded metadata up to date. Paths are relative to the library root.
type FileChange struct {
	FileID int64  `json:"fileId"`
	From   string `json:"from"`
	To     string `json:"to"`
	Rename bool   `json:"rename"`
	Retag  bool   `json:"retag"`
	// Skip says why a file is left alone (not matched to a book, missing).
	Skip string `json:"skip,omitempty"`
}

// OrganizeResult summarises an applied organise run.
type OrganizeResult struct {
	Renamed  int      `json:"renamed"`
	Retagged int      `json:"retagged"`
	Errors   []string `json:"errors"`
}

type filePlan struct {
	FileChange
	file store.File
	dir  string // destination folder
	name string // destination file name, before any collision suffix
	info mediafile.ComicInfo
}

var reHashSuffix = regexp.MustCompile(` \[[a-f0-9]{64}\]$`)

func bookArchive(path string) bool { return mediafile.IsArchive(path) }

// organizePlans works out, for every file of a series, where the current
// settings would put it and what metadata it should carry.
func (im *Importer) organizePlans(ctx context.Context, seriesID, fileID int64) ([]filePlan, *store.Series, MediaSettings, error) {
	ms := LoadMediaSettings(ctx, im.store)
	series, err := im.store.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, nil, ms, err
	}
	// Computed, not recorded: a preview must not change where the series
	// is considered to live.
	volumesDir, chaptersDir := seriesFolders(im.libraryRoot, series, ms)
	files, err := im.store.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, nil, ms, err
	}
	coverage, err := im.store.Coverage(ctx, seriesID)
	if err != nil {
		return nil, nil, ms, err
	}
	versions, err := im.store.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, nil, ms, err
	}
	defaultVersion := map[int64]int64{} // edition → its default version
	for _, t := range versions {
		if t.IsDefault {
			defaultVersion[t.EditionID] = t.ID
		}
	}

	rel := func(p string) string {
		if r, err := filepath.Rel(im.libraryRoot, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
		return p
	}
	var plans []filePlan
	for _, f := range files {
		if fileID != 0 && f.ID != fileID {
			continue
		}
		p := filePlan{file: f, FileChange: FileChange{FileID: f.ID, From: rel(f.Path), To: rel(f.Path)}}
		// The books this file holds, once each, filed under the edition's
		// default version unless only an additional version claims it.
		var slots []store.BookRef
		home := map[int64]int64{} // book → the version it is filed under
		for _, c := range coverage {
			if c.FileID != f.ID || !c.Verified {
				continue
			}
			w, err := im.store.GetBook(ctx, c.BookID)
			if err != nil {
				continue
			}
			// One stable home per book: the default version when it claims
			// the file, otherwise the oldest additional version that does.
			chosen, seen := home[c.BookID]
			switch {
			case c.VersionID == defaultVersion[w.EditionID]:
				home[c.BookID] = c.VersionID
			case !seen || (chosen != defaultVersion[w.EditionID] && c.VersionID < chosen):
				home[c.BookID] = c.VersionID
			}
			if seen {
				continue
			}
			col, err := im.store.GetEdition(ctx, w.EditionID)
			if err != nil {
				delete(home, c.BookID)
				continue
			}
			slots = append(slots, store.BookRef{Ledger: col.Type, EditionID: col.ID, Number: w.SequenceNum, BookID: w.ID})
		}
		for i := range slots {
			slots[i].VersionID = home[slots[i].BookID]
		}
		sort.Slice(slots, func(i, j int) bool { return slots[i].Number < slots[j].Number })
		switch {
		case f.Presence == "missing" || f.Presence == "unavailable":
			p.Skip = "The file is not on disk."
		case f.NeedsReview || f.Unrequested || len(slots) == 0:
			p.Skip = "Not matched to a book yet."
		}
		if p.Skip != "" {
			plans = append(plans, p)
			continue
		}

		ext := filepath.Ext(f.Path)
		processable := ms.WriteComicInfo && bookArchive(f.Path)
		if processable {
			ext = ".cbz"
		}
		group := f.Facts.Group
		p.name = reHashSuffix.ReplaceAllString(strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path)), "") + ext
		p.info = im.fileInfo(ctx, series, slots, tagSource{Release: f.Claims.Title, Group: group, Via: f.Facts.Protocol, Site: f.Facts.Site, Flags: f.Facts.Flags})
		if ms.RenameImports {
			p.name = im.bookFileName(ctx, ms, series, slots, group, ext)
		}
		seriesDir := volumesDir
		if slots[0].Ledger == "chapter" {
			seriesDir = chaptersDir
		}
		p.dir = im.bookDir(ctx, ms, seriesDir, series, slots[0])

		plain := assetPath(p.dir, p.name, "")
		if ms.HashInName && f.Hash != "" {
			plain = assetPath(p.dir, p.name, f.Hash)
		}
		p.Rename = plain != f.Path
		p.To = rel(plain)
		if processable {
			// Out of date when the container is not a CBZ yet, or the merge
			// would change what the file already says.
			existing := mediafile.ReadComicInfo(f.Path)
			p.Retag = existing == nil || strings.ToLower(filepath.Ext(f.Path)) != ".cbz"
			if existing != nil {
				merged := mediafile.Merge(*existing, p.info)
				merged.PageCount = existing.PageCount
				p.Retag = p.Retag || !reflect.DeepEqual(merged, *existing)
			}
		}
		plans = append(plans, p)
	}
	return plans, series, ms, nil
}

// PlanOrganize previews what Organize would change, without touching
// anything.
func (im *Importer) PlanOrganize(ctx context.Context, seriesID int64) ([]FileChange, error) {
	plans, _, _, err := im.organizePlans(ctx, seriesID, 0)
	if err != nil {
		return nil, err
	}
	out := []FileChange{}
	for _, p := range plans {
		if p.Rename || p.Retag || p.Skip != "" {
			out = append(out, p.FileChange)
		}
	}
	return out, nil
}

// Organize brings a series' files in line with the current settings:
// rename moves them to their templated path, retag rewrites their embedded
// metadata. One file failing does not stop the others.
func (im *Importer) Organize(ctx context.Context, seriesID int64, rename, retag bool) (*OrganizeResult, error) {
	return im.organize(ctx, seriesID, 0, rename, retag)
}

// OrganizeReviewedFile finishes the import of the file just confirmed. Its
// naming and metadata follow the import settings; other files are untouched.
func (im *Importer) OrganizeReviewedFile(ctx context.Context, seriesID, fileID int64) error {
	res, err := im.organize(ctx, seriesID, fileID, true, true)
	if err != nil {
		return err
	}
	if len(res.Errors) > 0 {
		return fmt.Errorf("%s", strings.Join(res.Errors, "; "))
	}
	return nil
}

func (im *Importer) organize(ctx context.Context, seriesID, fileID int64, rename, retag bool) (*OrganizeResult, error) {
	plans, series, ms, err := im.organizePlans(ctx, seriesID, fileID)
	if err != nil {
		return nil, err
	}
	if rename {
		// The series now lives where the settings put it.
		if _, err := im.seriesDir(ctx, series, ms, "volume"); err != nil {
			return nil, err
		}
	}
	res := &OrganizeResult{Errors: []string{}}
	for _, p := range plans {
		doRename, doRetag := rename && p.Rename, retag && p.Retag
		if p.Skip != "" || (!doRename && !doRetag) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if err := im.organizeFile(ctx, p, ms, doRename, doRetag); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", p.From, err))
			continue
		}
		if doRename {
			res.Renamed++
			pruneEmptyDirs(filepath.Dir(p.file.Path), im.libraryRoot)
		}
		if doRetag {
			res.Retagged++
		}
	}
	return res, nil
}

func (im *Importer) organizeFile(ctx context.Context, p filePlan, ms MediaSettings, rename, retag bool) error {
	src := p.file.Path
	hash, err := fileHash(src)
	if err != nil {
		return err
	}
	// A rewrite without a rename still turns a .cbr into a .cbz.
	dest := src
	if rename || !strings.EqualFold(filepath.Ext(src), filepath.Ext(p.name)) {
		name := p.name
		if !rename {
			name = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src)) + filepath.Ext(p.name)
		}
		dir := filepath.Dir(src)
		if rename {
			dir = p.dir
		}
		if dest = assetPath(dir, name, ""); dest != src {
			if dest, err = placeAsset(dir, name, hash, ms.HashInName); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	switch {
	case retag:
		stage := filepath.Join(filepath.Dir(dest), ".sento-stage-"+filepath.Base(dest))
		defer func() { _ = os.Remove(stage) }()
		if err := mediafile.Process(src, stage, &p.info); err != nil {
			return err
		}
		if dest == src {
			// Same book, same place: replace it atomically.
			if err := os.Rename(stage, dest); err != nil {
				return err
			}
		} else if err := installAsset(stage, dest, hash, false); err != nil {
			return err
		}
	case dest != src:
		if err := installAsset(src, dest, hash, false); err != nil {
			return err
		}
	}
	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	if err := im.store.SetFileLocation(ctx, p.file.ID, dest, info.Size(), hash); err != nil {
		return err
	}
	if dest != src {
		_ = os.Remove(src)
	}
	return nil
}

// pruneEmptyDirs removes dir and its parents while they are empty, up to
// but not including stop.
func pruneEmptyDirs(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop+string(filepath.Separator)) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
