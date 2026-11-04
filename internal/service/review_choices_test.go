package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func reviewDefaults(plan *ReviewPlan) []ReviewFileChoice {
	out := []ReviewFileChoice{}
	for _, f := range plan.Files {
		c := ReviewFileChoice{FileID: f.FileID, Use: []store.BookRef{}, Disposition: "keep"}
		for _, b := range f.Books {
			if b.UseIncoming {
				c.Use = append(c.Use, b.Book)
			}
		}
		out = append(out, c)
	}
	return out
}

func waitingScoredFile(t *testing.T, st *store.Store, sid int64, dir, name, key string) int64 {
	t.Helper()
	fid := waitingFile(t, st, sid, dir, name, key)
	hash, err := fileHash(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	err = st.SetAssetFacts(context.Background(), fid, store.AssetFacts{Title: name, Group: "LuCaZ", Site: "Nyaa", Protocol: "torrent", ReleaseKey: key, FactsKnown: true, Flags: map[string]bool{"digital": true}}, hash)
	if err != nil {
		t.Fatal(err)
	}
	return fid
}

func TestReviewChoicesRejectsChangedBytesBeforePreview(t *testing.T) {
	for _, reviewed := range []bool{false, true} {
		t.Run(strconv.FormatBool(reviewed), func(t *testing.T) {
			t.Parallel()
			lib, st, sid, col, books, version := factsClaimsFixture(t)
			ctx := context.Background()
			root := t.TempDir()
			fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "")
			req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
			if reviewed {
				plan, err := lib.PreviewReviewChoices(ctx, sid, req)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{{FileID: fid, Disposition: "keep"}}}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := st.ListFiles(ctx, sid)
			if err != nil || len(before) != 1 {
				t.Fatalf("before change: %+v %v", before, err)
			}
			if err = os.WriteFile(before[0].Path, []byte("different unreviewed archive bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err = lib.PreviewReviewChoices(ctx, sid, req); !errors.Is(err, apperr.ErrConflict) || !strings.Contains(err.Error(), "rescan") {
				t.Fatalf("changed bytes accepted in preview: %v", err)
			}
			// Even a caller bypassing preview cannot replace the fingerprint
			// while retaining the prior contents and facts.
			revision, err := st.ReviewRevision(ctx, sid)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := fileHash(before[0].Path)
			if err != nil {
				t.Fatal(err)
			}
			err = st.CommitFileReviews(ctx, sid, revision, []store.FileReviewWrite{{FileID: fid, Facts: before[0].Facts, Hash: digest, Contents: req.Files[0].Books, Disposition: "keep"}})
			if !errors.Is(err, apperr.ErrConflict) {
				t.Fatalf("transaction renewed old evidence for new bytes: %v", err)
			}
			after, err := st.ListFiles(ctx, sid)
			if err != nil || len(after) != 1 || after[0].Hash != before[0].Hash || after[0].NeedsReview != before[0].NeedsReview {
				t.Fatalf("rejected review changed saved evidence: %+v %v", after, err)
			}
		})
	}
}

func TestReviewChoicesExplainsDeselectedPinTrashBlocker(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "")
	ref := modelRef(col, books[0], version)
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{ref}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{{FileID: fid, Disposition: "keep"}}}); err != nil {
		t.Fatal(err)
	}
	if err = st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ref.BookID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	cov, err := st.Coverage(ctx, sid)
	if err != nil || len(cov) != 0 {
		t.Fatalf("fixture must have only a deselected pin: %+v %v", cov, err)
	}
	plan, err = lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil || !strings.Contains(plan.Files[0].TrashReason, "Unpin") {
		t.Fatalf("preview did not explain pin protection: %+v %v", plan, err)
	}
}

