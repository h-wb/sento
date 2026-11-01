package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/h-wb/sento/internal/apperr"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// Importer advances the download queue for every provider: poll state,
// fetch finished payloads, import into the library, link books, clean up.
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

	// again holds the series whose download failed since the last
	// TakeSearches: they are searched again within minutes, not at the
	// next sweep.
	againMu sync.Mutex
	again   []SearchTarget
}

// searchAgain asks for the job's series to be searched again soon.
func (im *Importer) searchAgain(item store.QueueItem) {
	im.againMu.Lock()
	defer im.againMu.Unlock()
	for _, t := range jobTargets(item) {
		if !slices.Contains(im.again, t) {
			im.again = append(im.again, t)
		}
	}
}

// TakeSearches hands over the series to search again, once each.
func (im *Importer) TakeSearches() []SearchTarget {
	im.againMu.Lock()
	defer im.againMu.Unlock()
	out := im.again
	im.again = nil
	return out
}

// jobTargets are the series' ledgers a job's books belong to.
func jobTargets(item store.QueueItem) []SearchTarget {
	var books []store.BookRef
	_ = json.Unmarshal(item.Books, &books)
	var out []SearchTarget
	for _, book := range books {
		if t := (SearchTarget{SeriesID: item.SeriesID, Ledger: book.Ledger}); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
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

	items, err := im.store.ListRunnableQueue(ctx)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, item := range items {
		if ctx.Err() != nil {
			return imported, ctx.Err() // shutting down: leave the rest queued
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
		// A circuit breaker may have retired later jobs in this same pass.
		current, err := im.store.GetQueueItem(ctx, item.ID)
		if err != nil {
			return imported, err
		}
		if current.Status == store.QueueCancelled {
			continue
		}
		done, err := im.processItem(ctx, prov, item)
		if err != nil {
			slog.Warn("queue item failed", "id", item.ID, "title", item.Title, "error", err)
			var sourceFailure *provider.DownloadError
			dead := errors.As(err, &sourceFailure) && sourceFailure.Dead
			event := "import_failed"
			switch {
			case dead:
				// The source cannot give it (gone, or unreadable): no retry
				// or review brings it back, so it fails like a dead torrent.
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			case sourceFailure != nil:
				// Source attempts finish with a classified error. Their age
				// includes time waiting for a slot, so it is never evidence
				// that the release is dead. Keep its claims through retries
				// and review instead of blocklisting a temporary site outage.
				const maxSourceRetries = 5
				if !sourceFailure.Retryable || item.RetryCount >= maxSourceRetries {
					event = "download_needs_review"
					reason := err.Error()
					if sourceFailure.Retryable {
						reason = fmt.Sprintf("source download still failing after %d attempts: %s", item.RetryCount+1, reason)
					}
					_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueNeedsReview, reason)
				} else {
					event = "download_retry"
					_ = im.store.DeferQueueUntil(ctx, item, err.Error(), time.Now().Add(sourceFailure.RetryAfter))
				}
			case errors.Is(err, errDownloadDead):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
			case errors.Is(err, errImportReview):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueNeedsReview, err.Error())
			case errors.Is(err, errDispatchUnknown):
				_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueDispatchUnknown, err.Error())
			default:
				_ = im.store.DeferQueue(ctx, item, err.Error())
			}
			if errors.Is(err, errDownloadDead) || dead {
				// Sonarr semantics: a download that never completes gets
				// blocklisted so the next sweep picks another release.
				// Import errors don't — they're usually local (paths,
				// permissions), not the release's fault.
				event = "download_failed"
				_ = im.blockRelease(ctx, item, err.Error())
			}
			_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
				Event: event, Title: item.Title, Provider: prov.ID(),
				Key: item.ExternalID, Error: err.Error(),
			})
			if dead {
				im.listingFailed(ctx, item)
			}
			if dead || errors.Is(err, errDownloadDead) {
				// The release is blocklisted: another is looked for now.
				im.searchAgain(item)
			}
			continue
		}
		if done {
			imported++
			if listing := jobListing(item); listing > 0 {
				_ = im.store.ListingWorked(ctx, listing)
			}
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
	// Every call about this job goes to the client it was sent to.
	ctx = provider.InClient(ctx, item.DownloadClient)
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
	if err := im.store.ObserveDownload(ctx, item, state.Progress, state.Detail, state.StallEligible, state.Transferring, time.Now()); err != nil {
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
	src := releaseSource(item)
	group := src.Group
	info := im.fileInfo(ctx, series, slots, src)
	fileName := mediafile.Sanitize(item.Title) + ".cbz"
	if ms.RenameImports {
		fileName = im.bookFileName(ctx, ms, series, slots, group, ".cbz")
	}
	destPath := filepath.Join(seriesDir, fileName)

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
	destPath, err = placeAsset(im.bookDir(ctx, ms, seriesDir, series, first), filepath.Base(destPath), hash, ms.HashInName)
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
	for _, archive := range archives {
		if !mediafile.IsArchive(archive.name) {
			continue
		}
		if len(preservedHashes) > 0 {
			hash, e := fileHash(archive.path)
			if e != nil {
				return e
			}
			if preservedHashes[hash] {
				continue
			}
		}
		covered, e := im.importArchive(ctx, item, ms, series, seriesDir, titles, expected, archive, releaseNamed)
		if e != nil {
			return e
		}
		for _, sl := range covered {
			linked[store.BookKey(sl.VersionID, sl.BookID)] = true
		}
	}
	if len(linked) != len(expected) {
		return fmt.Errorf("partial import: verified %d of %d expected books; unmatched files need review", len(linked), len(expected))
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
	belongs := map[int64]bool{}
	for _, f := range known {
		if f.Facts.ReleaseKey == item.ReleaseKey && f.Presence == "present" && !f.NeedsReview && !f.Unrequested {
			belongs[f.ID] = true
			if f.Hash != "" {
				hashes[f.Hash] = true
			}
		}
	}
	for _, sl := range expected {
		for _, book := range view.Status[sl.VersionID] {
			if book.Book.ID != sl.BookID {
				continue
			}
			for _, fid := range book.FileIDs {
				if belongs[fid] {
					linked[store.BookKey(sl.VersionID, sl.BookID)] = true
				}
			}
		}
	}
	return linked, hashes, nil
}

// importArchive installs one archive of a download: into the library for
// the expected books its name covers, or into review when it is not
// recognisably this series' or covers none. A name with no title at all
// is the series' when releaseNamed. It returns the books covered.
func (im *Importer) importArchive(ctx context.Context, item store.QueueItem, ms MediaSettings, series *store.Series, seriesDir string, titles []string, expected []store.BookRef, archive payloadArchive, releaseNamed bool) ([]store.BookRef, error) {
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
	hash, err := fileHash(src)
	if err != nil {
		return nil, err
	}
	// A pack stays one asset with several verified coverage links.
	name := filepath.Base(rel)
	if ms.RenameImports {
		name = im.bookFileName(ctx, ms, series, contents, res.Group, strings.ToLower(filepath.Ext(rel)))
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
	dest, err := placeAsset(im.bookDir(ctx, ms, seriesDir, series, covered[0]), name, hash, ms.HashInName)
	if err != nil {
		return nil, err
	}
	if ms.WriteComicInfo {
		came := releaseSource(item)
		came.Group = res.Group
		err = installProcessed(src, dest, hash, im.fileInfo(ctx, series, contents, came))
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
	item, err := im.store.GetQueueItem(ctx, qid)
	if err != nil {
		return err
	}
	if err := im.blockRelease(ctx, *item, "blocked manually"); err != nil {
		return err
	}
	_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, "blocked manually")
	_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
		Event: "blocklisted", Title: item.Title, Key: item.ExternalID,
	})
	return nil
}

// blockRelease bans a job's release under every name it goes by. A search
// knows a release by what the indexer published (ReleaseKey: often only a
// link); the job knows it by the torrent hash learned while grabbing
// (ExternalID). Banning only the hash would let the same listing through
// on the next search.
func (im *Importer) blockRelease(ctx context.Context, item store.QueueItem, reason string) error {
	keys := []string{item.ExternalID}
	if item.ReleaseKey != "" && item.ReleaseKey != item.ExternalID {
		keys = append(keys, item.ReleaseKey)
	}
	// The feed guid the candidate was listed under: a later search may
	// surface the same release without an infohash, keyed by guid alone.
	var snap struct {
		GUID string `json:"guid"`
	}
	if _ = json.Unmarshal(item.Snapshot, &snap); snap.GUID != "" && !slices.Contains(keys, snap.GUID) {
		keys = append(keys, snap.GUID)
	}
	blocked := false
	for _, key := range keys {
		if key == "" {
			continue
		}
		if err := im.store.AddBlocklist(ctx, item.Protocol, key, reason); err != nil {
			return err
		}
		blocked = true
	}
	if !blocked {
		return apperr.Invalidf("this job has no release identity to block")
	}
	return nil
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
	var plan []store.BookRef
	if err := json.Unmarshal(item.Books, &plan); err != nil {
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
		for _, book := range view.Status[sl.VersionID] {
			if book.Book.ID == sl.BookID {
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

// SettleReviewedJobs finishes the series' jobs held for review once no
// file they delivered still waits for it (propose-dont-ask.md §A2): the
// last confirmation closes the job, not another run of the importer.
// History says how many files were confirmed and which planned books no
// file turned out to hold; those are wanted again.
func (im *Importer) SettleReviewedJobs(ctx context.Context, seriesID int64) error {
	jobs, err := im.store.SeriesQueueItems(ctx, seriesID)
	if err != nil {
		return err
	}
	files, err := im.store.ListFiles(ctx, seriesID)
	if err != nil {
		return err
	}
	var coverage []store.AssetCoverage
	for _, job := range jobs {
		if job.Status != store.QueueNeedsReview || job.ReleaseKey == "" {
			continue
		}
		delivered, waiting := map[int64]bool{}, false
		for _, f := range files {
			if f.Facts.ReleaseKey == job.ReleaseKey {
				delivered[f.ID] = true
				waiting = waiting || f.NeedsReview
			}
		}
		// No files at all is an import that never happened (a path the
		// client sees and Sento does not): that still needs a person.
		if waiting || len(delivered) == 0 {
			continue
		}
		if coverage == nil {
			if coverage, err = im.store.Coverage(ctx, seriesID); err != nil {
				return err
			}
		}
		held := map[int64]bool{}
		for _, c := range coverage {
			held[c.BookID] = held[c.BookID] || delivered[c.FileID]
		}
		var plan []store.BookRef
		_ = json.Unmarshal(job.Books, &plan)
		var missing []string
		for _, ref := range plan {
			if !held[ref.BookID] {
				missing = append(missing, im.bookName(ctx, ref))
			}
		}
		settled, err := im.store.SettleReviewedQueue(ctx, job.ID)
		if err != nil || !settled {
			return err
		}
		if listing := jobListing(job); listing > 0 {
			_ = im.store.ListingWorked(ctx, listing)
		}
		note := fmt.Sprintf("%d %s confirmed in review.", len(delivered), plural(len(delivered), "file"))
		if len(missing) > 0 {
			note += " Not in this download, wanted again: " + strings.Join(missing, ", ") + "."
		}
		if err := im.store.AddDecision(ctx, seriesID, store.DecisionBody{Event: "imported_after_review", Title: job.Title, Provider: job.ProviderID, Key: job.ExternalID, Note: note}); err != nil {
			return err
		}
	}
	return nil
}

// bookName names a planned book for people: "Box Set Edition volume 0".
func (im *Importer) bookName(ctx context.Context, ref store.BookRef) string {
	book, err := im.store.GetBook(ctx, ref.BookID)
	if err != nil {
		return ref.Ledger + " " + hundredths(parse.Num(ref.Number))
	}
	name := ref.Ledger + " " + book.SequenceStr
	if ed, err := im.store.GetEdition(ctx, book.EditionID); err == nil {
		name = ed.EditionName + " " + name
	}
	return name
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// jobListing is the source listing a job's release came from, 0 for none.
func jobListing(item store.QueueItem) int64 {
	if item.Protocol != provider.ProtocolSource {
		return 0
	}
	var snap ReleaseCandidate
	_ = json.Unmarshal(item.Snapshot, &snap)
	return snap.BindingID
}

// listingFailed counts a release its listing could not give, and records
// it when that switches the listing off: three in a row mean its other
// releases would fail the same way, so the next search uses other
// listings and releases. Sources lists it, switched off, to switch back on.
func (im *Importer) listingFailed(ctx context.Context, item store.QueueItem) {
	listing := jobListing(item)
	if listing == 0 {
		return
	}
	if off, err := im.store.ListingFailed(ctx, listing); err == nil && off {
		jobs, err := im.store.SeriesQueueItems(ctx, item.SeriesID)
		if err != nil {
			slog.Warn("list jobs from disabled listing", "error", err)
			return
		}
		for _, job := range jobs {
			if jobListing(job) != listing || job.Protocol != provider.ProtocolSource {
				continue
			}
			switch job.Status {
			case store.QueueQueued, store.QueueDownloading, store.QueueRetryWait:
			default:
				continue
			}
			_ = im.store.UpdateQueueItem(ctx, job.ID, job.Status, "Source listing switched off after three dead downloads")
			if err := im.store.CancelQueue(ctx, job.ID); err != nil {
				slog.Warn("cancel disabled listing job", "id", job.ID, "error", err)
				continue
			}
			prov := im.registry.Get(job.ProviderID)
			if stopper, ok := prov.(provider.DownloadStopper); ok {
				if err := stopper.Stop(ctx, job.ExternalID); err != nil {
					slog.Warn("stop disabled listing download", "id", job.ID, "error", err)
				}
			}
		}
		_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
			Event: "listing_switched_off", Title: item.Title,
			Note: "Three releases in a row from this listing could not be downloaded; its other releases are skipped. Switch it back on in Sources.",
		})
	}
}

// SearchTarget is a series' ledger to search again.
type SearchTarget struct {
	SeriesID int64
	Ledger   string
}

// CanTryAnother excludes jobs that delivered files: their import review must
// be resolved before giving up their reservations. Running downloads retain
// their separate, non-destructive replacement search.
func CanTryAnother(item store.QueueItem, files []store.File) bool {
	switch item.Status {
	case store.QueueFailed, store.QueueNeedsReview, store.QueueRetryWait:
	default:
		return false
	}
	if item.ReleaseKey == "" && (item.ExternalID == "" || item.Status == store.QueueNeedsReview) {
		return false
	}
	for _, f := range files {
		if f.Facts.ReleaseKey != "" && (f.Facts.ReleaseKey == item.ReleaseKey || f.Facts.ReleaseKey == item.ExternalID) {
			return false
		}
	}
	return true
}

// TryAnother retires failed downloads and returns the distinct ledgers to
// search. Serializing with the worker prevents an import racing recovery.
func (im *Importer) TryAnother(ctx context.Context, ids []int64) ([]SearchTarget, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	var jobs []store.QueueItem
	files := map[int64][]store.File{}
	views := map[int64]*ledgerView{}
	pending := map[int64]map[string][]int64{}
	for _, id := range slices.Compact(slices.Sorted(slices.Values(ids))) {
		item, err := im.store.GetQueueItem(ctx, id)
		if err != nil {
			return nil, err
		}
		if _, ok := files[item.SeriesID]; !ok {
			files[item.SeriesID], err = im.store.ListFiles(ctx, item.SeriesID)
			if err != nil {
				return nil, err
			}
		}
		if CanTryAnother(*item, files[item.SeriesID]) {
			if item.Status == store.QueueFailed {
				if _, ok := views[item.SeriesID]; !ok {
					var e error
					views[item.SeriesID], e = loadLedger(ctx, im.store, item.SeriesID)
					if e != nil {
						return nil, e
					}
					others, e := im.store.SeriesQueueItems(ctx, item.SeriesID)
					if e != nil {
						return nil, e
					}
					pending[item.SeriesID] = activityPending(others)
				}
				if failedResolution(*item, views[item.SeriesID], pending[item.SeriesID]) != "" {
					continue
				}
			}
			jobs = append(jobs, *item)
		}
	}
	var targets []SearchTarget
	for _, item := range jobs {
		const reason = "another release was asked for"
		if err := im.blockRelease(ctx, item, reason); err != nil {
			return targets, err
		}
		if err := im.store.CancelQueue(ctx, item.ID); err != nil {
			return targets, err
		}
		if stopper, ok := im.registry.Get(item.ProviderID).(provider.DownloadStopper); ok {
			if err := stopper.Stop(ctx, item.ExternalID); err != nil {
				slog.Warn("stop abandoned download", "id", item.ID, "error", err)
			}
		}
		_ = im.store.AddDecision(ctx, item.SeriesID, store.DecisionBody{
			Event: "blocklisted", Title: item.Title, Key: item.ExternalID, Note: "Another release was asked for.",
		})
		for _, t := range jobTargets(item) {
			if !slices.Contains(targets, t) {
				targets = append(targets, t)
			}
		}
	}
	return targets, nil
}
