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
	"sync"
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

	// mu serializes queue passes: the scheduler tick, the System "run now"
	// button and POST /queue/process can overlap, and two passes over the
	// same finished item would import it twice.
	mu sync.Mutex
}

func NewImporter(st *store.Store, registry *provider.Registry, libraryRoot string, pathMappings []string) *Importer {
	return &Importer{store: st, registry: registry, libraryRoot: libraryRoot, pathMappings: pathMappings}
}

// mapPath translates a download-client path to a local one (longest
// matching "remote=local" prefix wins; unmapped paths pass through). A
// mapping matches whole path components only: "/data" maps "/data/x" but
// leaves "/database" alone.
func (im *Importer) mapPath(p string) string {
	bestRemote, bestLocal := "", ""
	for _, m := range im.pathMappings {
		remote, local, ok := strings.Cut(m, "=")
		remote = strings.TrimRight(remote, "/")
		if !ok || remote == "" {
			continue
		}
		if (p == remote || strings.HasPrefix(p, remote+"/")) && len(remote) > len(bestRemote) {
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
	im.mu.Lock()
	defer im.mu.Unlock()

	items, err := im.store.ListQueueItems(ctx, true)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return imported, ctx.Err() // shutting down: leave the rest queued
		}
		if item.Status == store.QueueNeedsReview || item.Status == store.QueueDispatchUnknown {
			continue
		}
		if item.Status == store.QueueRetryWait && item.RetryAt != "" && time.Now().Before(parseTime(item.RetryAt)) {
			continue
		}
		prov := im.registry.Get(item.ProviderID)
		if item.ProviderID == "" {
			prov = im.registry.ForProtocol(item.Protocol)
		}
		if prov == nil {
			continue // provider currently unconfigured: leave the item queued
		}
		done, err := im.processItem(ctx, prov, item)
		if err != nil {
			slog.Warn("queue item failed", "id", item.ID, "title", item.Title, "error", err)
			switch {
			case errors.Is(err, errDownloadDead):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			case errors.Is(err, errImportReview):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueNeedsReview, err.Error())
			case errors.Is(err, errDispatchUnknown):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueDispatchUnknown, err.Error())
			default:
				_ = im.store.DeferQueue(ctx, item, err.Error())
			}
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
var errImportReview = errors.New("import needs review")
var errDispatchUnknown = errors.New("download dispatch needs review")

func (im *Importer) processItem(ctx context.Context, prov provider.Provider, item store.QueueItem) (bool, error) {
	if item.ExternalID == "" {
		// Reconcile an uncertain submission with its stable provider key; never submit twice.
		if item.ReleaseKey != "" {
			state, err := prov.State(ctx, item.ReleaseKey)
			if err == nil && (state.Done || state.InFlight) {
				if err = im.store.SetQueueExternalID(ctx, item.ID, item.ReleaseKey); err != nil {
					return false, err
				}
				item.ExternalID = item.ReleaseKey
			}
		}
		if item.ExternalID == "" {
			if time.Since(parseTime(item.CreatedAt)) > 10*time.Minute {
				return false, fmt.Errorf("%w: confirm the download in your client, then attach its ID or cancel this job", errDispatchUnknown)
			}
			return false, nil
		}
	}
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

	if item.ExternalID == "" {
		return false, fmt.Errorf("download was interrupted before its remote handle was recorded; retry the release")
	}
	frozen, err := im.store.BeginImport(ctx, item.ID)
	if err != nil {
		return false, err
	}
	item = *frozen

	if complete, checkErr := im.reviewedJobComplete(ctx, item); checkErr != nil {
		return false, checkErr
	} else if complete {
		_ = prov.Cleanup(ctx, item.ExternalID)
		return true, im.store.UpdateQueueItem(ctx, item.ID, store.QueueImported, "")
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
		return false, fmt.Errorf("%w: %v", errImportReview, err)
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
	slots, err := im.normalizeExpected(ctx, item)
	if err != nil {
		return err
	}
	first := slots[0]
	isVolume := first.Ledger == "volume"

	// Both ledgers carry a real collection id now; its identity names the file.
	lang := "en"
	editionName := ""
	if col, err := im.store.GetCollection(ctx, first.CollectionID); err == nil {
		editionName, lang = col.EditionName, col.Language
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
			Volume: -1, Chapter: first.Number, Edition: editionName, Lang: lang,
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
	if err := os.MkdirAll(seriesDir, 0o755); err != nil {
		return err
	}
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
	hash, err := fileHash(stagePath)
	if err != nil {
		return err
	}
	destPath = assetPath(seriesDir, filepath.Base(destPath), hash, first)
	if err = verifyDestination(destPath, hash); err != nil {
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
	expected, err := im.normalizeExpected(ctx, item)
	if err != nil {
		return err
	}
	seriesDir, err := im.seriesDir(ctx, series, naming)
	if err != nil {
		return err
	}
	linked := map[string]bool{}
	preservedHashes := map[string]bool{}
	if item.ReleaseKey != "" {
		view, e := loadLedger(ctx, im.store, item.SeriesID)
		if e != nil {
			return e
		}
		known, e := im.store.ListFiles(ctx, item.SeriesID)
		if e != nil {
			return e
		}
		belongs := map[int64]bool{}
		for _, f := range known {
			if f.Facts.ReleaseKey == item.ReleaseKey && f.Presence == "present" && !f.NeedsReview && !f.Unrequested {
				belongs[f.ID] = true
				if f.Hash != "" {
					preservedHashes[f.Hash] = true
				}
			}
		}
		for _, sl := range expected {
			for _, book := range view.Slots[sl.TargetID] {
				if book.Work.ID == sl.WorkID {
					for _, fid := range book.FileIDs {
						if belongs[fid] {
							linked[store.SlotKey("unit", sl.TargetID, sl.WorkID)] = true
						}
					}
				}
			}
		}
	}
	for _, rel := range files {
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(filepath.Clean(rel), ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid payload path")
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if ext != ".cbz" && ext != ".cbr" {
			continue
		}
		if len(preservedHashes) > 0 {
			hash, e := fileHash(filepath.Join(dir, rel))
			if e != nil {
				return e
			}
			if preservedHashes[hash] {
				continue
			}
		}
		res := parse.ParseAnchored(filepath.Base(rel), parse.ChannelFile, titles)
		if !res.Anchored || res.TitleLeftover != "" || res.ContentType != parse.ContentManga {
			if err := im.importReviewFile(ctx, item, seriesDir, filepath.Join(dir, rel), res, ms.AlwaysCopy); err != nil {
				return err
			}
			continue
		}
		ranges := res.Volumes
		if expected[0].Ledger == "chapter" {
			ranges = res.Chapters
		}
		var covered []store.ExpectedSlot
		for _, sl := range expected {
			w, e := im.store.GetWork(ctx, sl.WorkID)
			if e != nil {
				return e
			}
			col, e := im.store.GetCollection(ctx, sl.CollectionID)
			if e != nil {
				return e
			}
			if !editionCompatible(res, *col) {
				continue
			}
			for _, r := range ranges {
				if w.NumberKnown && w.SequenceNum >= int64(r.From) && w.SequenceNum <= int64(r.To) {
					covered = append(covered, sl)
					break
				}
			}
		}
		if len(covered) == 0 {
			if err := im.importReviewFile(ctx, item, seriesDir, filepath.Join(dir, rel), res, ms.AlwaysCopy); err != nil {
				return err
			}
			continue
		}
		src := filepath.Join(dir, rel)
		hash, e := fileHash(src)
		if e != nil {
			return e
		}
		// A pack stays one asset with several verified coverage links.
		name := filepath.Base(rel)
		if ms.RenameImports && len(covered) == 1 {
			sl := covered[0]
			col, e := im.store.GetCollection(ctx, sl.CollectionID)
			if e != nil {
				return e
			}
			token := mediafile.NameTokens{SeriesTitle: series.Title, SeriesID: series.MangaBakaID, Edition: col.EditionName, Lang: col.Language, Group: res.Group, Volume: -1, Chapter: -1}
			tmpl := naming.VolumeFile
			if sl.Ledger == "volume" {
				token.Volume = sl.Number
			} else {
				token.Chapter = sl.Number
				tmpl = naming.ChapterFile
			}
			name = mediafile.Render(tmpl, token)
			name = strings.TrimSuffix(name, filepath.Ext(name)) + ext
		}
		// Identical bytes can serve several confirmed versions. Keep one
		// archive with all coverage links; different releases get different
		// hashes and can never replace this pathname.
		dest := assetPath(seriesDir, name, hash, covered[0])
		if e = verifyDestination(dest, hash); e != nil {
			return e
		}
		if e = installAsset(src, dest, hash, ms.AlwaysCopy); e != nil {
			return e
		}
		if e = im.recordAsset(ctx, item, dest, covered, res); e != nil {
			return e
		}
		for _, sl := range covered {
			linked[store.SlotKey("unit", sl.TargetID, sl.WorkID)] = true
		}
	}
	if len(linked) != len(expected) {
		return fmt.Errorf("partial import: verified %d of %d expected units; unmatched files need review", len(linked), len(expected))
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

// Reviewed payloads can complete without redownloading or reparsing names that
// the user already corrected. Only assets from this offer can resolve its job.
func (im *Importer) reviewedJobComplete(ctx context.Context, item store.QueueItem) (bool, error) {
	if item.ReleaseKey == "" {
		return false, nil
	}
	if err := im.store.CheckFilePresence(ctx, item.SeriesID); err != nil {
		return false, err
	}
	var plan []store.ExpectedSlot
	if err := json.Unmarshal(item.ExpectedSlots, &plan); err != nil {
		return false, err
	}
	if len(plan) == 0 {
		return false, nil
	}
	view, err := loadLedger(ctx, im.store, item.SeriesID)
	if err != nil {
		return false, err
	}
	files, err := im.store.ListFiles(ctx, item.SeriesID)
	if err != nil {
		return false, err
	}
	belongs := map[int64]bool{}
	for _, f := range files {
		belongs[f.ID] = f.Facts.ReleaseKey == item.ReleaseKey
	}
	for _, sl := range plan {
		found := false
		for _, book := range view.Slots[sl.TargetID] {
			if book.Work.ID == sl.WorkID {
				for _, fid := range book.FileIDs {
					if belongs[fid] {
						found = true
					}
				}
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}
