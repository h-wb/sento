package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

func TestReviewChoicesProtectsCurrentlySelectedArchive(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "")
	ref := modelRef(col, books[0], version)
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{ref}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatal(err)
	}
	plan, err = lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Files[0].Books[0].CurrentSelected || !plan.Files[0].Books[0].UseIncoming || !strings.Contains(plan.Files[0].TrashReason, "currently selected") {
		t.Fatalf("selected archive not represented/protected: %+v", plan.Files[0])
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{{FileID: fid, Disposition: "trash"}}}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("selected archive could be trashed: %v", err)
	}
	// A batch cannot replace the owner first and then trash it, bypassing
	// protection by changing selections in an earlier write.
	incoming := waitingScoredFile(t, st, sid, root, "another.cbz", "")
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]store.File{}
	for _, f := range files {
		byID[f.ID] = f
	}
	revision, err := st.ReviewRevision(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	writes := []store.FileReviewWrite{
		{FileID: incoming, Facts: byID[incoming].Facts, Hash: byID[incoming].Hash, Contents: []store.BookRef{ref}, Selected: []store.BookRef{ref}, Replacements: []store.AssetCoverage{{FileID: fid, VersionID: ref.VersionID, BookID: ref.BookID}}, Disposition: "keep"},
		{FileID: fid, Facts: byID[fid].Facts, Hash: byID[fid].Hash, Reset: true, Disposition: "trash"},
	}
	if err = st.CommitFileReviews(ctx, sid, revision, writes, nil); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("batch bypassed pre-existing owner protection: %v", err)
	}
	owners, err := st.BookOwners(ctx, ref.VersionID, ref.BookID)
	if err != nil || len(owners) != 1 || owners[0].FileID != fid {
		t.Fatalf("rejected trash changed ownership: %+v %v", owners, err)
	}
	if _, err = os.Stat(byID[fid].Path); err != nil {
		t.Fatalf("rejected trash moved the archive: %v", err)
	}
}

func TestReviewChoicesRevalidatesLiveFilesInsideCommit(t *testing.T) {
	t.Parallel()
	_, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	req := ReviewPlanRequest{}
	for i, name := range []string{"first.cbz", "second.cbz"} {
		fid := waitingScoredFile(t, st, sid, root, name, "")
		req.Files = append(req.Files, ReviewFileRequest{FileID: fid, Books: []store.BookRef{modelRef(col, books[i], version)}})
	}
	plan, revision, p, err := reviewChoices(ctx, st, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	var writes []store.FileReviewWrite
	for _, row := range plan.Files {
		f := p.files[row.FileID]
		refs := []store.BookRef{row.Books[0].Book}
		writes = append(writes, store.FileReviewWrite{FileID: f.ID, Facts: f.Facts, Hash: f.Hash, Contents: refs, Selected: refs, Disposition: "keep"})
	}
	validated := false
	err = st.CommitFileReviews(ctx, sid, revision, writes, func() error {
		validated = true
		// Simulate replacement after preflight but before the transaction
		// acquired its locks; the guarded validation must observe it.
		if e := os.WriteFile(p.files[writes[1].FileID].Path, []byte("unreviewed replacement bytes"), 0o600); e != nil {
			return e
		}
		return validateReviewFiles(ctx, writes, p.files)
	})
	if !validated || !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("live bytes were not checked during commit: called=%v err=%v", validated, err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 2 {
		t.Fatalf("read rejected batch: %+v %v", files, err)
	}
	for _, f := range files {
		if !f.NeedsReview || f.ReviewAction != "" || f.Hash != p.files[f.ID].Hash {
			t.Fatalf("rejected batch partially changed file evidence: %+v", f)
		}
	}
	cov, err := st.Coverage(ctx, sid)
	if err != nil || len(cov) != 0 {
		t.Fatalf("rejected batch assigned ownership: %+v %v", cov, err)
	}
	if _, err = os.Stat(filepath.Join(root, "first.cbz")); err != nil {
		t.Fatal(err)
	}
}

func TestReviewChoicesFactEditPreservesLegacyDeselection(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01-02 (Digital).cbz", "")
	refs := []store.BookRef{modelRef(col, books[0], version), modelRef(col, books[1], version)}
	// Legacy mapping/upgrade paths confirmed contents and deselected copies
	// before files.review_action existed. Preserve that state on fact edits.
	if err := st.MapFile(ctx, sid, fid, refs); err != nil {
		t.Fatal(err)
	}
	if err := st.DeselectBook(ctx, fid, version.ID, refs[1].BookID); err != nil {
		t.Fatal(err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 || files[0].NeedsReview || files[0].ReviewAction != "" {
		t.Fatalf("legacy fixture: %+v %v", files, err)
	}
	group := "Corrected group"
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: refs, Group: &group}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Files[0].Books[0].UseIncoming || plan.Files[0].Books[1].UseIncoming || plan.Files[0].Books[1].CurrentSelected {
		t.Fatalf("editor reselected confirmed but unused contents: %+v", plan.Files[0])
	}
	if _, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatal(err)
	}
	for i, ref := range refs {
		owners, e := st.BookOwners(ctx, version.ID, ref.BookID)
		if e != nil || (i == 0 && (len(owners) != 1 || owners[0].FileID != fid)) || (i == 1 && len(owners) != 0) {
			t.Fatalf("fact-only edit changed book %d's selection: %+v %v", ref.BookID, owners, e)
		}
	}
	files, err = st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 || len(files[0].ContentBooks) != 2 || files[0].Facts.Group != group {
		t.Fatalf("fact correction lost confirmed contents: %+v %v", files, err)
	}
}
