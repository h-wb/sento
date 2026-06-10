// Package suwayomi implements the SourceProvider contract against a
// Suwayomi-Server instance (specs/suwayomi-adapter.md — live-validated).
package suwayomi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func New(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		// Sources behind FlareSolverr are slow (adapter spec §3).
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

type Source struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Lang    string `json:"lang"`
	IsNSFW  bool   `json:"isNsfw"`
	PkgName string `json:"pkgName,omitempty"`
}

type Manga struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	SourceID    string `json:"sourceId"`
	InLibrary   bool   `json:"inLibrary"`
	Initialized bool   `json:"initialized"`
}

type MangaPage struct {
	Mangas      []Manga `json:"mangas"`
	HasNextPage bool    `json:"hasNextPage"`
}

type Chapter struct {
	ID            int64   `json:"id"`
	SourceOrder   int     `json:"sourceOrder"`
	Name          string  `json:"name"`
	ChapterNumber float64 `json:"chapterNumber"`
	Scanlator     string  `json:"scanlator"`
	UploadDate    string  `json:"uploadDate"` // epoch millis as string
	IsDownloaded  bool    `json:"isDownloaded"`
}

type DownloadState struct {
	Downloaded bool    `json:"downloaded"`
	Queued     bool    `json:"queued"`
	State      string  `json:"state,omitempty"` // QUEUED|DOWNLOADING|ERROR|…
	Progress   float64 `json:"progress,omitempty"`
	Tries      int     `json:"tries,omitempty"`
}

// --- GraphQL plumbing ---

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

func (c *Client) gql(ctx context.Context, query string, vars map[string]any, out any) error {
	body, err := json.Marshal(gqlRequest{Query: query, Variables: vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
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
		return fmt.Errorf("suwayomi: HTTP %d: %s", resp.StatusCode, truncate(raw, 200))
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("suwayomi: decoding response: %w", err)
	}
	// Errors can arrive alongside partial data (adapter spec §3).
	if len(env.Errors) > 0 && (env.Data == nil || string(env.Data) == "null") {
		return fmt.Errorf("suwayomi: %s", env.Errors[0].Message)
	}
	if out != nil && env.Data != nil {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("suwayomi: decoding data: %w", err)
		}
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}

// --- operations (adapter spec §1, §2) ---

func (c *Client) About(ctx context.Context) (string, error) {
	var out struct {
		AboutServer struct {
			Version string `json:"version"`
		} `json:"aboutServer"`
	}
	err := c.gql(ctx, `{ aboutServer { version } }`, nil, &out)
	return out.AboutServer.Version, err
}

// ListSources excludes the special Local source (id "0").
func (c *Client) ListSources(ctx context.Context) ([]Source, error) {
	var out struct {
		Sources struct {
			Nodes []struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				Lang      string `json:"lang"`
				IsNsfw    bool   `json:"isNsfw"`
				Extension struct {
					PkgName string `json:"pkgName"`
				} `json:"extension"`
			} `json:"nodes"`
		} `json:"sources"`
	}
	if err := c.gql(ctx, `{ sources { nodes { id name lang isNsfw extension { pkgName } } } }`, nil, &out); err != nil {
		return nil, err
	}
	sources := make([]Source, 0, len(out.Sources.Nodes))
	for _, n := range out.Sources.Nodes {
		if n.ID == "0" {
			continue
		}
		sources = append(sources, Source{
			ID: n.ID, Name: n.Name, Lang: n.Lang, IsNSFW: n.IsNsfw, PkgName: n.Extension.PkgName,
		})
	}
	return sources, nil
}

func (c *Client) SearchManga(ctx context.Context, sourceID, query string, page int) (*MangaPage, error) {
	var out struct {
		FetchSourceManga struct {
			HasNextPage bool    `json:"hasNextPage"`
			Mangas      []Manga `json:"mangas"`
		} `json:"fetchSourceManga"`
	}
	err := c.gql(ctx, `
		mutation($source: LongString!, $query: String, $page: Int!) {
			fetchSourceManga(input: {type: SEARCH, source: $source, query: $query, page: $page}) {
				hasNextPage
				mangas { id title url sourceId inLibrary initialized }
			}
		}`,
		map[string]any{"source": sourceID, "query": query, "page": page}, &out)
	if err != nil {
		return nil, err
	}
	return &MangaPage{Mangas: out.FetchSourceManga.Mangas, HasNextPage: out.FetchSourceManga.HasNextPage}, nil
}

