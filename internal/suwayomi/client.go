// Package suwayomi implements the sourcehost.Host contract against a
// Suwayomi-Server instance (specs/suwayomi-adapter.md — live-validated).
// Suwayomi is used purely as a protocol adapter: search, chapter lists and
// page images. Its downloader, library and categories are not touched —
// Sento assembles CBZs itself from the page stream.
package suwayomi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/sourcehost"
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

func (c *Client) Name() string { return "suwayomi" }

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

// --- sourcehost.Host ---

func (c *Client) About(ctx context.Context) (string, error) {
	var out struct {
		AboutServer struct {
			Version string `json:"version"`
		} `json:"aboutServer"`
	}
	err := c.gql(ctx, `{ aboutServer { version } }`, nil, &out)
	return out.AboutServer.Version, err
}

// Sources excludes the special Local source (id "0").
func (c *Client) Sources(ctx context.Context) ([]sourcehost.Source, error) {
	var out struct {
		Sources struct {
			Nodes []struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Lang   string `json:"lang"`
				IsNsfw bool   `json:"isNsfw"`
			} `json:"nodes"`
		} `json:"sources"`
	}
	if err := c.gql(ctx, `{ sources { nodes { id name lang isNsfw } } }`, nil, &out); err != nil {
		return nil, err
	}
	sources := make([]sourcehost.Source, 0, len(out.Sources.Nodes))
	for _, n := range out.Sources.Nodes {
		if n.ID == "0" {
			continue
		}
		sources = append(sources, sourcehost.Source{
			ID: n.ID, Name: n.Name, Lang: n.Lang, NSFW: n.IsNsfw,
		})
	}
	return sources, nil
}

// Search runs a source text search (first page — binding resolution wants
// confident matches, not exhaustive listings). Manga keys are Suwayomi's
// database ids.
func (c *Client) Search(ctx context.Context, sourceID, query string) ([]sourcehost.Manga, error) {
	var out struct {
		FetchSourceManga struct {
			Mangas []struct {
				ID    int64  `json:"id"`
				Title string `json:"title"`
			} `json:"mangas"`
		} `json:"fetchSourceManga"`
	}
	err := c.gql(ctx, `
		mutation($source: LongString!, $query: String, $page: Int!) {
			fetchSourceManga(input: {type: SEARCH, source: $source, query: $query, page: $page}) {
				mangas { id title }
			}
		}`,
		map[string]any{"source": sourceID, "query": query, "page": 1}, &out)
	if err != nil {
		return nil, err
	}
	mangas := make([]sourcehost.Manga, 0, len(out.FetchSourceManga.Mangas))
	for _, m := range out.FetchSourceManga.Mangas {
		mangas = append(mangas, sourcehost.Manga{
			Key: strconv.FormatInt(m.ID, 10), Title: m.Title,
		})
	}
	return mangas, nil
}

// Chapters live-syncs and returns the source's chapter list (transient
// release candidates — never persisted as ledger state).
func (c *Client) Chapters(ctx context.Context, _ string, mangaKey string) ([]sourcehost.Chapter, error) {
	mangaID, err := strconv.ParseInt(mangaKey, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("suwayomi: bad manga key %q", mangaKey)
	}
	var out struct {
		FetchChapters struct {
			Chapters []struct {
				ID            int64   `json:"id"`
				Name          string  `json:"name"`
				ChapterNumber float64 `json:"chapterNumber"`
				Scanlator     string  `json:"scanlator"`
			} `json:"chapters"`
		} `json:"fetchChapters"`
	}
	err = c.gql(ctx, `
		mutation($mangaId: Int!) {
			fetchChapters(input: {mangaId: $mangaId}) {
				chapters { id name chapterNumber scanlator }
			}
		}`,
		map[string]any{"mangaId": mangaID}, &out)
	if err != nil {
		return nil, err
	}
	chapters := make([]sourcehost.Chapter, 0, len(out.FetchChapters.Chapters))
	for _, ch := range out.FetchChapters.Chapters {
		chapters = append(chapters, sourcehost.Chapter{
			Key:       strconv.FormatInt(ch.ID, 10),
			Name:      ch.Name,
			Number:    ch.ChapterNumber,
			Scanlator: ch.Scanlator,
		})
	}
	return chapters, nil
}

// Pages fetches the page list of a chapter; the refs are server-relative
// URLs into Suwayomi's page proxy (live-validated 2026-06-11).
func (c *Client) Pages(ctx context.Context, _ string, chapterKey string) ([]string, error) {
	chapterID, err := strconv.ParseInt(chapterKey, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("suwayomi: bad chapter key %q", chapterKey)
	}
	var out struct {
		FetchChapterPages struct {
			Pages []string `json:"pages"`
		} `json:"fetchChapterPages"`
	}
	err = c.gql(ctx, `
		mutation($chapterId: Int!) {
			fetchChapterPages(input: {chapterId: $chapterId}) { pages }
		}`,
		map[string]any{"chapterId": chapterID}, &out)
	if err != nil {
		return nil, err
	}
	return out.FetchChapterPages.Pages, nil
}

// Page streams one page image. Suwayomi proxies (and caches) the upstream
// fetch, so source auth never surfaces here.
func (c *Client) Page(ctx context.Context, _ string, pageRef string) (io.ReadCloser, error) {
	url := pageRef
	if strings.HasPrefix(pageRef, "/") {
		url = c.baseURL + pageRef
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	// No client timeout on the streaming body: rely on ctx (adapter §3).
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("suwayomi: page fetch returned %d", resp.StatusCode)
	}
	return resp.Body, nil
}

var _ sourcehost.Host = (*Client)(nil)
