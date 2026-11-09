package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/provider"
)

// A library run previews and applies the new settings series by series,
// trusts the stored fingerprints, and has nothing left to do afterwards.
func TestOrganizeLibrary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	lib, st := setup(t)
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cols, _ := st.ListEditions(ctx, seriesID)
	var standard int64
	for _, c := range cols {
		if c.EditionName == "Standard Edition" {
			standard = c.ID
		}
	}
	downloads, library := t.TempDir(), t.TempDir()
	files := []string{"ONE PIECE v01.cbz", "ONE PIECE v02.cbz"}
	for _, name := range files {
		writeCBZ(t, filepath.Join(downloads, name), name)
	}
	old := DefaultMediaSettings()
	old.BookFolder = BookFolderByID
	if err := SaveMediaSettings(ctx, st, old); err != nil {
		t.Fatal(err)
	}
	im := NewImporter(st, provider.NewRegistry(&payloadProvider{dir: downloads, files: files}), library, nil)
	queueVolumes(t, st, seriesID, standard, "abc123", 100, 200)
	if n, err := im.ProcessQueue(ctx); err != nil || n != 1 {
		t.Fatalf("import: %d %v", n, err)
	}
	before, _ := st.ListFiles(ctx, seriesID)
	// A stored fingerprint is not computed again.
	stored := strings.Repeat("e", 64)
	if err := st.SetFileLocation(ctx, before[0].ID, before[0].Path, before[0].Size, stored); err != nil {
		t.Fatal(err)
	}
	if err := SaveMediaSettings(ctx, st, DefaultMediaSettings()); err != nil {
		t.Fatal(err)
	}

	plans, err := im.PlanOrganizeLibrary(ctx, nil)
	if err != nil || len(plans) != 1 || plans[0].SeriesID != seriesID || len(plans[0].Changes) != 2 {
		t.Fatalf("library plan: %+v %v", plans, err)
	}
	if plans, err = im.PlanOrganizeLibrary(ctx, []int64{seriesID + 1000}); err != nil || len(plans) != 0 {
		t.Fatalf("plan for other series: %+v %v", plans, err)
	}

	if _, err := im.QueueLibraryOrganize(LibraryOrganizeRequest{}); !errors.Is(err, apperr.ErrInvalid) {
		t.Fatalf("empty request: %v", err)
	}
	if q, err := im.QueueLibraryOrganize(LibraryOrganizeRequest{SeriesIDs: []int64{seriesID}, Rename: true}); err != nil || !q.Queued {
		t.Fatalf("queue: %+v %v", q, err)
	}
	if _, err := im.QueueLibraryOrganize(LibraryOrganizeRequest{Rename: true}); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("second queue: %v", err)
	}
	if err := im.RunLibraryOrganize(ctx); err != nil {
		t.Fatal(err)
	}
	status := im.LibraryOrganizeStatus()
	if status.Running || status.Queued || status.Total != 1 || status.Done != 1 || status.Renamed != 2 || status.Retagged != 0 ||
		len(status.Series) != 1 || status.Series[0].Renamed != 2 || status.FinishedAt == "" {
		t.Fatalf("status: %+v", status)
	}
	after, _ := st.ListFiles(ctx, seriesID)
	for _, f := range after {
		if filepath.Dir(f.Path) != filepath.Join(library, "ONE PIECE [mangabaka-377]", "Standard Edition [en]") {
			t.Errorf("new path %s", f.Path)
		}
		if f.ID == before[0].ID && f.Hash != stored {
			t.Errorf("fingerprint recomputed: %s", f.Hash)
		}
	}
	// Resumable and idempotent: the next run finds nothing to move.
	if _, err := im.QueueLibraryOrganize(LibraryOrganizeRequest{Rename: true}); err != nil {
		t.Fatal(err)
	}
	if err := im.RunLibraryOrganize(ctx); err != nil {
		t.Fatal(err)
	}
	if status = im.LibraryOrganizeStatus(); status.Done != 1 || status.Renamed != 0 || len(status.Series) != 0 {
		t.Fatalf("second run: %+v", status)
	}
}
