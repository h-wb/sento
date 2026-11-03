package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/db/dbtest"
	"github.com/h-wb/sento/internal/store"
)

func TestUnblockEndpoints(t *testing.T) {
	ctx := context.Background()
	cfg := config.Default()
	cfg.Library.Path = t.TempDir()
	conn, dialect := dbtest.Open(t)
	srv := New(cfg, conn, dialect, noMetadata{})
	h := srv.Handler()
	for _, key := range []string{"selected", "search-result", "keep"} {
		if err := srv.store.AddBlocklist(ctx, "source", key, "old failure"); err != nil {
			t.Fatal(err)
		}
	}
	var entries []store.BlocklistEntry
	rec := do(t, h, http.MethodGet, "/api/v1/blocklist", "")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &entries) != nil || len(entries) != 3 {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{
		`{}`, `{"ids":[]}`, `{"ids":[0]}`, `{"ids":[-1]}`, `{"protocol":"other","keys":["keep"]}`,
		`{"protocol":"source","keys":[""]}`, `{"ids":[1],"protocol":"source","keys":["keep"]}`,
	} {
		if rec := do(t, h, http.MethodPost, "/api/v1/blocklist/unblock", body); rec.Code != http.StatusBadRequest {
			t.Errorf("invalid selection %s = %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if blocked, err := srv.store.BlockedKeys(ctx); err != nil || len(blocked) != 3 {
		t.Fatalf("invalid requests changed blocks: %v, %v", blocked, err)
	}
	selectedID := entries[2].ID
	rec = do(t, h, http.MethodPost, "/api/v1/blocklist/unblock", fmt.Sprintf(`{"ids":[%d]}`, selectedID))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("selected unblock = %d %s", rec.Code, rec.Body.String())
	}
	for range 2 { // Unblocking a search result is idempotent.
		rec = do(t, h, http.MethodPost, "/api/v1/blocklist/unblock", `{"protocol":"source","keys":["search-result","feed-alias"]}`)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("search unblock = %d %s", rec.Code, rec.Body.String())
		}
	}
	if blocked, err := srv.store.BlockedKeys(ctx); err != nil || len(blocked) != 1 || !blocked["source/keep"] {
		t.Fatalf("selected unblock changed unrelated blocks: %v, %v", blocked, err)
	}
	if rec := do(t, h, http.MethodDelete, "/api/v1/blocklist", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body.String())
	}
	if blocked, err := srv.store.BlockedKeys(ctx); err != nil || len(blocked) != 0 {
		t.Fatalf("clear left %v, %v", blocked, err)
	}
}