func TestReviewChoicesKeepsExistingVolumeAndTrashesPackDuplicate(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	for n := 4; n <= 6; n++ {
		b := books[0]
		b.ID, b.ProviderID, b.SequenceNum, b.SequenceStr = 0, "", int64(n*100), strconv.Itoa(n)
		id, e := st.SaveBook(ctx, sid, b)
		if e != nil {
			t.Fatal(e)
		}
		b.ID = id
		books = append(books, b)
	}
	old := addFactFile(t, st, sid, col, books[3], version, "Madokami volume 4.cbz", store.AssetFacts{Title: "Madokami volume 4", Site: "Madokami", Protocol: "source", FactsKnown: true, Flags: map[string]bool{"digital": true}}, store.AssetFacts{})
	refs := []store.BookRef{}
	req := ReviewPlanRequest{VersionID: version.ID}
	for i := range 6 {
		ref := modelRef(col, books[i], version)
		refs = append(refs, ref)
		fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v0"+string(rune('1'+i))+" (Digital).cbz", "pack")
		req.Files = append(req.Files, ReviewFileRequest{FileID: fid})
	}
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "pack", Title: "ONE PIECE pack"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, "review pack"); err != nil {
		t.Fatal(err)
	}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 6 || len(plan.Files[3].Books) != 1 {
		t.Fatalf("bad pack preview: %+v", plan)
	}
	b := plan.Files[3].Books[0]
	if b.UseIncoming || b.Incoming.Score != 250 || len(b.Owners) != 1 || b.Owners[0].Score.Score != 100 || b.Owners[0].Source != "Madokami" {
		t.Fatalf("higher-scoring duplicate should keep existing: %+v", b)
	}
	choices := reviewDefaults(plan)
	choices[3].Disposition = "trash"
	out, err := im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: choices})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range out {
		if f.Note != "" {
			t.Fatalf("unexpected file failure: %+v", f)
		}
	}
	for i := range 6 {
		owners, err := st.BookOwners(ctx, version.ID, books[i].ID)
		if err != nil {
			t.Fatal(err)
		}
		want := req.Files[i].FileID
		if i == 3 {
			want = old
		}
		if len(owners) != 1 || owners[0].FileID != want {
			t.Fatalf("book %d owners %+v, want %d", i+1, owners, want)
		}
	}
	trashed, err := st.GetFileTrash(ctx, req.Files[3].FileID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(trashed.TrashPath); err != nil {
		t.Fatal(err)
	}
	job, err := st.GetQueueItem(ctx, qid)
	if err != nil || job.Status != store.QueueImported {
		t.Fatalf("pack did not settle: %+v %v", job, err)
	}
	history, err := st.ListDecisions(ctx, sid, 1)
	if err != nil || len(history) == 0 {
		t.Fatal(err)
	}
	var body store.DecisionBody
	if err = json.Unmarshal(history[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.Note, "Kept existing") || strings.Contains(body.Note, "wanted again") {
		t.Fatalf("misleading history: %s", body.Note)
	}
	linked, hashes, err := im.alreadyImported(ctx, *job, refs)
	if err != nil || len(linked) != 5 || len(hashes) != 6 {
		t.Fatalf("retry lost skipped identity or delivery attribution: linked=%v hashes=%v err=%v", linked, hashes, err)
	}
	complete, err := im.reviewedJobComplete(ctx, *job)
	if err != nil || !complete {
		t.Fatalf("reviewed pack would reopen on retry: %v %v", complete, err)
	}
	if err = im.RestoreTrashedFile(ctx, sid, trashed.FileID); err != nil {
		t.Fatal(err)
	}
	im.finishReviewTrash(ctx)
	if _, err = st.GetFileTrash(ctx, trashed.FileID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("restored file was trashed again: %v", err)
	}
	owners, _ := st.BookOwners(ctx, version.ID, books[3].ID)
	if len(owners) != 1 || owners[0].FileID != old {
		t.Fatalf("restore selected rejected copy: %+v", owners)
	}
}

func TestReviewChoicesRejectsStaleOwnersPinsProfilesAndBytes(t *testing.T) {
	for _, change := range []string{"owner", "pin", "profile", "bytes"} {
		t.Run(change, func(t *testing.T) {
			t.Parallel()
			lib, st, sid, col, books, version := factsClaimsFixture(t)
			ctx := context.Background()
			root := t.TempDir()
			im := NewImporter(st, nil, root, nil)
			old := addFactFile(t, st, sid, col, books[0], version, "old.cbz", store.AssetFacts{FactsKnown: true}, store.AssetFacts{})
			fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "")
			req := ReviewPlanRequest{VersionID: version.ID, Files: []ReviewFileRequest{{FileID: fid}}}
			plan, err := lib.PreviewReviewChoices(ctx, sid, req)
			if err != nil {
				t.Fatal(err)
			}
			choices := reviewDefaults(plan)
			choices[0].Use = []store.BookRef{plan.Files[0].Books[0].Book}
			switch change {
			case "owner":
				_ = addFactFile(t, st, sid, col, books[0], version, "another.cbz", store.AssetFacts{FactsKnown: true}, store.AssetFacts{})
			case "pin":
				err = st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: old, VersionID: version.ID, BookID: books[0].ID, Pinned: true})
			case "profile":
				var p *profile.Profile
				p, err = st.GetProfile(ctx, version.ProfileID)
				if err == nil {
					p.MinFormatScore = 300
					err = st.UpdateProfile(ctx, *p)
				}
			case "bytes":
				err = os.WriteFile(filepath.Join(root, "ONE PIECE v01 (Digital).cbz"), []byte("different archive bytes"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: choices}); !errors.Is(err, apperr.ErrConflict) {
				t.Fatalf("stale %s applied: %v", change, err)
			}
			files, _ := st.ListFiles(ctx, sid)
			for _, f := range files {
				if f.ID == fid && !f.NeedsReview {
					t.Fatal("stale plan partially applied")
				}
			}
		})
	}
}

