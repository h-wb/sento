package server

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

func TestReviewMappingOrganizesOnlyConfirmedFile(t *testing.T) {
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
	if versionID == 0 {
		t.Fatal("custom edition has no version")
	}
	books, err := srv.store.ListBooks(ctx, eid)
	if err != nil {
		t.Fatal(err)
	}
	review := filepath.Join(cfg.Library.Path, "Yotsuba review")
	if err = os.MkdirAll(review, 0o755); err != nil {
		t.Fatal(err)
	}
	add := func(name string) (int64, string) {
		t.Helper()
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
		fid, err := srv.store.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fid, path
	}
	fid, oldPath := add("Yotsuba v01.cbz")
	otherID, otherPath := add("Yotsuba v02.cbz")
	mapping := func(book store.Book) string {
		t.Helper()
		body, err := json.Marshal(FileMappingBody{Books: []store.BookRef{{VersionID: versionID, EditionID: eid, BookID: book.ID, Ledger: "volume", Number: book.SequenceNum}}})
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	h := srv.Handler()
	save := func(fileID int64, book store.Book) (int, string) {
		t.Helper()
		rec := do(t, h, http.MethodPut, fmt.Sprintf("/api/v1/series/%d/files/%d/mapping", sid, fileID), mapping(book))
		return rec.Code, rec.Body.String()
	}
	if code, body := save(fid, books[0]); code != http.StatusNoContent {
		t.Fatalf("save: %d %s", code, body)
	}
	files, err := srv.store.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		switch f.ID {
		case fid:
			if f.Path == oldPath || f.NeedsReview {
				t.Fatalf("confirmed file not organized: %+v", f)
			}
			info := mediafile.ReadComicInfo(f.Path)
			if info == nil || info.Series != "Yotsuba&!" || info.Volume != "1" {
				t.Fatalf("metadata not written: %+v", info)
			}
			if _, err = os.Stat(oldPath); !os.IsNotExist(err) {
				t.Fatalf("old review path remains: %v", err)
			}
		case otherID:
			if f.Path != otherPath || !f.NeedsReview {
				t.Fatalf("unrelated file changed: %+v", f)
			}
		}
	}
	if code, body := save(fid, books[0]); code != http.StatusNoContent {
		t.Fatalf("repeat save: %d %s", code, body)
	}
	// A file move failure must be visible while preserving the saved mapping.
	if err = os.Remove(otherPath); err != nil {
		t.Fatal(err)
	}
	if code, body := save(otherID, books[1]); code != http.StatusInternalServerError || !strings.Contains(body, "mapping saved, but organizing the file failed") {
		t.Fatalf("organize failure hidden: %d %s", code, body)
	}
	coverage, err := srv.store.Coverage(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range coverage {
		if c.FileID == otherID && c.BookID == books[1].ID && c.Verified {
			found = true
		}
	}
	if !found {
		t.Fatal("organize failure lost the saved mapping")
	}
}
