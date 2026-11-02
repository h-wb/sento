package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func TestCleanupTrashRestoresIdentityAndProtectsSharedAssets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st, sid, edition, books, version := modelFixture(t)
	fid := ownedModelFile(t, st, sid, edition, books[0], version)
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	files, _ := st.ListFiles(ctx, sid)
	path := files[0].Path
	// The download client's hardlink is never moved or removed.
	seed := filepath.Join(t.TempDir(), "seeding.cbz")
	if err := os.Link(path, seed); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := im.CleanupPreview(ctx, sid, edition.ID)
	if err != nil || len(preview) != 1 || preview[0].Eligible {
		t.Fatalf("collected file unprotected: %+v %v", preview, err)
	}
	version.Monitored = false
	if _, err = st.SaveVersion(ctx, sid, version); err != nil {
		t.Fatal(err)
	}
	preview, err = im.CleanupPreview(ctx, sid, edition.ID)
	if err != nil || !preview[0].Eligible || preview[0].Category != "uncollected" {
		t.Fatal(preview, err)
	}
	result, err := im.CleanupFiles(ctx, sid, edition.ID, []int64{fid}, 30)
	if err != nil || len(result.Moved) != 1 || len(result.Skipped) != 0 {
		t.Fatal(result, err)
	}
	trash, err := st.ListFileTrash(ctx, sid)
	if err != nil || len(trash) != 1 {
		t.Fatal(trash, err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still at original path: %v", err)
	}
	if data, err := os.ReadFile(seed); err != nil || string(data) != string(original) {
		t.Fatal("download payload changed", err)
	}
	if files, err = st.ListFiles(ctx, sid); err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
	byVersion, err := lib.BooksByVersion(ctx, sid)
	if err != nil || len(byVersion[version.ID][0].FileIDs) != 0 || len(byVersion[version.ID][0].OnDiskFileIDs) != 0 {
		t.Fatal("trash counted as ownership", byVersion, err)
	}
	scanner := NewScanner(st, im.libraryRoot)
	scan, err := scanner.ScanSeriesFolder(ctx, sid, filepath.Dir(path))
	if err != nil || scan.Scanned != 0 {
		t.Fatal("scan adopted trash", scan, err)
	}
	ref := modelRef(edition, books[0], version)
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{ref}); err == nil {
		t.Fatal("edited trashed file")
	}
	if err = st.PinCoverage(ctx, sid, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: books[0].ID, Pinned: true}); err == nil {
		t.Fatal("pinned trashed file")
	}
	if _, err = im.RemoveSeries(ctx, sid, RemoveOptions{}); err == nil {
		t.Fatal("forgot a recoverable trash entry")
	}
	// A restore must never overwrite a new file placed at the original path.
	if err = os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = im.RestoreTrashedFile(ctx, sid, fid); err == nil {
		t.Fatal("restore overwrote a different file")
	}
	if data, _ := os.ReadFile(path); string(data) != "replacement" {
		t.Fatal("replacement bytes lost")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = im.RestoreTrashedFile(ctx, sid, fid); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(original) {
		t.Fatal("restored bytes changed", err)
	}
	if trash, err = st.ListFileTrash(ctx, sid); err != nil || len(trash) != 0 {
		t.Fatal(trash, err)
	}
	byVersion, err = lib.BooksByVersion(ctx, sid)
	if err != nil || len(byVersion[version.ID][0].FileIDs) != 1 || byVersion[version.ID][0].FileIDs[0] != fid {
		t.Fatal("mapping identity lost", byVersion, err)
	}
	current, err := st.GetVersion(ctx, version.ID)
	if err != nil || current.Monitored {
		t.Fatal("restore resumed collection", current, err)
	}
}

