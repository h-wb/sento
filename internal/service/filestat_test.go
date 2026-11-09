package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// archiveChanges alter an archive's bytes while keeping its size.
var archiveChanges = map[string]func(path string) error{
	"swapped by rename": func(path string) error {
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		swap := path + ".swap"
		if err = os.WriteFile(swap, append(raw[:len(raw)-1:len(raw)-1], raw[len(raw)-1]^1), 0o600); err != nil {
			return err
		}
		return os.Rename(swap, path)
	},
	"rewritten in place": func(path string) error {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return err
		}
		b := make([]byte, 1)
		if _, err = f.ReadAt(b, 1); err == nil {
			_, err = f.WriteAt([]byte{b[0] ^ 1}, 1)
		}
		return errors.Join(err, f.Close())
	},
}

// changeKeepingTimes applies change and restores the archive's mtime.
func changeKeepingTimes(t *testing.T, path string, change func(string) error) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = change(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
}

// settleArchives waits until archives written so far have trusted signatures.
func settleArchives(t *testing.T) {
	t.Helper()
	time.Sleep(racyStat + 100*time.Millisecond)
}

func fileRow(t *testing.T, st *store.Store, sid, fid int64) store.File {
	t.Helper()
	files, err := st.ListFiles(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.ID == fid {
			return f
		}
	}
	t.Fatalf("file %d not found", fid)
	return store.File{}
}

func TestTrustedSignatureWaitsForRacyTimestamps(t *testing.T) {
	t.Parallel()
	info, err := os.Stat(waitingPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if statSignature(info) == "" {
		t.Skip("platform reports no inode metadata")
	}
	if trustedSignature(info, time.Now()) != "" {
		t.Fatal("a just-written archive has a trusted signature")
	}
	if trustedSignature(info, time.Now().Add(racyStat+time.Second)) != statSignature(info) {
		t.Fatal("a settled archive has no trusted signature")
	}
}

func waitingPath(t *testing.T) string {
	t.Helper()
	path := t.TempDir() + "/new.cbz"
	if err := os.WriteFile(path, []byte("pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordSignature gives an archive's row a fingerprint that is deliberately not
// the archive's, with its live signature: reading the archive would conflict.
func recordSignature(t *testing.T, st *store.Store, sid, fid int64) store.File {
	t.Helper()
	ctx := context.Background()
	f := fileRow(t, st, sid, fid)
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	sig := statSignature(info)
	if sig == "" {
		t.Skip("platform reports no inode metadata")
	}
	if err = st.SetAssetFacts(ctx, fid, f.Facts, "recorded"); err != nil {
		t.Fatal(err)
	}
	if err = st.SetFileHashStat(ctx, fid, "recorded", sig); err != nil {
		t.Fatal(err)
	}
	return fileRow(t, st, sid, fid)
}

func TestReviewChoicesTrustRecordedSignature(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "first.cbz", "")
	settleArchives(t)
	recordSignature(t, st, sid, fid)
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
	plan, err := lib.PreviewReviewChoices(ctx, sid, req)
	if err != nil {
		t.Fatalf("preview read the archive despite its recorded signature: %v", err)
	}
	choice := ReviewFileChoice{FileID: fid, Disposition: "keep", Use: []store.BookRef{plan.Files[0].Books[0].Book}}
	if _, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); err != nil {
		t.Fatalf("apply read the archive despite its recorded signature: %v", err)
	}
}

func TestReviewChoicesFingerprintArchiveWithChangedSignature(t *testing.T) {
	for name, change := range archiveChanges {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib, st, sid, col, books, version := factsClaimsFixture(t)
			fid := waitingScoredFile(t, st, sid, t.TempDir(), "first.cbz", "")
			settleArchives(t)
			f := recordSignature(t, st, sid, fid)
			changeKeepingTimes(t, f.Path, change)
			settleArchives(t) // so only the signature comparison reveals it
			req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
			if _, err := lib.PreviewReviewChoices(context.Background(), sid, req); !errors.Is(err, apperr.ErrConflict) {
				t.Fatalf("preview trusted the signature of a changed archive: %v", err)
			}
		})
	}
}

// Apply reuses the preview's fingerprint only while the archive keeps the
// signature the preview read.
func TestApplyReviewChoicesRejectsArchiveChangedSincePreview(t *testing.T) {
	for name, change := range archiveChanges {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			lib, st, sid, col, books, version := factsClaimsFixture(t)
			ctx := context.Background()
			root := t.TempDir()
			fid := waitingScoredFile(t, st, sid, root, "first.cbz", "")
			settleArchives(t)
			req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
			plan, err := lib.PreviewReviewChoices(ctx, sid, req)
			if err != nil {
				t.Fatal(err)
			}
			beforeHash := fileRow(t, st, sid, fid).Hash
			changeKeepingTimes(t, fileRow(t, st, sid, fid).Path, change)
			settleArchives(t) // so only the token's signature reveals it
			choice := ReviewFileChoice{FileID: fid, Disposition: "keep", Use: []store.BookRef{plan.Files[0].Books[0].Book}}
			_, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}})
			if !errors.Is(err, apperr.ErrConflict) {
				t.Fatalf("apply accepted an archive changed since its preview: %v", err)
			}
			// The preview may have recorded the signature it read, but the
			// rejected apply must not change the review state or fingerprint.
			if got := fileRow(t, st, sid, fid); !got.NeedsReview || got.Hash != beforeHash {
				t.Fatalf("rejected apply changed the file: %+v", got)
			}
		})
	}
}

