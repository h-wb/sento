package service

import (
	"context"
	"math"
	"time"

	"github.com/h-wb/sento/internal/store"
)

const (
	// observationGap is the longest gap between observations that still
	// counts as continuous watching.
	observationGap = 5 * time.Minute
	// stallAfter is how long an eligible download may sit without progress.
	stallAfter = 20 * time.Minute
)

// observeDownload records progress independently of queue status. Paused,
// queued and checking torrents are not stalled. Long gaps in observation
// (server/client downtime) restart the timer instead of implying a stall.
func observeDownload(ctx context.Context, st *store.Store, item store.QueueItem, progress float64, state string, eligible, transferring bool, now time.Time) error {
	if math.IsNaN(progress) || math.IsInf(progress, 0) {
		progress = 0
	}
	progress = max(0, min(1, progress))
	since, _ := time.Parse(time.RFC3339, item.ProgressAt)
	observed, _ := time.Parse(time.RFC3339, item.ObservedAt)
	if !eligible || transferring || since.IsZero() || now.Before(since) || now.Sub(observed) > observationGap || progress != item.DownloadProgress {
		since = now
	}
	stalled := eligible && !transferring && now.Sub(since) >= stallAfter
	return st.RecordDownloadObservation(ctx, item.ID, progress, state, since, now, stalled)
}
