package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
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
			if c.Category != "unmapped" || c.Reason != "Unmapped file, already reviewed" {
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
