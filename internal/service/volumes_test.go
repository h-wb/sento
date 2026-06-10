package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/prowlarr"
)

func TestVolumeFunnel(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, MonitorAll: true})
	if err != nil {
		t.Fatal(err)
	}

	releases := []map[string]any{
		{"guid": "g1", "title": "ONE PIECE v01 (Digital) (LuCaZ)", "size": 100 << 20, "seeders": 30, "indexer": "nyaa", "magnetUrl": "magnet:?xt=1", "infoHash": "aaa"},
		{"guid": "g2", "title": "ONE PIECE v01-v38 (Digital)", "size": 38 * 100 << 20, "seeders": 10, "indexer": "nyaa", "magnetUrl": "magnet:?xt=2", "infoHash": "bbb"},
		{"guid": "g3", "title": "ONE PIECE (Novel) [Yen Press]", "size": 100 << 20, "seeders": 5, "indexer": "nyaa", "magnetUrl": "magnet:?xt=3"},
		{"guid": "g4", "title": "Totally Different Series v01", "size": 100 << 20, "seeders": 5, "indexer": "nyaa", "magnetUrl": "magnet:?xt=4"},
		{"guid": "g5", "title": "ONE PIECE v01 (Digital)", "size": 1 << 20, "seeders": 5, "indexer": "nyaa", "magnetUrl": "magnet:?xt=5"},
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer fake.Close()

	registry := provider.NewRegistry(provider.NewTorrent(prowlarr.New(fake.URL, "key"), nil, "sento"))
	vs := NewVolumeSearcher(st, registry)

	result, err := vs.FindVolumeCandidates(ctx, seriesID, nil)
	if err != nil {
		t.Fatal(err)
	}
	candidates := result.Candidates
	byGUID := map[string]VolumeCandidate{}
	for _, c := range candidates {
		byGUID[c.GUID] = c
	}
	if len(result.Searched) == 0 {
		t.Error("search diagnostics should list searched sites")
	}

	if c := byGUID["g1"]; len(c.Rejections) != 0 || c.Score < 1000 {
		t.Errorf("g1 should be eligible with coverage score: %+v", c)
	}
	// The test fixture monitors 3 volumes (1-3); a v01-38 pack covers 38
	// with only 3 wanted → pack threshold rejection (D12).
	if c := byGUID["g2"]; len(c.Rejections) == 0 || c.Rejections[0] != "rejected.pack_threshold" {
		t.Errorf("g2 should hit pack threshold: %+v", c.Rejections)
	}
	if c := byGUID["g3"]; len(c.Rejections) == 0 || c.Rejections[0] != "rejected.content_type" {
		t.Errorf("g3 should be content-type rejected: %+v", c.Rejections)
	}
	if c := byGUID["g4"]; len(c.Rejections) == 0 {
		t.Errorf("g4 (wrong series) should be rejected: %+v", c)
	}
	if c := byGUID["g5"]; len(c.Rejections) == 0 || c.Rejections[0] != "rejected.size" {
		t.Errorf("g5 (1MB volume) should be size rejected: %+v", c.Rejections)
	}

	// Eligible candidates sort first.
	if candidates[0].GUID != "g1" {
		t.Errorf("g1 should rank first, got %s", candidates[0].GUID)
	}
}
