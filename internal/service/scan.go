package service

import (
	"context"
	"fmt"
	"github.com/h-wb/sento/internal/mediafile"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Scanner implements per-series existing-library import (spec.md §11.4):
// files are linked where they are — nothing is moved, renamed or rewritten.
type Scanner struct {
	store       *store.Store
	libraryRoot string
}

func NewScanner(st *store.Store, libraryRoot string) *Scanner {
	return &Scanner{store: st, libraryRoot: libraryRoot}
}

type ScanResult struct {
	Scanned     int      `json:"scanned"`
	Linked      int      `json:"linked"`
	Unrequested int      `json:"unrequested"`
	Skipped     []string `json:"skipped,omitempty"` // already known paths
	// Changed counts known files whose pages were replaced on disk; they
	// were matched again from scratch.
	Changed int `json:"changed"`
}

// ScanSeriesFolder walks dir (default: the series folder, derived from the
// naming template when unset) and links every archive it can identify. A
// series without a folder adopts the one scanned.
func (sc *Scanner) ScanSeriesFolder(ctx context.Context, seriesID int64, dir string) (*ScanResult, error) {
	return sc.scan(ctx, seriesID, dir, true)
}

// LinkFolder links the archives of a folder outside the series' own, in
// place: the series keeps its folder (or still has none), so downloads
// keep landing where they did.
func (sc *Scanner) LinkFolder(ctx context.Context, seriesID int64, dir string) (*ScanResult, error) {
	return sc.scan(ctx, seriesID, dir, false)
}

func (sc *Scanner) scan(ctx context.Context, seriesID int64, dir string, adopt bool) (*ScanResult, error) {
	if err := sc.store.CheckFilePresence(ctx, seriesID); err != nil {
		return nil, err
	}
	if err := sc.store.EnsureModels(ctx, seriesID); err != nil {
		return nil, err
	}
	series, err := sc.store.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	titles, err := sc.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	// Default: everywhere the series lives — its folder, plus the separate
	// chapter folder when chapters are kept apart.
	dirs := []string{dir}
	if dir == "" {
		dir = series.Path
		dirs = []string{dir}
		if dir == "" {
			dir, _ = seriesFolders(sc.libraryRoot, series, LoadMediaSettings(ctx, sc.store))
			dirs = []string{dir}
		}
		if series.ChapterPath != "" && series.ChapterPath != dir {
			dirs = append(dirs, series.ChapterPath)
		}
	}

	// Adopt the folder as the series path if none was set.
	if adopt && series.Path == "" {
		if err := sc.store.SetSeriesPath(ctx, seriesID, dir); err != nil {
			return nil, err
		}
	}

	collections, err := sc.store.ListEditions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	sh := newShelf(collections)
	// Edition evidence determines file contents even when edition is paused.
	// Ambiguous archives remain available for explicit review.
	var monitored, chapterCols []store.Edition
	for _, c := range collections {
		switch c.Type {
		case "volume":
			monitored = append(monitored, c)
		case "chapter":
			if c.Monitored {
				chapterCols = append([]store.Edition{c}, chapterCols...)
			} else {
				chapterCols = append(chapterCols, c)
			}
		}
	}

	known := map[string]store.File{}
	existing, err := sc.store.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, f := range existing {
		known[f.Path] = f
	}

	res := &ScanResult{}
	err = walkDirs(dirs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, keep scanning
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !mediafile.IsBook(path) {
			return nil
		}
		if f, ok := known[path]; ok {
			replaced, err := sc.replaced(ctx, f, d)
			if err != nil {
				return err
			}
			if !replaced {
				res.Skipped = append(res.Skipped, path)
				return nil
			}
			// Different pages at a known path: nothing recorded about the
			// old file holds, manual confirmations included. Match it anew.
			if err := sc.store.ForgetFile(ctx, f.ID); err != nil {
				return err
			}
			res.Changed++
		}
		res.Scanned++

		parsed := parse.ParseAnchored(filepath.Base(path), parse.ChannelFile, titles)
		links := sc.linksFor(ctx, sh, parsed, monitored, chapterCols)

		info, ierr := d.Info()
		size := int64(0)
		if ierr == nil {
			size = info.Size()
		}
		hash, herr := fileHash(path)
		if herr != nil {
			return herr
		}
		_, aerr := sc.store.AddFile(ctx, store.File{
			Facts:       store.AssetFacts{Sources: map[string]string{"language": "filename", "group": "filename", "quality": "filename"}, Title: filepath.Base(path), Protocol: "torrent", FactsKnown: true, Flags: candidateFlags(ReleaseCandidate{Parse: parsed}), Language: parsed.Language, Group: parsed.Group, Format: strings.TrimPrefix(filepath.Ext(path), "."), Resolution: parsed.Quality.Resolution, Revision: parsed.Version, Digital: parsed.Quality.Digital},
			Hash:        hash,
			SeriesID:    seriesID,
			Path:        path,
			Size:        size,
			Provenance:  "library-import",
			Unrequested: len(links) == 0,
			NeedsReview: len(links) == 0,
		}, parsed, links)
		if aerr != nil {
			return aerr
		}
		if len(links) == 0 {
			res.Unrequested++
		} else {
			res.Linked++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scanning %s: %w", dir, err)
	}
	if err := sc.store.CheckFilePresence(ctx, seriesID); err != nil {
		return nil, err
	}
	return res, nil
}

// replaced reports whether a known file now holds different pages. Reading
// every archive on every scan would be slow, so the fingerprint is only
// recomputed when the file looks touched: its size differs from the
// record, or it was modified after it was last checked. Sento's own
// retagging trips this too and is told apart by the fingerprint, which
// ignores metadata.
func (sc *Scanner) replaced(ctx context.Context, f store.File, d fs.DirEntry) (bool, error) {
	info, err := d.Info()
	if err != nil || f.Hash == "" || !mediafile.IsArchive(f.Path) {
		return false, nil
	}
	touched := info.Size() != f.Size
	if checked, e := time.Parse("2006-01-02 15:04:05", f.CheckedAt); e == nil && info.ModTime().UTC().After(checked) {
		touched = true
	}
	if !touched {
		return false, nil
	}
	hash, err := fileHash(f.Path)
	if err != nil {
		return false, nil // unreadable right now: presence checks report it
	}
	if hash != f.Hash {
		return true, nil
	}
	if info.Size() != f.Size {
		return false, sc.store.SetFileSize(ctx, f.ID, info.Size())
	}
	return false, nil
}

// linksFor maps a parsed file onto book links: a volume file to the routed
// volume edition, a chapter file to the first monitored chapter
// edition (language-matched when the filename is tagged). A file is one
// or the other (bookRanges).
func (sc *Scanner) linksFor(ctx context.Context, sh *shelf, parsed parse.Result, vols, chapters []store.Edition) []store.BookLink {
	if parsed.ContentType != parse.ContentManga || !parsed.Anchored || parsed.TitleLeftover != "" {
		return nil
	}
	var links []store.BookLink
	for _, axis := range []struct {
		kind   string
		cols   []store.Edition
		ranges []parse.Range
	}{{"volume", vols, bookRanges(parsed, LedgerVolume)}, {"chapter", chapters, bookRanges(parsed, LedgerChapter)}} {
		if len(axis.ranges) == 0 {
			continue
		}
		var matches []store.Edition
		for _, c := range axis.cols {
			if sh.fits(parsed, c) {
				matches = append(matches, c)
			}
		}
		matches = sh.prefer(parsed, matches)
		if len(matches) != 1 {
			continue
		}
		col := matches[0]
		versions, err := sc.store.ListVersions(ctx, col.SeriesID)
		if err != nil {
			continue
		}
		var versionID int64
		for _, version := range versions {
			if version.EditionID == col.ID && version.IsDefault {
				versionID = version.ID
			}
		}
		works, err := sc.store.ListBooks(ctx, col.ID)
		if err != nil {
			continue
		}
		// Two books sharing a number (revisions, a bonus numbered like a
		// volume) cannot be told apart by a file name: leave the file for
		// review instead of confirming it as both.
		matched, ambiguous := resolveBooks(works, axis.ranges, parsed.Extra)
		if ambiguous {
			continue
		}
		for _, w := range matched {
			links = append(links, store.BookLink{Ledger: axis.kind, EditionID: col.ID, BookID: w.ID, VersionID: versionID, Number: w.SequenceNum})
		}
	}
	return links
}

// LowestOwnedChapters returns the lowest linked chapter number per chapter
// edition, used for the import-time cutoff default (§11.4: lowest owned
// chapter so owned chapters land in-ledger and the frontier continues).
func (sc *Scanner) LowestOwnedChapters(ctx context.Context, seriesID int64) (map[int64]int64, error) {
	return sc.store.LowestOwnedChapters(ctx, seriesID)
}

// ScanAndAdoptCutoff runs ScanSeriesFolder and, when chapter files were
// linked, lowers each chapter edition's cutoff to just below its lowest
// owned chapter so owned chapters land in-ledger (spec.md §11.4).
func (sc *Scanner) ScanAndAdoptCutoff(ctx context.Context, seriesID int64, dir string) (*ScanResult, error) {
	result, err := sc.ScanSeriesFolder(ctx, seriesID, dir)
	if err != nil {
		return nil, err
	}
	if result.Linked == 0 {
		return result, nil
	}
	lowest, err := sc.LowestOwnedChapters(ctx, seriesID)
	if err != nil || len(lowest) == 0 {
		return result, nil
	}
	collections, err := sc.store.ListEditions(ctx, seriesID)
	if err != nil {
		return result, nil
	}
	for _, col := range collections {
		if low, ok := lowest[col.ID]; ok && col.Type == "chapter" && low-100 < col.CutoffNum {
			_ = sc.store.SetEditionCutoff(ctx, seriesID, col.ID, low-100)
		}
	}
	return result, nil
}

// walkDirs walks several roots with one visitor, stopping at the first
// error the visitor returns.
func walkDirs(dirs []string, fn fs.WalkDirFunc) error {
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, fn); err != nil {
			return err
		}
	}
	return nil
}
