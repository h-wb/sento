package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// A source that no longer has the listing (404) fails the job and
// blocklists the release, so the next sweep picks another one, instead
// of leaving the job for review.
func TestGoneSourceListingIsBlocklisted(t *testing.T) {
	ctx := context.Background()
	_, st, sid, edition, books, version := modelFixture(t)
	failure := &provider.DownloadError{Err: errors.New("listing pages: host: HTTP error 404"), Dead: true}
	p := &recoveryProvider{stateErr: failure}
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, ProviderID: p.ID(), Protocol: p.Protocol(), ReleaseKey: "gone-release"}, []store.BookRef{modelRef(edition, books[0], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetQueueExternalID(ctx, qid, "remote"); err != nil {
		t.Fatal(err)
	}
	if _, err = NewImporter(st, provider.NewRegistry(p), t.TempDir(), nil).ProcessQueue(ctx); err != nil {
		t.Fatal(err)
	}
	q, err := st.GetQueueItem(ctx, qid)
	if err != nil || q.Status != store.QueueFailed {
		t.Fatalf("a gone listing is not failed: %+v %v", q, err)
	}
	blocked, err := st.BlockedKeys(ctx)
	if err != nil || len(blocked) == 0 {
		t.Fatalf("a gone listing is not blocklisted: %v %v", blocked, err)
	}
}

// Three releases in a row that a listing cannot give switch it off, and
// history says so; fewer leave it on.
func TestAListingThatKeepsFailingIsSwitchedOff(t *testing.T) {
	ctx := context.Background()
	_, st, sid, edition, books, version := modelFixture(t)
	bid, err := st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: "1", SourceMangaKey: "broken", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	p := &recoveryProvider{stateErr: &provider.DownloadError{Err: errors.New("page 3: host: Not a local file header"), Dead: true}}
	im := NewImporter(st, provider.NewRegistry(p), t.TempDir(), nil)
	state := func() string {
		bindings, _ := st.ListBindings(ctx, sid)
		for _, b := range bindings {
			if b.ID == bid {
				return b.State
			}
		}
		return ""
	}
	for i := range 3 {
		snap, _ := json.Marshal(map[string]any{"bindingId": bid})
		qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, ProviderID: p.ID(), Protocol: provider.ProtocolSource, ReleaseKey: fmt.Sprintf("broken-%d", i), Snapshot: snap}, []store.BookRef{modelRef(edition, books[i], version)})
		if err != nil {
			t.Fatal(err)
		}
		if err = st.SetQueueExternalID(ctx, qid, fmt.Sprintf("remote-%d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err = im.ProcessQueue(ctx); err != nil {
			t.Fatal(err)
		}
		if want := map[bool]string{true: "disabled", false: "active"}[i == 2]; state() != want {
			t.Fatalf("after %d failures the listing is %s, want %s", i+1, state(), want)
		}
	}
	events, _ := st.ListDecisions(ctx, sid, 20)
	if len(events) == 0 || !strings.Contains(string(events[0].Body), "listing_switched_off") {
		t.Fatalf("switching the listing off is not in history: %+v", events)
	}
}

// Trying another release gives up a stuck job's release and says which
// series to search again; a job already importing is left alone.
func TestTryAnotherGivesUpTheRelease(t *testing.T) {
	ctx := context.Background()
	_, st, sid, edition, books, version := modelFixture(t)
	reserve := func(key string, book int) int64 {
		qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, ProviderID: "host", Protocol: provider.ProtocolSource, ReleaseKey: key}, []store.BookRef{modelRef(edition, books[book], version)})
		if err != nil {
			t.Fatal(err)
		}
		return qid
	}
	stuck, importing := reserve("stuck", 0), reserve("busy", 1)
	if err := st.UpdateQueueItem(ctx, stuck, store.QueueNeedsReview, "listing pages: host: HTTP error 404"); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateQueueItem(ctx, importing, store.QueueImporting, ""); err != nil {
		t.Fatal(err)
	}
	targets, err := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil).TryAnother(ctx, []int64{stuck, importing})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0] != (SearchTarget{SeriesID: sid, Ledger: edition.Type}) {
		t.Fatalf("targets = %+v", targets)
	}
	if q, _ := st.GetQueueItem(ctx, stuck); q.Status != store.QueueCancelled {
		t.Fatalf("stuck job is %s", q.Status)
	}
	if q, _ := st.GetQueueItem(ctx, importing); q.Status != store.QueueImporting {
		t.Fatalf("an importing job was given up: %s", q.Status)
	}
	if blocked, _ := st.BlockedKeys(ctx); len(blocked) == 0 {
		t.Fatal("the release was not blocklisted")
	}
}

