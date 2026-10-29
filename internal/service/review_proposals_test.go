package service

import (
	"context"
	"encoding/json"
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
