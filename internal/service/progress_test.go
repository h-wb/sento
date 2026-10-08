package service

import (
	"context"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/store"
)

func TestPersistentStallDetectionAndRecovery(t *testing.T) {
	_, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	qid, err := st.AddQueueItem(ctx, store.QueueItem{SeriesID: id, Protocol: "torrent", ExternalID: "hash", Title: "book", Status: store.QueueDownloading, Books: store.JSONText(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	observe := func(at time.Time, fraction float64, eligible, transferring bool) *store.QueueItem {
		item, e := st.GetQueueItem(ctx, qid)
		if e != nil {
			t.Fatal(e)
		}
		if e = st.ObserveDownload(ctx, *item, fraction, "downloading", eligible, transferring, at); e != nil {
			t.Fatal(e)
		}
		item, e = st.GetQueueItem(ctx, qid)
		if e != nil {
			t.Fatal(e)
		}
		return item
	}
	for i := 0; i <= 20; i++ {
		item := observe(now.Add(time.Duration(i)*time.Minute), 0.2, true, false)
		if item.Stalled != (i == 20) {
			t.Fatalf("minute %d: %+v", i, item)
		}
	}
	if item := observe(now.Add(21*time.Minute), 0.21, true, false); item.Stalled {
		t.Fatal("progress did not clear stall")
	}
	if item := observe(now.Add(60*time.Minute), 0.21, true, false); item.Stalled {
		t.Fatal("downtime counted as stall")
	}
	for i := 61; i <= 85; i++ {
		if item := observe(now.Add(time.Duration(i)*time.Minute), 0.21, false, false); item.Stalled {
			t.Fatal("paused torrent stalled")
		}
	}
	for i := 86; i <= 110; i++ {
		if item := observe(now.Add(time.Duration(i)*time.Minute), 0.21, true, true); item.Stalled {
			t.Fatal("active transfer stalled")
		}
	}
}
