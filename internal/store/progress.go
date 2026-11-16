package store

import (
	"context"
	"time"
)

// RecordDownloadObservation stores one progress observation, independently
// of queue status.
func (s *Store) RecordDownloadObservation(ctx context.Context, id int64, progress float64, state string, progressAt, observedAt time.Time, stalled bool) error {
	return s.touch(ctx, `UPDATE queue_items SET download_progress=?,progress_at=?,observed_at=?,download_state=?,stalled=? WHERE id=? AND status NOT IN('cancelled','imported')`, progress, progressAt.UTC().Format(time.RFC3339), observedAt.UTC().Format(time.RFC3339), state, stalled, id)
}
