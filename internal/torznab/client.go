// Package torznab implements the Torznab/Newznab search protocol — the
// lingua franca of indexers. Every Prowlarr/Jackett indexer is exposed
// as one Torznab endpoint, so Sento's per-indexer entries speak it
// directly and need no aggregator concept.
package torznab

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	url    string
	apiKey string
	http   *http.Client
}

func New(endpoint, apiKey string) *Client {
	return &Client{
		url:    strings.TrimRight(endpoint, "/"),
		apiKey: apiKey,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

// Release is one feed item with the torznab attributes Sento uses.
type Release struct {
	Title       string
	GUID        string
	InfoURL     string // release details page (comments link)
	DownloadURL string // .torrent (item link)
	MagnetURL   string
	InfoHash    string
	Size        int64
	Seeders     int
	// Categories are the indexer's category ids for the release: Newznab
	// standard ones and the indexer's own (ids from 100000).
	Categories []int
}

type feed struct {
	Channel struct {
		Items []item `xml:"item"`
	} `xml:"channel"`
}

type item struct {
	Title     string `xml:"title"`
	GUID      string `xml:"guid"`
	Link      string `xml:"link"`
	Comments  string `xml:"comments"`
	Size      int64  `xml:"size"`
	Enclosure struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
	} `xml:"enclosure"`
	Attrs []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	} `xml:"attr"`
}

func (c *Client) call(ctx context.Context, params url.Values) ([]byte, error) {
	params.Set("apikey", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	// A reverse proxy that guards the endpoint with a login answers an API
	// call by redirecting to its sign-in page, which then loads fine.
	if final := resp.Request.URL; final.Host != req.URL.Host {
		return nil, fmt.Errorf("torznab: %s redirected to %s instead of answering (is the endpoint behind a login proxy?)", req.URL.Host, final.Host)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("torznab: HTTP %d: %s", resp.StatusCode, truncate(raw, 160))
	}
	// Newznab transports errors as XML with HTTP 200.
	var apiErr struct {
		XMLName     xml.Name `xml:"error"`
		Description string   `xml:"description,attr"`
	}
	if xml.Unmarshal(raw, &apiErr) == nil && apiErr.XMLName.Local == "error" {
		return nil, fmt.Errorf("torznab: %s", apiErr.Description)
	}
	return raw, nil
}

// CategoryBooks is the Newznab parent category for reading material; it
// covers its subcategories (7010 magazines, 7020 ebooks, 7030 comics).
const CategoryBooks = 7000

// Search runs a free-text query (t=search) restricted to the given
// Newznab categories (none = everything the indexer has).
func (c *Client) Search(ctx context.Context, query string, categories ...int) ([]Release, error) {
	params := url.Values{"t": {"search"}, "q": {query}}
	if len(categories) > 0 {
		ids := make([]string, len(categories))
		for i, id := range categories {
			ids[i] = strconv.Itoa(id)
		}
		params.Set("cat", strings.Join(ids, ","))
	}
	raw, err := c.call(ctx, params)
	if err != nil {
		return nil, err
	}
	var f feed
	if err := xml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("torznab: decoding feed: %w", err)
	}
	out := make([]Release, 0, len(f.Channel.Items))
	for _, it := range f.Channel.Items {
		rel := Release{
			Title:       it.Title,
			GUID:        it.GUID,
			InfoURL:     it.Comments,
			DownloadURL: it.Link,
			Size:        it.Size,
		}
		// Prowlarr/Jackett usually mint the guid as the details-page URL;
		// use it when the feed has no comments link.
		if rel.InfoURL == "" && strings.HasPrefix(it.GUID, "http") {
			rel.InfoURL = it.GUID
		}
		if rel.DownloadURL == "" {
			rel.DownloadURL = it.Enclosure.URL
		}
		if rel.Size == 0 {
			rel.Size = it.Enclosure.Length
		}
		for _, a := range it.Attrs {
			switch a.Name {
			case "category":
				if id, err := strconv.Atoi(a.Value); err == nil {
					rel.Categories = append(rel.Categories, id)
				}
			case "seeders":
				rel.Seeders, _ = strconv.Atoi(a.Value)
			case "infohash":
				rel.InfoHash = strings.ToLower(a.Value)
			case "magneturl":
				rel.MagnetURL = a.Value
			case "size":
				if rel.Size == 0 {
					rel.Size, _ = strconv.ParseInt(a.Value, 10, 64)
				}
			}
		}
		if strings.HasPrefix(rel.DownloadURL, "magnet:") && rel.MagnetURL == "" {
			rel.MagnetURL = rel.DownloadURL
			rel.DownloadURL = ""
		}
		if rel.GUID == "" {
			rel.GUID = rel.Title
		}
		out = append(out, rel)
	}
	return out, nil
}

// Categories returns the indexer's category names by id, subcategories
// included (caps request). Indexers describe their own sections there,
// e.g. "Literature - English-translated".
func (c *Client) Categories(ctx context.Context) (map[int]string, error) {
	raw, err := c.call(ctx, url.Values{"t": {"caps"}})
	if err != nil {
		return nil, err
	}
	type category struct {
		ID   int    `xml:"id,attr"`
		Name string `xml:"name,attr"`
	}
	var caps struct {
		Categories []struct {
			category
			Subcats []category `xml:"subcat"`
		} `xml:"categories>category"`
	}
	if err := xml.Unmarshal(raw, &caps); err != nil {
		return nil, fmt.Errorf("torznab: decoding capabilities: %w", err)
	}
	out := map[int]string{}
	for _, c := range caps.Categories {
		out[c.ID] = c.Name
		for _, s := range c.Subcats {
			out[s.ID] = s.Name
		}
	}
	return out, nil
}

// Ping verifies the endpoint via a caps request.
func (c *Client) Ping(ctx context.Context) error {
	raw, err := c.call(ctx, url.Values{"t": {"caps"}})
	if err != nil {
		return err
	}
	var caps struct {
		XMLName xml.Name `xml:"caps"`
	}
	if err := xml.Unmarshal(raw, &caps); err != nil || caps.XMLName.Local != "caps" {
		return fmt.Errorf("torznab: endpoint did not return capabilities")
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
