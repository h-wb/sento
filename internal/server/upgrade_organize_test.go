package server

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

// A displaced file is moved to review/ and its DB path follows it, so
// organizing the winning file afterwards never lands on a path another row
// records (the files.path unique index).
func TestUpgradeDisplacementThenOrganizeDoesNotCollide(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	srv := New(cfg, conn, dialect, noMetadata{})
	sid, err := srv.store.AddSeries(ctx, store.AddSeriesParams{Series: &metadata.Series{MangaBakaID: 6766, Title: "Yotsuba&!", Type: "manga"}})
	if err != nil {
		t.Fatal(err)
	}
	eid, err := srv.store.AddCustomEdition(ctx, store.Edition{SeriesID: sid, EditionName: "Standard Edition", Language: "en", Type: "volume"}, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := srv.store.ListVersions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	var versionID int64
	for _, v := range versions {
		if v.EditionID == eid {
			versionID = v.ID
		}
	}
	books, err := srv.store.ListBooks(ctx, eid)
	if err != nil {
		t.Fatal(err)
	}
	slot := store.BookRef{VersionID: versionID, EditionID: eid, BookID: books[0].ID, Ledger: "volume", Number: books[0].SequenceNum}

	review := filepath.Join(cfg.Library.Path, "Yotsuba review")
	if err := os.MkdirAll(review, 0o755); err != nil {
		t.Fatal(err)
	}
	add := func(name string) (int64, string) {
		t.Helper()
		if err := os.MkdirAll(review, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(review, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		page, err := zw.Create("001.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = page.Write([]byte("pages " + name)); err != nil {
			t.Fatal(err)
		}
		if err = zw.Close(); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		fid, err := srv.store.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name, FactsKnown: true}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fid, path
	}

	// The first file claims the book and is organized into the book folder.
	first, _ := add("Yotsuba v01.cbz")
	if _, err := srv.importer.ApplyUpgrades(ctx, sid, first, []store.BookRef{slot}, false); err != nil {
		t.Fatal(err)
	}
	if err := srv.importer.OrganizeReviewedFile(ctx, sid, first); err != nil {
		t.Fatal(err)
	}
	// The second (better, forced) file takes the book; the first is exiled.
	second, _ := add("Yotsuba v01 (Digital).cbz")
	res, err := srv.importer.ApplyUpgrades(ctx, sid, second, []store.BookRef{slot}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replaced) != 1 || res.Replaced[0].FileID != first {
		t.Fatalf("expected first file displaced, got %+v", res.Replaced)
	}
	// Organizing the winner must not collide with the exiled file's path.
	if err := srv.importer.OrganizeReviewedFile(ctx, sid, second); err != nil {
		t.Fatalf("organizing the winning file failed: %v", err)
	}
	files, err := srv.store.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int64{}
	var firstPath, secondPath string
	for _, f := range files {
		if other, dup := seen[f.Path]; dup {
			t.Fatalf("paths not unique: files %d and %d both at %s", other, f.ID, f.Path)
		}
		seen[f.Path] = f.ID
		if f.ID == first {
			firstPath = f.Path
		}
		if f.ID == second {
			secondPath = f.Path
		}
	}
	if !strings.Contains(firstPath, "review") {
		t.Fatalf("displaced file not exiled to review/: %s", firstPath)
	}
	if strings.Contains(secondPath, "review") {
		t.Fatalf("winning file still in review/: %s", secondPath)
	}
	for _, f := range files {
		if _, err := os.Stat(f.Path); err != nil {
			t.Fatalf("db path missing on disk: %s", f.Path)
		}
	}
}
