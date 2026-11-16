package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

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
	pathMappings []string // "remote=local" download-client path translations
	files        *LibraryFiles
}

func NewImporter(st *store.Store, registry *provider.Registry, files *LibraryFiles, pathMappings []string) *Importer {
	return &Importer{store: st, registry: registry, pathMappings: pathMappings, files: files}
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
	im.files.mu.Lock()
	defer im.files.mu.Unlock()
	im.files.finishReviewTrash(ctx)

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
			im.recordFailure(ctx, prov, item, err)
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

// recordFailure classifies one failed queue item and records its effects.
func (im *Importer) recordFailure(ctx context.Context, prov provider.Provider, item store.QueueItem, err error) {
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
			_ = im.store.DeferQueue(ctx, item.ID, err.Error(), queueRetryAt(item.RetryCount, time.Now().Add(sourceFailure.RetryAfter)))
		}
	case errors.Is(err, errDownloadDead):
		_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueFailed, err.Error())
	case errors.Is(err, errImportReview):
		_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueNeedsReview, err.Error())
	case errors.Is(err, errDispatchUnknown):
		_ = im.store.UpdateQueueItem(ctx, item.ID, store.QueueDispatchUnknown, err.Error())
	default:
		_ = im.store.DeferQueue(ctx, item.ID, err.Error(), queueRetryAt(item.RetryCount, time.Time{}))
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
		im.files.again.add(item)
	}
}

// queueRetryAt respects both the queue's backoff and the source's
// Retry-After deadline. The latter may exceed the usual backoff cap.
func queueRetryAt(retryCount int, notBefore time.Time) time.Time {
	at := time.Now().Add(30 * time.Second * time.Duration(1<<min(retryCount, 6)))
	if notBefore.After(at) {
		return notBefore
	}
	return at
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
	if err := observeDownload(ctx, im.store, item, state.Progress, state.Detail, state.StallEligible, state.Transferring, time.Now()); err != nil {
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
		// Neither done nor in flight: stuck. After two minutes, kick a
		// recoverable provider on every pass (Recover is idempotent); fail
		// after ten.
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

// dbTimeLayout is the timestamp layout the database stores.
const dbTimeLayout = "2006-01-02 15:04:05"

func parseTime(s string) time.Time {
	for _, layout := range []string{dbTimeLayout, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}
