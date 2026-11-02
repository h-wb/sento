package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

func TestFileCleanupAPIRequiresSelectionAndScopesRecovery(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	s := New(cfg, conn, dialect, noMetadata{})
	h := s.Handler()
	sid, err := s.store.AddSeries(ctx, store.AddSeriesParams{Series: &metadata.Series{MangaBakaID: 123456, Title: "Example"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg.Library.Path, "unused.cbz")
	if err = os.WriteFile(path, []byte("original pages"), 0o600); err != nil {
		t.Fatal(err)
	}
	fid, err := s.store.AddFile(ctx, store.File{SeriesID: sid, Path: path, Size: 14}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/api/v1/series/%d/files", sid)
	rec := do(t, h, "GET", base+"/cleanup", "")
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var files []service.CleanupFile
	if err = json.Unmarshal(rec.Body.Bytes(), &files); err != nil || len(files) != 1 || !files[0].Eligible {
		t.Fatal(files, err)
	}
	if rec = do(t, h, "POST", base+"/cleanup", `{"fileIds":[],"retentionDays":30}`); rec.Code != 400 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("preview or empty selection moved bytes", err)
	}
	body := fmt.Sprintf(`{"fileIds":[%d],"retentionDays":30}`, fid)
	if rec = do(t, h, "POST", base+"/cleanup", body); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if rec = do(t, h, "GET", base+"/trash", ""); rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var trash []store.TrashedFile
	if err = json.Unmarshal(rec.Body.Bytes(), &trash); err != nil || len(trash) != 1 {
		t.Fatal(trash, err)
	}
	wrong := fmt.Sprintf("/api/v1/series/%d/files/trash/%d", sid+1000, fid)
	if rec = do(t, h, "POST", wrong+"/restore", ""); rec.Code != 404 {
		t.Fatal("cross-series restore", rec.Code, rec.Body.String())
	}
	if rec = do(t, h, "DELETE", wrong, ""); rec.Code != 404 {
		t.Fatal("cross-series purge", rec.Code, rec.Body.String())
	}
	if rec = do(t, h, "POST", fmt.Sprintf("%s/trash/%d/restore", base, fid), ""); rec.Code != 204 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if bytes, err := os.ReadFile(path); err != nil || string(bytes) != "original pages" {
		t.Fatal("restore lost bytes", err)
	}
}
