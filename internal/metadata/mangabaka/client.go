// Package mangabaka implements metadata.Provider against the MangaBaka API.
// All knowledge of MangaBaka response shapes lives here (spec.md §5.1).
package mangabaka

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

	"golang.org/x/time/rate"

	"github.com/h-wb/sento/internal/metadata"
)

const SchemaVersion = 1

type Client struct {
	baseURL string
	http    *http.Client
	// MangaBaka rate limits (spec.md §5.2): search 30/min, lookup 120/min.
	searchLimit *rate.Limiter
	lookupLimit *rate.Limiter
}

func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "https://api.mangabaka.org/v1"
	}
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		http:        &http.Client{Timeout: 30 * time.Second},
		searchLimit: rate.NewLimiter(rate.Every(2*time.Second), 5),
		lookupLimit: rate.NewLimiter(rate.Every(500*time.Millisecond), 10),
	}
}

var _ metadata.Provider = (*Client)(nil)

type envelope struct {
	Status     int             `json:"status"`
	Data       json.RawMessage `json:"data"`
	Pagination *pagination     `json:"pagination"`
}

type pagination struct {
	Count int     `json:"count"`
	Next  *string `json:"next"`
}

func (c *Client) get(ctx context.Context, limiter *rate.Limiter, rawURL string) (*envelope, error) {
	if err := limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sento (github.com/h-wb/sento)")

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
		return nil, fmt.Errorf("mangabaka: %s returned %d", rawURL, resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("mangabaka: decoding %s: %w", rawURL, err)
	}
	return &env, nil
}

// --- provider-shape structs (unstable surface, contained here) ---

type mbSeries struct {
	ID            int64           `json:"id"`
	State         string          `json:"state"`
	MergedWith    *int64          `json:"merged_with"`
	Title         string          `json:"title"`
	NativeTitle   string          `json:"native_title"`
	Authors       []string        `json:"authors"`
	Artists       []string        `json:"artists"`
	Description   string          `json:"description"`
	Year          int             `json:"year"`
	Status        string          `json:"status"`
	ContentRating string          `json:"content_rating"`
	Type          string          `json:"type"`
	FinalVolume   string          `json:"final_volume"`
	TotalChapters string          `json:"total_chapters"`
	Genres        []string        `json:"genres"`
	Cover         json.RawMessage `json:"cover"`
	Titles        []mbTitle       `json:"titles"`
}

type mbTitle struct {
	Language string `json:"language"`
	Title    string `json:"title"`
	Primary  bool   `json:"is_primary"`
}

type mbCover struct {
	Raw struct {
		URL string `json:"url"`
	} `json:"raw"`
	X350 struct {
		X1 string `json:"x1"`
	} `json:"x350"`
}

type mbCollection struct {
	ID        string `json:"id"`
	Publisher struct {
		Name string `json:"name"`
	} `json:"publisher"`
	Edition struct {
		Name string `json:"name"`
	} `json:"edition"`
	Language struct {
		ISO string `json:"iso"`
	} `json:"language"`
	Type      string `json:"type"`
	Medium    string `json:"medium"`
	CountMain int    `json:"count_main"`
	CountExt  int    `json:"count_extra"`
	CountOth  int    `json:"count_other"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

type mbWork struct {
	ID          string      `json:"id"`
	CountType   string      `json:"count_type"`
	ReleaseDate string      `json:"release_date"`
	SeqString   string      `json:"sequence_string"`
	SeqNumeric  json.Number `json:"sequence_numeric"`
	Pages       int         `json:"pages"`
	Identifiers []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"identifiers"`
	Collections []struct {
		ID string `json:"id"`
	} `json:"collections"`
}

// --- Provider implementation ---

func (c *Client) Search(ctx context.Context, query string) ([]metadata.SeriesRef, error) {
	u := fmt.Sprintf("%s/series/search?q=%s", c.baseURL, url.QueryEscape(query))
	env, err := c.get(ctx, c.searchLimit, u)
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		return nil, err
	}
	refs := make([]metadata.SeriesRef, 0, len(rows))
	for _, raw := range rows {
		var s mbSeries
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		refs = append(refs, metadata.SeriesRef{
			MangaBakaID: s.ID,
			Title:       s.Title,
			Year:        s.Year,
			Type:        s.Type,
			Status:      s.Status,
			CoverURL:    coverURL(s.Cover),
		})
	}
	return refs, nil
}

