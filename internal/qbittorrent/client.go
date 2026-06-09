// Package qbittorrent is a minimal qBittorrent Web API client (spec.md §7):
// add by magnet/URL with a category, poll completion. Seeding is qBit's
// business; Sento imports by hardlink and never moves torrent data.
package qbittorrent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"encoding/json"
)

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client

	mu       sync.Mutex
	loggedIn bool
}

func New(baseURL, username, password string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loggedIn {
		return nil
	}
	form := url.Values{"username": {c.username}, "password": {c.password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Ok") {
		return fmt.Errorf("qbittorrent: login failed (%d: %s)", resp.StatusCode, body)
	}
	c.loggedIn = true
	return nil
}

// do retries once on 403 (expired session).
func (c *Client) do(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, err
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusForbidden {
			c.mu.Lock()
			c.loggedIn = false
			c.mu.Unlock()
			if err := c.login(ctx); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("qbittorrent: %s returned %d", path, resp.StatusCode)
		}
		return data, nil
	}
	return nil, fmt.Errorf("qbittorrent: authentication loop on %s", path)
}

// AddTorrent adds a magnet link or torrent URL under the given category.
func (c *Client) AddTorrent(ctx context.Context, urlOrMagnet, category string) error {
	form := url.Values{"urls": {urlOrMagnet}, "category": {category}}
	_, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/add", form)
	return err
}

type Torrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	Progress    float64 `json:"progress"`
	State       string  `json:"state"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
}

// Completed reports whether the torrent has finished downloading
// (seeding/paused-upload states included).
func (t Torrent) Completed() bool {
	return t.Progress >= 1
}

func (c *Client) List(ctx context.Context, category string) ([]Torrent, error) {
	path := "/api/v2/torrents/info"
	if category != "" {
		path += "?category=" + url.QueryEscape(category)
	}
	data, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var torrents []Torrent
	if err := json.Unmarshal(data, &torrents); err != nil {
		return nil, fmt.Errorf("qbittorrent: decoding torrent list: %w", err)
	}
	return torrents, nil
}

// Files returns the file paths inside a torrent (relative to its save path).
func (c *Client) Files(ctx context.Context, hash string) ([]string, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/files?hash="+url.QueryEscape(hash), nil)
	if err != nil {
		return nil, err
	}
	var files []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &files); err != nil {
		return nil, err
	}
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Name
	}
	return out, nil
}

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/api/v2/app/version", nil)
	return err
}
