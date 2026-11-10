package server

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/service"
)

// The settings page reads its token table and reader fields from the
// server, and a preview says which field a bad template belongs to.
func TestMediaSchemaAndPreviewErrors(t *testing.T) {
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	h := New(cfg, conn, dialect, noMetadata{}).Handler()

	rec := do(t, h, http.MethodGet, "/api/v1/settings/media/schema", "")
	var schema service.MediaSchema
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("schema: %d %v", rec.Code, err)
	}
	if len(schema.Tokens) == 0 || len(schema.ReaderFields) == 0 {
		t.Fatalf("schema: %+v", schema)
	}

	m := service.DefaultMediaSettings()
	m.SeriesFolder = "{Seris Title}"
	body, _ := json.Marshal(m)
	rec = do(t, h, http.MethodPost, "/api/v1/settings/media/preview", string(body))
	var preview NamingPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("preview: %d %v", rec.Code, err)
	}
	if preview.Errors["seriesFolder"] == "" || len(preview.Errors) != 1 {
		t.Fatalf("preview errors: %v", preview.Errors)
	}
	if rec = do(t, h, http.MethodPut, "/api/v1/settings/media", string(body)); rec.Code != http.StatusBadRequest {
		t.Fatalf("saving a misspelled token: %d %s", rec.Code, rec.Body)
	}
}
