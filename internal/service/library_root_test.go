package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

func TestPrepareLibraryRootChecksMovedFiles(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	old, root := t.TempDir(), t.TempDir()
	src := filepath.Join(old, "book.cbz")
	writeCBZ(t, src, "book")
	if _, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: src, Size: 1, Provenance: "scan"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLibraryRoot(ctx, st, old); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLibraryRoot(ctx, st, root); err == nil {
		t.Fatal("accepted a move before the file arrived")
	}
	dest := filepath.Join(root, "book.cbz")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLibraryRoot(ctx, st, root); err == nil {
		t.Fatal("accepted a directory as an archive")
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, dest); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLibraryRoot(ctx, st, root); err != nil {
		t.Fatal(err)
	}
	files, _ := st.ListFiles(ctx, sid)
	if len(files) != 1 || files[0].Path != dest {
		t.Fatalf("file not relocated: %+v", files)
	}
}

func TestPrepareLibraryRootNormalizesLegacyRelativePaths(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	sid, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, root)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(relative, "book.cbz")
	writeCBZ(t, src, "book")
	if err := st.SetSeriesPath(ctx, sid, relative); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddFile(ctx, store.File{SeriesID: sid, Path: src, Size: 1, Provenance: "scan"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := PrepareLibraryRoot(ctx, st, relative); err != nil {
		t.Fatal(err)
	}
	series, _ := st.GetSeries(ctx, sid)
	files, _ := st.ListFiles(ctx, sid)
	if series.Path != root || len(files) != 1 || files[0].Path != filepath.Join(root, "book.cbz") {
		t.Fatalf("relative paths remain: %+v %+v", series, files)
	}
}
