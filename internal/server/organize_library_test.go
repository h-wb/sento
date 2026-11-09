package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/scheduler"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// The per-series organize routes answer on their own, outside the global
// request timeout group (a big series may organize for minutes; the run
// still stops when the client disconnects, and answers busy when another
// run holds the importer).
func TestSeriesOrganizeRoutes(t *testing.T) {
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	srv := New(cfg, conn, dialect, noMetadata{})
	ctx := context.Background()
	sid, err := srv.store.AddSeries(ctx, store.AddSeriesParams{Series: &metadata.Series{MangaBakaID: 6766, Title: "Yotsuba&!", Type: "manga"}})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	// A real series: an unmounted route would 404 indistinguishably.
	if rec := do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/series/%d/files/organize", sid), ""); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("plan route: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, fmt.Sprintf("/api/v1/series/%d/files/organize", sid), `{"rename":true}`); rec.Code != http.StatusOK {
		t.Fatalf("apply route: %d %s", rec.Code, rec.Body)
	}
}

// A library organize is accepted at once and finishes in the background,
// reporting through its status endpoint.
func TestOrganizeLibraryRunsInBackground(t *testing.T) {
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	srv := New(cfg, conn, dialect, noMetadata{})
	h := srv.Handler()

	if rec := do(t, h, http.MethodGet, "/api/v1/library/organize?series=1,x", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodGet, "/api/v1/library/organize", ""); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("empty preview: %d %q", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/library/organize", `{"rename":true}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("without a scheduler: %d %s", rec.Code, rec.Body)
	}

	srv.SetScheduler(scheduler.New(srv.store, srv.library, srv.search, srv.importer, srv.scanner, srv.extensions, scheduler.DefaultIntervals()))
	if rec := do(t, h, http.MethodPost, "/api/v1/library/organize", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("nothing to apply: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, http.MethodPost, "/api/v1/library/organize", `{"rename":true,"retag":true}`); rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec := do(t, h, http.MethodGet, "/api/v1/library/organize/status", "")
		var st service.LibraryOrganizeStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		if st.FinishedAt != "" && !st.Running && !st.Queued {
			if st.Error != "" || st.Series == nil {
				t.Fatalf("finished status: %+v", st)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("organize never finished: %+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
