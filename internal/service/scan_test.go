package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/parse"
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

func TestResolutionScoresReleasesAndScannedFiles(t *testing.T) {
	t.Parallel()
	_, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	if err := NewProfiles(st).EnsureDefaults(ctx); err != nil {
		t.Fatal(err)
	}
	profiles, err := st.ListProfiles(ctx)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("default profile: %+v, %v", profiles, err)
	}
	formats, err := st.ListFormats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prof := &profiles[0]
	dir := t.TempDir()
	cases := []struct {
		name       string
		resolution int
		revision   int
		score      int
	}{
		{"ONE PIECE v01 (Digital) (danke-Empire) (1920px).cbz", 1920, 0, 275},
		{"ONE PIECE v02 (Digital) (danke-Empire) (1800px-Repack).cbz", 1800, 2, 275},
		{"ONE PIECE v03 (Digital) (danke-Empire).cbz", 0, 0, 250},
	}
	for _, tc := range cases {
		candidate := ReleaseCandidate{Title: tc.name, Protocol: "torrent", Parse: parse.Parse(tc.name, parse.ChannelTorrent)}
		candidate.evalFormats(prof, formats)
		if candidate.FormatScore != tc.score {
			t.Errorf("%s: release score = %d, want %d; formats: %+v", tc.name, candidate.FormatScore, tc.score, candidate.Formats)
		}
		writeCBZ(t, filepath.Join(dir, tc.name), tc.name)
	}
	res, err := NewScanner(st, dir).ScanSeriesFolder(ctx, sid, dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != len(cases) || res.Linked != len(cases) {
		t.Fatalf("scan: %+v", res)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, file := range files {
				if filepath.Base(file.Path) != tc.name {
					continue
				}
				facts := file.Facts
				if facts.Resolution != tc.resolution || facts.Group != "danke-Empire" || facts.Revision != tc.revision {
					t.Errorf("persisted facts: %+v", facts)
				}
				if j := assetJudgement(facts, prof, formats); j.Score != tc.score {
					t.Errorf("file score = %d, want %d; formats: %+v", j.Score, tc.score, j.Matched)
				}
				return
			}
			t.Fatal("scanned file missing")
		})
	}
}
