// Package qbittorrent is a minimal qBittorrent Web API client (spec.md §7):
// add by magnet/URL with a category, poll completion. Seeding is qBit's
// business; Sento imports by hardlink and never moves torrent data.
package qbittorrent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
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
	// qBittorrent ≤4 answers 200 "Ok."; 5.x answers 204 with just the
	// session cookie. Bad credentials are 200 "Fails." or 403.
	ok := resp.StatusCode == http.StatusNoContent ||
		(resp.StatusCode == http.StatusOK && strings.Contains(string(body), "Ok"))
	if !ok {
		return fmt.Errorf("qbittorrent: login failed (%d: %s)", resp.StatusCode, body)
	}
	c.loggedIn = true
	return nil
}

// send performs one API call, logging in first and retrying once on 403
// (qBittorrent expires idle sessions; the cookie we hold may be stale).
// It returns the final status and a bounded body for the caller to judge.
func (c *Client) send(ctx context.Context, method, path, contentType string, body []byte) (int, []byte, error) {
	if err := c.login(ctx); err != nil {
		return 0, nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
		if err != nil {
			return 0, nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return 0, nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if err != nil {
			return 0, nil, err
		}
		if resp.StatusCode == http.StatusForbidden {
			c.mu.Lock()
			c.loggedIn = false
			c.mu.Unlock()
			if err := c.login(ctx); err != nil {
				return 0, nil, err
			}
			continue
		}
		return resp.StatusCode, data, nil
	}
	return 0, nil, fmt.Errorf("qbittorrent: authentication loop on %s", path)
}

// do sends a form (or bodyless) request and requires a 2xx answer.
func (c *Client) do(ctx context.Context, method, path string, form url.Values) ([]byte, error) {
	var body []byte
	contentType := ""
	if form != nil {
		body = []byte(form.Encode())
		contentType = "application/x-www-form-urlencoded"
	}
	status, data, err := c.send(ctx, method, path, contentType, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("qbittorrent: %s returned %d", path, status)
	}
	return data, nil
}

// AddTorrent adds a magnet link or torrent URL under the given category.
func (c *Client) AddTorrent(ctx context.Context, urlOrMagnet, category string) error {
	form := url.Values{"urls": {urlOrMagnet}, "category": {category}}
	_, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/add", form)
	return err
}

// AddTorrentFile uploads .torrent bytes (multipart) under the category.
// Preferred over URL adds: qBittorrent fetches URLs asynchronously from
// its own network, where an indexer/proxy may be unreachable, and drops
// failures silently.
func (c *Client) AddTorrentFile(ctx context.Context, name string, torrent []byte, category string) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("torrents", name)
	if err != nil {
		return err
	}
	if _, err := part.Write(torrent); err != nil {
		return err
	}
	if err := w.WriteField("category", category); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	status, body, err := c.send(ctx, http.MethodPost, "/api/v2/torrents/add",
		w.FormDataContentType(), buf.Bytes())
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return nil // already added: the goal state holds
	}
	if status < 200 || status >= 300 || bytes.Contains(body, []byte("Fails")) {
		return fmt.Errorf("qbittorrent: add failed (%d: %s)", status, truncate(body, 200))
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
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