func TestScanTrustsRecordedSignature(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	sc := NewScanner(st, root)
	fid := waitingScoredFile(t, st, sid, root, "first.cbz", "")
	settleArchives(t)
	f := fileRow(t, st, sid, fid)
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	if statSignature(info) == "" {
		t.Skip("platform reports no inode metadata")
	}
	// The first scan fingerprints the file and records its signature.
	if replaced, e := sc.replaced(ctx, f, fs.FileInfoToDirEntry(info)); e != nil || replaced {
		t.Fatalf("unchanged archive replaced: %v %v", replaced, e)
	}
	f = fileRow(t, st, sid, fid)
	if f.HashStat != statSignature(info) {
		t.Fatalf("scan did not record the signature: %q", f.HashStat)
	}
	// A presence check marks the file checked without reading it, so only
	// the signature can reveal a rewrite that kept its size and mtime.
	changeKeepingTimes(t, f.Path, archiveChanges["rewritten in place"])
	if err = st.CheckFilePresence(ctx, sid); err != nil {
		t.Fatal(err)
	}
	f = fileRow(t, st, sid, fid)
	if info, err = os.Stat(f.Path); err != nil {
		t.Fatal(err)
	}
	if replaced, e := sc.replaced(ctx, f, fs.FileInfoToDirEntry(info)); e != nil || !replaced {
		t.Fatalf("rewritten archive not replaced: %v %v", replaced, e)
	}
}

// A preview that had to fingerprint an archive records the signature it read,
// so the next preview trusts the recorded fingerprint instead of re-reading
// the archive — the same rule a scan applies.
func TestReviewPreviewRecordsSignatureItRead(t *testing.T) {
	t.Parallel()
	lib, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "first.cbz", "")
	settleArchives(t)
	if f := fileRow(t, st, sid, fid); f.HashStat != "" {
		t.Fatalf("fixture should start without a signature: %q", f.HashStat)
	}
	info, err := os.Stat(filepath.Join(root, "first.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	sig := statSignature(info)
	if sig == "" {
		t.Skip("platform reports no inode metadata")
	}
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
	if _, err := lib.PreviewReviewChoices(ctx, sid, req); err != nil {
		t.Fatal(err)
	}
	if f := fileRow(t, st, sid, fid); f.HashStat != sig {
		t.Fatalf("preview did not record the signature it read: %q want %q", f.HashStat, sig)
	}
}

// A file recorded before signatures existed gets one on the next scan without
// being re-read, so review previews stop re-fingerprinting it.
func TestScanBackfillsSignatureForUntouchedLegacyFile(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "legacy.cbz")
	if err := os.WriteFile(path, []byte("pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	sig := statSignature(info)
	if sig == "" {
		t.Skip("platform reports no inode metadata")
	}
	hash, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(ctx, store.File{
		SeriesID: sid, Path: path, Size: info.Size(), Hash: hash, Presence: "present",
		Facts: store.AssetFacts{Title: "legacy.cbz", FactsKnown: true},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	settleArchives(t)
	sc := NewScanner(st, root)
	f := fileRow(t, st, sid, fid)
	if f.HashStat != "" {
		t.Fatalf("fixture should start without a signature: %q", f.HashStat)
	}
	live, err := statArchive(path)
	if err != nil {
		t.Fatal(err)
	}
	if replaced, e := sc.replaced(ctx, f, fs.FileInfoToDirEntry(live)); e != nil || replaced {
		t.Fatalf("unchanged legacy archive reported replaced: %v %v", replaced, e)
	}
	if got := fileRow(t, st, sid, fid); got.HashStat != sig {
		t.Fatalf("scan did not backfill the signature: %q want %q", got.HashStat, sig)
	}
}

// An archive written just before its preview has no trusted signature yet.
// It becomes trusted as time passes without changing, which must not make
// apply see a different review.
func TestApplyReviewChoicesAcceptsArchiveSettledSincePreview(t *testing.T) {
	t.Parallel()
	_, st, sid, col, books, version := factsClaimsFixture(t)
	ctx := context.Background()
	root := t.TempDir()
	fid := waitingScoredFile(t, st, sid, root, "first.cbz", "")
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: []store.BookRef{modelRef(col, books[0], version)}}}}
	plan, _, p, err := reviewChoices(ctx, st, sid, req, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if p.stats[fid] != "" {
		t.Fatal("preview must capture an archive inside the stat trust window")
	}
	settleArchives(t)
	choice := ReviewFileChoice{FileID: fid, Disposition: "keep", Use: []store.BookRef{plan.Files[0].Books[0].Book}}
	if _, err = NewImporter(st, nil, root, nil).ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}}); err != nil {
		t.Fatalf("apply rejected an unchanged archive: %v", err)
	}
}
