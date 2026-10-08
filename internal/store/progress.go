package store

import (
	"context"
	"math"
	"time"
)

// ObserveDownload records progress independently of queue status. Paused,
// queued and checking torrents are not stalled. Long gaps in observation
// (server/client downtime) restart the timer instead of implying a stall.
func (s *Store) ObserveDownload(ctx context.Context, item QueueItem, progress float64, state string, eligible, transferring bool, now time.Time) error {
	if math.IsNaN(progress) || math.IsInf(progress, 0) {
		progress = 0
	}
	progress = max(0, min(1, progress))
	since, _ := time.Parse(time.RFC3339, item.ProgressAt)
	observed, _ := time.Parse(time.RFC3339, item.ObservedAt)
	if !eligible || transferring || since.IsZero() || now.Before(since) || now.Sub(observed) > 5*time.Minute || progress != item.DownloadProgress {
		since = now
	}
	stalled := eligible && !transferring && now.Sub(since) >= 20*time.Minute
	return s.touch(ctx, `UPDATE queue_items SET download_progress=?,progress_at=?,observed_at=?,download_state=?,stalled=? WHERE id=? AND status NOT IN('cancelled','imported')`, progress, since.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339), state, stalled, item.ID)
}
