package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// waitingFile writes a real archive into the library root and records it
// as a file waiting for review, the way an import leaves a pack's files.
func waitingFile(t *testing.T, st *store.Store, sid int64, dir, name, key string) int64 {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	page, err := zw.Create("001.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = page.Write([]byte("pages " + name)); err != nil {
		t.Fatal(err)
	}
	if err = zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(context.Background(), store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name, ReleaseKey: key}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

// A pack's file reviews resolve at once: every file is marked reviewed and
// unmapped in one transaction, the download it came in settles, and the
// files turn up in cleanup as trashed-able unmapped files. A file that no
// longer waits is skipped with its reason, never overwritten.
func TestBulkDeclineResolvesAPackAtOnce(t *testing.T) {
	t.Parallel()
	_, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "pack", Title: "ONE PIECE pack"}, []store.BookRef{modelRef(standard, books[0], version), modelRef(standard, books[1], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import: verified 0 of 2 expected books; unmatched files need review"); err != nil {
		t.Fatal(err)
	}
	fids := []int64{
		waitingFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "pack"),
		waitingFile(t, st, sid, root, "ONE PIECE v02 (Digital).cbz", "pack"),
		waitingFile(t, st, sid, root, "ONE PIECE 005 (Digital).cbz", "pack"),
	}
	owned := ownedModelFile(t, st, sid, standard, books[2], version)
	res, err := im.ResolveFileReviews(ctx, sid, append(append([]int64{}, fids...), owned, 999999))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Resolved) != len(fids) {
		t.Fatalf("resolved %v, want the pack's files %v", res.Resolved, fids)
	}
	if len(res.Skipped) != 2 {
		t.Fatalf("skipped %+v, want the reviewed file and a missing id", res.Skipped)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.ID == owned {
			if f.NeedsReview {
				t.Error("the already-reviewed file was touched")
			}
			continue
		}
		if f.NeedsReview {
			t.Errorf("file %d still waits for review", f.ID)
		}
		if !f.Unrequested {
			t.Errorf("declined file %d claims a book", f.ID)
		}
	}
	job, err := st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueImported {
		t.Fatalf("pack download not settled: %s %v", job.Status, err)
	}
	history, err := st.ListDecisions(ctx, sid, 1)
	if err != nil || len(history) == 0 {
		t.Fatalf("history: %+v %v", history, err)
	}
	var last store.DecisionBody
	if err = json.Unmarshal(history[0].Body, &last); err != nil || last.Event != "imported_after_review" {
		t.Fatalf("last event: %+v %v", last, err)
	}
	if !strings.Contains(last.Note, "wanted again") {
		t.Errorf("declined books must be wanted again: %q", last.Note)
	}
	cleanup, err := im.CleanupPreview(ctx, sid, 0)
	if err != nil {
		t.Fatal(err)
	}
	eligible := 0
	for _, c := range cleanup {
		if c.Eligible {
			eligible++
			if c.Category != "unmapped" || c.Reason != "Reviewed, holds no book" {
				t.Errorf("declined file in cleanup: %s (%s)", c.Reason, c.Category)
			}
		}
	}
	if eligible != len(fids) {
		t.Errorf("%d eligible files, want the declined pack (%d)", eligible, len(fids))
	}
	for _, ids := range [][]int64{nil, make([]int64, 501)} {
		if _, err = im.ResolveFileReviews(ctx, sid, ids); !errors.Is(err, apperr.ErrInvalid) {
			t.Errorf("resolve of %d files: got %v, want invalid", len(ids), err)
		}
	}
}