// Recovery must never give up files already delivered to the review inbox.
func TestTryAnotherPreservesImportReview(t *testing.T) {
	ctx := context.Background()
	_, st, sid, edition, books, version := modelFixture(t)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: provider.ProtocolSource, ReleaseKey: "review-pack"}, []store.BookRef{modelRef(edition, books[0], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import"); err != nil {
		t.Fatal(err)
	}
	_, err = st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/review.cbz", NeedsReview: true, Facts: store.AssetFacts{ReleaseKey: "review-pack"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil).TryAnother(ctx, []int64{qid})
	if err != nil || len(targets) != 0 {
		t.Fatalf("recovery = %v, %v", targets, err)
	}
	if q, _ := st.GetQueueItem(ctx, qid); q.Status != store.QueueNeedsReview {
		t.Fatalf("review was retired: %+v", q)
	}
	pending, err := st.PendingBooks(ctx, sid)
	if err != nil || !pending[store.BookKey(version.ID, books[0].ID)] {
		t.Fatal("review lost its reservation")
	}
	blocked, _ := st.ListBlocklist(ctx)
	if len(blocked) != 0 {
		t.Fatal("review release was blocklisted")
	}
}

type countingDeadProvider struct {
	recoveryProvider
	attempts int
}

func (p *countingDeadProvider) State(context.Context, string) (provider.DownloadState, error) {
	p.attempts++
	return provider.DownloadState{}, &provider.DownloadError{Err: errors.New("HTTP error 404"), Dead: true}
}

func TestBrokenListingRetiresRemainingQueuedDownloads(t *testing.T) {
	ctx := context.Background()
	_, st, sid, edition, _, version := modelFixture(t)
	bid, err := st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: "1", SourceMangaKey: "broken", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	for _, num := range []int64{400, 500} {
		if _, err := st.SaveBook(ctx, sid, store.Book{EditionID: edition.ID, SequenceNum: num, SequenceStr: fmt.Sprint(num / 100), NumberKnown: true, CountType: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	books, err := st.ListBooks(ctx, edition.ID)
	if err != nil {
		t.Fatal(err)
	}

	p := &countingDeadProvider{}
	var ids []int64
	for i := range 5 {
		snap, _ := json.Marshal(map[string]any{"bindingId": bid})
		qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: provider.ProtocolSource, ProviderID: p.ID(), ReleaseKey: fmt.Sprintf("dead-%d", i), Snapshot: snap}, []store.BookRef{modelRef(edition, books[i], version)})
		if err != nil {
			t.Fatal(err)
		}
		if err = st.SetQueueExternalID(ctx, qid, fmt.Sprintf("remote-%d", i)); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, qid)
	}
	if _, err = NewImporter(st, provider.NewRegistry(p), t.TempDir(), nil).ProcessQueue(ctx); err != nil {
		t.Fatal(err)
	}
	if p.attempts != 3 {
		t.Fatalf("polled %d dead downloads, want 3", p.attempts)
	}
	for i, id := range ids {
		q, err := st.GetQueueItem(ctx, id)
		want := store.QueueFailed
		if i >= 3 {
			want = store.QueueCancelled
		}
		if err != nil || q.Status != want {
			t.Fatalf("job %d = %+v, %v", i, q, err)
		}
	}
	pending, err := st.PendingBooks(ctx, sid)
	if err != nil || len(pending) != 0 {
		t.Fatalf("dead listing kept reservations: %v %v", pending, err)
	}
}

func TestListingSuccessAndManualEnableResetFailures(t *testing.T) {
	ctx := context.Background()
	_, st, sid, _, _, _ := modelFixture(t)
	bid, err := st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: "1", SourceMangaKey: "sometimes-broken", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	fail := func(want bool) {
		t.Helper()
		off, err := st.ListingFailed(ctx, bid)
		if err != nil || off != want {
			t.Fatalf("listing disabled=%v, want %v: %v", off, want, err)
		}
	}
	fail(false)
	fail(false)
	if err = st.ListingWorked(ctx, bid); err != nil {
		t.Fatal(err)
	}
	fail(false)
	fail(false)
	fail(true)
	if err = st.SetBindingState(ctx, sid, bid, "active"); err != nil {
		t.Fatal(err)
	}
	fail(false)
	fail(false)
	fail(true)
}