func TestReviewChoicesSharedArchiveAndExplicitReplacement(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	old := addFactFile(t, st, sid, col, books[0], version, "old.cbz", store.AssetFacts{FactsKnown: true}, store.AssetFacts{})
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01-02 (Digital).cbz", "")
	req := ReviewPlanRequest{VersionID: version.ID, Files: []ReviewFileRequest{{FileID: fid}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	choices := reviewDefaults(plan)
	if len(choices[0].Use) != 1 || choices[0].Use[0].BookID != books[1].ID {
		t.Fatalf("shared archive defaults: %+v", choices)
	}
	choices[0].Disposition = "trash"
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: choices}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("trashed selected shared archive: %v", err)
	}
	choices[0].Disposition = "keep"
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: choices}); err != nil {
		t.Fatal(err)
	}
	owners, _ := st.BookOwners(ctx, version.ID, books[0].ID)
	if len(owners) != 1 || owners[0].FileID != old {
		t.Fatal("shared archive displaced old volume")
	}
	contents, err := st.Contents(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range contents {
		if c.FileID == fid && c.Verified {
			count++
		}
	}
	if count != 2 {
		t.Fatal("non-selected book lost intrinsic contents")
	}
	// Select that copy deliberately through the single-file editor. Its
	// other book remains selected and only the requested ownership changes.
	req.Files[0].Books = []store.BookRef{modelRef(col, books[0], version), modelRef(col, books[1], version)}
	plan, err = lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	choices = reviewDefaults(plan)
	choices[0].Use = append(choices[0].Use, plan.Files[0].Books[0].Book)
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: choices}); err != nil {
		t.Fatal(err)
	}
	for _, b := range books[:2] {
		owners, _ = st.BookOwners(ctx, version.ID, b.ID)
		if len(owners) != 1 || owners[0].FileID != fid {
			t.Fatal("explicit selection not applied")
		}
	}
}

