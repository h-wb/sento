package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAliasSearchKeepsOffersPartialErrorsAndCaches(t *testing.T) {
	var mu sync.Mutex
	queries := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if r.URL.Query().Get("t") == "caps" {
			_, _ = w.Write([]byte(`<caps/>`))
			return
		}
		mu.Lock()
		queries[q]++
		mu.Unlock()
		if q == "broken alias" {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		_, _ = fmt.Fprint(w, `<rss><channel><item><title>Same release</title><guid>shared</guid></item>`)
		if q == "titre francais" {
			_, _ = fmt.Fprint(w, `<item><title>French release</title><guid>localized</guid></item>`)
		}
		_, _ = fmt.Fprint(w, `</channel></rss>`)
	}))
	defer server.Close()
	p := NewTorrent(func(context.Context) ([]IndexerEntry, error) {
		return []IndexerEntry{{ID: 1, Name: "one", URL: server.URL, Enabled: true}, {ID: 2, Name: "two", URL: server.URL, Enabled: true}}, nil
	})
	req := SearchRequest{Query: "Primary Title", Titles: []string{"Primary.Title", "Titre français", "Broken alias"}}
	for range 2 {
		got, err := p.Search(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "broken alias") {
			t.Fatalf("lost query error: %v", err)
		}
		if len(got) != 4 {
			t.Fatalf("lost alternate offer or duplicated alias: %+v", got)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if queries["primary title"] != 1 || queries["titre francais"] != 1 {
		t.Fatalf("cache/dedup: %v", queries)
	}
	if queries["broken alias"] != 4 {
		t.Fatalf("failures were cached: %v", queries)
	}
}

func TestTorrentQueriesBoundedAndDeterministic(t *testing.T) {
	got := TorrentQueries(SearchRequest{Query: "One.Piece", Titles: []string{"ONE PIECE", "フランス", "French Title", "Romanized Title", "Last Title"}})
	if len(got) != 4 || got[0] != "one piece" {
		t.Fatalf("queries: %v", got)
	}
}

func TestTorrentPausedAndQueuedAreNotStalls(t *testing.T) {
	for _, state := range []string{"pausedDL", "stoppedDL", "queuedDL", "checkingDL", "error", "missingFiles", "moving", "checkingResumeData"} {
		if torrentCanStall(state) {
			t.Fatalf("%s treated as stalled", state)
		}
	}
	for _, state := range []string{"downloading", "metaDL", "stalledDL", "forcedDL"} {
		if !torrentCanStall(state) {
			t.Fatalf("%s ignored", state)
		}
	}
}
