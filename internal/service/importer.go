package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Importer advances the download queue for every provider: poll state,
// fetch finished payloads, import into the library, link slots, clean up.
// The import pipeline itself (naming, ComicInfo, linking) is provider-
// agnostic; only the payload shape varies (stream vs directory).
type Importer struct {
	store        *store.Store
	registry     *provider.Registry
	libraryRoot  string
	pathMappings []string // "remote=local" download-client path translations
}

func NewImporter(st *store.Store, registry *provider.Registry, libraryRoot string, pathMappings []string) *Importer {
	return &Importer{store: st, registry: registry, libraryRoot: libraryRoot, pathMappings: pathMappings}
}

// mapPath translates a download-client path to a local one (longest
// matching "remote=local" prefix wins; unmapped paths pass through).
func (im *Importer) mapPath(p string) string {
	bestRemote, bestLocal := "", ""
	for _, m := range im.pathMappings {
		remote, local, ok := strings.Cut(m, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(p, remote) && len(remote) > len(bestRemote) {
			bestRemote, bestLocal = remote, local
		}
	}
	if bestRemote == "" {
		return p
	}
	return filepath.Join(bestLocal, strings.TrimPrefix(p, bestRemote))
}

// ProcessQueue advances all active queue items. Returns the number of
// items imported this pass.
func (im *Importer) ProcessQueue(ctx context.Context) (int, error) {
	items, err := im.store.ListQueueItems(ctx, true)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, item := range items {
		prov := im.registry.ForProtocol(item.Protocol)
		if prov == nil {
			continue // provider currently unconfigured: leave the item queued
		}
		done, err := im.processItem(ctx, prov, item)
		if err != nil {
			slog.Warn("queue item failed", "id", item.ID, "title", item.Title, "error", err)
			_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			event := "import_failed"
			if errors.Is(err, errDownloadDead) {
				// Sonarr semantics: a download that never completes gets
				// blocklisted so the next sweep picks another release.
				// Import errors don't — they're usually local (paths,
				// permissions), not the release's fault.
				event = "download_failed"
				_ = im.store.AddBlocklist(ctx, item.Protocol, item.ExternalID, err.Error())
			}
			_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
				Event: event, Title: item.Title, Provider: prov.ID(),
				Key: item.ExternalID, Error: err.Error(),
			})
			continue
		}
		if done {
			imported++
			_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
				Event: "imported", Title: item.Title, Provider: prov.ID(),
				Key: item.ExternalID,
			})
		}
	}
	return imported, nil
}

// errDownloadDead marks downloads that never completed (vs import errors).
var errDownloadDead = errors.New("download dead")

func (im *Importer) processItem(ctx context.Context, prov provider.Provider, item store.QueueItem) (bool, error) {
	state, err := prov.State(ctx, item.ExternalID)
	if err != nil {
		return false, err
	}

	switch {
	case state.Done:
		// fall through to import
	case state.InFlight:
		if item.Status != store.QueueDownloading {
			return false, im.store.UpdateQueueItem(ctx, item.ID, store.QueueDownloading, "")
		}
		return false, nil
	default:
		// Neither done nor in flight: stuck. Kick recoverable providers
		// once, fail after a grace period.
		age := time.Since(parseTime(item.CreatedAt))
		if age > 2*time.Minute {
			if rec, ok := prov.(provider.Recoverer); ok {
				_ = rec.Recover(ctx)
			}
			if age > 10*time.Minute {
				return false, fmt.Errorf("%w: neither in flight nor completed after %s (%s)",
					errDownloadDead, age.Round(time.Second), state.Detail)
			}
		}
		return false, nil
	}

	if err := im.store.UpdateQueueItem(ctx, item.ID, store.QueueImporting, ""); err != nil {
		return false, err
	}

	payload, err := prov.Fetch(ctx, item.ExternalID)
	if err != nil {
		return false, err
	}
	if payload.Stream != nil {
		defer func() { _ = payload.Stream.Close() }()
		err = im.importStream(ctx, item, payload.Stream)
	} else {
		err = im.importFiles(ctx, item, im.mapPath(payload.Dir), payload.Files)
	}
	if err != nil {
		return false, err
	}

	if err := prov.Cleanup(ctx, item.ExternalID); err != nil {
		slog.Warn("provider cleanup failed", "provider", prov.ID(), "external", item.ExternalID, "error", err)
	}
	return true, im.store.UpdateQueueItem(ctx, item.ID, store.QueueImported, "")
}

