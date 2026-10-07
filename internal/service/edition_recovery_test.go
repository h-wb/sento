package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/h-wb/sento/internal/db/dbtest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func TestEditionDisappearanceSuspendsCollection(t *testing.T) {
	lib, st, id, c, _, version := modelFixture(t)
	ctx := context.Background()
	p := testProvider()
	if err := st.ReconcileMetadata(ctx, id, p.series, nil, nil); err != nil {
		t.Fatal(err)
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, sl := range slots[version.ID] {
		if sl.Wanted || sl.Monitored || sl.Reason == "" {
			t.Fatalf("stale parent remained collectible: %+v", sl)
		}
	}
	if err = st.EditCollection(ctx, id, c.ID, store.CollectionMetadataPatch{MakeLocal: true}); err != nil {
		t.Fatal(err)
	}
	slots, err = lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slots[version.ID][0].Wanted {
		t.Fatal("explicit local continuation did not restore intent")
	}
}

func TestContentsSurviveTargetDeletionAndPolicyChanges(t *testing.T) {
	lib, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	version.ID = 0
	version.Name = "Group A"
	version.RequiredGroup = "^A$"
	tid, err := st.SaveVersion(ctx, id, version)
	if err != nil {
		t.Fatal(err)
	}
	version.ID = tid
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	if err = st.SetAssetFacts(ctx, fid, store.AssetFacts{Language: "en", Group: "A", FactsKnown: true}, ""); err != nil {
		t.Fatal(err)
	}
	version.RequiredGroup = "^B$"
	if _, err = st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	sl := slots[tid][0]
	if sl.State != "unsatisfactory" || !sl.Wanted || len(sl.OnDiskFileIDs) != 1 || len(sl.FileIDs) != 0 {
		t.Fatalf("lost on-disk distinction: %+v", sl)
	}
	if err = st.DeleteVersion(ctx, id, tid); err != nil {
		t.Fatal(err)
	}
	contents, err := st.Contents(ctx, id)
	if err != nil || len(contents) != 1 {
		t.Fatalf("target deletion lost intrinsic contents: %v %v", contents, err)
	}
	version.ID = 0
	version.Name = "New A copy"
	version.RequiredGroup = "^A$"
	tid, err = st.SaveVersion(ctx, id, version)
	if err != nil {
		t.Fatal(err)
	}
	slots, err = lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if slots[tid][0].State != "owned" {
		t.Fatal("new policy did not reuse confirmed contents")
	}
	overview, err := lib.EditionOverview(ctx, id)
	if err != nil || overview.BooksOnDisk != 1 {
		t.Fatalf("copies inflated unique books: %+v %v", overview, err)
	}
}

func TestUnavailableRootBlocksRedownloadButMissingArchiveDoesNot(t *testing.T) {
	lib, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	ownedModelFile(t, st, id, c, ws[0], version)
	files, _ := st.ListFiles(ctx, id)
	root := filepath.Dir(files[0].Path)
	if err := st.SetSeriesPath(ctx, id, root); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-offline"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Rename(root+"-offline", root) })
	if err := st.CheckFilePresence(ctx, id); err != nil {
		t.Fatal(err)
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, sl := range slots[version.ID] {
		if sl.Wanted || sl.State != "storage_unavailable" {
			t.Fatalf("storage outage became missing: %+v", sl)
		}
	}
	if err = os.Rename(root+"-offline", root); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(files[0].Path); err != nil {
		t.Fatal(err)
	}
	if err = st.CheckFilePresence(ctx, id); err != nil {
		t.Fatal(err)
	}
	slots, err = lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slots[version.ID][0].Wanted {
		t.Fatal("a genuinely missing file was not wanted")
	}
}

func TestBulkMonitoringRollsBackForeignBook(t *testing.T) {
	lib, st, id, _, ws, version := modelFixture(t)
	ctx := context.Background()
	if err := st.BulkMonitor(ctx, id, version.ID, []int64{ws[0].ID, 999999}, false); err == nil {
		t.Fatal("foreign book accepted")
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slots[version.ID][0].Monitored {
		t.Fatal("partial bulk edit committed")
	}
	if err = st.BulkMonitor(ctx, id, version.ID, []int64{ws[0].ID, ws[1].ID}, false); err != nil {
		t.Fatal(err)
	}
	books, err := lib.VersionBooks(ctx, id, version.ID)
	paused := 0
	for _, b := range books {
		if b.State == "unmonitored" {
			paused++
		}
	}
	if err != nil || paused != 2 {
		t.Fatalf("paused books: %d of %d, %v", paused, len(books), err)
	}
	if _, err = lib.VersionBooks(ctx, id, version.ID+999); err == nil {
		t.Fatal("books of an unknown version should not be found")
	}
}

func TestIndependentEditionTraitsAndUnknownEvidence(t *testing.T) {
	lib, st, id, c, _, _ := modelFixture(t)
	_ = lib
	ctx := context.Background()
	traits := []string{"omnibus", "deluxe"}
	if err := st.EditCollection(ctx, id, c.ID, store.CollectionMetadataPatch{EditionTraits: &traits}); err != nil {
		t.Fatal(err)
	}
	c2, err := st.GetCollection(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c2.EditionTraits, []string{"deluxe", "omnibus"}) {
		t.Fatalf("combined traits lost: %v", c2.EditionTraits)
	}
	p := parse.ParseAnchored("ONE PIECE Deluxe Omnibus v01.cbz", parse.ChannelFile, []string{"ONE PIECE"})
	if !editionCompatible(p, *c2) {
		t.Fatalf("combined edition rejected: %+v", p.Edition)
	}
	if len(metadata.EditionTraits("Publisher edition")) != 0 || metadata.ContentVariant("Publisher edition") != "unknown" {
		t.Fatal("missing evidence became standard/original")
	}
}

// A provider that counts payload retrieval and can simulate a transient outage.
type recoveryProvider struct {
	payloadProvider
	fetches  int
	stateErr error
}

func (p *recoveryProvider) State(context.Context, string) (provider.DownloadState, error) {
	return provider.DownloadState{Done: true}, p.stateErr
}
func (p *recoveryProvider) Fetch(ctx context.Context, key string) (*provider.Payload, error) {
	p.fetches++
	return p.payloadProvider.Fetch(ctx, key)
}

func TestImportReviewRetainsClaimsAndCompletesExistingPayload(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "unknown.cbz"), []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	p := &recoveryProvider{payloadProvider: payloadProvider{dir: dir, files: []string{"unknown.cbz"}}}
	reg := provider.NewRegistry(p)
	plan := []store.BookRef{modelRef(c, ws[0], version)}
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: id, ProviderID: p.ID(), Protocol: p.Protocol(), ReleaseKey: "review-offer", Title: "ONE PIECE v01"}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetQueueExternalID(ctx, qid, "remote"); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, reg, t.TempDir(), nil)
	if n, err := im.ProcessQueue(ctx); err != nil || n != 0 {
		t.Fatalf("unexpected import: %d %v", n, err)
	}
	queue, _ := st.ListQueueItems(ctx, true)
	if len(queue) != 1 || queue[0].Status != store.QueueNeedsReview {
		t.Fatalf("review became terminal: %+v", queue)
	}
	pending, err := st.PendingBooks(ctx, id)
	if err != nil || !pending[store.BookKey(version.ID, ws[0].ID)] {
		t.Fatal("review released its reservation")
	}
	files, err := st.ListFiles(ctx, id)
	if err != nil || len(files) != 1 {
		t.Fatalf("unmatched payload not retained: %v", err)
	}
	if err = st.MapFile(ctx, id, files[0].ID, plan); err != nil {
		t.Fatal(err)
	}
	if err = st.ResumeQueue(ctx, qid, ""); err != nil {
		t.Fatal(err)
	}
	if n, err := im.ProcessQueue(ctx); err != nil || n != 1 {
		t.Fatalf("reviewed download did not complete: %d %v", n, err)
	}
	if p.fetches != 1 {
		t.Fatalf("review downloaded the payload again: %d", p.fetches)
	}
}

