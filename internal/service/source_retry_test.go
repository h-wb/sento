package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func TestSourceFailureQueueRecovery(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(fmt.Sprintf("permanent=%v", permanent), func(t *testing.T) {
			ctx := context.Background()
			conn, dialect := dbtest.Open(t)
			st := store.New(conn, dialect)
			lib := NewLibrary(st, testProvider())
			sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
			if err != nil {
				t.Fatal(err)
			}
			editions, _ := st.ListEditions(ctx, sid)
			edition := collectionByName(t, editions, "Standard Edition", "en")
			books, _ := st.ListBooks(ctx, edition.ID)
			versions, _ := st.ListVersions(ctx, sid)
			var version store.CollectedVersion
			for _, v := range versions {
				if v.EditionID == edition.ID && v.IsDefault {
					version = v
				}
			}
			failure := &provider.DownloadError{Err: errors.New("page 5: HTTP error 503"), Retryable: !permanent, RetryAfter: time.Hour}
			p := &recoveryProvider{stateErr: failure}
			qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, ProviderID: p.ID(), Protocol: p.Protocol(), ReleaseKey: "source-release"}, []store.BookRef{modelRef(edition, books[0], version)})
			if err != nil {
				t.Fatal(err)
			}
			if err = st.SetQueueExternalID(ctx, qid, "remote"); err != nil {
				t.Fatal(err)
			}
			// The job can be much older than the legacy dead-download grace.
			createdQuery := `UPDATE queue_items SET created_at='2020-01-01 00:00:00' WHERE id=?`
			if dialect == "postgres" {
				createdQuery = `UPDATE queue_items SET created_at='2020-01-01 00:00:00' WHERE id=$1`
			}
			if _, err = conn.ExecContext(ctx, createdQuery, qid); err != nil {
				t.Fatal(err)
			}
			im := NewImporter(st, provider.NewRegistry(p), t.TempDir(), nil)
			for attempt := 0; attempt < 6; attempt++ {
				before := time.Now().Add(time.Hour)
				if _, err = im.ProcessQueue(ctx); err != nil {
					t.Fatal(err)
				}
				items, _ := st.ListQueueItems(ctx, true)
				if len(items) != 1 {
					t.Fatalf("job lost: %+v", items)
				}
				q := items[0]
				wantReview := permanent || attempt == 5
				if wantReview {
					if q.Status != store.QueueNeedsReview {
						t.Fatalf("unbounded failure: %+v", q)
					}
				} else {
					if q.Status != store.QueueRetryWait || q.RetryCount != attempt+1 || parseTime(q.RetryAt).Before(before) {
						t.Fatalf("Retry-After/backoff not persisted: %+v", q)
					}
					if _, err = im.ProcessQueue(ctx); err != nil {
						t.Fatal(err)
					}
					after, _ := st.ListQueueItems(ctx, true)
					if after[0].RetryCount != q.RetryCount {
						t.Fatal("retried before due")
					}
				}
				pending, err := st.PendingBooks(ctx, sid)
				if err != nil || !pending[store.BookKey(version.ID, books[0].ID)] {
					t.Fatal("failure released reservation")
				}
				blocked, err := st.ListBlocklist(ctx)
				if err != nil || len(blocked) != 0 {
					t.Fatal("source failure blocklisted release")
				}
				if wantReview {
					break
				}
				// Advance only the persisted due time, as if the worker restarted
				// after the backoff. Preserve the accumulated retry count.
				query := `UPDATE queue_items SET retry_at='2020-01-01 00:00:00' WHERE id=?`
				if dialect == "postgres" {
					query = `UPDATE queue_items SET retry_at='2020-01-01 00:00:00' WHERE id=$1`
				}
				if _, err = conn.ExecContext(ctx, query, qid); err != nil {
					t.Fatal(err)
				}
				im = NewImporter(st, provider.NewRegistry(p), t.TempDir(), nil)
			}
		})
	}
}