// importStream imports a single-archive payload into the slot(s) the queue
// item expects — ledger-aware: volume slots get volume naming.
func (im *Importer) importStream(ctx context.Context, item store.QueueItem, stream io.Reader) error {
	ms := LoadMediaSettings(ctx, im.store)
	naming := ms.Naming()
	series, err := im.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	var slots []store.ExpectedSlot
	if err := json.Unmarshal(item.ExpectedSlots, &slots); err != nil || len(slots) == 0 {
		return fmt.Errorf("queue item has no expected slots")
	}
	first := slots[0]
	isVolume := first.Ledger == "volume"

	lang := "en"
	editionName := ""
	if isVolume {
		if col, err := im.store.GetCollection(ctx, first.CollectionID); err == nil {
			editionName, lang = col.EditionName, col.Language
		}
	} else if cl, err := im.store.GetChapterLedger(ctx, item.SeriesID); err == nil {
		lang = cl.Language
	}

	seriesDir, err := im.seriesDir(ctx, series, naming)
	if err != nil {
		return err
	}

	var destPath string
	info := mediafile.ComicInfo{
		Series:      series.Title,
		Summary:     series.Description,
		Web:         fmt.Sprintf("https://mangabaka.org/%d", series.MangaBakaID),
		LanguageISO: lang,
		Notes:       fmt.Sprintf("mangabaka:%d sento:v1", series.MangaBakaID),
	}
	if isVolume {
		fileName := mediafile.Render(naming.VolumeFile, mediafile.NameTokens{
			SeriesTitle: series.Title, SeriesID: series.MangaBakaID,
			Volume: first.Number, Chapter: -1, Edition: editionName, Lang: lang,
		})
		if !ms.RenameImports {
			fileName = mediafile.Sanitize(item.Title) + ".cbz"
		}
		destPath = filepath.Join(seriesDir, fileName)
		info.Volume = mediafile.RenderPlainNum(first.Number)
		// Also set Number: the merge would otherwise preserve whatever the
		// source's ComicInfo guessed (often a year mis-parse).
		info.Number = info.Volume
	} else {
		fileName := mediafile.Render(naming.ChapterFile, mediafile.NameTokens{
			SeriesTitle: series.Title, SeriesID: series.MangaBakaID,
			Volume: -1, Chapter: first.Number, Lang: lang,
		})
		if !ms.RenameImports {
			fileName = mediafile.Sanitize(item.Title) + ".cbz"
		}
		destPath = seriesDir
		if naming.ChapterDir != "" {
			destPath = filepath.Join(destPath, naming.ChapterDir)
		}
		destPath = filepath.Join(destPath, fileName)
		info.Number = mediafile.RenderPlainNum(first.Number)
	}

	writeInfo := &info
	if !ms.WriteComicInfo {
		writeInfo = nil
	}
	size, err := mediafile.ImportCBZ(stream, destPath, writeInfo)
	if err != nil {
		return err
	}

	links := make([]store.SlotLink, 0, len(slots))
	for _, sl := range slots {
		links = append(links, store.SlotLink{Ledger: sl.Ledger, CollectionID: sl.CollectionID, Number: sl.Number})
	}
	_, err = im.store.AddFile(ctx, store.File{
		SeriesID:   item.SeriesID,
		Path:       destPath,
		Size:       size,
		Provenance: item.Protocol,
	}, nil, links)
	return err
}