func TestTransientProviderErrorBacksOffAndRetainsClaims(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	p := &recoveryProvider{stateErr: errors.New("client unavailable")}
	reg := provider.NewRegistry(p)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: id, ProviderID: p.ID(), Protocol: p.Protocol(), ReleaseKey: "transient"}, []store.BookRef{modelRef(c, ws[0], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetQueueExternalID(ctx, qid, "remote"); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, reg, t.TempDir(), nil)
	if _, err = im.ProcessQueue(ctx); err != nil {
		t.Fatal(err)
	}
	queue, _ := st.ListQueueItems(ctx, true)
	if len(queue) != 1 || queue[0].Status != store.QueueRetryWait || queue[0].RetryCount != 1 || queue[0].RetryAt == "" {
		t.Fatalf("missing backoff: %+v", queue)
	}
	if _, err = im.ProcessQueue(ctx); err != nil {
		t.Fatal(err)
	}
	queue, _ = st.ListQueueItems(ctx, true)
	if queue[0].RetryCount != 1 {
		t.Fatal("retry ignored backoff")
	}
	if err = st.DeleteBook(ctx, id, ws[0].ID); err == nil {
		t.Fatal("pending plan allowed book deletion")
	}
	if err = st.CancelQueue(ctx, qid); err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "late worker"); err == nil {
		t.Fatal("worker resurrected cancelled job")
	}
	pending, _ := st.PendingBooks(ctx, id)
	if pending[store.BookKey(version.ID, ws[0].ID)] {
		t.Fatal("cancel left a reservation")
	}
}

