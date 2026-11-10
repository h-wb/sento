package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

// staleFacts gives a scanned file the provenance, facts and claims an
// import and an older parser would have left, so the next scan re-reads it.
// Inlined values go straight to the driver, bypassing the store's ?->$N
// rebind.
func staleFacts(t *testing.T, conn *sql.DB, id int64, provenance string, facts, claims store.AssetFacts) {
	t.Helper()
	quote := func(v any) string {
		raw, _ := json.Marshal(v)
		return strings.ReplaceAll(string(raw), "'", "''")
	}
	if _, err := conn.ExecContext(context.Background(), fmt.Sprintf(`UPDATE files SET parser_version=0, provenance='%s', facts_json='%s', claims_json='%s' WHERE id=%d`, provenance, quote(facts), quote(claims), id)); err != nil {
		t.Fatal(err)
	}
}

// A parser re-read replaces what a name says and nothing else: the download
// a file came in, its site, profile, score and claims stand, and facts that
// were not read from a name still known are left as they are.
func TestScanRederiveKeepsWhatANameDoesNotSay(t *testing.T) {
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
	const (
		reread  = "ONE PIECE v01 (Digital) (Colored Council).cbz" // read from its own name
		renamed = "ONE PIECE v02 [Standard Edition][en].cbz"      // renamed at import: the name lost what was read
		offer   = "ONE PIECE v03 [Standard Edition][en].cbz"      // facts from the offer, not from a name
		bare    = "ONE PIECE v04 [Standard Edition][en].cbz"      // renamed, and its payload named no group
	)
	names := []string{reread, renamed, offer, bare}
	for _, name := range names {
		writeCBZ(t, filepath.Join(dir, name), name)
	}
	sc := NewScanner(st, dir)
	if _, err := sc.ScanSeriesFolder(ctx, seriesID, dir); err != nil {
		t.Fatal(err)
	}
	files := mustFiles(t, st, seriesID)
	if len(files) != len(names) {
		t.Fatalf("scanned %d files", len(files))
	}
	claims := store.AssetFacts{Title: "ONE PIECE v01-04 (Digital) (1r0n)", Group: "1r0n", ReleaseKey: "nyaa:42", FactsKnown: true}
	filename := map[string]string{"language": "filename", "group": "filename", "quality": "filename"}
	want := map[string]store.AssetFacts{
		// The old parser read the group as a color claim.
		reread: {Sources: filename, Title: reread, Site: "Nyaa", Protocol: "torrent", FactsKnown: true, Group: "Colored Council", Flags: map[string]bool{"colored": true, "digital": true}, Digital: true, Format: "cbz", ProfileID: 3, FormatScore: 120, ReleaseKey: "nyaa:42"},
		// Its facts came from "… (Digital) (1r0n).cbz".
		renamed: {Sources: filename, Title: renamed, Site: "Nyaa", Protocol: "torrent", FactsKnown: true, Group: "1r0n", Flags: map[string]bool{"digital": true}, Digital: true, Resolution: 2048, Format: "cbz", ProfileID: 3, ReleaseKey: "nyaa:42"},
		// A source download keeps its offer's evidence.
		offer: {Sources: map[string]string{"language": "offer", "group": "offer", "quality": "offer"}, Title: "ONE PIECE Vol. 3", Site: "MangaDex", Protocol: "source", FactsKnown: true, Group: "TCB Scans", Language: "en", Format: "cbz", ReleaseKey: "src:7"},
		// Nothing in the facts gives the rename away, so the name is
		// re-read, but its brackets are not taken for a group and a language.
		bare: {Sources: filename, Title: bare, Site: "Nyaa", Protocol: "torrent", FactsKnown: true, Flags: map[string]bool{"colored": true}, Format: "cbz", ReleaseKey: "nyaa:42"},
	}
	for _, f := range files {
		staleFacts(t, conn, f.ID, "torrent", want[filepath.Base(f.Path)], claims)
	}

	if _, err := sc.ScanSeriesFolder(ctx, seriesID, dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range mustFiles(t, st, seriesID) {
		name := filepath.Base(f.Path)
		before := want[name]
		if f.ParserVersion != parse.Version {
			t.Errorf("%s: parser_version=%d", name, f.ParserVersion)
		}
		if f.Claims.ReleaseKey != claims.ReleaseKey || f.Claims.Group != claims.Group {
			t.Errorf("%s: claims lost: %+v", name, f.Claims)
		}
		if f.Facts.ReleaseKey != before.ReleaseKey || f.Facts.Site != before.Site || f.Facts.Protocol != before.Protocol || f.Facts.ProfileID != before.ProfileID || f.Facts.FormatScore != before.FormatScore || f.Facts.Title != before.Title {
			t.Errorf("%s: what the name does not say changed:\n got %+v\nwant %+v", name, f.Facts, before)
		}
		switch name {
		case reread:
			if f.Facts.Flags["colored"] || !f.Facts.Flags["digital"] || !f.Facts.Digital || f.Facts.Group != "Colored Council" {
				t.Errorf("%s: name not re-read: %+v", name, f.Facts)
			}
		case bare:
			if f.Facts.Flags["colored"] || f.Facts.Group != "" || f.Facts.Language != "" {
				t.Errorf("%s: re-read should drop the stale flag and add nothing: %+v", name, f.Facts)
			}
		default:
			if !reflect.DeepEqual(f.Facts, before) {
				t.Errorf("%s: facts not read from this name were rewritten:\n got %+v\nwant %+v", name, f.Facts, before)
			}
		}
	}
}

// A downloaded file whose facts were rebuilt from its Notes line, losing
// the download it came in and its claims, gets both back from the series'
// download of that name; a name two releases share restores nothing.
func TestScanRestoresALostReleaseLink(t *testing.T) {
	t.Parallel()
	conn, dialect := dbtest.Open(t)
	st := store.New(conn, dialect)
	lib := NewLibrary(st, testProvider())
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}
	queue := func(title, key string) {
		t.Helper()
		snap, _ := json.Marshal(ReleaseCandidate{Title: title, Site: "Nyaa", ProfileID: 3})
		id, err := st.AddQueueItem(ctx, store.QueueItem{SeriesID: seriesID, Protocol: "torrent", Title: title, Books: []byte(`[]`)})
		if err != nil {
			t.Fatal(err)
		}
		// AddQueueItem records neither; inlined, as in staleFacts.
		if _, err := conn.ExecContext(ctx, fmt.Sprintf(`UPDATE queue_items SET release_key='%s', snapshot_json='%s' WHERE id=%d`, key, strings.ReplaceAll(string(snap), "'", "''"), id)); err != nil {
			t.Fatal(err)
		}
	}
	queue("ONE PIECE v01 (Digital) (1r0n)", "nyaa:1")
	queue("ONE PIECE v02 (Digital) (1r0n)", "nyaa:2")
	queue("ONE PIECE v02 (Digital) (1r0n)", "nyaa:2-repost")

	dir := t.TempDir()
	names := []string{"ONE PIECE v01 [Standard Edition][en].cbz", "ONE PIECE v02 [Standard Edition][en].cbz", "ONE PIECE v03 [Standard Edition][en].cbz"}
	for _, name := range names {
		writeCBZ(t, filepath.Join(dir, name), name)
	}
	sc := NewScanner(st, dir)
	if _, err := sc.ScanSeriesFolder(ctx, seriesID, dir); err != nil {
		t.Fatal(err)
	}
	comicinfo := map[string]string{"language": "comicinfo", "group": "comicinfo", "quality": "comicinfo"}
	titles := map[string]string{names[0]: "ONE PIECE v01 (Digital) (1r0n)", names[1]: "ONE PIECE v02 (Digital) (1r0n)", names[2]: "ONE PIECE v03 (Digital) (1r0n)"}
	for _, f := range mustFiles(t, st, seriesID) {
		lost := store.AssetFacts{Sources: comicinfo, Title: titles[filepath.Base(f.Path)], Protocol: "torrent", FactsKnown: true, Group: "1r0n", Format: "cbz"}
		staleFacts(t, conn, f.ID, "torrent", lost, store.AssetFacts{})
	}
	for range 2 { // the second scan finds nothing left to restore
		if _, err := sc.ScanSeriesFolder(ctx, seriesID, dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range mustFiles(t, st, seriesID) {
		wantKey := map[string]string{names[0]: "nyaa:1"}[filepath.Base(f.Path)]
		if f.Facts.ReleaseKey != wantKey || f.Claims.ReleaseKey != wantKey {
			t.Errorf("%s: release key facts=%q claims=%q, want %q", filepath.Base(f.Path), f.Facts.ReleaseKey, f.Claims.ReleaseKey, wantKey)
		}
		if wantKey != "" && (f.Facts.Site != "Nyaa" || f.Claims.Title != titles[filepath.Base(f.Path)] || f.Facts.Group != "1r0n" || f.Facts.Sources["group"] != "comicinfo") {
			t.Errorf("%s: restored facts %+v, claims %+v", filepath.Base(f.Path), f.Facts, f.Claims)
		}
	}
}
