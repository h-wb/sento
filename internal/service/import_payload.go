package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// importStream imports a single-archive payload into the book(s) the queue
// item expects — ledger-aware: volume books get volume naming.
func (im *Importer) importStream(ctx context.Context, item store.QueueItem, stream io.Reader) error {
	ms := LoadMediaSettings(ctx, im.store)
	series, err := im.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	slots, err := im.normalizeExpected(ctx, item)
	if err != nil {
		return err
	}
	first, _ := slotSpan(slots)
	seriesDir, err := im.seriesDir(ctx, series, ms, first.Ledger)
	if err != nil {
		return err
	}

	// One book, or an archive holding several (a volume's worth of
	// chapters): named and tagged for what it actually holds.
	place := im.newPlacement(ms, series)
	src := releaseSource(item)
	group := src.Group
	info := place.info(ctx, slots, src)
	fileName := mediafile.Sanitize(item.Title) + ".cbz"
	if ms.RenameImports {
		fileName = place.fileName(ctx, slots, group, ".cbz")
	}
	destPath := filepath.Join(seriesDir, fileName)

	writeInfo := &info
	if !ms.WriteComicInfo {
		writeInfo = nil
	}
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		return err
	}
	mediafile.CleanStaleTemps(seriesDir)
	stage, err := os.CreateTemp(seriesDir, ".sento-stage-*.cbz")
	if err != nil {
		return err
	}
	stagePath := stage.Name()
	_ = stage.Close()
	defer func() { _ = os.Remove(stagePath) }()
	if _, err = mediafile.ImportCBZ(stream, stagePath, writeInfo); err != nil {
		return err
	}
	hash, err := fingerprint(stagePath)
	if err != nil {
		return err
	}
	destPath, err = placeAsset(place.dir(ctx, seriesDir, first), filepath.Base(destPath), hash, ms.HashInName)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if err = installAsset(stagePath, destPath, hash, false); err != nil {
		return err
	}
	return im.recordAsset(ctx, item, destPath, slots, nil)

}

