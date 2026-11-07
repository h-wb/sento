package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

func pendingReviewTrash(t *testing.T) (*Importer, *store.Store, store.File) {
	t.Helper()
	_, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "unmatched.cbz", "")
	files, err := st.ListFiles(ctx, sid)
	if err != nil || len(files) != 1 {
		t.Fatalf("file fixture: %+v %v", files, err)
	}
	f := files[0]
	revision, err := st.ReviewRevision(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate interruption immediately after the review transaction commits.
	if err = st.CommitFileReviews(ctx, sid, revision, []store.FileReviewWrite{{FileID: fid, Facts: f.Facts, Hash: f.Hash, Reset: true, Disposition: "trash"}}, nil); err != nil {
		t.Fatal(err)
	}
	return NewImporter(st, nil, root, nil), st, f
}

func journalReviewTrash(t *testing.T, st *store.Store, f store.File) string {
	t.Helper()
	dest := filepath.Join(filepath.Dir(f.Path), fileTrashDir, strconv.FormatInt(f.ID, 10), filepath.Base(f.Path))
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	const expired = "2000-01-01 00:00:00"
	if err := st.BeginFileTrash(context.Background(), f.SeriesID, f.ID, f.Path, dest, f.Hash, expired, expired); err != nil {
		t.Fatal(err)
	}
	return dest
}

func TestReviewTrashRecoversBeyondBlockedFirstBatch(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	var writes []store.FileReviewWrite
	// Fill an entire batch with inaccessible originals. A later healthy move
	// must still run, and the failures must remain retryable next pass.
	for i := range 500 {
		fid, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: filepath.Join(root, "missing-"+strconv.Itoa(i)+".cbz"), Hash: "saved", NeedsReview: true}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		writes = append(writes, store.FileReviewWrite{FileID: fid, Hash: "saved", Disposition: "trash"})
	}
	revision, err := st.ReviewRevision(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CommitFileReviews(ctx, sid, revision, writes, nil); err != nil {
		t.Fatal(err)
	}
	fid := waitingScoredFile(t, st, sid, root, "healthy.cbz", "")
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	var healthy store.File
	for _, f := range files {
		if f.ID == fid {
			healthy = f
		}
	}
	revision, err = st.ReviewRevision(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.CommitFileReviews(ctx, sid, revision, []store.FileReviewWrite{{FileID: fid, Facts: healthy.Facts, Hash: healthy.Hash, Disposition: "trash"}}, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(healthy.Path)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, nil, root, nil)
	im.finishReviewTrash(ctx)
	entry, err := st.GetFileTrash(ctx, fid)
	if err != nil || entry.Pending {
		t.Fatalf("blocked first batch starved healthy move: %+v %v", entry, err)
	}
	if after, e := os.ReadFile(entry.TrashPath); e != nil || string(after) != string(before) {
		t.Fatalf("recovered archive changed: %v", e)
	}
	pending, err := st.PendingReviewTrash(ctx, 0)
	if err != nil || len(pending) != 500 {
		t.Fatalf("blocked intents lost retryability: %d %v", len(pending), err)
	}
	// Bring one previously inaccessible original back. Recovery starts from
	// the beginning on the next pass rather than forgetting the failed batch.
	if err = os.WriteFile(pending[0].Path, []byte("changed archive needs fresh review"), 0o600); err != nil {
		t.Fatal(err)
	}
	im.finishReviewTrash(ctx)
	pending, err = st.PendingReviewTrash(ctx, 0)
	if err != nil || len(pending) != 499 {
		t.Fatalf("earlier failure was not retried: %d %v", len(pending), err)
	}
}

func TestReviewTrashRecoversEveryInterruptedMoveStage(t *testing.T) {
	for _, stage := range []string{"before journal", "after journal", "after link", "after unlink"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			im, st, f := pendingReviewTrash(t)
			ctx := context.Background()
			bytes, err := os.ReadFile(f.Path)
			if err != nil {
				t.Fatal(err)
			}
			var dest string
			if stage != "before journal" {
				dest = journalReviewTrash(t, st, f)
			}
			if stage == "after link" || stage == "after unlink" {
				if err = os.Link(f.Path, dest); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "after unlink" {
				if err = os.Remove(f.Path); err != nil {
					t.Fatal(err)
				}
			}
			im.finishReviewTrash(ctx)
			entry, err := st.GetFileTrash(ctx, f.ID)
			if err != nil || entry.Pending {
				t.Fatalf("move did not finish: %+v %v", entry, err)
			}
			retained, err := os.ReadFile(entry.TrashPath)
			if err != nil || string(retained) != string(bytes) {
				t.Fatalf("retained bytes changed: %v", err)
			}
			if _, err = os.Lstat(f.Path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("original name remains after recovery: %v", err)
			}
			pending, err := st.PendingReviewTrash(ctx, 0)
			if err != nil || len(pending) != 0 {
				t.Fatalf("completed moves would fill future retry batches: %+v %v", pending, err)
			}
			im.finishReviewTrash(ctx)
			if err = im.RestoreTrashedFile(ctx, f.SeriesID, f.ID); err != nil {
				t.Fatal(err)
			}
			im.finishReviewTrash(ctx)
			if restored, e := os.ReadFile(f.Path); e != nil || string(restored) != string(bytes) {
				t.Fatalf("restore lost bytes or retried a completed choice: %v", e)
			}
		})
	}
}

