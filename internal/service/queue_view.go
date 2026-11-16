package service

import (
	"context"
	"time"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// withLiveProgress refreshes one queue item's view fields: a stall only
// counts while the last observation is recent, and a downloading item's
// progress comes from the stored observation and the provider's live state.
func withLiveProgress(registry *provider.Registry, item *store.QueueItem) {
	observed, _ := time.Parse(time.RFC3339, item.ObservedAt)
	if time.Since(observed) > observationGap {
		item.Stalled = false
	}
	if item.Status != store.QueueDownloading {
		return
	}
	if item.ObservedAt != "" {
		item.Progress = &item.DownloadProgress
	}
	prov := registry.Get(item.ProviderID)
	if pr, ok := prov.(provider.Progresser); ok {
		if frac, ok := pr.Progress(item.ExternalID); ok {
			item.Progress = &frac
		}
	}
}

// QueueView is the GET /queue listing: the stored items with CanTryAnother
// resolved against each series' files and the live progress filled in.
func (im *Importer) QueueView(ctx context.Context, activeOnly bool) ([]store.QueueItem, error) {
	items, err := im.store.ListQueueItems(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	files := map[int64][]store.File{}
	for i := range items {
		item := &items[i]
		if item.Status != store.QueueFailed && item.Status != store.QueueNeedsReview && item.Status != store.QueueRetryWait {
			continue
		}
		if _, ok := files[item.SeriesID]; !ok {
			files[item.SeriesID], err = im.store.ListFiles(ctx, item.SeriesID)
			if err != nil {
				return nil, err
			}
		}
		item.CanTryAnother = canTryAnother(*item, files[item.SeriesID])
	}
	for i := range items {
		withLiveProgress(im.registry, &items[i])
	}
	return items, nil
}

// canTryAnother excludes jobs that delivered files: their import review must
// be resolved before giving up their reservations. Running downloads retain
// their separate, non-destructive replacement search.
func canTryAnother(item store.QueueItem, files []store.File) bool {
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
