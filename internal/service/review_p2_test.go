package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// A release is banned under the name the indexer lists it by as well as
// the torrent hash learned while grabbing, so the same listing does not
// come back on the next search.
func TestBlocklistCoversTheListingAndItsHash(t *testing.T) {
	t.Parallel()
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	const guid = "https://indexer/release/123"
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ProviderID: "torrent", ReleaseKey: guid, Title: "ONE PIECE v01"}, []store.BookRef{modelRef(c, ws[0], v)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetQueueExternalID(ctx, qid, "derived-infohash"); err != nil {
		t.Fatal(err)
	}
	if err = NewImporter(st, nil, t.TempDir(), nil).BlocklistQueueItem(ctx, qid); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, torrentRegistry(st))
	run, err := svc.prepare(ctx, sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []provider.TorrentRef{
		{GUID: guid, DownloadURL: "https://indexer/download/123", Size: 100 << 20, Seeders: 30},
		{GUID: "elsewhere", InfoHash: "derived-infohash", Size: 100 << 20, Seeders: 30},
	} {
		cs, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE v01 (Digital)", Torrent: &ref}})
		if len(cs) == 0 {
			t.Fatalf("%s: not evaluated", ref.GUID)
		}
		for _, rc := range cs {
			if !slices.Contains(rc.Rejections, "rejected.blocklisted") {
				t.Errorf("%s: blocked release offered again: %v", ref.GUID, rc.Rejections)
			}
		}
	}
	// Unblocking a feed result clears the download identity too. Re-run
	// evaluation against fresh state rather than overriding a rejection.
	if err := st.UnblockReleases(ctx, nil, "torrent", []string{guid}); err != nil {
		t.Fatal(err)
	}
	run, err = svc.prepare(ctx, sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE v01 (Digital)",
		Torrent: &provider.TorrentRef{GUID: "elsewhere", InfoHash: "derived-infohash", Size: 100 << 20, Seeders: 30}}})
	if len(cs) == 0 {
		t.Fatal("unblocked release not evaluated")
	}
	for _, rc := range cs {
		if slices.Contains(rc.Rejections, "rejected.blocklisted") || rc.ExternalKey != "derived-infohash" {
			t.Errorf("unblocked candidate = %+v", rc)
		}
	}
	job, err := st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueFailed {
		t.Fatalf("unblocking changed failed attempt: %v, %v", job, err)
	}
}