// Matching a whole release maps each file by its own numbers onto one
// chosen card, through the same path as one file's review; files whose
// numbers do not resolve are left for individual review, and the pack's
// download only settles once they are answered.
func TestMatchWholeReleaseMapsFilesByTheirNumbers(t *testing.T) {
	t.Parallel()
	_, st, sid, standard, books, version := modelFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "pack", Title: "ONE PIECE pack"}, []store.BookRef{modelRef(standard, books[0], version), modelRef(standard, books[1], version)})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "partial import: verified 0 of 2 expected books; unmatched files need review"); err != nil {
		t.Fatal(err)
	}
	names := []struct {
		name, key, note string
		books           int
	}{
		{"ONE PIECE v01 (Digital).cbz", "pack", "", 1},
		// A range maps to every book of the card it covers, like a proposal.
		{"ONE PIECE v02-03 (Digital).cbz", "pack", "", 2},
		{"ONE PIECE c005 (Digital).cbz", "pack", "collects volumes", 0},
		{"ONE PIECE v07 (Digital).cbz", "pack", "not volumes of this edition", 0},
		{"ONE PIECE v06-07 (Digital).cbz", "pack", "not volumes of this edition", 0},
		{"ONE PIECE v02-07 (Digital).cbz", "pack", "not volumes of this edition", 0},
		{"ONE PIECE v00-02 (Digital).cbz", "pack", "not volumes of this edition", 0},
		{"3.cbz", "", "does not say which books", 0},
	}
	var ids []int64
	for _, n := range names {
		fid := waitingFile(t, st, sid, root, n.name, n.key)
		ids = append(ids, fid)
	}
	matches, err := im.MatchReleaseFiles(ctx, sid, version.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != len(ids) {
		t.Fatalf("%d matches, want %d", len(matches), len(ids))
	}
	mapped := 0
	for i, m := range matches {
		if names[i].note == "" {
			mapped++
			if len(m.Books) != names[i].books {
				t.Errorf("%s: mapped to %+v, want %d books", names[i].name, m.Books, names[i].books)
			}
			if m.Note != "" {
				t.Errorf("%s: unexpected note %q", names[i].name, m.Note)
			}
			continue
		}
		if len(m.Books) != 0 || !strings.Contains(m.Note, names[i].note) {
			t.Errorf("%s: left with %q, want it to say %q", names[i].name, m.Note, names[i].note)
		}
	}
	if mapped != 2 {
		t.Fatalf("%d files mapped, want the ones whose numbers resolve", mapped)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		resolved := f.ID == ids[0] || f.ID == ids[1]
		if resolved && (f.NeedsReview || f.Unrequested) {
			t.Error("a mapped file still waits for review")
		}
		if !resolved && !f.NeedsReview {
			t.Errorf("file %d was taken off review without resolving", f.ID)
		}
	}
	job, err := st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueNeedsReview {
		t.Fatalf("pack settled while files still wait: %s %v", job.Status, err)
	}
	// Answering the rest finishes the import, like any other review.
	if _, err = im.ResolveFileReviews(ctx, sid, ids[1:]); err != nil {
		t.Fatal(err)
	}
	job, err = st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueImported {
		t.Fatalf("pack not settled after the last answer: %s %v", job.Status, err)
	}
	if _, err = im.MatchReleaseFiles(ctx, sid, version.ID, nil); !errors.Is(err, apperr.ErrInvalid) {
		t.Errorf("match of no files: got %v, want invalid", err)
	}
}

