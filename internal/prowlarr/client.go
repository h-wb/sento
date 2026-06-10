// Package prowlarr is a minimal client for Prowlarr's search API
// (spec.md §7): Prowlarr owns the per-indexer adapters, Sento just searches.
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Comics/manga torznab categories.
var DefaultCategories = []int{7000, 7030}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type Release struct {
	GUID        string `json:"guid"`
	Title       string `json:"title"`
	Size        int64  `json:"size"`
	Indexer     string `json:"indexer"`
	IndexerID   int    `json:"indexerId"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	DownloadURL string `json:"downloadUrl"`
	MagnetURL   string `json:"magnetUrl"`
	InfoHash    string `json:"infoHash"`
	Protocol    string `json:"protocol"`
	PublishDate string `json:"publishDate"`
}

func (c *Client) Search(ctx context.Context, query string, categories []int, indexerIDs []int) ([]Release, error) {
	if len(categories) == 0 {
		categories = DefaultCategories
	}
	values := url.Values{}
	values.Set("query", query)
	values.Set("type", "search")
	for _, cat := range categories {
		values.Add("categories", strconv.Itoa(cat))
	}
	for _, id := range indexerIDs {
		values.Add("indexerIds", strconv.Itoa(id))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/search?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr: search returned %d", resp.StatusCode)
	}
	var releases []Release
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("prowlarr: decoding search results: %w", err)
	}
	return releases, nil
}

type Indexer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Enable   bool   `json:"enable"`
}

func (c *Client) ListIndexers(ctx context.Context) ([]Indexer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/indexer", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr: indexer list returned %d", resp.StatusCode)
	}
	var indexers []Indexer
	if err := json.Unmarshal(body, &indexers); err != nil {
		return nil, err
	}
	return indexers, nil
}

func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prowlarr: health returned %d", resp.StatusCode)
	}
	return nil
}
