package server

import (
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
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// The pack-level review endpoints behind the Review page's group actions:
// one decline resolves a pack at once, one match applies a card by each
// file's numbers, and both report what they left alone.
func TestReviewBulkEndpoints(t *testing.T) {
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
	var versionID int64
	for _, v := range mustVersions(t, srv, sid) {
		if v.EditionID == eid {
			versionID = v.ID
		}
	}
	books, err := srv.store.ListBooks(ctx, eid)
	if err != nil {
		t.Fatal(err)
	}
	add := func(name string) int64 {
		t.Helper()
		path := filepath.Join(cfg.Library.Path, name)
		if err = os.WriteFile(path, []byte("pages "+name), 0o600); err != nil {
			t.Fatal(err)
		}
		fid, err := srv.store.AddFile(ctx, store.File{SeriesID: sid, Path: path, NeedsReview: true, Unrequested: true, Presence: "present", Facts: store.AssetFacts{Title: name, ReleaseKey: "pack"}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return fid
	}
	v1 := add("Yotsuba v01.cbz")
	v2 := add("Yotsuba v02.cbz")
	extra := add("Yotsuba omake.cbz")
	h := srv.Handler()
	base := fmt.Sprintf("/api/v1/series/%d/review", sid)

	// Decline is explicit and bounded, like the cleanup batch.
	if rec := do(t, h, http.MethodPost, base+"/resolve", `{"fileIds":[]}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}
	body := fmt.Sprintf(`{"fileIds":[%d,%d,%d,999999]}`, v1, v2, extra)
	rec := do(t, h, http.MethodPost, base+"/resolve", body)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var resolution store.ReviewResolution
	if err = json.Unmarshal(rec.Body.Bytes(), &resolution); err != nil || len(resolution.Resolved) != 3 || len(resolution.Skipped) != 1 {
		t.Fatalf("resolution: %+v %v", resolution, err)
	}
	// The pack drained: its files no longer ask questions.
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/review?seriesId=%d", sid), "")
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Confirm the edition and books") {
		t.Fatal("declined files still in review", rec.Body.String())
	}

	// A fresh pack maps onto one card by each file's own numbers; the file
	// whose name says nothing stays for its own review.
	m1 := add("Yotsuba v02 (Digital).cbz")
	m2 := add("Yotsuba extras.cbz")
	body = fmt.Sprintf(`{"versionId":%d,"fileIds":[%d,%d]}`, versionID, m1, m2)
	rec = do(t, h, http.MethodPost, base+"/match", body)
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var matches []service.ReleaseFileMatch
	if err = json.Unmarshal(rec.Body.Bytes(), &matches); err != nil || len(matches) != 2 {
		t.Fatalf("matches: %+v %v", matches, err)
	}
	for _, m := range matches {
		switch m.FileID {
		case m1:
			if len(m.Books) != 1 || m.Books[0].BookID != books[1].ID || m.Label != "volume 2" {
				t.Errorf("v02 mapped to %+v (%q), want volume 2", m.Books, m.Label)
			}
		case m2:
			if len(m.Books) != 0 || m.Note == "" {
				t.Errorf("unreadable file mapped or left without a reason: %+v", m)
			}
		}
	}
	files, err := srv.store.ListFiles(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.ID == m1 && f.NeedsReview {
			t.Error("mapped file still waits for review")
		}
		if f.ID == m2 && !f.NeedsReview {
			t.Error("unreadable file was taken off review")
		}
	}
	// A card of another series is not this series' choice.
	other, err := srv.store.AddSeries(ctx, store.AddSeriesParams{Series: &metadata.Series{MangaBakaID: 42, Title: "Other", Type: "manga"}})
	if err != nil {
		t.Fatal(err)
	}
	body = fmt.Sprintf(`{"versionId":%d,"fileIds":[%d]}`, versionID, m2)
	rec = do(t, h, http.MethodPost, fmt.Sprintf("/api/v1/series/%d/review/match", other), body)
	if rec.Code != http.StatusNotFound {
		t.Fatal("cross-series match", rec.Code, rec.Body.String())
	}
}

func mustVersions(t *testing.T, srv *Server, sid int64) []store.CollectedVersion {
	t.Helper()
	versions, err := srv.store.ListVersions(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("no versions")
	}
	return versions
}
