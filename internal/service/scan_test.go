package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/ledger"
	"github.com/h-wb/sento/internal/store"
)

func TestScanSeriesFolder(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, MonitorAll: true})
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

	// Volume slots 1+2 satisfied now.
	cols, _ := st.ListCollections(ctx, seriesID)
	var standard store.Collection
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c
		}
	}
	works, _ := st.ListWorks(ctx, standard.ID)
	links, _ := st.SlotLinks(ctx, seriesID)
	slots := ledger.BuildVolumeSlots(standard, works, links, nil, time.Now())
	if len(slots[0].FileIDs) == 0 || len(slots[1].FileIDs) == 0 {
		t.Errorf("vols 1-2 should be satisfied after scan: %+v", slots[:2])
	}

	// Chapter 1180 linked.
	if len(links[store.SlotKey("chapter", 0, 118000)]) != 1 {
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
	lowest, err := sc.LowestOwnedChapter(ctx, seriesID)
	if err != nil || lowest != 118000 {
		t.Errorf("lowest owned chapter: want 118000, got %d (%v)", lowest, err)
	}
}
