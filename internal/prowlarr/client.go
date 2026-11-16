// Package prowlarr is a minimal client for Prowlarr's indexer API
// (spec.md §7): Sento lists the indexers Prowlarr owns to offer them as
// sources; searching goes through the indexers' own Torznab feeds.
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

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