func TestReviewChoicesScoresDoNotConsentToReplacement(t *testing.T) {
	for _, score := range []int{-100, 100, 250} {
		t.Run(strconv.Itoa(score), func(t *testing.T) {
			t.Parallel()
			lib, st, sid, col, books, v := factsClaimsFixture(t)
			ctx := context.Background()
			root := t.TempDir()
			im := NewImporter(st, nil, root, nil)
			old := addFactFile(t, st, sid, col, books[0], v, "current.cbz", store.AssetFacts{FactsKnown: true, Flags: map[string]bool{"digital": true}}, store.AssetFacts{})
			fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "")
			prof, err := st.GetProfile(ctx, v.ProfileID)
			if err != nil {
				t.Fatal(err)
			}
			formats, err := st.ListFormats(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range formats {
				if f.Name == "Known group" {
					prof.FormatScores[f.ID] = score - 100
				}
			}
			if err = st.UpdateProfile(ctx, *prof); err != nil {
				t.Fatal(err)
			}
			req := ReviewPlanRequest{VersionID: v.ID, Files: []ReviewFileRequest{{FileID: fid}}}
			plan, err := lib.PreviewReviewChoices(ctx, sid, req)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Files[0].Books[0].Incoming.Score != score || plan.Files[0].Books[0].UseIncoming {
				t.Fatalf("score implied replacement: %+v", plan.Files[0].Books[0])
			}
			if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
				t.Fatal(err)
			}
			owners, _ := st.BookOwners(ctx, v.ID, books[0].ID)
			if len(owners) != 1 || owners[0].FileID != old {
				t.Fatal("score displaced existing owner")
			}
		})
	}
}

