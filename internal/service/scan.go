package service

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
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
}

var scanExts = map[string]bool{".cbz": true, ".cbr": true, ".zip": true, ".pdf": true, ".epub": true}

// ScanSeriesFolder walks dir (default: the series folder, derived from the
// naming template when unset) and links every archive it can identify.
func (sc *Scanner) ScanSeriesFolder(ctx context.Context, seriesID int64, dir string) (*ScanResult, error) {
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
	if dir == "" {
		dir = series.Path
	}
	if dir == "" {
		naming := LoadMediaSettings(ctx, sc.store).Naming()
		dir = filepath.Join(sc.libraryRoot, mediafile.Render(naming.SeriesFolder, mediafile.NameTokens{
			SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
		}))
	}

	// Adopt the folder as the series path if none was set.
	if series.Path == "" {
		if err := sc.store.SetSeriesPath(ctx, seriesID, dir); err != nil {
			return nil, err
		}
	}

	collections, err := sc.store.ListCollections(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	// Edition evidence determines file contents even when collection is paused.
	// Ambiguous archives remain available for explicit review.
	var monitored, chapterCols []store.Collection
	for _, c := range collections {
		switch c.Type {
		case "volume":
			monitored = append(monitored, c)
		case "chapter":
			if c.Monitored {
				chapterCols = append([]store.Collection{c}, chapterCols...)
			} else {
				chapterCols = append(chapterCols, c)
			}
		}
	}

	known := map[string]bool{}
	existing, err := sc.store.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, f := range existing {
		known[f.Path] = true
	}

	res := &ScanResult{}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtree: skip, keep scanning
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() || !scanExts[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		if known[path] {
			res.Skipped = append(res.Skipped, path)
			return nil
		}
		res.Scanned++

		parsed := parse.ParseAnchored(filepath.Base(path), parse.ChannelFile, titles)
		links := sc.linksFor(ctx, parsed, monitored, chapterCols)

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

// linksFor maps a parsed file onto slot links: volumes to the routed
// volume collection, integer chapters to the first monitored chapter
// collection (language-matched when the filename is tagged).
func (sc *Scanner) linksFor(ctx context.Context, parsed parse.Result, vols, chapters []store.Collection) []store.SlotLink {
	if parsed.ContentType != parse.ContentManga || !parsed.Anchored || parsed.TitleLeftover != "" {
		return nil
	}
	var links []store.SlotLink
	for _, axis := range []struct {
		kind   string
		cols   []store.Collection
		ranges []parse.Range
	}{{"volume", vols, parsed.Volumes}, {"chapter", chapters, parsed.Chapters}} {
		if len(axis.ranges) == 0 {
			continue
		}
		var matches []store.Collection
		for _, c := range axis.cols {
			if editionCompatible(parsed, c) {
				matches = append(matches, c)
			}
		}
		if len(matches) != 1 {
			continue
		}
		col := matches[0]
		targets, err := sc.store.ListTargets(ctx, col.SeriesID)
		if err != nil {
			continue
		}
		var targetID int64
		for _, target := range targets {
			if target.CollectionID == col.ID && target.IsDefault {
				targetID = target.ID
			}
		}
		works, err := sc.store.ListWorks(ctx, col.ID)
		if err != nil {
			continue
		}
		for _, w := range works {
			if !w.NumberKnown {
				continue
			}
			for _, r := range axis.ranges {
				if w.SequenceNum >= int64(r.From) && w.SequenceNum <= int64(r.To) {
					links = append(links, store.SlotLink{Ledger: axis.kind, CollectionID: col.ID, WorkID: w.ID, TargetID: targetID, Number: w.SequenceNum})
					break
				}
			}
		}
	}
	return links
}

// LowestOwnedChapters returns the lowest linked chapter number per chapter
// collection, used for the import-time cutoff default (§11.4: lowest owned
// chapter so owned chapters land in-ledger and the frontier continues).
func (sc *Scanner) LowestOwnedChapters(ctx context.Context, seriesID int64) (map[int64]int64, error) {
	links, err := sc.store.SlotLinks(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	lowest := map[int64]int64{}
	for key := range links {
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[0] != "chapter" {
			continue
		}
		colID, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		num, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil || num <= 0 {
			continue
		}
		if cur, ok := lowest[colID]; !ok || num < cur {
			lowest[colID] = num
		}
	}
	return lowest, nil
}

// ScanAndAdoptCutoff runs ScanSeriesFolder and, when chapter files were
// linked, lowers each chapter collection's cutoff to just below its lowest
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
	collections, err := sc.store.ListCollections(ctx, seriesID)
	if err != nil {
		return result, nil
	}
	for _, col := range collections {
		if low, ok := lowest[col.ID]; ok && col.Type == "chapter" && low-100 < col.CutoffNum {
			_ = sc.store.SetCollectionCutoff(ctx, seriesID, col.ID, low-100)
		}
	}
	return result, nil
}