// GetChapters live-syncs and returns the source's chapter list (transient
// release candidates — never persisted as ledger state).
func (c *Client) GetChapters(ctx context.Context, mangaID int64) ([]Chapter, error) {
	var out struct {
		FetchChapters struct {
			Chapters []Chapter `json:"chapters"`
		} `json:"fetchChapters"`
	}
	err := c.gql(ctx, `
		mutation($mangaId: Int!) {
			fetchChapters(input: {mangaId: $mangaId}) {
				chapters { id sourceOrder name chapterNumber scanlator uploadDate isDownloaded }
			}
		}`,
		map[string]any{"mangaId": mangaID}, &out)
	if err != nil {
		return nil, err
	}
	return out.FetchChapters.Chapters, nil
}

// EnqueueDownload queues a chapter; Suwayomi self-processes the queue
// (adapter spec §1.4 — do not call startDownloader routinely).
func (c *Client) EnqueueDownload(ctx context.Context, chapterID int64) error {
	return c.gql(ctx, `
		mutation($id: Int!) {
			enqueueChapterDownload(input: {id: $id}) { downloadStatus { state } }
		}`,
		map[string]any{"id": chapterID}, nil)
}

// DownloadStateOf is a composite read: chapter.isDownloaded is the success
// signal; the queue entry only describes in-flight state.
func (c *Client) DownloadStateOf(ctx context.Context, chapterID int64) (*DownloadState, error) {
	var out struct {
		Chapter struct {
			IsDownloaded bool `json:"isDownloaded"`
		} `json:"chapter"`
		DownloadStatus struct {
			State string `json:"state"`
			Queue []struct {
				Chapter struct {
					ID int64 `json:"id"`
				} `json:"chapter"`
				State    string  `json:"state"`
				Progress float64 `json:"progress"`
				Tries    int     `json:"tries"`
			} `json:"queue"`
		} `json:"downloadStatus"`
	}
	err := c.gql(ctx, `
		query($id: Int!) {
			chapter(id: $id) { isDownloaded }
			downloadStatus { state queue { chapter { id } state progress tries } }
		}`,
		map[string]any{"id": chapterID}, &out)
	if err != nil {
		return nil, err
	}
	st := &DownloadState{Downloaded: out.Chapter.IsDownloaded}
	for _, q := range out.DownloadStatus.Queue {
		if q.Chapter.ID == chapterID {
			st.Queued = true
			st.State = q.State
			st.Progress = q.Progress
			st.Tries = q.Tries
		}
	}
	return st, nil
}

// FetchCBZ streams the downloaded chapter as a CBZ over REST.
func (c *Client) FetchCBZ(ctx context.Context, chapterID int64) (io.ReadCloser, string, error) {
	url := fmt.Sprintf("%s/api/v1/chapter/%d/download", c.baseURL, chapterID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	// No client timeout on the streaming body: rely on ctx (adapter §3).
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, "", fmt.Errorf("suwayomi: CBZ fetch returned %d", resp.StatusCode)
	}
	return resp.Body, resp.Header.Get("Content-Disposition"), nil
}

func (c *Client) DeleteDownload(ctx context.Context, chapterID int64) error {
	return c.gql(ctx, `
		mutation($id: Int!) {
			deleteDownloadedChapter(input: {id: $id}) { chapters { id isDownloaded } }
		}`,
		map[string]any{"id": chapterID}, nil)
}

// Recover kicks a stuck queue. The 30s timeout error is benign here
// (adapter spec §1.4 gotcha 1).
func (c *Client) Recover(ctx context.Context) error {
	err := c.gql(ctx, `mutation { startDownloader(input: {}) { downloadStatus { state } } }`, nil, nil)
	if err != nil && strings.Contains(err.Error(), "Timed out") {
		return nil
	}
	return err
}
