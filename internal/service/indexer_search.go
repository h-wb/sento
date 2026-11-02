package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
	"github.com/h-wb/sento/internal/torznab"
)

// IndexerSearchCheck compares the real Books query with an unrestricted
// query when Books returns nothing. It never evaluates or queues releases.
type IndexerSearchCheck struct {
	Query           string                `json:"query"`
	BooksFound      int                   `json:"booksFound"`
	UnfilteredFound *int                  `json:"unfilteredFound,omitempty"`
	Samples         []IndexerSearchSample `json:"samples"`
	Warnings        []string              `json:"warnings"`
}

type IndexerSearchSample struct {
	Title      string   `json:"title"`
	Categories []string `json:"categories"`
}

func TestIndexerSearch(ctx context.Context, ix store.Indexer, query string) (*IndexerSearchCheck, error) {
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 200 {
		return nil, apperr.Invalidf("enter a title of 1–200 characters to test")
	}
	if ix.Protocol != store.IndexerTorznab {
		return nil, apperr.Invalidf("search testing is available for Torznab indexers")
	}
	e, err := parseEntry(ix)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client := torznab.New(e.URL, e.APIKey)
	releases, err := client.Search(ctx, query, torznab.CategoryBooks)
	if err != nil {
		return nil, apperr.Upstreamf("Books search: %w", err)
	}
	result := &IndexerSearchCheck{Query: query, BooksFound: len(releases), Samples: []IndexerSearchSample{}, Warnings: []string{}}
	if len(releases) == 0 {
		releases, err = client.Search(ctx, query)
		if err != nil {
			return nil, apperr.Upstreamf("unfiltered comparison: %w", err)
		}
		count := len(releases)
		result.UnfilteredFound = &count
		if count > 0 {
			result.Warnings = append(result.Warnings, "The Books query returned nothing, but an unfiltered query found releases. The indexer may apply category filtering after its result limit. Check its category setting in Prowlarr or Jackett.")
		}
	}
	names, err := client.Categories(ctx)
	if err != nil {
		result.Warnings = append(result.Warnings, "Category names could not be read; samples show category IDs instead.")
	}
	for _, release := range releases[:min(5, len(releases))] {
		sample := IndexerSearchSample{Title: release.Title, Categories: []string{}}
		for _, id := range release.Categories {
			name := names[id]
			if name == "" {
				sample.Categories = append(sample.Categories, strconv.Itoa(id))
			} else {
				sample.Categories = append(sample.Categories, fmt.Sprintf("%s (%d)", name, id))
			}
		}
		result.Samples = append(result.Samples, sample)
	}
	return result, nil
}