func TestFileFactsAreSpecificAndManualCorrectionsSurviveRetry(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	snap, _ := json.Marshal(ReleaseCandidate{Title: "ONE PIECE v01 [Advertised]", Parse: parse.Result{Group: "Advertised", Language: "fr"}})
	path := filepath.Join(t.TempDir(), "ONE PIECE v01 [Actual].cbz")
	if err := os.WriteFile(path, []byte("bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	item := store.QueueItem{SeriesID: id, Protocol: "torrent", ReleaseKey: "facts", Snapshot: snap}
	plan := []store.BookRef{modelRef(c, ws[0], version)}
	if err := im.recordAsset(ctx, item, path, plan, parse.Result{Group: "Actual", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	fs, _ := st.ListFiles(ctx, id)
	f := fs[0]
	if f.Facts.Group != "Actual" || f.Claims.Group != "Advertised" || f.Facts.Sources["group"] != "filename" {
		t.Fatalf("file inherited offer: %+v", f)
	}
	group := "Corrected"
	if err := st.CorrectFileFacts(ctx, id, f.ID, store.AssetFactPatch{Group: &group}); err != nil {
		t.Fatal(err)
	}
	if err := st.MapFile(ctx, id, f.ID, plan); err != nil {
		t.Fatal(err)
	}
	if err := im.recordAsset(ctx, item, path, plan, parse.Result{Group: "Actual"}); err != nil {
		t.Fatal(err)
	}
	fs, _ = st.ListFiles(ctx, id)
	if fs[0].Facts.Group != "Corrected" || fs[0].Facts.Sources["group"] != "manual" {
		t.Fatal("retry erased user correction")
	}
}

func TestChapterExtrasZeroAndReadOnlyLedger(t *testing.T) {
	lib, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	c, err := lib.CreateCollection(ctx, id, CreateCollectionParams{EditionName: "Local chapters", Language: "en", Type: "chapter", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	versions, _ := st.ListVersions(ctx, id)
	var version store.CollectedVersion
	for _, v := range versions {
		if v.CollectionID == c.ID {
			version = v
		}
	}
	var zero, bonus int64
	for _, v := range []struct {
		number      int64
		label, kind string
	}{{0, "0", "main"}, {1250, "12.5", "extra"}} {
		wid, e := st.SaveBook(ctx, id, store.Book{CollectionID: c.ID, SequenceNum: v.number, SequenceStr: v.label, CountType: v.kind, NumberKnown: true})
		if e != nil {
			t.Fatal(e)
		}
		if v.number == 0 {
			zero = wid
		} else {
			bonus = wid
		}
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slots[version.ID] {
		if s.Monitored {
			t.Fatal("cutoff zero or excluded bonus was collected by default")
		}
	}
	version.IncludeExtras = true
	if _, err = st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	if err = st.BulkMonitor(ctx, id, version.ID, []int64{zero}, true); err != nil {
		t.Fatal(err)
	}
	slots, err = lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range slots[version.ID] {
		if (s.Book.ID == zero || s.Book.ID == bonus) && !s.Monitored {
			t.Fatal("explicit zero/extra collection was ignored")
		}
	}

}

func TestTrustedChaptersRequireVerifiedBindingAndDoNotFillGaps(t *testing.T) {
	lib, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	c, err := lib.CreateCollection(ctx, id, CreateCollectionParams{EditionName: "Reader chapters", Language: "en", Type: "chapter", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if err = st.EditCollection(ctx, id, c.ID, store.CollectionMetadataPatch{DiscoverChapters: &yes}); err != nil {
		t.Fatal(err)
	}
	bid, err := st.AddBinding(ctx, store.Binding{SeriesID: id, SourceID: "reader", SourceMangaKey: "manga", State: "active", CollectionID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetBindingTrust(ctx, id, bid, true); err != nil {
		t.Fatal(err)
	}
	c, _ = st.GetCollection(ctx, c.ID)
	run := &searchRun{seriesID: id, ledger: LedgerChapter, collections: []store.Collection{*c}, sourceEditions: map[int64]int64{bid: c.ID}, books: map[int64][]store.Book{}, titles: []string{"ONE PIECE"}}
	svc := &SearchService{store: st}
	svc.observeCandidates(ctx, run, []provider.Candidate{{Title: "ONE PIECE ch12.5", Protocol: provider.ProtocolSource, Source: &provider.SourceRef{BindingID: bid, SourceID: "reader", ChapterKey: "twelve-half", ChapterNumber: 12.5, Lang: "en"}}})
	works, err := st.ListBooks(ctx, c.ID)
	if err != nil || len(works) != 1 || works[0].SequenceNum != 1250 {
		t.Fatalf("trusted chapter missing or gap filled: %+v %v", works, err)
	}
	if err = st.SetBindingEdition(ctx, id, bid, 0); err != nil {
		t.Fatal(err)
	}
	bindings, _ := st.ListBindings(ctx, id)
	for _, b := range bindings {
		if b.ID == bid && b.TrustChapters {
			t.Fatal("changing edition preserved trust")
		}
	}
}

func TestConcurrentBookDeletionAndReservationDoNotLeaveDanglingPlan(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	start := make(chan struct{})
	done := make(chan error, 2)
	go func() { <-start; done <- st.DeleteBook(ctx, id, ws[0].ID) }()
	go func() {
		<-start
		_, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: id, ProviderID: "race", Protocol: "torrent", ReleaseKey: "race"}, []store.BookRef{modelRef(c, ws[0], version)})
		done <- err
	}()
	close(start)
	a, b := <-done, <-done
	if a == nil && b == nil {
		t.Fatal("deletion and reservation both committed")
	}
	pending, err := st.PendingBooks(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if pending[store.BookKey(version.ID, ws[0].ID)] {
		w, e := st.GetBook(ctx, ws[0].ID)
		if e != nil || w.SyncState == "retired" {
			t.Fatalf("reservation refers to deleted/retired work: %+v %v", w, e)
		}
	}
}

func TestLedgerReadsDoNotInferNewChapters(t *testing.T) {
	ctx := context.Background()
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM works`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.ExecContext(ctx, fmt.Sprintf(`UPDATE series SET total_chapters=1300 WHERE id=%d`, id)); err != nil {
		t.Fatal(err)
	}
	if _, err = st.ListVersions(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.BooksByVersion(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err = lib.EditionOverview(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM works`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("GET inferred books: before=%d after=%d", before, after)
	}
	if err = st.EnsureModels(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM works`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after <= before {
		t.Fatal("explicit initialization did not infer updated chapter count")
	}
}