// The worker's list holds every job it can advance, oldest first; jobs
// waiting on a person never crowd one out.
func TestQueueWorkerSeesOldJobsBehindReviewJobs(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	old, err := st.AddQueueItem(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ExternalID: "runnable", Status: store.QueueQueued, Books: []byte(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 205 {
		status := store.QueueNeedsReview
		if i%2 == 0 {
			status = store.QueueFailed
		}
		if _, err := st.AddQueueItem(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ExternalID: fmt.Sprint(i), Status: status, Books: []byte(`[]`)}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := st.ListRunnableQueue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != old {
		t.Fatalf("worker list = %d jobs, want only the runnable one", len(items))
	}
	// Activity must keep failed and review jobs visible beyond 200 items.
	active, err := st.ListQueueItems(ctx, true)
	if err != nil || len(active) != 206 {
		t.Fatalf("activity = %d jobs, %v", len(active), err)
	}
	for _, job := range active {
		if job.SeriesTitle == "" {
			t.Fatal("activity is missing its series title")
		}
	}
	for range 205 {
		if _, err := st.AddQueueItem(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", Status: store.QueueImported, Books: []byte(`[]`)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := st.ListQueueItems(ctx, false)
	if err != nil || len(all) != 406 {
		t.Fatalf("all activity = %d jobs, %v", len(all), err)
	}
	if got, err := st.GetQueueItem(ctx, old); err != nil || got.ExternalID != "runnable" {
		t.Fatalf("job by id: %+v, %v", got, err)
	}
}

// With chapters in their own folder, that folder being unreachable does
// not stop volumes from being wanted.
func TestUnreachableChapterFolderLeavesVolumesWanted(t *testing.T) {
	t.Parallel()
	_, st, sid, _, ws, v := modelFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	volumes := filepath.Join(root, "volumes")
	if err := os.MkdirAll(volumes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSeriesPath(ctx, sid, volumes); err != nil {
		t.Fatal(err)
	}
	chapters := filepath.Join(root, "offline-chapters")
	if err := st.SetSeriesChapterPath(ctx, sid, chapters); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: filepath.Join(chapters, "chapter.cbz")}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := st.CheckFilePresence(ctx, sid); err != nil {
		t.Fatal(err)
	}
	view, err := loadLedger(ctx, st, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, sl := range view.Status[v.ID] {
		if sl.Book.ID == ws[0].ID && !sl.Wanted {
			t.Fatalf("volume blocked by the chapter folder: %s / %s", sl.State, sl.Reason)
		}
	}
}

// A plain .zip is a comic archive like a .cbz and is imported, not skipped.
func TestZipPayloadIsImported(t *testing.T) {
	t.Parallel()
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	const name = "ONE PIECE v01 (Digital).zip"
	writeCBZ(t, filepath.Join(dir, name), "pages")
	raw, _ := json.Marshal([]store.BookRef{modelRef(c, ws[0], v)})
	err := NewImporter(st, nil, t.TempDir(), nil).importFiles(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", Books: raw}, dir, []string{name})
	if err != nil {
		t.Fatalf("zip payload was not imported: %v", err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 || files[0].NeedsReview {
		t.Fatalf("imported files: %+v, %v", files, err)
	}
}

// Replacing an archive at a path Sento already knows drops what was
// recorded about the old bytes; the file is matched again as new.
func TestScanNoticesAReplacedArchive(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "ONE PIECE v01 (Digital).cbz")
	writeCBZ(t, path, "original pages")
	sc := NewScanner(st, dir)
	if _, err := sc.ScanSeriesFolder(ctx, sid, dir); err != nil {
		t.Fatal(err)
	}
	before, err := st.ListFiles(ctx, sid)
	if err != nil || len(before) != 1 {
		t.Fatalf("setup: %+v, %v", before, err)
	}
	again, err := sc.ScanSeriesFolder(ctx, sid, dir)
	if err != nil || again.Changed != 0 || len(again.Skipped) != 1 {
		t.Fatalf("an untouched file should be skipped: %+v, %v", again, err)
	}

	writeCBZ(t, path, "different pages, and more of them")
	res, err := sc.ScanSeriesFolder(ctx, sid, dir)
	if err != nil {
		t.Fatal(err)
	}
	after, err := st.ListFiles(ctx, sid)
	if err != nil || len(after) != 1 {
		t.Fatalf("after: %+v, %v", after, err)
	}
	current, _ := fileHash(path)
	if res.Changed != 1 || after[0].Hash != current || after[0].Hash == before[0].Hash {
		t.Fatalf("replaced archive kept its old record: changed=%d hash=%s", res.Changed, after[0].Hash)
	}
}

// A copy that only an additional version claims is filed under that
// version, the way import placed it.
func TestOrganizeKeepsAnAdditionalVersionsCopyInItsFolder(t *testing.T) {
	t.Parallel()
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	other := v
	other.ID, other.Name = 0, "Scanlation copy"
	var err error
	if other.ID, err = st.SaveVersion(ctx, sid, other); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "ONE PIECE v01.cbz")
	writeCBZ(t, path, "pages")
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path}, nil, []store.BookLink{{EditionID: c.ID, BookID: ws[0].ID, VersionID: other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, nil, dir, nil)
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(c, ws[0], other)}); err != nil {
		t.Fatal(err)
	}
	plans, err := im.PlanOrganize(ctx, sid)
	if err != nil || len(plans) != 1 || !strings.Contains(plans[0].To, "Scanlation copy") {
		t.Fatalf("additional-only copy: %+v, %v", plans, err)
	}
	// Claimed by both: the default version is its home.
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(c, ws[0], v), modelRef(c, ws[0], other)}); err != nil {
		t.Fatal(err)
	}
	plans, err = im.PlanOrganize(ctx, sid)
	if err != nil || len(plans) != 1 || strings.Contains(plans[0].To, "Scanlation copy") {
		t.Fatalf("shared copy should sit under the default version: %+v, %v", plans, err)
	}
}
