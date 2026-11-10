package service

import (
	"context"
	"fmt"
	"github.com/h-wb/sento/internal/mediafile"
	"io/fs"
	"path/filepath"
	"slices"
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
	if inFileTrash(dir) {
		return nil, fmt.Errorf("restore files from trash before scanning this folder")
	}
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
	// Default: everywhere the series lives — its folder, the separate
	// chapter folder when chapters are kept apart, and its edition folders.
	dirs := []string{dir}
	def := dir == ""
	ms := LoadMediaSettings(ctx, sc.store)
	if def {
		dir = series.Path
		dirs = []string{dir}
		if dir == "" {
			dir, _ = seriesFolders(sc.libraryRoot, series, ms)
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
	if def {
		versions, _ := sc.store.ListVersions(ctx, seriesID)
		for _, d := range editionFolders(sc.libraryRoot, series, ms, collections, versions) {
			if !slices.Contains(dirs, d) {
				dirs = append(dirs, d)
			}
		}
		slices.Sort(dirs[1:])
	}
	sh := seriesShelf(collections, series)
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

	trash, err := sc.store.ListFileTrash(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, f := range trash {
		held[f.Path] = true
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
		if held[path] {
			return nil
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
				if f.ParserVersion < parse.Version {
					// Same bytes, older parser stamp: re-read the name with
					// the current parser. Facts only — links, review state and
					// manual corrections survive the AddFile merge.
					if err := sc.rederiveFacts(ctx, f, path, titles, series, collections); err != nil {
						return err
					}
				}
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
		facts := store.AssetFacts{Sources: map[string]string{"language": "filename", "group": "filename", "quality": "filename"}, Title: filepath.Base(path), Protocol: "torrent", FactsKnown: true, Flags: candidateFlags(ReleaseCandidate{Parse: parsed}), Language: parsed.Language, Group: parsed.Group, Format: strings.TrimPrefix(filepath.Ext(path), "."), Resolution: parsed.Quality.Resolution, Revision: parsed.Version, Digital: parsed.Quality.Digital}
		// A file Sento tagged says what it is; its name is the fallback.
		if ci := mediafile.ReadComicInfo(path); ci != nil {
			if p, ok := mediafile.ReadProvenance(ci.Notes); ok && p.MangaBaka == series.MangaBakaID && series.MangaBakaID > 0 {
				if own := sc.linksFromProvenance(ctx, p, ci.LanguageISO, collections); len(own) > 0 {
					links, facts = own, provenanceFacts(p, ci, path)
				}
			}
		}

		info, ierr := d.Info()
		size := int64(0)
		if ierr == nil {
			size = info.Size()
		}
		// Stat before hashing, through an open descriptor so NFS revalidates
		// cached attributes; record the signature with the fingerprint so
		// review previews trust it instead of re-reading the archive.
		live, serr := statArchive(path)
		hash, herr := fileHash(path)
		if herr != nil {
			return herr
		}
		hashStat := ""
		if serr == nil {
			hashStat = statSignature(live)
		}
		_, aerr := sc.store.AddFile(ctx, store.File{
			Facts:         facts,
			Hash:          hash,
			HashStat:      hashStat,
			SeriesID:      seriesID,
			Path:          path,
			Size:          size,
			Provenance:    "library-import",
			Unrequested:   len(links) == 0,
			NeedsReview:   len(links) == 0,
			ParserVersion: parse.Version,
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

// rederiveFacts re-reads a known file's name with the current parser after a
// parser-version bump, updating only the filename-sourced facts (an embedded
// provenance line still wins, as for a fresh scan). Bytes, hash, links,
// presence and review state are kept; AddFile's merge preserves manual
// language/group corrections.
func (sc *Scanner) rederiveFacts(ctx context.Context, f store.File, path string, titles []string, series *store.Series, collections []store.Edition) error {
	parsed := parse.ParseAnchored(filepath.Base(path), parse.ChannelFile, titles)
	facts := store.AssetFacts{Sources: map[string]string{"language": "filename", "group": "filename", "quality": "filename"}, Title: filepath.Base(path), Protocol: f.Provenance, FactsKnown: true, Flags: candidateFlags(ReleaseCandidate{Parse: parsed}), Language: parsed.Language, Group: parsed.Group, Format: strings.TrimPrefix(filepath.Ext(path), "."), Resolution: parsed.Quality.Resolution, Revision: parsed.Version, Digital: parsed.Quality.Digital}
	if ci := mediafile.ReadComicInfo(path); ci != nil {
		if p, ok := mediafile.ReadProvenance(ci.Notes); ok && p.MangaBaka == series.MangaBakaID && series.MangaBakaID > 0 {
			if own := sc.linksFromProvenance(ctx, p, ci.LanguageISO, collections); len(own) > 0 {
				facts = provenanceFacts(p, ci, path)
			}
		}
	}
	// A dedicated rewrite, not AddFile: reviewed or manually corrected files
	// take AddFile's preservation path, which would keep the stale facts.
	// RewriteFileFacts keeps manual language/group and the review action and
	// updates only the facts and the parser stamp.
	return sc.store.RewriteFileFacts(ctx, f.ID, facts, parse.Version)
}

// replaced reports whether a known file now holds different pages. Reading
// every archive on every scan would be slow, so the fingerprint is only
// recomputed when the file looks touched: its stat signature differs from the
// one recorded with its fingerprint. A file without one looks touched when
// its size differs from the record or it was modified after it was last
// checked. Sento's own retagging trips this too and is told apart by the
// fingerprint, which ignores metadata.
func (sc *Scanner) replaced(ctx context.Context, f store.File, d fs.DirEntry) (bool, error) {
	info, err := d.Info()
	if err != nil || f.Hash == "" || !mediafile.IsArchive(f.Path) {
		return false, nil
	}
	now := time.Now()
	live, err := statArchive(f.Path)
	if err != nil {
		return false, nil // unreadable right now: presence checks report it
	}
	touched := statSignature(live) != f.HashStat
	if f.HashStat == "" {
		touched = info.Size() != f.Size
		if checked, e := time.Parse("2006-01-02 15:04:05", f.CheckedAt); e == nil && info.ModTime().UTC().After(checked) {
			touched = true
		}
	}
	if !touched {
		// A file recorded before signatures existed gets one now, so review
		// previews stop re-reading it. Only a settled signature is trusted.
		if f.HashStat == "" {
			if sig := trustedSignature(live, now); sig != "" {
				return false, sc.store.SetFileHashStat(ctx, f.ID, f.Hash, sig)
			}
		}
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
		if err = sc.store.SetFileSize(ctx, f.ID, info.Size()); err != nil {
			return false, err
		}
	}
	return false, sc.store.SetFileHashStat(ctx, f.ID, f.Hash, trustedSignature(live, now))
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

// linksFromProvenance links a file by the Notes line Sento wrote into it
// (docs/domain/metadata.md): the edition by MangaBaka's edition id, or a
// local edition by its name, in the file's language and ledger; the books
// by number, the works telling apart two that share one. Nil when any of
// it does not resolve, and the file is matched by its name instead.
func (sc *Scanner) linksFromProvenance(ctx context.Context, p mediafile.Provenance, language string, editions []store.Edition) []store.BookLink {
	if (p.Ledger != "volume" && p.Ledger != "chapter") || len(p.Books) == 0 {
		return nil
	}
	var col *store.Edition
	for i, c := range editions {
		if c.Type != p.Ledger || c.Language != language {
			continue
		}
		catalog := p.CatalogEdition != "" && !c.Custom && c.ProviderEditionID == p.CatalogEdition
		named := p.CatalogEdition == "" && c.EditionName == p.Edition && (c.Custom || c.ProviderEditionID == "")
		if !catalog && !named {
			continue
		}
		if col != nil {
			return nil // two editions answer: not the line's to choose
		}
		col = &editions[i]
	}
	if col == nil {
		return nil
	}
	versions, err := sc.store.ListVersions(ctx, col.SeriesID)
	if err != nil {
		return nil
	}
	var versionID int64
	for _, v := range versions {
		if v.EditionID == col.ID && v.IsDefault {
			versionID = v.ID
		}
	}
	books, err := sc.store.ListBooks(ctx, col.ID)
	if err != nil {
		return nil
	}
	var links []store.BookLink
	for _, n := range p.Books {
		var found []store.Book
		for _, b := range books {
			if mediafile.RenderPlainNum(b.SequenceNum) == n {
				found = append(found, b)
			}
		}
		if len(found) > 1 {
			found = slices.DeleteFunc(found, func(b store.Book) bool { return !slices.Contains(p.Works, b.ProviderID) })
		}
		if len(found) != 1 {
			return nil
		}
		links = append(links, store.BookLink{Ledger: p.Ledger, EditionID: col.ID, BookID: found[0].ID, VersionID: versionID, Number: found[0].SequenceNum})
	}
	return links
}

// provenanceFacts are a tagged file's facts: the release's, as the line
// and the standard fields recorded them when Sento imported it.
func provenanceFacts(p mediafile.Provenance, ci *mediafile.ComicInfo, path string) store.AssetFacts {
	via := p.Via
	if via == "" {
		via = "torrent"
	}
	release := parse.Parse(p.Release, parse.Channel(via))
	flags := map[string]bool{}
	for _, f := range p.Flags {
		flags[f] = true
	}
	return store.AssetFacts{
		Sources: map[string]string{"language": "comicinfo", "group": "comicinfo", "quality": "comicinfo"},
		Title:   p.Release, Site: p.Site, Protocol: via, FactsKnown: true, Flags: flags,
		Language: ci.LanguageISO, Group: ci.ScanInformation, Format: strings.TrimPrefix(filepath.Ext(path), "."),
		Resolution: release.Quality.Resolution, Revision: release.Version, Digital: flags["digital"],
	}
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
		if inFileTrash(dir) {
			return fmt.Errorf("restore files from trash before scanning this folder")
		}
		if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && d.Name() == fileTrashDir {
				return filepath.SkipDir
			}
			return fn(path, d, err)
		}); err != nil {
			return err
		}
	}
	return nil
}
