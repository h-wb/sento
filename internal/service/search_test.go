package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/prowlarr"
	"github.com/h-wb/sento/internal/store"
)

func fakeProwlarr(t *testing.T, releases []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/indexer" {
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": 7, "name": "nyaa", "protocol": "torrent", "enable": true},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUnifiedVolumeFunnel(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, MonitorAll: true})
	if err != nil {
		t.Fatal(err)
	}

	releases := []map[string]any{
		{"guid": "g1", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE v01 (Digital) (LuCaZ)", "size": 100 << 20, "seeders": 30, "magnetUrl": "magnet:?xt=1", "infoHash": "aaa"},
		{"guid": "g2", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE v01-v38 (Digital)", "size": 38 * 100 << 20, "seeders": 10, "magnetUrl": "magnet:?xt=2", "infoHash": "bbb"},
		{"guid": "g3", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE (Novel) [Yen Press]", "size": 100 << 20, "seeders": 5, "magnetUrl": "magnet:?xt=3"},
		{"guid": "g4", "indexerId": 7, "indexer": "nyaa", "title": "Totally Different Series v01", "size": 100 << 20, "seeders": 5, "magnetUrl": "magnet:?xt=4"},
		{"guid": "g5", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE: Ace's Story v01 (Digital)", "size": 100 << 20, "seeders": 9, "magnetUrl": "magnet:?xt=5"},
		{"guid": "g6", "indexerId": 7, "indexer": "nyaa", "title": "One.Piece.T01.FRENCH.RETAiL", "size": 100 << 20, "seeders": 9, "magnetUrl": "magnet:?xt=6"},
		{"guid": "g7", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE 1056 (Digital)", "size": 30 << 20, "seeders": 9, "magnetUrl": "magnet:?xt=7"},
	}
	fake := fakeProwlarr(t, releases)
	registry := provider.NewRegistry(provider.NewTorrent(prowlarr.New(fake.URL, "key"), nil, "sento"))
	svc := NewSearchService(st, registry)

	result, err := svc.FindCandidates(ctx, seriesID, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	byGUID := map[string]ReleaseCandidate{}
	for _, c := range result.Candidates {
		byGUID[c.GUID] = c
	}

	checkReject := func(guid, code string) {
		t.Helper()
		c, ok := byGUID[guid]
		if !ok {
			t.Errorf("%s missing from funnel", guid)
			return
		}
		if len(c.Rejections) == 0 || c.Rejections[0] != code {
			t.Errorf("%s: want %s, got %v", guid, code, c.Rejections)
		}
	}
	if c := byGUID["g1"]; len(c.Rejections) != 0 || c.Score < 1000 {
		t.Errorf("g1 should be eligible: %+v", c.Rejections)
	}
	checkReject("g2", "rejected.pack_threshold")
	checkReject("g4", "rejected.series_match")
	checkReject("g5", "rejected.series_match") // spin-off leftover
	checkReject("g6", "rejected.language")     // French vs en collection
	// g3 (numberless novel) and g7 (chapter release) carry nothing for the
	// volume ledger: dropped before the funnel.
	for _, guid := range []string{"g3", "g7"} {
		if _, ok := byGUID[guid]; ok {
			t.Errorf("%s should be dropped from the volume funnel", guid)
		}
	}
	if result.Dropped != 2 {
		t.Errorf("dropped: want 2, got %d", result.Dropped)
	}

	// Chapter ledger search over the same releases: only g7 matches a slot
	// if wanted. Set cutoff to 1055 so 1056+ is wanted.
	cl, _ := st.GetChapterLedger(ctx, seriesID)
	cl.CutoffNum = 105500
	if err := st.UpdateChapterLedger(ctx, *cl); err != nil {
		t.Fatal(err)
	}
	chResult, err := svc.FindCandidates(ctx, seriesID, LedgerChapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	var g7 *ReleaseCandidate
	for i := range chResult.Candidates {
		if chResult.Candidates[i].GUID == "g7" {
			g7 = &chResult.Candidates[i]
		}
	}
	if g7 == nil || len(g7.Rejections) != 0 {
		t.Fatalf("torrent chapter 1056 should be eligible on chapter ledger: %+v", g7)
	}
	if g7.Covered[0] != 105600 {
		t.Errorf("g7 coverage: %v", g7.Covered)
	}
}

func TestSiteConfigFiltering(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, MonitorAll: true})
	if err != nil {
		t.Fatal(err)
	}
	releases := []map[string]any{
		{"guid": "g1", "indexerId": 7, "indexer": "nyaa", "title": "ONE PIECE v01 (Digital)", "size": 100 << 20, "seeders": 30, "magnetUrl": "magnet:?xt=1", "infoHash": "aaa"},
	}
	fake := fakeProwlarr(t, releases)
	registry := provider.NewRegistry(provider.NewTorrent(prowlarr.New(fake.URL, "key"), nil, "sento"))
	svc := NewSearchService(st, registry)

	// Globally disable the site for volumes.
	if err := st.UpsertSiteSetting(ctx, store.SiteSetting{
		Provider: "torrent", SiteID: "7",
		Enabled: true, UseVolumes: false, UseChapters: true,
	}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.FindCandidates(ctx, seriesID, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("volume-disabled site must contribute no candidates: %d", len(result.Candidates))
	}

	// Series override re-enables it for volumes.
	on := true
	if err := st.UpsertSiteOverride(ctx, store.SiteOverride{
		SeriesID: seriesID, Provider: "torrent", SiteID: "7", UseVolumes: &on,
	}); err != nil {
		t.Fatal(err)
	}
	result, err = svc.FindCandidates(ctx, seriesID, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("override should re-enable the site: got %d candidates", len(result.Candidates))
	}
}
