package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

func TestScanSeriesFolder(t *testing.T) {
	t.Parallel()
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for _, name := range []string{
		"ONE PIECE v01 (Digital).cbz",
		"ONE PIECE v02.cbz",
		"chapters/ONE PIECE c1180.cbz",
		"ONE PIECE (Artbook).cbz", // content-type still parses; links by numbers only → unrequested
		"random notes.txt",        // ignored: not an archive
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sc := NewScanner(st, dir)
	res, err := sc.ScanSeriesFolder(ctx, seriesID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 4 {
		t.Errorf("scanned: want 4 archives, got %d", res.Scanned)
	}
	if res.Linked != 3 {
		t.Errorf("linked: want 3 (v01, v02, c1180), got %d", res.Linked)
	}
	if res.Unrequested != 1 {
		t.Errorf("unrequested: want 1 (artbook), got %d", res.Unrequested)
	}

	// Volume books 1+2 satisfied now.
	cols, _ := st.ListEditions(ctx, seriesID)
	var standard store.Edition
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c
		}
	}
	slots := booksOf(t, lib, st, seriesID, standard.ID)
	if len(slots[0].OnDiskFileIDs) == 0 || len(slots[1].OnDiskFileIDs) == 0 {
		t.Errorf("vols 1-2 should be satisfied after scan: %+v", slots[:2])
	}

	// Chapter 1180 linked to the chapter edition.
	chapters := chapterCollection(t, cols)
	linked := 0
	for _, b := range booksOf(t, lib, st, seriesID, chapters.ID) {
		if b.Book.SequenceNum == 118000 {
			linked = len(b.OnDiskFileIDs)
		}
	}
	if linked != 1 {
		t.Errorf("chapter 1180 should be linked")
	}

	// Rescan: already-known files are skipped, not duplicated.
	res2, err := sc.ScanSeriesFolder(ctx, seriesID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Scanned != 0 || len(res2.Skipped) != 4 {
		t.Errorf("rescan should skip all: %+v", res2)
	}

	// Lowest owned chapter feeds the import cutoff default (§11.4).
	lowest, err := sc.LowestOwnedChapters(ctx, seriesID)
	if err != nil || lowest[chapters.ID] != 118000 {
		t.Errorf("lowest owned chapter: want 118000, got %v (%v)", lowest, err)
	}
}
