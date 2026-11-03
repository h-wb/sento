package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/store"
)

func TestSearchWithoutDownloadsReturnsEmptyQueuedArray(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	srv := New(cfg, conn, dialect, noMetadata{})
	sid, err := srv.store.AddSeries(ctx, store.AddSeriesParams{Series: &metadata.Series{MangaBakaID: 103, Title: "No releases", Type: "manga"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, ledger := range []string{"volume", "chapter"} {
		_, err := srv.store.AddCustomEdition(ctx, store.Edition{SeriesID: sid, EditionName: ledger, Language: "en", Type: ledger}, 0, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dryRun=%t", ledger, dryRun), func(t *testing.T) {
				body := fmt.Sprintf(`{"ledger":%q,"dryRun":%t}`, ledger, dryRun)
				rec := do(t, srv.Handler(), http.MethodPost, fmt.Sprintf("/api/v1/series/%d/search", sid), body)
				if rec.Code != http.StatusOK {
					t.Fatalf("search = %d: %s", rec.Code, rec.Body.String())
				}
				var result struct {
					Queued []int64 `json:"queued"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.Queued == nil || len(result.Queued) != 0 {
					t.Fatalf("expected an empty queued array: %s", rec.Body.String())
				}
			})
		}
	}
}
