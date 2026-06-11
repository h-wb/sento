package sourcehost

import (
	"bytes"
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

// Client talks to a sento-host sidecar (host/), the native implementation
// of this package's contract. All keys pass through opaquely.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		// Extension sources can be slow (Cloudflare, rate limits).
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Name() string { return "host" }

func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return fmt.Errorf("host: %s", e.Error)
		}
		return fmt.Errorf("host: HTTP %d", resp.StatusCode)
	}
	return json.Unmarshal(raw, out)
}

func (c *Client) Sources(ctx context.Context) ([]Source, error) {
	var out []Source
	err := c.get(ctx, "/api/v1/sources", nil, &out)
	return out, err
}

func (c *Client) Search(ctx context.Context, sourceID, query string) ([]Manga, error) {
	var out []Manga
	err := c.get(ctx, "/api/v1/sources/"+url.PathEscape(sourceID)+"/search",
		url.Values{"q": {query}}, &out)
	return out, err
}

func (c *Client) Chapters(ctx context.Context, sourceID, mangaKey string) ([]Chapter, error) {
	var out []Chapter
	err := c.get(ctx, "/api/v1/sources/"+url.PathEscape(sourceID)+"/chapters",
		url.Values{"manga": {mangaKey}}, &out)
	return out, err
}

func (c *Client) Pages(ctx context.Context, sourceID, chapterKey string) ([]string, error) {
	var out []string
	err := c.get(ctx, "/api/v1/sources/"+url.PathEscape(sourceID)+"/pages",
		url.Values{"chapter": {chapterKey}}, &out)
	return out, err
}

func (c *Client) Page(ctx context.Context, sourceID, pageRef string) (io.ReadCloser, error) {
	u := c.baseURL + "/api/v1/sources/" + url.PathEscape(sourceID) + "/page?" +
		url.Values{"ref": {pageRef}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	// No client timeout on the streaming body: rely on ctx.
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("host: page fetch returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// --- extension management (sidecar only) ---

func (c *Client) Extensions(ctx context.Context) ([]ExtensionInfo, error) {
	var out []ExtensionInfo
	err := c.get(ctx, "/api/v1/extensions", nil, &out)
	return out, err
}

func (c *Client) request(ctx context.Context, method, path string) error {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	// Installs download + dex2jar the APK: allow more than the read
	// timeout of the shared client.
	resp, err := (&http.Client{Timeout: 3 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return fmt.Errorf("host: %s", e.Error)
		}
		return fmt.Errorf("host: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) InstallExtension(ctx context.Context, pkgName string) error {
	return c.request(ctx, http.MethodPost, "/api/v1/extensions/"+url.PathEscape(pkgName))
}

func (c *Client) UninstallExtension(ctx context.Context, pkgName string) error {
	return c.request(ctx, http.MethodDelete, "/api/v1/extensions/"+url.PathEscape(pkgName))
}

// --- source preferences (sidecar only) ---

func (c *Client) SourcePreferences(ctx context.Context, sourceID string) ([]Preference, error) {
	var out []Preference
	err := c.get(ctx, "/api/v1/sources/"+url.PathEscape(sourceID)+"/preferences", nil, &out)
	return out, err
}

func (c *Client) SetSourcePreference(ctx context.Context, sourceID, key string, value json.RawMessage) error {
	body, err := json.Marshal(map[string]json.RawMessage{
		"key":   json.RawMessage(strconv.Quote(key)),
		"value": value,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/sources/"+url.PathEscape(sourceID)+"/preferences", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error != "" {
			return fmt.Errorf("host: %s", e.Error)
		}
		return fmt.Errorf("host: HTTP %d", resp.StatusCode)
	}
	return nil
}

var (
	_ Host              = (*Client)(nil)
	_ ExtensionManager  = (*Client)(nil)
	_ PreferenceManager = (*Client)(nil)
)