// importFiles imports a directory payload (torrents): every archive is
// parsed and hardlinked into the library for each expected slot it covers.
// Payload data is never moved (seeding, spec.md §7).
func (im *Importer) importFiles(ctx context.Context, item store.QueueItem, dir string, files []string) error {
	ms := LoadMediaSettings(ctx, im.store)
	naming := ms.Naming()
	series, err := im.store.GetSeries(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	titles, err := im.store.SeriesTitles(ctx, item.SeriesID)
	if err != nil {
		return err
	}
	var expected []store.ExpectedSlot
	if err := json.Unmarshal(item.ExpectedSlots, &expected); err != nil {
		return err
	}
	if len(expected) == 0 {
		return fmt.Errorf("queue item has no expected slots")
	}
	expectedSet := map[int64]store.ExpectedSlot{}
	var collectionID int64
	slotLedger := "volume"
	for _, sl := range expected {
		expectedSet[sl.Number] = sl
		collectionID = sl.CollectionID
		slotLedger = sl.Ledger
	}

	editionName, lang := "", ""
	if slotLedger == "volume" {
		if col, err := im.store.GetCollection(ctx, collectionID); err == nil {
			editionName, lang = col.EditionName, col.Language
		}
	} else if cl, err := im.store.GetChapterLedger(ctx, item.SeriesID); err == nil {
		lang = cl.Language
	}

	seriesDir, err := im.seriesDir(ctx, series, naming)
	if err != nil {
		return err
	}

	linked := 0
	for _, rel := range files {
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".cbz" && ext != ".cbr" {
			continue
		}
		res := parse.ParseAnchored(filepath.Base(rel), parse.ChannelFile, titles)
		ranges := res.Volumes
		if slotLedger == "chapter" {
			ranges = res.Chapters
		}
		for _, r := range ranges {
			for n := (int64(r.From) + 99) / 100 * 100; n <= int64(r.To); n += 100 {
				sl, ok := expectedSet[n]
				if !ok {
					continue
				}
				var dest string
				switch {
				case !ms.RenameImports:
					dest = filepath.Join(seriesDir, filepath.Base(rel))
					if slotLedger == "chapter" && naming.ChapterDir != "" {
						dest = filepath.Join(seriesDir, naming.ChapterDir, filepath.Base(rel))
					}
				case slotLedger == "volume":
					dest = filepath.Join(seriesDir, mediafile.Render(naming.VolumeFile, mediafile.NameTokens{
						SeriesTitle: series.Title, SeriesID: series.MangaBakaID,
						Volume: n, Chapter: -1, Edition: editionName, Lang: lang,
					}))
				default:
					dest = filepath.Join(seriesDir, naming.ChapterDir,
						mediafile.Render(naming.ChapterFile, mediafile.NameTokens{
							SeriesTitle: series.Title, SeriesID: series.MangaBakaID,
							Volume: -1, Chapter: n, Lang: lang,
						}))
				}
				// Hardlinked as-is: keep the source's real format extension
				// (a CBR renamed .cbz would lie about its contents).
				dest = strings.TrimSuffix(dest, filepath.Ext(dest)) + ext
				src := filepath.Join(dir, rel)
				transfer := mediafile.HardlinkOrCopy
				if ms.AlwaysCopy {
					transfer = mediafile.CopyFile
				}
				if err := transfer(src, dest); err != nil {
					return fmt.Errorf("linking %s: %w", rel, err)
				}
				size := int64(0)
				if fi, err := os.Stat(dest); err == nil {
					size = fi.Size()
				}
				if _, err := im.store.AddFile(ctx, store.File{
					SeriesID:   item.SeriesID,
					Path:       dest,
					Size:       size,
					Provenance: item.Protocol,
				}, res, []store.SlotLink{{Ledger: sl.Ledger, CollectionID: sl.CollectionID, Number: sl.Number}}); err != nil {
					return err
				}
				linked++
			}
		}
	}
	if linked == 0 {
		return fmt.Errorf("no archive in payload matched the expected slots")
	}
	return nil
}

func (im *Importer) seriesDir(ctx context.Context, series *store.Series, naming mediafile.NamingConfig) (string, error) {
	if series.Path != "" {
		return series.Path, nil
	}
	dir := filepath.Join(im.libraryRoot, mediafile.Render(naming.SeriesFolder, mediafile.NameTokens{
		SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Volume: -1, Chapter: -1,
	}))
	if err := im.store.SetSeriesPath(ctx, series.ID, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func parseTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

// BlocklistQueueItem manually bans a queue item's release and marks the
// item failed (typically a failed import whose release is actually bad).
func (im *Importer) BlocklistQueueItem(ctx context.Context, qid int64) error {
	items, err := im.store.ListQueueItems(ctx, false)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != qid {
			continue
		}
		if err := im.store.AddBlocklist(ctx, item.Protocol, item.ExternalID, "blocked manually"); err != nil {
			return err
		}
		_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, "blocked manually")
		_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
			Event: "blocklisted", Title: item.Title, Key: item.ExternalID,
		})
		return nil
	}
	return store.ErrNotFound
}
