package service

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// A file waiting for review is proposed the books its own name gives,
// read against every edition; a number with no volume or chapter marker
// takes its kind from the download it came in, and stays unread outside
// one.
func TestReviewProposesWhatTheFileNameSays(t *testing.T) {
	t.Parallel()
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

func addReviewFile(t *testing.T, st *store.Store, sid int64, name, key string) int64 {
	t.Helper()
	fid, err := st.AddFile(context.Background(), store.File{SeriesID: sid, Path: t.TempDir() + "/" + name, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name, ReleaseKey: key}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

func proposalOf(t *testing.T, lib *Library, sid, fid int64) *store.ReviewProposal {
	t.Helper()
	items, err := lib.ReviewItems(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range items {
		if i.Kind == "file" && i.ItemID == fid {
			return i.Proposal
		}
	}
	return nil
}

// packJob reserves a download of one unnumbered completed pack, planned
// against the given books.
func packJob(t *testing.T, st *store.Store, sid int64, key string, refs []store.BookRef) {
	t.Helper()
	ctx := context.Background()
	release := "ONE PIECE (Scanlated) (Void) | Completed"
	snap, _ := json.Marshal(ReleaseCandidate{Parse: parse.ParseAnchored(release, parse.ChannelTorrent, []string{"ONE PIECE"})})
	if _, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: key, Title: release, Snapshot: snap}, refs); err != nil {
		t.Fatal(err)
	}
}

// An unnumbered completed pack says nothing about its numbering, so the
// plan's kind is only trusted while the pack's own numbers are books of
// it. A pack numbered past the planned edition (200 chapter files against
// a 15-volume edition) is another numbering, and none of its files is
// proposed onto the collected volumes — not even the ones whose number
// happens to be a volume.
func TestUnnumberedPackProposesOnlyWhatItsNumbersFit(t *testing.T) {
	t.Parallel()
	lib, st, sid, standard, books, version := modelFixture(t)
	packJob(t, st, sid, "beyond", []store.BookRef{modelRef(standard, books[0], version), modelRef(standard, books[1], version), modelRef(standard, books[2], version)})
	var files []int64
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		files = append(files, addReviewFile(t, st, sid, "ONE PIECE "+n+" (Scanlated) (Void).cbz", "beyond"))
	}
	for _, fid := range files {
		if got := proposalOf(t, lib, sid, fid); got != nil {
			t.Errorf("file %d: proposed %s of %s, want nothing", fid, got.Label, got.Edition)
		}
	}
}

// A number zero-padded to three or more digits ("001") is written the
// chapter way: a download of volumes cannot read it as a volume number,
// even when the pack's numbers are all books of the volume edition.
func TestZeroPaddedPackNumbersAreNotVolumeNumbers(t *testing.T) {
	t.Parallel()
	lib, st, sid, standard, books, version := modelFixture(t)
	packJob(t, st, sid, "padded", []store.BookRef{modelRef(standard, books[0], version), modelRef(standard, books[1], version), modelRef(standard, books[2], version)})
	var files []int64
	for _, n := range []string{"001", "002", "003"} {
		files = append(files, addReviewFile(t, st, sid, "ONE PIECE "+n+" (Scanlated) (Void).cbz", "padded"))
	}
	for _, fid := range files {
		if got := proposalOf(t, lib, sid, fid); got != nil {
			t.Errorf("file %d: proposed %s of %s, want nothing", fid, got.Label, got.Edition)
		}
	}
}

// A file is proposed only when every number in its name resolves to a
// book of the edition: "v01, v99" against a three-volume edition is not
// volume 1 alone.
func TestPartialNumbersProposeNothing(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := modelFixture(t)
	partial := addReviewFile(t, st, sid, "ONE PIECE v01, v99 (Digital).cbz", "")
	if got := proposalOf(t, lib, sid, partial); got != nil {
		t.Errorf("partially resolving name: proposed %s of %s, want nothing", got.Label, got.Edition)
	}
	whole := addReviewFile(t, st, sid, "ONE PIECE v01, v02 (Digital).cbz", "")
	got := proposalOf(t, lib, sid, whole)
	if got == nil || got.Label != "volumes 1–2" {
		t.Errorf("fully resolving name: proposed %+v, want volumes 1–2", got)
	}
}

// A side story's number never becomes a book number: inside a download
// of chapters, the plainly numbered file is proposed and the side-story
// one is left to a person.
func TestSideStoryNumbersAreNotBookNumbers(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	cols, err := st.ListEditions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	chapters := collectionByName(t, cols, "Chapters", "en")
	books, err := st.ListBooks(ctx, chapters.ID)
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[int64]store.Book{}
	for _, b := range books {
		byNumber[b.SequenceNum] = b
	}
	versions, err := st.ListVersions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	var version store.CollectedVersion
	for _, v := range versions {
		if v.EditionID == chapters.ID && v.IsDefault {
			version = v
		}
	}
	var refs []store.BookRef
	for _, n := range []int64{700, 800} {
		refs = append(refs, modelRef(chapters, byNumber[n], version))
	}
	packJob(t, st, sid, "side", refs)
	plain := addReviewFile(t, st, sid, "ONE PIECE 7 (Digital).cbz", "side")
	story := addReviewFile(t, st, sid, "ONE PIECE 7 - Side Story 08 (Digital).cbz", "side")
	if got := proposalOf(t, lib, sid, plain); got == nil || got.Label != "chapter 7" {
		t.Errorf("plainly numbered file: proposed %+v, want chapter 7", got)
	}
	if got := proposalOf(t, lib, sid, story); got != nil {
		t.Errorf("side story file: proposed %s of %s, want nothing", got.Label, got.Edition)
	}
}

// A download held for review closes on the last confirmation of its
// files, says what was confirmed, and wants again the books no file held.
func TestReviewedDownloadClosesItself(t *testing.T) {
	t.Parallel()
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
	path := t.TempDir() + "/1.cbz"
	if err = os.WriteFile(path, []byte("verified pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Facts: store.AssetFacts{Title: "1.cbz", ReleaseKey: "pack"}}, nil, nil)
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
	if searches := im.TakeSearches(); len(searches) != 1 || searches[0].SeriesID != sid || searches[0].Ledger != LedgerVolume {
		t.Fatalf("unmatched books did not schedule recovery: %+v", searches)
	}
	if err = im.SettleReviewedJobs(ctx, sid); err != nil {
		t.Fatal(err)
	}
	if searches := im.TakeSearches(); len(searches) != 0 {
		t.Fatalf("repeated recovery: %+v", searches)
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
	t.Parallel()
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
	t.Parallel()
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

// Adding a Colored collection must not make review propose its colored files
// for a black-and-white card. Overlapping policies need a choice, not a
// silent fallback to the default card.
func TestReviewProposesOnlyProfileCompatibleVersion(t *testing.T) {
	t.Parallel()
	lib, st, sid, edition, _, version := modelFixture(t)
	ctx := context.Background()
	fidFormat, err := st.AddFormat(ctx, profile.Format{Name: "Colored", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "colored"}}})
	if err != nil {
		t.Fatal(err)
	}
	standard := profile.Profile{Name: "Standard", Refused: []int64{fidFormat}, MinFormatScore: profile.MinScoreOff}
	standard.ID, err = st.AddProfile(ctx, standard)
	if err != nil {
		t.Fatal(err)
	}
	version.ProfileID = standard.ID
	if _, err = lib.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	colored := profile.Profile{Name: "Colored", Required: []int64{fidFormat}, MinFormatScore: profile.MinScoreOff}
	colored.ID, err = st.AddProfile(ctx, colored)
	if err != nil {
		t.Fatal(err)
	}
	colorID, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: edition.ID, Name: "Colored", ProfileID: colored.ID, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string, isColor bool) int64 {
		t.Helper()
		id, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/" + name, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name, FactsKnown: true, Flags: map[string]bool{"colored": isColor}}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	colorFile := add("ONE PIECE v01 (Colored).cbz", true)
	plainFile := add("ONE PIECE v02.cbz", false)
	check := func(fileID, wantVersion int64, wantLabel string) {
		t.Helper()
		items, err := lib.ReviewItems(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			if item.ItemID != fileID || item.Kind != "file" {
				continue
			}
			if wantVersion == 0 {
				if item.Proposal != nil {
					t.Fatalf("ambiguous file proposed: %+v", item.Proposal)
				}
				return
			}
			if item.Proposal == nil || len(item.Proposal.Books) != 1 || item.Proposal.Books[0].VersionID != wantVersion || item.Proposal.Edition != wantLabel {
				t.Fatalf("wrong proposal: %+v", item.Proposal)
			}
			return
		}
		t.Fatal("file missing from review")
	}
	check(colorFile, colorID, "Standard Edition · Colored · en")
	check(plainFile, version.ID, "Standard Edition · Standard · en")
	standard.Refused = nil
	if err = st.UpdateProfile(ctx, standard); err != nil {
		t.Fatal(err)
	}
	check(colorFile, 0, "") // Both profiles accept it; ask instead of picking default.
	standard.Refused = []int64{fidFormat}
	if err = st.UpdateProfile(ctx, standard); err != nil {
		t.Fatal(err)
	}
	colorVersion, err := st.GetVersion(ctx, colorID)
	if err != nil {
		t.Fatal(err)
	}
	colorVersion.Monitored = false
	if _, err = lib.SaveVersion(ctx, sid, *colorVersion); err != nil {
		t.Fatal(err)
	}
	check(colorFile, 0, "") // Never propose a file that the only collected card refuses.
}

// A file whose name parses but fits no edition gets no proposal — yet
// the hint still carries the ledger and numbers, so review can prefill
// them and map them once an edition is chosen.
func TestReviewHintSurvivesWithoutProposal(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name, ledger string
		numbers      []string
	}{
		{"ONE PIECE v02 (Digital).cbz", "volume", []string{"2"}},  // also proposed
		{"ONE PIECE v99 (Digital).cbz", "volume", []string{"99"}}, // no such book: no proposal
		{"ONE PIECE v03-04 (Digital).cbz", "volume", []string{"3–4"}},
		{"ONE PIECE c07 (Digital).cbz", "chapter", []string{"7"}},
	} {
		fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: t.TempDir() + "/" + tc.name, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: tc.name}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		items, err := lib.ReviewItems(ctx, sid)
		if err != nil {
			t.Fatal(err)
		}
		var hint *store.FileHint
		var proposed bool
		for _, i := range items {
			if i.Kind == "file" && i.ItemID == fid {
				hint = i.Hint
				proposed = i.Proposal != nil
			}
		}
		if hint == nil {
			t.Errorf("%s: no hint, want ledger %q numbers %v", tc.name, tc.ledger, tc.numbers)
			continue
		}
		if hint.Ledger != tc.ledger || strings.Join(hint.Numbers, ",") != strings.Join(tc.numbers, ",") {
			t.Errorf("%s: hint %+v, want ledger %q numbers %v", tc.name, *hint, tc.ledger, tc.numbers)
		}
		if tc.name == "ONE PIECE v99 (Digital).cbz" && proposed {
			t.Errorf("%s: proposed despite no matching book", tc.name)
		}
		if tc.name == "ONE PIECE v02 (Digital).cbz" && !proposed {
			t.Errorf("%s: no proposal although the book exists", tc.name)
		}
	}
}
