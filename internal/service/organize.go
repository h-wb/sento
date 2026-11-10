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
	dest string // the path the settings choose, before any collision suffix
	info mediafile.ComicInfo
	// moved is set while applying: the file has left its original path.
	moved bool
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
	// The catalog rows a whole-series pass touches are loaded in bulk, so
	// planning costs a fixed number of queries whatever the file count.
	// One confirmed file needs a handful of rows; the cache loads those
	// lazily, per row as before, without fetching the whole series per file.
	place := im.newPlacement(ms, series)
	cat := place.cat
	if fileID == 0 {
		if err := cat.preload(ctx); err != nil {
			return nil, nil, ms, err
		}
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
	versions, err := cat.versionsAll(ctx)
	if err != nil {
		return nil, nil, ms, err
	}
	defaultVersion := map[int64]int64{} // edition → its default version
	for _, t := range versions {
		if t.IsDefault {
			defaultVersion[t.EditionID] = t.ID
		}
	}

	// Choose a placed file per collected version, respecting its profile
	// and pins. Distinct copies may keep distinct files of the same book.
	profiles, err := NewProfiles(im.store).seriesProfiles(ctx, seriesID)
	if err != nil {
		return nil, nil, ms, err
	}
	editions, err := cat.editionsAll(ctx)
	if err != nil {
		return nil, nil, ms, err
	}
	byFile := map[int64]store.File{}
	for _, f := range files {
		byFile[f.ID] = f
	}
	placed := preferredFileOwners(versions, editions, coverage, byFile, profiles)
	anyCov := map[int64]bool{}
	for _, c := range coverage {
		if c.Verified {
			anyCov[c.FileID] = true
		}
	}

	// Every folder the series' books may sit in: its own, plus one per
	// copy whose rendered series folder differs from the series'.
	volumeTrees, chapterTrees := []string{volumesDir}, []string{chaptersDir}
	folders := editionFolders(im.libraryRoot, series, ms, editions, versions)
	for _, e := range editions {
		for k, dir := range folders {
			if k.EditionID != e.ID {
				continue
			}
			if e.Type == "chapter" {
				chapterTrees = append(chapterTrees, dir)
			} else {
				volumeTrees = append(volumeTrees, dir)
			}
		}
	}
	under := func(p string, dirs []string) bool {
		for _, d := range dirs {
			if strings.HasPrefix(p, d+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	reviewDirs := []string{filepath.Join(volumesDir, "review"), filepath.Join(chaptersDir, "review")}
	inReview := func(p string) bool {
		for _, r := range reviewDirs {
			if strings.HasPrefix(p, r+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	insideTree := func(p string) bool { return under(p, volumeTrees) || under(p, chapterTrees) }

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
		if err := ctx.Err(); err != nil {
			return nil, nil, ms, err
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
			// A book already placed through another file does not place this
			// one next to it.
			if placed[store.BookKey(c.VersionID, c.BookID)] != f.ID {
				continue
			}
			w, err := cat.book(ctx, c.BookID)
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
			col, err := cat.edition(ctx, w.EditionID)
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
		exile := false
		switch {
		case f.Presence == "missing" || f.Presence == "unavailable":
			p.Skip = "The file is not on disk."
		case f.NeedsReview || f.Unrequested:
			p.Skip = "Not matched to a book yet."
		case len(slots) == 0 && anyCov[f.ID]:
			// Holds books whose placed copy is another file: out of the book
			// folder, into review/ — never two copies of one book there.
			if insideTree(f.Path) && !inReview(f.Path) {
				exile = true
			} else {
				p.Skip = "Another file is placed for these books."
			}
		case len(slots) == 0:
			p.Skip = "Not matched to a book yet."
		}
		if p.Skip != "" {
			plans = append(plans, p)
			continue
		}
		if exile {
			p.dir = reviewDirs[0]
			if under(f.Path, chapterTrees) {
				p.dir = reviewDirs[1]
			}
			p.name = filepath.Base(f.Path)
			p.Rename = true
			p.dest = assetPath(p.dir, p.name, "")
			p.To = rel(p.dest)
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
		p.info = place.info(ctx, slots, tagSource{Release: f.Claims.Title, Group: group, Via: f.Facts.Protocol, Site: f.Facts.Site, Flags: f.Facts.Flags})
		if ms.RenameImports {
			p.name = place.fileName(ctx, slots, group, ext)
		}
		seriesDir := volumesDir
		if slots[0].Ledger == "chapter" {
			seriesDir = chaptersDir
		}
		p.dir = place.dir(ctx, seriesDir, slots[0])

		p.dest = assetPath(p.dir, p.name, "")
		if ms.HashInName && f.Hash != "" {
			p.dest = assetPath(p.dir, p.name, f.Hash)
		}
		p.Rename = p.dest != f.Path
		p.To = rel(p.dest)
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
	if fileID == 0 {
		settleFallbackNames(plans, rel)
	}
	return plans, series, ms, nil
}

// settleFallbackNames takes out of the plan the files that are where they
// can be: on their fingerprinted name because a different book of the
// series keeps the plain one and is not going anywhere. Planning them again
// would move nothing, run after run.
func settleFallbackNames(plans []filePlan, rel func(string) string) {
	stays := map[string]string{} // path → hash of the file staying on it
	for _, p := range plans {
		if p.Skip == "" && !p.Rename {
			stays[p.file.Path] = p.file.Hash
		}
	}
	for i := range plans {
		p := &plans[i]
		if !p.Rename || p.Skip != "" || p.file.Hash == "" {
			continue
		}
		owner, held := stays[p.dest]
		if held && owner != p.file.Hash && p.file.Path == assetPath(p.dir, p.name, p.file.Hash) {
			p.Rename, p.To = false, rel(p.file.Path)
		}
	}
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
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.organize(ctx, seriesID, 0, rename, retag)
}

// OrganizeReviewedFile finishes the import of the file just confirmed. Its
// naming and metadata follow the import settings; other files are untouched.
func (im *Importer) OrganizeReviewedFile(ctx context.Context, seriesID, fileID int64) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.organizeReviewedFile(ctx, seriesID, fileID)
}

// organizeReviewedFile runs with im.mu held, including during a bulk review.
func (im *Importer) organizeReviewedFile(ctx context.Context, seriesID, fileID int64) error {
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
	if rename {
		plans = moveOrder(plans)
	}
	// Files that landed beside their name because another file held it.
	var beside []filePlan
	for _, p := range plans {
		doRename, doRetag := rename && p.Rename, retag && p.Retag
		if p.Skip != "" || (!doRename && !doRetag) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		from := p.file.Path
		dest, err := im.organizeFile(ctx, p, ms, doRename, doRetag)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", p.From, err))
			continue
		}
		if doRetag {
			res.Retagged++
		}
		if !doRename {
			continue
		}
		p.moved = dest != from
		if p.moved {
			res.Renamed++
			pruneEmptyDirs(filepath.Dir(from), im.libraryRoot)
		}
		if dest != p.dest {
			p.file.Path = dest
			beside = append(beside, p)
		}
	}
	// Two files on each other's names: whichever went first found its name
	// taken, and it is free now.
	for _, p := range beside {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if _, err := os.Lstat(p.dest); !os.IsNotExist(err) {
			continue // still held: a different book keeps the name
		}
		from := p.file.Path
		dest, err := im.organizeFile(ctx, p, ms, true, false)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", p.From, err))
			continue
		}
		if dest != from {
			if !p.moved {
				res.Renamed++
			}
			pruneEmptyDirs(filepath.Dir(from), im.libraryRoot)
		}
	}
	return res, nil
}

// moveOrder puts each file after the one sitting on its destination, so a
// name a sibling is about to leave is free when its turn comes. Files in a
// cycle (two on each other's names) keep their order; organize settles
// those in a second step.
func moveOrder(plans []filePlan) []filePlan {
	at := map[string]int{} // current path → the plan moving it away
	for i, p := range plans {
		if p.Rename && p.Skip == "" {
			at[p.file.Path] = i
		}
	}
	out := make([]filePlan, 0, len(plans))
	const visiting, done = 1, 2
	state := make([]uint8, len(plans))
	var visit func(int)
	visit = func(i int) {
		if state[i] != 0 {
			return
		}
		state[i] = visiting
		if holder, ok := at[plans[i].dest]; ok && holder != i && plans[i].Rename && plans[i].Skip == "" {
			visit(holder)
		}
		state[i] = done
		out = append(out, plans[i])
	}
	for i := range plans {
		visit(i)
	}
	return out
}

// organizeFile moves and re-tags one file and returns where it is now: its
// planned path, or a fallback name beside it when that path is held.
func (im *Importer) organizeFile(ctx context.Context, p filePlan, ms MediaSettings, rename, retag bool) (string, error) {
	src := p.file.Path
	// Re-tagging and moving never change a fingerprint, so the stored one
	// stands; hashing every page again is the slow part of a library run.
	hash := p.file.Hash
	var err error
	if hash == "" {
		if hash, err = fileHash(src); err != nil {
			return "", err
		}
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
				return "", err
			}
			// files.path is unique across the table: never land on a path
			// another row already records, even if the disk slot looks free.
			base := dest
			for i := 2; ; i++ {
				var taken bool
				if taken, err = im.store.PathTaken(ctx, dest, p.file.ID); err != nil {
					return "", err
				}
				if !taken {
					break
				}
				ext := filepath.Ext(base)
				dest = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(base, ext), i, ext)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}

	switch {
	case retag:
		stage := filepath.Join(filepath.Dir(dest), ".sento-stage-"+filepath.Base(dest))
		defer func() { _ = os.Remove(stage) }()
		if err := mediafile.Process(src, stage, &p.info); err != nil {
			return "", err
		}
		if dest == src {
			// Same book, same place: replace it atomically.
			if err := os.Rename(stage, dest); err != nil {
				return "", err
			}
		} else if err := installAsset(stage, dest, hash, false); err != nil {
			return "", err
		}
	case dest != src:
		if err := installAsset(src, dest, hash, false); err != nil {
			return "", err
		}
	}
	info, err := os.Stat(dest)
	if err != nil {
		return "", err
	}
	if err := im.store.SetFileLocation(ctx, p.file.ID, dest, info.Size(), hash); err != nil {
		return "", err
	}
	if dest != src {
		_ = os.Remove(src)
	}
	return dest, nil
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
