package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/db/dbtest"
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
		{"ONE PIECE v01 (Digital) (danke-Empire) (10K).cbz", 10240, 0, 275},
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

// When the parser version moves on, a scan re-reads a known file's name and
// re-derives its filename-sourced facts (a group name like "Colored Council"
// no longer claims color), while hash, links, review state and manual fact
// corrections survive.
func TestScanRederivesFactsWhenParserVersionMovesOn(t *testing.T) {
	t.Parallel()
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	name := "ONE PIECE v01 (Colored Council).cbz"
	path := filepath.Join(dir, name)
	writeCBZ(t, path, name)

	sc := NewScanner(st, dir)
	if _, err := sc.ScanSeriesFolder(ctx, seriesID, dir); err != nil {
		t.Fatal(err)
	}
	before, err := st.ListFiles(ctx, seriesID)
	if err != nil || len(before) != 1 {
		t.Fatalf("scan recorded %+v, %v", before, err)
	}
	f0 := before[0]
	if f0.Facts.Flags["colored"] {
		t.Fatal("current parser should not read the group name as color")
	}
	if f0.ParserVersion != parse.Version {
		t.Fatalf("stamped parser_version=%d, want %d", f0.ParserVersion, parse.Version)
	}

	// Simulate a fact recorded by an older parser (color read from the
	// group name), a manual language correction, and a recorded review
	// decision — the re-derivation must update the facts but keep both
	// human marks. Inline the id: this goes straight to the driver,
	// bypassing the store's ?->$N rebind, so a placeholder fails on PG.
	facts := f0.Facts
	flags := map[string]bool{}
	for k, v := range facts.Flags {
		flags[k] = v
	}
	flags["colored"] = true
	facts.Flags = flags
	facts.Language = "fr"
	facts.Sources["language"] = "manual"
	raw, _ := json.Marshal(facts)
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE files SET parser_version=0, facts_json='%s', review_action='keep' WHERE id=%d`, strings.ReplaceAll(string(raw), "'", "''"), f0.ID)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf(`INSERT INTO asset_contents(file_id,book_id,verified,evidence) SELECT file_id,book_id,TRUE,'manual' FROM asset_contents WHERE file_id=%d ON CONFLICT(file_id,book_id) DO UPDATE SET evidence='manual',verified=TRUE`, f0.ID)); err != nil {
		t.Fatal(err)
	}

	res, err := sc.ScanSeriesFolder(ctx, seriesID, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 || res.Scanned != 0 {
		t.Fatalf("re-derive pass: %+v", res)
	}
	after, err := st.ListFiles(ctx, seriesID)
	if err != nil || len(after) != 1 {
		t.Fatalf("after re-derive: %+v, %v", after, err)
	}
	f1 := after[0]
	if f1.Facts.Flags["colored"] {
		t.Error("stale colored flag survived re-derivation")
	}
	if f1.ParserVersion != parse.Version {
		t.Errorf("parser_version=%d, want %d", f1.ParserVersion, parse.Version)
	}
	if f1.Hash != f0.Hash {
		t.Error("fingerprint changed on a facts-only re-derivation")
	}
	if f1.Facts.Language != "fr" || f1.Facts.Sources["language"] != "manual" {
		t.Errorf("manual correction lost: %+v", f1.Facts)
	}
	if f1.ReviewAction != "keep" {
		t.Errorf("review action lost on re-derivation: %q", f1.ReviewAction)
	}

	// A settled stamp is left alone on the next scan.
	res2, err := sc.ScanSeriesFolder(ctx, seriesID, dir)
	if err != nil || len(res2.Skipped) != 1 || res2.Scanned != 0 {
		t.Fatalf("second scan should skip: %+v, %v", res2, err)
	}
}