// importFiles imports a directory payload (torrents): every archive is
// parsed and installed into the library for each expected book it covers —
// as a tagged CBZ copy, or a hardlink when metadata embedding is off.
// Payload data is never moved or modified (seeding, spec.md §7).
func (im *Importer) importFiles(ctx context.Context, item store.QueueItem, dir string, files []string) error {
	ms := LoadMediaSettings(ctx, im.store)
	series, err := im.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	titles, err := im.store.SeriesTitles(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	expected, err := im.normalizeExpected(ctx, item)
	if err != nil {
		return err
	}
	archives, cleanup, err := payloadArchives(ctx, dir, files, titles)
	defer cleanup()
	if err != nil {
		return err
	}
	seriesDir, err := im.seriesDir(ctx, series, ms, expected[0].Ledger)
	if err != nil {
		return err
	}
	// A file named by numbers alone ("Volume 1.cbz") is named by its
	// release: a source download is always the series', a torrent when its
	// name says so.
	release := parse.ParseAnchored(item.Title, parse.ChannelTorrent, titles)
	releaseNamed := item.Protocol == provider.ProtocolSource || (release.Anchored && release.TitleLeftover == "")
	linked, preservedHashes, err := im.alreadyImported(ctx, item, expected)
	if err != nil {
		return err
	}
	// One placement for the pack: its catalog rows and folder layout are
	// read once, not per archive.
	place := im.newPlacement(ms, series)
	// One archive waiting on a person must not abort the pack: the job is
	// not re-run once it waits for review, so every file of the payload is
	// recorded here, and the job is held for review at the end.
	waiting := 0
	for _, archive := range archives {
		if !mediafile.IsArchive(archive.name) {
			continue
		}
		if len(preservedHashes) > 0 {
			hash, e := fingerprint(archive.path)
			if e != nil {
				return e
			}
			if preservedHashes[hash] {
				continue
			}
		}
		covered, e := im.importArchive(ctx, item, place, seriesDir, titles, expected, archive, releaseNamed)
		if e != nil {
			if errors.Is(e, errAssetNeedsReview) {
				waiting++
				continue
			}
			return e
		}
		for _, sl := range covered {
			linked[store.BookKey(sl.VersionID, sl.BookID)] = true
		}
	}
	if len(linked) != len(expected) {
		if complete, e := im.reviewedJobComplete(ctx, item); e != nil {
			return e
		} else if complete {
			return nil
		}
		return fmt.Errorf("partial import: verified %d of %d expected books; unmatched files need review", len(linked), len(expected))
	}
	if waiting > 0 {
		return fmt.Errorf("%w (%d %s recorded for review)", errAssetNeedsReview, waiting, plural(waiting, "file"))
	}
	return nil
}

// alreadyImported is what an earlier attempt at this download brought in:
// the expected books a file of this release already holds, and the
// hashes of those files, so a retry neither redoes nor duplicates them.
func (im *Importer) alreadyImported(ctx context.Context, item store.QueueItem, expected []store.BookRef) (linked, hashes map[string]bool, err error) {
	linked, hashes = map[string]bool{}, map[string]bool{}
	if item.ReleaseKey == "" {
		return linked, hashes, nil
	}
	view, err := loadLedger(ctx, im.store, item.SeriesID)
	if err != nil {
		return nil, nil, err
	}
	known, err := im.store.ListFiles(ctx, item.SeriesID)
	if err != nil {
		return nil, nil, err
	}
	linked, hashes = releaseCoverage(item.ReleaseKey, expected, view, known)
	recorded, err := im.store.RecordedFiles(ctx, item.SeriesID)
	if err != nil {
		return nil, nil, err
	}
	for _, f := range recorded {
		if f.Facts.ReleaseKey != item.ReleaseKey || f.Hash == "" {
			continue
		}
		// Reviewed files keep their recorded row, and so do files still
		// waiting in review: re-importing either would install the same
		// bytes at a new path (a waiting file lives in review/) and record
		// a duplicate. Archives with no recorded row are still imported.
		if f.ReviewAction != "" || (f.NeedsReview && f.Presence == "present") {
			hashes[f.Hash] = true
		}
	}
	return linked, hashes, nil
}

// releaseCoverage identifies accepted books delivered by this release, rather
// than copies held from other downloads. Import retries and Activity share it.
func releaseCoverage(key string, expected []store.BookRef, view *ledgerView, files []store.File) (linked, hashes map[string]bool) {
	linked, hashes = map[string]bool{}, map[string]bool{}
	if key == "" {
		return linked, hashes
	}
	belongs := map[int64]bool{}
	for _, f := range files {
		if f.Facts.ReleaseKey == key && f.Presence == "present" && !f.NeedsReview && !f.Unrequested {
			belongs[f.ID] = true
			if f.Hash != "" {
				hashes[f.Hash] = true
			}
		}
	}
	for _, sl := range expected {
		if book := view.book(sl.VersionID, sl.BookID); book != nil {
			for _, fid := range book.FileIDs {
				if belongs[fid] {
					linked[store.BookKey(sl.VersionID, sl.BookID)] = true
				}
			}
		}
	}
	return linked, hashes
}

// importArchive installs one archive of a download: into the library for
// the expected books its name covers, or into review when it is not
// recognisably this series' or covers none. A name with no title at all
// is the series' when releaseNamed. It returns the books covered.
func (im *Importer) importArchive(ctx context.Context, item store.QueueItem, place *placement, seriesDir string, titles []string, expected []store.BookRef, archive payloadArchive, releaseNamed bool) ([]store.BookRef, error) {
	ms := place.ms
	rel, src := archive.name, archive.path
	res := parse.ParseAnchored(filepath.Base(rel), parse.ChannelFile, titles)
	named := (res.Anchored && res.TitleLeftover == "") || (res.Title == "" && releaseNamed)
	if archive.review || !named || res.ContentType != parse.ContentManga {
		return nil, im.importReviewFile(ctx, item, seriesDir, src, res, ms.AlwaysCopy)
	}
	contents, covered, err := im.fileBooks(ctx, res, expected)
	if err != nil {
		return nil, err
	}
	if len(covered) == 0 {
		return nil, im.importReviewFile(ctx, item, seriesDir, src, res, ms.AlwaysCopy)
	}
	hash, err := fingerprint(src)
	if err != nil {
		return nil, err
	}
	// A pack stays one asset with several verified coverage links.
	name := filepath.Base(rel)
	if ms.RenameImports {
		name = place.fileName(ctx, contents, res.Group, strings.ToLower(filepath.Ext(rel)))
	}
	// Identical bytes can serve several confirmed versions. Keep one
	// archive with all coverage links; different releases get different
	// hashes and can never replace this pathname.
	// With metadata embedding on, the library gets its own tagged CBZ
	// and the payload stays untouched for seeding; off, the payload is
	// hardlinked as it is.
	if ms.WriteComicInfo {
		name = strings.TrimSuffix(name, filepath.Ext(name)) + ".cbz"
	}
	dest, err := placeAsset(place.dir(ctx, seriesDir, covered[0]), name, hash, ms.HashInName)
	if err != nil {
		return nil, err
	}
	if ms.WriteComicInfo {
		came := releaseSource(item)
		came.Group = res.Group
		err = installProcessed(src, dest, hash, place.info(ctx, contents, came))
	} else {
		err = installAsset(src, dest, hash, ms.AlwaysCopy)
	}
	if err != nil {
		return nil, err
	}
	return covered, im.recordAsset(ctx, item, dest, covered, res, contents)
}

// seriesDir returns the folder for a series' books of the given type
// under the current settings, recording it on the series.
func (im *Importer) seriesDir(ctx context.Context, series *store.Series, ms MediaSettings, ledger string) (string, error) {
	volumes, chapters := seriesFolders(im.libraryRoot, series, ms)
	if series.Path != volumes {
		if err := im.store.SetSeriesPath(ctx, series.ID, volumes); err != nil {
			return "", err
		}
		series.Path = volumes
	}
	separate := ""
	if chapters != volumes {
		separate = chapters
	}
	if series.ChapterPath != separate {
		if err := im.store.SetSeriesChapterPath(ctx, series.ID, separate); err != nil {
			return "", err
		}
		series.ChapterPath = separate
	}
	if ledger == "chapter" {
		return chapters, nil
	}
	return volumes, nil
}
