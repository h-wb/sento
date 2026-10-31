package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// A file Sento tagged is adopted back from its tags: the right book and
// the release's facts, whatever its name now says (docs/domain/metadata.md).
func TestRescanAdoptsATaggedFileFromItsNotes(t *testing.T) {
	_, st, sid, c, ws, version := modelFixture(t)
	ctx := context.Background()
	series, err := st.GetSeries(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, nil, t.TempDir(), nil)
	info := im.fileInfo(ctx, series, []store.BookRef{modelRef(c, ws[1], version)}, tagSource{
		Release: "ONE PIECE v02 (Digital) (1r0n)", Group: "1r0n", Via: "torrent", Site: "Nyaa",
		Flags: map[string]bool{"digital": true, "raw": false},
	})
	if info.Publisher != c.Publisher || info.Translator != "" || info.ScanInformation != "1r0n" {
		t.Fatalf("published volume tagged as %+v", info)
	}
	if !strings.Contains(info.Notes, "books:2") || !strings.Contains(info.Notes, "flags:digital") {
		t.Fatalf("notes = %q", info.Notes)
	}

	dir := t.TempDir()
	if links := NewScanner(st, t.TempDir()).linksFor(ctx, newShelf([]store.Edition{c}), parse.ParseAnchored("scan-0042.cbz", parse.ChannelFile, []string{"ONE PIECE"}), []store.Edition{c}, nil); len(links) > 0 {
		t.Fatal("the name alone already places the file")
	}
	raw := filepath.Join(t.TempDir(), "raw.cbz")
	writeCBZ(t, raw, "volume two")
	// A name that says nothing: only the tags can place it.
	if err = mediafile.Process(raw, filepath.Join(dir, "scan-0042.cbz"), &info); err != nil {
		t.Fatal(err)
	}
	if _, err = NewScanner(st, t.TempDir()).ScanSeriesFolder(ctx, sid, dir); err != nil {
		t.Fatal(err)
	}
	files, err := st.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	var f *store.File
	for i := range files {
		if filepath.Base(files[i].Path) == "scan-0042.cbz" {
			f = &files[i]
		}
	}
	if f == nil || f.NeedsReview {
		t.Fatalf("tagged file not adopted: %+v", f)
	}
	if f.Facts.Group != "1r0n" || f.Facts.Site != "Nyaa" || !f.Facts.Flags["digital"] || f.Facts.Title != "ONE PIECE v02 (Digital) (1r0n)" {
		t.Fatalf("release facts lost: %+v", f.Facts)
	}
	coverage, err := st.Coverage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	linked := false
	for _, a := range coverage {
		if a.FileID == f.ID {
			linked = a.BookID == ws[1].ID
		}
	}
	if !linked {
		t.Fatalf("file not linked to volume 2: %+v", coverage)
	}
}

// Publisher means the publisher's book: a scanlator's chapter credits the
// group as translator instead, unless the release names a publisher.
func TestTagsTellPublishedFromFanTranslated(t *testing.T) {
	_, st, sid, c, ws, version := modelFixture(t)
	ctx := context.Background()
	series, _ := st.GetSeries(ctx, sid)
	im := NewImporter(st, nil, t.TempDir(), nil)
	if c.Publisher == "" {
		t.Fatal("the fixture edition needs a publisher")
	}
	slots := []store.BookRef{modelRef(c, ws[0], version)}
	fan := im.fileInfo(ctx, series, slots, tagSource{Group: "Some Scans", Via: "source"})
	if fan.Publisher != "" || fan.Translator != "Some Scans" {
		t.Fatalf("fan translation tagged as %+v", fan)
	}
	official := im.fileInfo(ctx, series, slots, tagSource{Group: "MANGA Plus", Via: "source", Flags: map[string]bool{"official": true}})
	if official.Publisher != c.Publisher || official.Translator != "" {
		t.Fatalf("official source tagged as %+v", official)
	}
}

func TestEditionFormat(t *testing.T) {
	for name, want := range map[string]string{
		"Standard Edition": "", "Chapters": "", "Deluxe Edition": "Deluxe", "Box Set Edition": "Box Set",
		"Full Color Edition": "Full Color", "Fullmetal Edition": "Fullmetal", "Kanzenban": "Kanzenban",
	} {
		if got := editionFormat(name); got != want {
			t.Errorf("%q: Format %q, want %q", name, got, want)
		}
	}
}

// A folder of files Sento tagged is identified by them before its name is
// looked up; files that disagree decide nothing.
func TestImportProposalsReadTaggedFiles(t *testing.T) {
	lib, _ := setup(t)
	root := t.TempDir()
	tag := func(dir, name string, id int64, series string) {
		t.Helper()
		raw := filepath.Join(t.TempDir(), "raw.cbz")
		writeCBZ(t, raw, dir+name)
		info := mediafile.ComicInfo{Series: series, Notes: mediafile.Provenance{MangaBaka: id}.Notes()}
		if err := mediafile.Process(raw, filepath.Join(root, dir, "Volumes", name), &info); err != nil {
			t.Fatal(err)
		}
	}
	tag("old backup 2019", "a.cbz", 377, "ONE PIECE")
	tag("old backup 2019", "b.cbz", 377, "ONE PIECE")
	tag("mixed", "a.cbz", 9001, "One")
	tag("mixed", "b.cbz", 9002, "Other")
	proposals, err := lib.ProposeImports(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range proposals {
		switch filepath.Base(p.Dir) {
		case "old backup 2019":
			if p.BestID != 377 || p.Confidence != 1 || len(p.Candidates) != 1 || p.Candidates[0].Title != "ONE PIECE" {
				t.Errorf("tagged folder proposed as %+v", p)
			}
		case "mixed":
			if p.BestID == 9001 || p.BestID == 9002 {
				t.Errorf("disagreeing tags decided the series: %+v", p)
			}
		}
	}
}
