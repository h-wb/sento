package service

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/h-wb/sento/internal/library"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Scanner implements per-series existing-library import (spec.md §11.4):
// files are linked where they are — nothing is moved, renamed or rewritten.
type Scanner struct {
	store       *store.Store
	naming      library.NamingConfig
	libraryRoot string
}

func NewScanner(st *store.Store, naming library.NamingConfig, libraryRoot string) *Scanner {
	return &Scanner{store: st, naming: naming, libraryRoot: libraryRoot}
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
		dir = filepath.Join(sc.libraryRoot, library.Render(sc.naming.SeriesFolder, library.NameTokens{
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
	// Volume links route to the sole monitored volume collection, falling
	// back to edition-hint matching (R6-lite; review UI is post-v1).
	var monitored []store.Collection
	for _, c := range collections {
		if c.Monitored && c.Type == "volume" {
			monitored = append(monitored, c)
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
		links := sc.linksFor(parsed, monitored)

		info, ierr := d.Info()
		size := int64(0)
		if ierr == nil {
			size = info.Size()
		}
		_, aerr := sc.store.AddFile(ctx, store.File{
			SeriesID:    seriesID,
			Path:        path,
			Size:        size,
			Provenance:  "library-import",
			Unrequested: len(links) == 0,
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
	return res, nil
}

// linksFor maps a parsed file onto slot links: volumes to the routed
// collection, integer chapters to the chapter ledger.
func (sc *Scanner) linksFor(parsed parse.Result, monitored []store.Collection) []store.SlotLink {
	var links []store.SlotLink

	if len(parsed.Volumes) > 0 && len(monitored) > 0 {
		col := monitored[0]
		hintMatched := false
		for _, c := range monitored {
			name := strings.ToLower(c.EditionName)
			switch {
			case parsed.Edition.Omnibus && strings.Contains(name, "omnibus"),
				parsed.Edition.BoxSet && strings.Contains(name, "box"),
				parsed.Edition.Colored && strings.Contains(name, "color"),
				parsed.Edition.Deluxe && strings.Contains(name, "deluxe"):
				col = c
				hintMatched = true
			}
		}
		if !hintMatched {
			// Plain files prefer the standard edition (R6 default).
			for _, c := range monitored {
				if strings.Contains(strings.ToLower(c.EditionName), "standard") {
					col = c
					break
				}
			}
		}
		for _, r := range parsed.Volumes {
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				links = append(links, store.SlotLink{Ledger: "volume", CollectionID: col.ID, Number: n})
			}
		}
	}
	for _, r := range parsed.Chapters {
		for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
			links = append(links, store.SlotLink{Ledger: "chapter", Number: n})
		}
	}
	return links
}

// LowestOwnedChapter returns the lowest linked chapter number of a series,
// used for the import-time cutoff default (§11.4: lowest owned chapter so
// owned chapters land in-ledger and the frontier continues).
func (sc *Scanner) LowestOwnedChapter(ctx context.Context, seriesID int64) (int64, error) {
	links, err := sc.store.SlotLinks(ctx, seriesID)
	if err != nil {
		return 0, err
	}
	lowest := int64(0)
	for key := range links {
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[0] != "chapter" {
			continue
		}
		num, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}
		if num > 0 && (lowest == 0 || num < lowest) {
			lowest = num
		}
	}
	return lowest, nil
}
