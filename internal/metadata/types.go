// Package metadata defines Sento's domain types for series metadata and the
// MetadataProvider interface. Provider-specific shapes (MangaBaka) never
// leak past their adapter package (spec.md §5.1, D13).
package metadata

import (
	"context"
	"strings"
)

// Num is a fixed-point volume/chapter number in hundredths (parser convention):
// volume 1 → 100, chapter 34.5 → 3450.
type Num int64

type SeriesRef struct {
	MangaBakaID int64  `json:"mangabakaId"`
	Title       string `json:"title"`
	Year        int    `json:"year,omitempty"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	CoverURL    string `json:"coverUrl,omitempty"`
}

type Series struct {
	MangaBakaID   int64
	Title         string
	NativeTitle   string
	Titles        []Title // all titles/aliases for matching
	Authors       []string
	Artists       []string
	Description   string
	Year          int
	Type          string // manga|manhwa|manhua|…
	Status        string // releasing|completed|…
	ContentRating string
	TotalChapters int // count; 0 = unknown
	FinalVolume   int
	CoverURL      string
	Genres        []string
	RawJSON       []byte // exact provider payload (schema-versioned cache)
}

type Title struct {
	Language string
	Title    string
	Primary  bool
}

// Collection supplies edition facts. Sento owns local identity; provider
// references and descriptors are reconciliation evidence.
type Collection struct {
	ProviderID  string // volatile
	EditionName string
	Language    string
	Publisher   string
	Medium      string
	Type        string // volume|chapter
	CountMain   int
	CountExtra  int
	CountOther  int
	StartDate   string
	EndDate     string
	RawJSON     []byte
}

// Descriptor returns a comparison tuple, not an identity key.
func (c Collection) Descriptor() [5]string {
	return [5]string{c.EditionName, c.Language, c.Publisher, c.Medium, c.Type}
}

// Work is one released unit (volume) inside a collection.
type Work struct {
	ProviderID  string
	NumberKnown bool
	Revision    string
	SequenceNum Num
	SequenceStr string
	CountType   string // main|extra|other
	ReleaseDate string
	ISBN        string
	Pages       int
	RawJSON     []byte
}

// Provider is the anti-corruption boundary for series metadata.
type Provider interface {
	Search(ctx context.Context, query string) ([]SeriesRef, error)
	GetSeries(ctx context.Context, id int64) (*Series, error)
	GetCollections(ctx context.Context, seriesID int64) ([]Collection, error)
	// GetWorks returns all works of a series keyed by collection provider ID.
	GetWorks(ctx context.Context, seriesID int64) (map[string][]Work, error)
}

func EditionLayout(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "omnibus"):
		return "omnibus"
	case strings.Contains(n, "box"):
		return "boxset"
	case strings.Contains(n, "deluxe"):
		return "deluxe"
	default:
		return "standard"
	}
}
func ContentVariant(name string) string {
	n := strings.ToLower(name)
	if strings.Contains(n, "color") || strings.Contains(n, "colour") {
		return "colored"
	}
	return "original"
}
