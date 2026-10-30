package service

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// A file waiting for review is proposed the books its own name gives,
// read against every edition; a number with no volume or chapter marker
// takes its kind from the download it came in, and stays unread outside
// one.
func TestReviewProposesWhatTheFileNameSays(t *testing.T) {
	lib, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	titles := []string{"ONE PIECE"}
	job := func(key, release string, book store.Book) {
		t.Helper()
		snap, _ := json.Marshal(ReleaseCandidate{Parse: parse.ParseAnchored(release, parse.ChannelTorrent, titles)})
		if _, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: key, Title: release, Snapshot: snap}, []store.BookRef{modelRef(standard, book, version)}); err != nil {
			t.Fatal(err)
		}
	}
	job("mixed", "ONE PIECE v01-02,005-010 (Digital)", books[0])
	job("bare", "ONE PIECE [English]", books[1])
	for _, tc := range []struct {
		name, key, label, edition string
	}{
		{"ONE PIECE v02 (Digital).cbz", "", "volume 2", "Standard Edition · en"},
		{"ONE PIECE 007 (Digital).cbz", "mixed", "chapter 7", "Chapters · en"},
		{"ONE PIECE 002 (Digital).cbz", "mixed", "volume 2", "Standard Edition · en"},
		{"Volume_01.cbz", "mixed", "volume 1", "Standard Edition · en"},
		{"3.cbz", "bare", "volume 3", "Standard Edition · en"},
		{"ONE PIECE 001.cbz", "", "", ""}, // volume or chapter: no download to say
		{"3.cbz", "", "", ""},             // nothing ties it to the series
		{"ONE PIECE 050 (Digital).cbz", "mixed", "", ""},
	} {
		fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/" + tc.name, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: tc.name, ReleaseKey: tc.key}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		items, err := lib.ReviewItems(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		var got *store.ReviewProposal
		for _, i := range items {
			if i.Kind == "file" && i.ItemID == fid {
				got = i.Proposal
			}
		}
		switch {
		case tc.label == "" && got != nil:
			t.Errorf("%s (%s): proposed %+v, want nothing", tc.name, tc.key, *got)
		case tc.label != "" && got == nil:
			t.Errorf("%s (%s): no proposal, want %s", tc.name, tc.key, tc.label)
		case got != nil && (got.Label != tc.label || got.Edition != tc.edition || len(got.Books) != 1):
			t.Errorf("%s (%s): proposed %s of %s (%d books), want %s of %s", tc.name, tc.key, got.Label, got.Edition, len(got.Books), tc.label, tc.edition)
		}
	}
}

// A download held for review closes on the last confirmation of its
// files, says what was confirmed, and wants again the books no file held.
func TestReviewedDownloadClosesItself(t *testing.T) {
	_, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	im := NewImporter(st, nil, t.TempDir(), nil)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "pack", Title: "ONE PIECE pack"}, []store.BookRef{modelRef(standard, books[0], version), modelRef(standard, books[1], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import: verified 0 of 2 expected books; unmatched files need review"); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/1.cbz", NeedsReview: true, Unrequested: true, Facts: store.AssetFacts{Title: "1.cbz", ReleaseKey: "pack"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	status := func() string {
		t.Helper()
		job, err := st.GetQueueItem(ctx, qid)
		if err != nil {
			t.Fatal(err)
		}
		return job.Status
	}
	if err = im.SettleReviewedJobs(ctx, sid); err != nil || status() != store.QueueNeedsReview {
		t.Fatalf("closed with a file still waiting: %s %v", status(), err)
	}
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(standard, books[0], version)}); err != nil {
		t.Fatal(err)
	}
	if err = im.SettleReviewedJobs(ctx, sid); err != nil || status() != store.QueueImported {
		t.Fatalf("not closed after the last confirmation: %s %v", status(), err)
	}
	job, _ := st.GetQueueItem(ctx, qid)
	if job.Error != "" {
		t.Errorf("stale error kept: %q", job.Error)
	}
	history, err := st.ListDecisions(ctx, sid, 10)
	if err != nil || len(history) == 0 {
		t.Fatalf("history: %+v %v", history, err)
	}
	var last store.DecisionBody
	if err = json.Unmarshal(history[0].Body, &last); err != nil || last.Event != "imported_after_review" {
		t.Fatalf("last event: %+v %v", last, err)
	}
	want := "1 file confirmed in review. Not in this download, wanted again: Standard Edition volume " + books[1].SequenceStr + "."
	if last.Note != want {
		t.Errorf("note = %q, want %q", last.Note, want)
	}
	// Its books are free for another download.
	if _, _, err = st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "next", Title: "ONE PIECE v02"}, []store.BookRef{modelRef(standard, books[1], version)}); err != nil {
		t.Errorf("volume still reserved: %v", err)
	}
}

// Keeping a file without claiming a book is an answer: it leaves review,
// and a download waiting on it closes (an extra chapter like 92.1 that the
// catalog has no book for).
func TestKeepingAnExtraResolvesReview(t *testing.T) {
	lib, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "pack", Title: "ONE PIECE pack"}, []store.BookRef{modelRef(standard, books[0], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import"); err != nil {
		t.Fatal(err)
	}
	extra := t.TempDir() + "/ONE PIECE 092.1.cbz"
	if err = os.WriteFile(extra, []byte("pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: extra, NeedsReview: true, Unrequested: true, Facts: store.AssetFacts{Title: "ONE PIECE 092.1.cbz", ReleaseKey: "pack"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.MapFile(ctx, sid, fid, nil); err != nil {
		t.Fatal(err)
	}
	items, err := lib.ReviewItems(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range items {
		if i.Kind == "file" && i.ItemID == fid {
			t.Fatalf("a file kept as an extra is still waiting: %+v", i)
		}
	}
	if err = NewImporter(st, nil, t.TempDir(), nil).SettleReviewedJobs(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if job, _ := st.GetQueueItem(ctx, qid); job.Status != store.QueueImported {
		t.Fatalf("download still held: %s", job.Status)
	}
}

// A source that found nothing for a series is a question for a person —
// until it finds a match or is turned off for that series.
func TestUnmatchedSourceIsAReviewQuestion(t *testing.T) {
	lib, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	ix, err := st.AddIndexer(ctx, store.Indexer{Name: "Weeb Central (en)", Protocol: store.IndexerSource, Enabled: true, UseVolumes: true, UseChapters: true, Settings: []byte(`{"sourceId":"42"}`)})
	if err != nil {
		t.Fatal(err)
	}
	src := strconv.FormatInt(ix, 10)
	if _, err = st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: src, SourceMangaKey: "wrong", SourceName: "Weeb Central (en)", State: "unmatched"}); err != nil {
		t.Fatal(err)
	}
	asked := func() bool {
		t.Helper()
		items, err := lib.ReviewItems(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		for _, i := range items {
			if i.Kind == "source" && i.ItemID == ix {
				return true
			}
		}
		return false
	}
	if !asked() {
		t.Fatal("an unmatched source is not in review")
	}
	off := false
	if err = st.UpsertSiteOverride(ctx, store.SiteOverride{SeriesID: sid, IndexerID: ix, Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if asked() {
		t.Fatal("a source turned off for the series is still asked about")
	}
	on := true
	if err = st.UpsertSiteOverride(ctx, store.SiteOverride{SeriesID: sid, IndexerID: ix, Enabled: &on}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: src, SourceMangaKey: "right", SourceName: "Weeb Central (en)", State: "active"}); err != nil {
		t.Fatal(err)
	}
	if asked() {
		t.Fatal("a source with a match is still asked about")
	}
}
