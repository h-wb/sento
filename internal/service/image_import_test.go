package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func writePage(t *testing.T, path string, shade uint8) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	im := image.NewGray(image.Rect(0, 0, 2, 2))
	im.SetGray(0, 0, color.Gray{Y: shade})
	if err = png.Encode(f, im); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLoosePagesImportInNaturalOrderAndKeepOriginals(t *testing.T) {
	_, st, id, col, books, version := modelFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	root := t.TempDir()
	files := []string{"Volume 1/page10.png", "Volume 1/page2.png", "Volume 1/page1.png"}
	for i, f := range files {
		writePage(t, filepath.Join(dir, f), uint8(10+i))
	}
	claims, _ := json.Marshal([]store.BookRef{modelRef(col, books[0], version)})
	im := NewImporter(st, provider.NewRegistry(), root, nil)
	if err := im.importFiles(ctx, store.QueueItem{SeriesID: id, Title: "ONE PIECE v01", Protocol: "torrent", Books: claims}, dir, files); err != nil {
		t.Fatal(err)
	}
	owned, err := st.ListFiles(ctx, id)
	if err != nil || len(owned) != 1 {
		t.Fatalf("files: %+v %v", owned, err)
	}
	zr, err := zip.OpenReader(owned[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	var shades []uint8
	for _, f := range zr.File {
		if filepath.Ext(f.Name) != ".png" {
			continue
		}
		rc, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		page, e := png.Decode(rc)
		_ = rc.Close()
		if e != nil {
			t.Fatal(e)
		}
		shades = append(shades, color.GrayModel.Convert(page.At(0, 0)).(color.Gray).Y)
	}
	if len(shades) != 3 || shades[0] != 12 || shades[1] != 11 || shades[2] != 10 {
		t.Fatalf("page order: %v", shades)
	}
	for _, f := range files {
		if _, e := os.Stat(filepath.Join(dir, f)); e != nil {
			t.Fatal("seeding original removed", e)
		}
	}
}

func TestLoosePagesAmbiguityAndEscapingPaths(t *testing.T) {
	for _, folder := range []string{"1", "Part 1", "ONE PIECE v01-v02"} {
		t.Run(folder, func(t *testing.T) {
			_, st, id, col, books, version := modelFixture(t)
			dir := t.TempDir()
			rel := filepath.Join(folder, "page.png")
			writePage(t, filepath.Join(dir, rel), 1)
			claims, _ := json.Marshal([]store.BookRef{modelRef(col, books[0], version)})
			im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
			if err := im.importFiles(context.Background(), store.QueueItem{SeriesID: id, Protocol: "torrent", Books: claims}, dir, []string{rel}); err == nil {
				t.Fatal("ambiguous image folder claimed a book")
			}
			coverage, _ := st.Coverage(context.Background(), id)
			if len(coverage) != 0 {
				t.Fatalf("unexpected coverage: %+v", coverage)
			}
		})
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "page.png")
	writePage(t, outside, 1)
	if err := os.Symlink(outside, filepath.Join(dir, "page.png")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"page.png", "../page.png", outside} {
		_, cleanup, err := payloadArchives(context.Background(), dir, []string{rel}, []string{"ONE PIECE"})
		cleanup()
		if err == nil {
			t.Fatalf("accepted escaping path %q", rel)
		}
	}
}

func TestImageArchiveRejectsInvalidPageAndCleansStage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "001.png")
	dest := filepath.Join(dir, "book.cbz")
	if err := os.WriteFile(src, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := mediafile.WriteImageArchive(context.Background(), dest, []string{src}); err == nil {
		t.Fatal("invalid page accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("partial archive left behind")
	}
}

func TestOverlappingPackImportKeepsFullContentsButClaimsSubset(t *testing.T) {
	_, st, id, col, books, version := modelFixture(t)
	ctx := context.Background()
	dir := t.TempDir()
	name := "ONE PIECE v01-v03 (Digital) [English].cbz"
	writeCBZ(t, filepath.Join(dir, name), "pack")
	claims, _ := json.Marshal([]store.BookRef{modelRef(col, books[2], version)})
	im := NewImporter(st, provider.NewRegistry(), t.TempDir(), nil)
	if err := im.importFiles(ctx, store.QueueItem{SeriesID: id, Protocol: "torrent", Title: name, Books: claims}, dir, []string{name}); err != nil {
		t.Fatal(err)
	}
	contents, _ := st.Contents(ctx, id)
	coverage, _ := st.Coverage(ctx, id)
	if len(contents) != 3 || len(coverage) != 3 {
		t.Fatalf("contents: %+v coverage: %+v", contents, coverage)
	}
	files, _ := st.ListFiles(ctx, id)
	zr, err := zip.OpenReader(files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		if f.Name == "ComicInfo.xml" {
			rc, _ := f.Open()
			var info mediafile.ComicInfo
			err := xml.NewDecoder(rc).Decode(&info)
			_ = rc.Close()
			if err != nil || info.Title != "Volumes 1–3" || info.Number != "" {
				t.Fatalf("pack was mislabeled as its claimed subset: %+v %v", info, err)
			}
		}
	}
}