func TestCleanupRechecksSelectionsPinsAndReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, st, sid, e, books, v := modelFixture(t)
	fid := ownedModelFile(t, st, sid, e, books[0], v)
	v.Monitored = false
	if _, err := st.SaveVersion(ctx, sid, v); err != nil {
		t.Fatal(err)
	}
	other := v
	other.ID = 0
	other.Name = "Another copy"
	other.IsDefault = false
	other.Monitored = true
	id, err := st.SaveVersion(ctx, sid, other)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = id
	if err = st.MapFile(ctx, sid, fid, []store.BookRef{modelRef(e, books[0], v), modelRef(e, books[0], other)}); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	preview, err := im.CleanupPreview(ctx, sid, e.ID)
	if err != nil || preview[0].Eligible {
		t.Fatal("shared file not protected", preview, err)
	}
	if err = st.DeselectBook(ctx, fid, other.ID, books[0].ID); err != nil {
		t.Fatal(err)
	}
	pin := store.AssetCoverage{FileID: fid, VersionID: other.ID, BookID: books[0].ID, Pinned: true}
	if err = st.PinCoverage(ctx, sid, pin); err != nil {
		t.Fatal(err)
	}
	preview, err = im.CleanupPreview(ctx, sid, 0)
	if err != nil || preview[0].Eligible {
		t.Fatal("deselected pin not protected", preview, err)
	}
	pin.Pinned = false
	if err = st.PinCoverage(ctx, sid, pin); err != nil {
		t.Fatal(err)
	}
	preview, err = im.CleanupPreview(ctx, sid, 0)
	if err != nil || !preview[0].Eligible {
		t.Fatal(preview, err)
	}
	// Collection intent can change after the user has opened the preview.
	v.Monitored = true
	if _, err = st.SaveVersion(ctx, sid, v); err != nil {
		t.Fatal(err)
	}
	result, err := im.CleanupFiles(ctx, sid, 0, []int64{fid}, 7)
	if err != nil || len(result.Moved) != 0 || len(result.Skipped) != 1 {
		t.Fatal("stale preview removed an active copy", result, err)
	}
	path := filepath.Join(t.TempDir(), "unmatched.cbz")
	if err = os.WriteFile(path, []byte("archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	review, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: path, Size: 7, NeedsReview: true}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = im.CleanupFiles(ctx, sid, 0, []int64{review}, 7)
	if err != nil || len(result.Moved) != 0 {
		t.Fatal("unfinished file review was cleaned", result, err)
	}
	if err = st.MapFile(ctx, sid, review, nil); err != nil {
		t.Fatal(err)
	}
	result, err = im.CleanupFiles(ctx, sid, 0, []int64{review}, 7)
	if err != nil || len(result.Moved) != 1 {
		t.Fatal("reviewed unmapped file not eligible", result, err)
	}
}

func TestExpiredTrashPurgeAndInterruptedMoveRecovery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, st, sid, e, books, v := modelFixture(t)
	fid := ownedModelFile(t, st, sid, e, books[0], v)
	v.Monitored = false
	if _, err := st.SaveVersion(ctx, sid, v); err != nil {
		t.Fatal(err)
	}
	files, _ := st.ListFiles(ctx, sid)
	path := files[0].Path
	dest := filepath.Join(filepath.Dir(path), fileTrashDir, "retained.cbz")
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := fileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-48 * time.Hour).Format("2006-01-02 15:04:05")
	if err = st.BeginFileTrash(ctx, sid, fid, path, dest, digest, at, at); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	if err = im.PurgeExpiredTrash(ctx); err == nil {
		t.Fatal("interrupted move lost its original")
	}
	if err = im.RestoreTrashedFile(ctx, sid, fid); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = st.BeginFileTrash(ctx, sid, fid, path, dest, digest, at, at); err != nil {
		t.Fatal(err)
	}
	if err = os.Link(path, dest); err != nil {
		t.Fatal(err)
	}
	if err = im.PurgeExpiredTrash(ctx); err == nil {
		t.Fatal("partially linked move discarded its record")
	}
	if err = im.RestoreTrashedFile(ctx, sid, fid); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err = st.BeginFileTrash(ctx, sid, fid, path, dest, digest, at, at); err != nil {
		t.Fatal(err)
	}
	if err = moveArchive(path, dest); err != nil {
		t.Fatal(err)
	}
	if err = im.PurgeExpiredTrash(ctx); err != nil {
		t.Fatal(err)
	}
	if trash, err := st.ListFileTrash(ctx, sid); err != nil || len(trash) != 0 {
		t.Fatal(trash, err)
	}
	if _, err = os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("expired bytes retained", err)
	}
}
