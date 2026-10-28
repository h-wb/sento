// Package metadata defines Sento's domain types for series metadata and the
// MetadataProvider interface. Provider-specific shapes (MangaBaka) never
// leak past their adapter package (spec.md §5.1, D13).
package metadata

import "context"

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

// Collection is one product line of a series as the catalog lists it: a
// language, a publisher, an edition and a medium. Collections that share
// an edition, a language and a type hold the same books in different media.
type Collection struct {
	ProviderID  string // the catalog's collection id
	EditionID   string // the catalog's edition id, shared across series
	EditionName string
	Language    string
	Publisher   string
	Medium      string // digital|paperback|hardcover
	Format      string // paged|longstrip
	Type        string // volume|chapter|one-shot|other
	Status      string
	RelatedID   string // a related collection (publisher change)
	CountMain   int
	CountExtra  int
	CountOther  int
	StartDate   string
	EndDate     string
	RawJSON     []byte
}

// Work is one released book (volume) inside a collection.
type Work struct {
	ProviderID  string
	NumberKnown bool
	Revision    string
	SubTitle    string
	SequenceNum Num
	SequenceStr string
	CountType   string // main|extra|other
	ReleaseDate string
	ISBN        string
	Pages       int
	RawJSON     []byte
}

// Edition is one of the catalog's edition names ("Standard Edition",
// "3-in-1 Edition"), shared by every series that has it.
type Edition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Provider is the anti-corruption boundary for series metadata.
type Provider interface {
	// GetEditions lists every edition the catalog knows.
	GetEditions(ctx context.Context) ([]Edition, error)
	Search(ctx context.Context, query string) ([]SeriesRef, error)
	GetSeries(ctx context.Context, id int64) (*Series, error)
	GetCollections(ctx context.Context, seriesID int64) ([]Collection, error)
	// GetWorks returns all works of a series keyed by collection provider ID.
	GetWorks(ctx context.Context, seriesID int64) (map[string][]Work, error)
}