func (c *Client) GetSeries(ctx context.Context, id int64) (*metadata.Series, error) {
	u := fmt.Sprintf("%s/series/%d", c.baseURL, id)
	env, err := c.get(ctx, c.lookupLimit, u)
	if err != nil {
		return nil, err
	}
	var s mbSeries
	if err := json.Unmarshal(env.Data, &s); err != nil {
		return nil, err
	}
	if s.MergedWith != nil && *s.MergedWith != 0 && *s.MergedWith != id {
		// Follow merges transparently (spec.md §5.2).
		return c.GetSeries(ctx, *s.MergedWith)
	}

	out := &metadata.Series{
		MangaBakaID:   s.ID,
		Title:         s.Title,
		NativeTitle:   s.NativeTitle,
		Authors:       s.Authors,
		Artists:       s.Artists,
		Description:   s.Description,
		Year:          s.Year,
		Type:          s.Type,
		Status:        s.Status,
		ContentRating: s.ContentRating,
		TotalChapters: atoiLoose(s.TotalChapters),
		FinalVolume:   atoiLoose(s.FinalVolume),
		CoverURL:      coverURL(s.Cover),
		Genres:        s.Genres,
		RawJSON:       env.Data,
	}
	for _, t := range s.Titles {
		out.Titles = append(out.Titles, metadata.Title{Language: t.Language, Title: t.Title, Primary: t.Primary})
	}
	return out, nil
}

func (c *Client) GetCollections(ctx context.Context, seriesID int64) ([]metadata.Collection, error) {
	u := fmt.Sprintf("%s/series/%d/collections", c.baseURL, seriesID)
	env, err := c.get(ctx, c.lookupLimit, u)
	if err != nil {
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		return nil, err
	}
	cols := make([]metadata.Collection, 0, len(rows))
	for _, raw := range rows {
		var mc mbCollection
		if err := json.Unmarshal(raw, &mc); err != nil {
			continue
		}
		cols = append(cols, metadata.Collection{
			ProviderID:  mc.ID,
			EditionName: mc.Edition.Name,
			Language:    mc.Language.ISO,
			Publisher:   mc.Publisher.Name,
			Medium:      mc.Medium,
			Type:        mc.Type,
			CountMain:   mc.CountMain,
			CountExtra:  mc.CountExt,
			CountOther:  mc.CountOth,
			StartDate:   mc.StartDate,
			EndDate:     mc.EndDate,
			RawJSON:     raw,
		})
	}
	return cols, nil
}

func (c *Client) GetWorks(ctx context.Context, seriesID int64) (map[string][]metadata.Work, error) {
	out := map[string][]metadata.Work{}
	next := fmt.Sprintf("%s/series/%d/works", c.baseURL, seriesID)
	for next != "" {
		env, err := c.get(ctx, c.lookupLimit, next)
		if err != nil {
			return nil, err
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			return nil, err
		}
		for _, raw := range rows {
			var w mbWork
			if err := json.Unmarshal(raw, &w); err != nil {
				continue
			}
			work := metadata.Work{
				ProviderID:  w.ID,
				SequenceNum: seqToNum(w.SeqNumeric, w.SeqString),
				SequenceStr: w.SeqString,
				CountType:   w.CountType,
				ReleaseDate: w.ReleaseDate,
				Pages:       w.Pages,
				RawJSON:     raw,
			}
			for _, ident := range w.Identifiers {
				if ident.Name == "isbn" {
					work.ISBN = ident.ID
					break
				}
			}
			for _, col := range w.Collections {
				out[col.ID] = append(out[col.ID], work)
			}
		}
		next = ""
		if env.Pagination != nil && env.Pagination.Next != nil {
			next = *env.Pagination.Next
		}
	}
	return out, nil
}

// --- helpers ---

func coverURL(raw json.RawMessage) string {
	var c mbCover
	if err := json.Unmarshal(raw, &c); err != nil {
		return ""
	}
	if c.X350.X1 != "" {
		return c.X350.X1
	}
	return c.Raw.URL
}

// atoiLoose parses MangaBaka's stringly-typed counts ("1184", "114", "").
func atoiLoose(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func seqToNum(n json.Number, fallback string) metadata.Num {
	s := n.String()
	if s == "" && fallback != "" {
		s = fallback // volume 0 (prologue) is a legal sequence
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return metadata.Num(f*100 + 0.5)
}
