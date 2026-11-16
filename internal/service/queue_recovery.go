package service

import (
	"context"
	"log/slog"
	"slices"
	"sync"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// retrySearches collects the series to search again soon, once each.
type retrySearches struct {
	mu      sync.Mutex
	targets []SearchTarget
}

func (r *retrySearches) add(item store.QueueItem) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range jobTargets(item) {
		if !slices.Contains(r.targets, t) {
			r.targets = append(r.targets, t)
		}
	}
}

func (r *retrySearches) take() []SearchTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.targets
	r.targets = nil
	return out
}

// searchAgain asks for the job's series to be searched again soon.
func (im *Importer) searchAgain(item store.QueueItem) {
	im.files.again.add(item)
}

// TakeSearches hands over the series to search again, once each.
func (im *Importer) TakeSearches() []SearchTarget {
	return im.files.again.take()
}

// jobTargets are the series' ledgers a job's books belong to.
func jobTargets(item store.QueueItem) []SearchTarget {
	books := queueBooks(&item)
	var out []SearchTarget
	for _, book := range books {
		if t := (SearchTarget{SeriesID: item.SeriesID, Ledger: book.Ledger}); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
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
	if snap := jobSnapshot(item); snap.GUID != "" && !slices.Contains(keys, snap.GUID) {
		keys = append(keys, snap.GUID)
	}
	return im.store.AddReleaseBlocklist(ctx, item.Protocol, keys, item.Title, reason)
}

// jobListing is the source listing a job's release came from, 0 for none.
func jobListing(item store.QueueItem) int64 {
	if item.Protocol != provider.ProtocolSource {
		return 0
	}
	return jobSnapshot(item).BindingID
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

// UnblockReleases clears blocklist entries selected by ID, or one release
// by its protocol and known keys.
func (im *Importer) UnblockReleases(ctx context.Context, ids []int64, protocol string, keys []string) error {
	byID := len(ids) > 0 && protocol == "" && len(keys) == 0
	byKey := len(ids) == 0 && (protocol == provider.ProtocolSource || protocol == provider.ProtocolTorrent) && len(keys) > 0
	if !byID && !byKey {
		return apperr.Invalidf("select blocklist IDs or release keys with a protocol")
	}
	for _, id := range ids {
		if id <= 0 {
			return apperr.Invalidf("blocklist IDs must be positive")
		}
	}
	for _, key := range keys {
		if key == "" {
			return apperr.Invalidf("release keys must not be empty")
		}
	}
	return im.store.UnblockReleases(ctx, ids, protocol, keys)
}

// TryAnother retires failed downloads and returns the distinct ledgers to
// search. Serializing with the worker prevents an import racing recovery.
func (im *Importer) TryAnother(ctx context.Context, ids []int64) ([]SearchTarget, error) {
	im.files.mu.Lock()
	defer im.files.mu.Unlock()
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
		if canTryAnother(*item, files[item.SeriesID]) {
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
