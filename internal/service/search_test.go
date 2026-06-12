package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

type fakeRelease struct {
	guid, title, magnet, infohash string
	size                          int64
	seeders                       int
}

// fakeTorznab serves a Torznab feed for any search.
func fakeTorznab(t *testing.T, releases []fakeRelease) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps><searching><search available="yes"/></searching></caps>`))
			return
		}
		var b strings.Builder
		b.WriteString(`<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
		for _, rel := range releases {
			fmt.Fprintf(&b, `<item><title>%s</title><guid>%s</guid><size>%d</size>`,
				rel.title, rel.guid, rel.size)
			fmt.Fprintf(&b, `<torznab:attr name="seeders" value="%d"/>`, rel.seeders)
			fmt.Fprintf(&b, `<torznab:attr name="magneturl" value="%s"/>`, rel.magnet)
			if rel.infohash != "" {
				fmt.Fprintf(&b, `<torznab:attr name="infohash" value="%s"/>`, rel.infohash)
			}
			b.WriteString(`</item>`)
		}
		b.WriteString(`</channel></rss>`)
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// addTorznabEntry registers one torznab indexer entry pointed at a fake.
func addTorznabEntry(t *testing.T, st *store.Store, name, url string) int64 {
	t.Helper()
	settings, _ := json.Marshal(TorznabSettings{URL: url, APIKey: "key"})
	id, err := st.AddIndexer(context.Background(), store.Indexer{
		Name: name, Protocol: store.IndexerTorznab,
		Enabled: true, UseVolumes: true, UseChapters: true, Settings: settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func torrentRegistry(st *store.Store) *provider.Registry {
	return provider.NewRegistry(
		provider.NewTorrent(IndexerEntries(st, store.IndexerTorznab), nil, "sento"))
}

func TestUnifiedVolumeFunnel(t *testing.T) {
	lib, st := setup(t)
	ctx := context.Background()
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}

	releases := []fakeRelease{
		{"g1", "ONE PIECE v01 (Digital) (LuCaZ)", "magnet:?xt=1", "aaa", 100 << 20, 30},
		{"g2", "ONE PIECE v01-v38 (Digital)", "magnet:?xt=2", "bbb", 38 * 100 << 20, 10},
		{"g3", "ONE PIECE (Novel) [Yen Press]", "magnet:?xt=3", "", 100 << 20, 5},
		{"g4", "Totally Different Series v01", "magnet:?xt=4", "", 100 << 20, 5},
		{"g5", "ONE PIECE: Ace&#39;s Story v01 (Digital)", "magnet:?xt=5", "", 100 << 20, 9},
		{"g6", "One.Piece.T01.FRENCH.RETAiL", "magnet:?xt=6", "", 100 << 20, 9},
		{"g7", "ONE PIECE 1056 (Digital)", "magnet:?xt=7", "", 30 << 20, 9},
	}
	fake := fakeTorznab(t, releases)
	addTorznabEntry(t, st, "nyaa", fake.URL)
	svc := NewSearchService(st, torrentRegistry(st))

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
	if c := byGUID["g1"]; c.Site != "nyaa" {
		t.Errorf("g1 site should be the entry name, got %q", c.Site)
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
	cols, _ := st.ListCollections(ctx, seriesID)
	chapters := chapterCollection(t, cols)
	if err := st.SetCollectionCutoff(ctx, seriesID, chapters.ID, 105500); err != nil {
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
	seriesID, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377, Monitor: "all"})
	if err != nil {
		t.Fatal(err)
	}
	releases := []fakeRelease{
		{"g1", "ONE PIECE v01 (Digital)", "magnet:?xt=1", "aaa", 100 << 20, 30},
	}
	fake := fakeTorznab(t, releases)
	entryID := addTorznabEntry(t, st, "nyaa", fake.URL)
	svc := NewSearchService(st, torrentRegistry(st))

	// Globally disable the entry for volumes.
	ix, err := st.GetIndexer(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	ix.UseVolumes = false
	if err := st.UpdateIndexer(ctx, *ix); err != nil {
		t.Fatal(err)
	}
	result, err := svc.FindCandidates(ctx, seriesID, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 0 {
		t.Fatalf("volume-disabled entry must contribute no candidates: %d", len(result.Candidates))
	}

	// Series override re-enables it for volumes.
	on := true
	if err := st.UpsertSiteOverride(ctx, store.SiteOverride{
		SeriesID: seriesID, IndexerID: entryID, UseVolumes: &on,
	}); err != nil {
		t.Fatal(err)
	}
	result, err = svc.FindCandidates(ctx, seriesID, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("override should re-enable the entry: got %d candidates", len(result.Candidates))
	}
}
