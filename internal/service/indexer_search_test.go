package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/store"
)

func TestIndexerSearchChecksCategoryFiltering(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		books      bool
		unfiltered bool
	}{
		{name: "book results", books: true},
		{name: "only unrelated results", unfiltered: true},
		{name: "no matching releases"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var searches []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("t") == "caps" {
					_, _ = fmt.Fprint(w, `<caps><categories><category id="7000" name="Books"/><category id="5000" name="TV"><subcat id="5070" name="TV/Anime"/></category></categories></caps>`)
					return
				}
				if r.URL.Query().Get("q") != "Example" {
					t.Errorf("query = %q", r.URL.Query().Get("q"))
				}
				category := r.URL.Query().Get("cat")
				searches = append(searches, category)
				_, _ = fmt.Fprint(w, `<rss xmlns:torznab="http://torznab.com/schemas/2015/feed"><channel>`)
				if category == "7000" && tc.books {
					_, _ = fmt.Fprint(w, `<item><title>Example v01</title><torznab:attr name="category" value="7000"/></item>`)
				} else if category == "" && tc.unfiltered {
					_, _ = fmt.Fprint(w, `<item><title>Example S01E01</title><torznab:attr name="category" value="5070"/></item>`)
				}
				_, _ = fmt.Fprint(w, `</channel></rss>`)
			}))
			defer upstream.Close()
			settings, err := json.Marshal(TorznabSettings{URL: upstream.URL, APIKey: "fixture-key"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := TestIndexerSearch(context.Background(), store.Indexer{Protocol: store.IndexerTorznab, Settings: settings}, " Example ")
			if err != nil {
				t.Fatal(err)
			}
			if result.Query != "Example" || len(searches) == 0 || searches[0] != "7000" {
				t.Fatalf("result = %+v, searches = %v", result, searches)
			}
			if tc.books {
				if result.BooksFound != 1 || result.UnfilteredFound != nil || len(searches) != 1 || len(result.Warnings) != 0 {
					t.Fatalf("successful Books search should need no comparison: %+v, %v", result, searches)
				}
			} else {
				if result.BooksFound != 0 || result.UnfilteredFound == nil || len(searches) != 2 || searches[1] != "" {
					t.Fatalf("empty Books search should be compared: %+v, %v", result, searches)
				}
				if tc.unfiltered {
					if *result.UnfilteredFound != 1 || len(result.Warnings) != 1 || len(result.Samples) != 1 || result.Samples[0].Categories[0] != "TV/Anime (5070)" {
						t.Fatalf("missing category diagnostic: %+v", result)
					}
				} else if *result.UnfilteredFound != 0 || len(result.Warnings) != 0 || len(result.Samples) != 0 {
					t.Fatalf("a query without results is not a category mismatch: %+v", result)
				}
			}
		})
	}
}

func TestIndexerSearchRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	for _, query := range []string{" ", strings.Repeat("a", 201)} {
		if _, err := TestIndexerSearch(context.Background(), store.Indexer{Protocol: store.IndexerTorznab}, query); err == nil {
			t.Errorf("query %q should be invalid", query)
		}
	}
	if _, err := TestIndexerSearch(context.Background(), store.Indexer{Protocol: store.IndexerSource}, "Example"); err == nil {
		t.Fatal("source indexer should not use Torznab diagnostics")
	}
}