func TestReleaseMatchPreviewUsesBookIdentityAndRevalidatesOnSave(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, books, version := modelFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	bonus := books[0]
	bonus.ID, bonus.ProviderID, bonus.CountType, bonus.SequenceStr = 0, "", "extra", "Bonus 1"
	bonusID, err := st.SaveBook(ctx, sid, bonus)
	if err != nil {
		t.Fatal(err)
	}
	revision := books[1]
	revision.ID, revision.ProviderID, revision.SequenceStr = 0, "", "2 (revised)"
	if _, err = st.SaveBook(ctx, sid, revision); err != nil {
		t.Fatal(err)
	}
	main := waitingFile(t, st, sid, root, "ONE PIECE v01.cbz", "")
	extra := waitingFile(t, st, sid, root, "ONE PIECE v01 extra.cbz", "")
	ambiguous := waitingFile(t, st, sid, root, "ONE PIECE v02.cbz", "")
	ids := []int64{main, extra, ambiguous}
	preview, err := lib.PreviewReleaseFiles(ctx, sid, version.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 3 || len(preview[0].Books) != 1 || preview[0].Books[0].BookID != books[0].ID || len(preview[1].Books) != 1 || preview[1].Books[0].BookID != bonusID || len(preview[2].Books) != 0 {
		t.Fatalf("preview guessed book identity from a display number: %+v", preview)
	}
	// A decision made after the preview must survive a stale submission.
	if _, err = im.ResolveFileReviews(ctx, sid, []int64{main}); err != nil {
		t.Fatal(err)
	}
	matches, err := im.MatchReleaseFiles(ctx, sid, version.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches[0].Books) != 0 || !strings.Contains(matches[0].Note, "no longer waiting") || len(matches[1].Books) != 1 || len(matches[2].Books) != 0 {
		t.Fatalf("submission ignored current review state: %+v", matches)
	}
	contents, err := st.Contents(ctx, sid)
	if err != nil || len(contents) != 1 || contents[0].BookID != bonusID || contents[0].FileID != extra {
		t.Fatalf("preview changed contents or submission guessed: %+v %v", contents, err)
	}
}

// A pack whose files the import claimed on a card whose profile refuses
// them (waiting, unconfirmed) can be matched in bulk onto another collected
// version of the same edition — a colored pack onto a Colored card. Files
// with claims a person confirmed are still left for individual review.
func TestMatchReleaseRemapsImportClaimsToAnotherVersion(t *testing.T) {
	t.Parallel()
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	ctx := context.Background()
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListEditions(ctx, sid)
	var standard store.Edition
	for _, e := range cols {
		if e.EditionName == "Standard Edition" && e.Type == "volume" {
			standard = e
		}
	}
	books, _ := st.ListBooks(ctx, standard.ID)
	versions, _ := st.ListVersions(ctx, sid)
	var version store.CollectedVersion
	for _, v := range versions {
		if v.EditionID == standard.ID && v.IsDefault {
			version = v
		}
	}
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)

	colored, err := st.AddFormat(ctx, profile.Format{Name: "Colored", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "colored"}}})
	if err != nil {
		t.Fatal(err)
	}
	bw, err := st.AddProfile(ctx, profile.Profile{Name: "Black and white", MinFormatScore: profile.MinScoreOff, Refused: []int64{colored}})
	if err != nil {
		t.Fatal(err)
	}
	version.ProfileID = bw
	if _, err := lib.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	colorProfID, err := st.AddProfile(ctx, profile.Profile{Name: "Colored", MinFormatScore: profile.MinScoreOff, Required: []int64{colored}})
	if err != nil {
		t.Fatal(err)
	}
	card := store.CollectedVersion{EditionID: standard.ID, Name: "Colored", Monitored: true, ProfileID: colorProfID}
	cardID, err := lib.SaveVersion(ctx, sid, card)
	if err != nil {
		t.Fatal(err)
	}
	card.ID = cardID

	// The pack's files: claimed by the import on the default card, waiting.
	claimedFile := func(name string, b store.Book) int64 {
		t.Helper()
		path := filepath.Join(root, name)
		writeCBZ(t, path, name)
		fid, err := st.AddFile(ctx, store.File{
			SeriesID: sid, Path: path, NeedsReview: true, Presence: "present",
			Facts: store.AssetFacts{Title: name, FactsKnown: true, Flags: map[string]bool{"colored": true}, ReleaseKey: "pack"},
		}, nil, []store.BookLink{{Ledger: "volume", EditionID: standard.ID, Number: b.SequenceNum, VersionID: version.ID, BookID: b.ID}})
		if err != nil {
			t.Fatal(err)
		}
		return fid
	}
	ids := []int64{claimedFile("ONE PIECE v01 (Colored).cbz", books[0]), claimedFile("ONE PIECE v02 (Colored).cbz", books[1])}
	// A file whose claim a person confirmed is not bulk-mappable.
	owned := ownedModelFile(t, st, sid, standard, books[2], version)
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE files SET needs_review=TRUE WHERE id=%d; UPDATE asset_contents SET evidence='manual' WHERE file_id=%d`, owned, owned)); err != nil {
		t.Fatal(err)
	}
	ids = append(ids, owned)

	matches, err := im.MatchReleaseFiles(ctx, sid, card.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range matches[:2] {
		if len(m.Books) != 1 || m.Books[0].VersionID != card.ID || m.Note != "" {
			t.Errorf("file %d: %+v, want mapped onto the Colored card", ids[i], m)
		}
	}
	if len(matches[2].Books) != 0 || !strings.Contains(matches[2].Note, "already claims books") {
		t.Errorf("confirmed file: %+v, want it left for individual review", matches[2])
	}
	cv, err := st.Coverage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	onCard := 0
	for _, a := range cv {
		if a.FileID == ids[0] || a.FileID == ids[1] {
			if a.VersionID == card.ID {
				onCard++
			}
		}
	}
	if onCard != 2 {
		t.Fatalf("%d coverage rows on the Colored card, want 2: %+v", onCard, cv)
	}
}