func TestReviewChoicesKeepsOtherVersionsAndRequestedIntent(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, v := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	old := addFactFile(t, st, sid, col, books[0], v, "old.cbz", store.AssetFacts{FactsKnown: true}, store.AssetFacts{})
	pid, err := st.AddProfile(ctx, profile.Profile{Name: "Another version", MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	other, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: col.ID, ProfileID: pid, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "planned-upgrade")
	ref := modelRef(col, books[0], v)
	snap, _ := json.Marshal(ReleaseCandidate{RequestedReplacements: []store.BookRef{ref}})
	qid, _, err := st.ReserveQueue(ctx, store.QueueItem{SeriesID: sid, Protocol: "torrent", ReleaseKey: "planned-upgrade", Snapshot: snap}, []store.BookRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.UpdateQueueItem(ctx, qid, store.QueueNeedsReview, ""); err != nil {
		t.Fatal(err)
	}
	req := ReviewPlanRequest{VersionID: v.ID, Files: []ReviewFileRequest{{FileID: fid}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Files[0].Books[0].RequestedReplacement || !plan.Files[0].Books[0].UseIncoming {
		t.Fatal("explicit acquisition intent was lost")
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatal(err)
	}
	owners, _ := st.BookOwners(ctx, v.ID, books[0].ID)
	if len(owners) != 1 || owners[0].FileID != fid {
		t.Fatal("requested replacement not selected")
	}
	owners, _ = st.BookOwners(ctx, other, books[0].ID)
	if len(owners) != 1 || owners[0].FileID != old {
		t.Fatal("another version's selection was displaced")
	}
}

func TestReviewChoicesTrashFailureRetainsIntentAndRetries(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "")
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(root, fileTrashDir)
	if err = os.WriteFile(blocker, []byte("temporary path obstruction"), 0o600); err != nil {
		t.Fatal(err)
	}
	choice := ReviewFileChoice{FileID: fid, Disposition: "trash"}
	out, err := im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}})
	if err != nil || len(out) != 1 || out[0].Status != "trash_pending" || out[0].Note == "" {
		t.Fatalf("partial failure was hidden: %+v %v", out, err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 || files[0].NeedsReview || files[0].ReviewAction != "trash" {
		t.Fatalf("trash intent not durable: %+v %v", files, err)
	}
	if err = os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	im.finishReviewTrash(ctx)
	if _, err = st.GetFileTrash(ctx, fid); err != nil {
		t.Fatalf("pending trash did not recover: %v", err)
	}
}

func TestReviewChoicesPreservesUsesInPausedVersions(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, v := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	pid, err := st.AddProfile(ctx, profile.Profile{Name: "Paused version", MinFormatScore: profile.MinScoreOff})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := lib.SaveVersion(ctx, sid, store.CollectedVersion{EditionID: col.ID, ProfileID: pid, Monitored: false})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.GetVersion(ctx, otherID)
	if err != nil {
		t.Fatal(err)
	}
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01 (Digital).cbz", "")
	ref := modelRef(col, books[0], *other)
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{ref}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatal(err)
	}
	// Revisiting a file without correcting its confirmed contents must retain
	// selections outside the requested version, including paused versions.
	req = ReviewPlanRequest{VersionID: v.ID, Files: []ReviewFileRequest{{FileID: fid}}}
	plan, err = lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	choice := ReviewFileChoice{FileID: fid, Disposition: "trash"}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("paused version's archive could be trashed: %v", err)
	}
	choice.Disposition = "keep"
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); err != nil {
		t.Fatal(err)
	}
	owners, err := st.BookOwners(ctx, otherID, ref.BookID)
	if err != nil || len(owners) != 1 || owners[0].FileID != fid {
		t.Fatalf("paused selection lost: %+v %v", owners, err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 || files[0].Unrequested {
		t.Fatalf("shared file mislabeled unused: %+v %v", files, err)
	}
}

func TestReviewChoicesUnusedDecisionSurvivesIdenticalRescan(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "skipped-pack")
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatal(err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 {
		t.Fatal(err)
	}
	rescanned := files[0]
	rescanned.NeedsReview, rescanned.Unrequested = true, true
	if _, err = st.AddFile(ctx, rescanned, nil, nil); err != nil {
		t.Fatal(err)
	}
	files, err = st.ListFiles(ctx, sid)
	if err != nil || files[0].NeedsReview || files[0].ReviewAction != "keep" {
		t.Fatalf("identical scan reopened rejected file: %+v %v", files, err)
	}
}

func TestReviewChoicesSurvivesSchedulerTicks(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "")
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.PutSetting(ctx, "scheduler", []byte(`{"queue":"2026-11-03T23:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatalf("routine scheduler bookkeeping invalidated the review: %v", err)
	}
}

func TestReviewChoicesDefaultsToOneIncomingCopyPerBook(t *testing.T) {
	t.Parallel()
	lib, st, sid, _, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	req := ReviewPlanRequest{VersionID: version.ID}
	for _, name := range []string{"ONE PIECE v01 (Digital).cbz", "ONE PIECE v01 (Digital) [LuCaZ].cbz", "ONE PIECE v02 (Digital).cbz"} {
		fid := waitingScoredFile(t, st, sid, root, name, "")
		req.Files = append(req.Files, ReviewFileRequest{FileID: fid})
	}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range plan.Files {
		if len(f.Books) != 1 || f.Books[0].UseIncoming != (i != 1) {
			t.Fatalf("duplicate incoming default at %d: %+v", i, f)
		}
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
		t.Fatalf("duplicate blocked unrelated imports: %v", err)
	}
	for i, fileIndex := range []int{0, 2} {
		owners, err := st.BookOwners(ctx, version.ID, books[i].ID)
		if err != nil || len(owners) != 1 || owners[0].FileID != req.Files[fileIndex].FileID {
			t.Fatalf("wrong incoming selection: %+v %v", owners, err)
		}
	}
}

func TestReviewChoicesPolicyRefusalsAndUnknownFacts(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, v := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	im := NewImporter(st, nil, root, nil)
	fid := waitingScoredFile(t, st, sid, root, "ONE PIECE v01.cbz", "")
	prof, err := st.GetProfile(ctx, v.ProfileID)
	if err != nil {
		t.Fatal(err)
	}
	prof.MinFormatScore = 300
	if err = st.UpdateProfile(ctx, *prof); err != nil {
		t.Fatal(err)
	}
	ref := modelRef(col, books[0], v)
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{ref}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Books[0].Reason == "" || plan.Files[0].Books[0].UseIncoming {
		t.Fatal("policy refusal was not shown")
	}
	choice := ReviewFileChoice{FileID: fid, Disposition: "keep", Use: []store.BookRef{ref}}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatal("refused file was selected", err)
	}
	prof.MinFormatScore = profile.MinScoreOff
	if err = st.UpdateProfile(ctx, *prof); err != nil {
		t.Fatal(err)
	}
	if err = st.SetAssetFacts(ctx, fid, store.AssetFacts{}, ""); err != nil {
		t.Fatal(err)
	}
	plan, err = lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Files[0].Books[0].Incoming.FactsKnown || plan.Files[0].Books[0].UseIncoming {
		t.Fatal("unknown facts silently selected")
	}
	if _, err = im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); err != nil {
		t.Fatalf("explicit content confirmation refused unknown quality: %v", err)
	}
}