func TestReviewTrashRejectsChangedBytesAtEveryStage(t *testing.T) {
	for _, stage := range []string{"before journal", "after journal", "after link", "after unlink", "completed"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			im, st, f := pendingReviewTrash(t)
			ctx := context.Background()
			changedPath := f.Path
			if stage != "before journal" {
				dest := journalReviewTrash(t, st, f)
				if stage == "after link" {
					if err := os.Link(f.Path, dest); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "after unlink" || stage == "completed" {
					if err := moveArchive(f.Path, dest); err != nil {
						t.Fatal(err)
					}
					changedPath = dest
				}
				if stage == "completed" {
					im.finishReviewTrash(ctx)
				}
			}
			const changed = "a different unreviewed archive"
			if err := os.WriteFile(changedPath, []byte(changed), 0o600); err != nil {
				t.Fatal(err)
			}
			im.finishReviewTrash(ctx)
			if stage == "before journal" || stage == "after journal" {
				if _, err := st.GetFileTrash(ctx, f.ID); !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("changed original was journaled as trash: %v", err)
				}
				files, err := st.ListFiles(ctx, f.SeriesID)
				if err != nil || len(files) != 1 || !files[0].NeedsReview || files[0].Hash != f.Hash {
					t.Fatalf("changed archive did not require fresh review: %+v %v", files, err)
				}
				questions, err := st.ReviewItems(ctx, f.SeriesID)
				if err != nil || !slices.ContainsFunc(questions, func(q store.ReviewItem) bool {
					return q.ItemID == f.ID && strings.Contains(q.Detail, "Archive contents changed")
				}) {
					t.Fatalf("fingerprint mismatch was not reported: %+v %v", questions, err)
				}
			} else {
				if err := im.PurgeExpiredTrash(ctx); err == nil {
					t.Fatal("changed retained bytes could be purged")
				}
			}
			if bytes, err := os.ReadFile(changedPath); err != nil || string(bytes) != changed {
				t.Fatalf("unreviewed archive was moved or deleted: %v", err)
			}
			im.finishReviewTrash(ctx)
			if bytes, err := os.ReadFile(changedPath); err != nil || string(bytes) != changed {
				t.Fatalf("a later pass retried an invalid choice: %v", err)
			}
			if stage == "before journal" || stage == "after journal" {
				// Different bytes must be rescanned to discard the old evidence
				// before a new keep choice can cancel the former trash intent.
				if _, err := NewScanner(st, im.libraryRoot).ScanSeriesFolder(ctx, f.SeriesID, filepath.Dir(f.Path)); err != nil {
					t.Fatal(err)
				}
				files, err := st.ListFiles(ctx, f.SeriesID)
				if err != nil || len(files) != 1 {
					t.Fatalf("rescan changed archive: %+v %v", files, err)
				}
				req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: files[0].ID, Books: []store.BookRef{}}}}
				plan, _, _, err := reviewChoices(ctx, st, f.SeriesID, req, false)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = im.ApplyReviewChoices(ctx, f.SeriesID, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: reviewDefaults(plan)}); err != nil {
					t.Fatalf("fresh review could not retain the changed archive: %v", err)
				}
				im.finishReviewTrash(ctx)
				if _, err = os.Stat(f.Path); err != nil {
					t.Fatalf("fresh keep choice did not cancel old trash intent: %v", err)
				}
			}
		})
	}
}

func TestReviewTrashRecoveryRejectsSubstitutedDirectory(t *testing.T) {
	t.Parallel()
	im, st, f := pendingReviewTrash(t)
	dest := journalReviewTrash(t, st, f)
	if err := os.Remove(filepath.Dir(dest)); err != nil {
		t.Fatal(err)
	}
	// The substituted destination resolves back to the sole original archive.
	if err := os.Symlink(filepath.Dir(f.Path), filepath.Dir(dest)); err != nil {
		t.Fatal(err)
	}
	im.finishReviewTrash(context.Background())
	if _, err := os.Stat(f.Path); err != nil {
		t.Fatalf("unsafe recovery removed the original archive: %v", err)
	}
	entry, err := st.GetFileTrash(context.Background(), f.ID)
	if err != nil || !entry.Pending {
		t.Fatalf("unsafe recovery marked a move complete: %+v %v", entry, err)
	}
	if err = im.RestoreTrashedFile(context.Background(), f.SeriesID, f.ID); err == nil {
		t.Fatal("restore followed a substituted trash directory")
	}
	if _, err = os.Stat(f.Path); err != nil {
		t.Fatalf("unsafe restore removed the sole original archive: %v", err)
	}
}
