package server

import (
	"encoding/json"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Request and response shapes of the HTTP API. Named (rather than inline
// maps/structs) so the TypeScript client types are generated from them —
// this file is the API schema (mise run gen).

// StatusResponse is GET /system/status.
type StatusResponse struct {
	App      string         `json:"app"`
	Version  string         `json:"version"`
	Commit   string         `json:"commit"`
	Database DatabaseStatus `json:"database"`
}

type DatabaseStatus struct {
	OK bool `json:"ok"`
}

// SeriesDetail is GET /series/{id}. The collections list carries both
// volume editions and chapter collections (ctype distinguishes them).
type SeriesDetail struct {
	Series      *store.Series      `json:"series"`
	Collections []store.Collection `json:"collections"`
}

// SearchBody is POST /series/{id}/search.
type SearchBody struct {
	GrabProvider string               `json:"grabProvider,omitempty"`
	GrabTargetID int64                `json:"grabTargetId,omitempty"`
	Ledger       string               `json:"ledger"`
	DryRun       bool                 `json:"dryRun"`
	Scope        *service.SearchScope `json:"scope"`
	Grab         string               `json:"grab"` // GUID of one candidate
	Force        bool                 `json:"force"`
	Stream       bool                 `json:"stream"`
}

// SearchResponse is the non-streaming POST /series/{id}/search reply.
type SearchResponse struct {
	service.SearchResult `tstype:",extends"`
	Queued               []int64 `json:"queued"`
}

// SeriesPatch is PUT /series/{id} (nil = keep): series-level monitoring
// (gates the scheduler's automatic searches) and the release profile
// (0 = back to the instance default).
type SeriesPatch struct {
	Monitored *bool  `json:"monitored"`
	ProfileID *int64 `json:"profileId"`
}

// CollectionPatch is PUT /series/{id}/collections/{cid} (nil = keep);
// ProfileID 0 = inherit the series profile. CutoffNum applies to chapter
// collections, Language to custom collections only.
type CollectionPatch struct {
	Monitored *bool   `json:"monitored"`
	ProfileID *int64  `json:"profileId"`
	CutoffNum *int64  `json:"cutoffNum"`
	Language  *string `json:"language"`
}

// CreateCollectionBody is POST /series/{id}/collections — a custom
// collection. Volume editions clone their book structure from basedOn
// (language variants) or generate volumes 1..N (editions MangaBaka
// doesn't know); chapter collections derive books from total_chapters.
type CreateCollectionBody struct {
	EditionName string `json:"editionName"`
	Language    string `json:"language"`
	Publisher   string `json:"publisher"`
	Medium      string `json:"medium"`
	Type        string `json:"type" tstype:"'' | 'volume' | 'chapter'"` // default volume
	BasedOn     int64  `json:"basedOn"`                                 // collection id; 0 = from scratch
	Volumes     int    `json:"volumes"`                                 // only when basedOn is 0
	CutoffNum   int64  `json:"cutoffNum"`                               // chapter type: monitor from N onward
	Monitored   bool   `json:"monitored"`
}

// MergeCollectionBody is POST /series/{id}/collections/{cid}/merge.
type MergeCollectionBody struct {
	Into int64 `json:"into"`
}

// MergeCollectionResponse lists volume numbers (hundredths) whose files
// have no matching book in the target edition.
type MergeCollectionResponse struct {
	Unmatched []int64 `json:"unmatched"`
}

// IndexerBody creates or updates one indexer entry. Settings is the
// protocol-specific document: {url, apiKey} for torznab, {sourceId} for
// source.
type IndexerBody struct {
	Name        string          `json:"name"`
	Protocol    string          `json:"protocol" tstype:"'torznab' | 'source'"`
	Enabled     bool            `json:"enabled"`
	UseVolumes  bool            `json:"useVolumes"`
	UseChapters bool            `json:"useChapters"`
	Priority    int             `json:"priority"`
	Settings    json.RawMessage `json:"settings" tstype:"any"`
}

// IndexerPatch toggles entry fields inline (nil = keep).
type IndexerPatch struct {
	Enabled     *bool `json:"enabled"`
	UseVolumes  *bool `json:"useVolumes"`
	UseChapters *bool `json:"useChapters"`
	Priority    *int  `json:"priority"`
}

// BrowseProwlarrBody asks for the Add Indexer discovery assist.
type BrowseProwlarrBody struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// OverridePatch is PUT /series/{id}/indexers/{iid}; all-nil clears the
// override (inherit globals).
type OverridePatch struct {
	Enabled     *bool `json:"enabled"`
	UseVolumes  *bool `json:"useVolumes"`
	UseChapters *bool `json:"useChapters"`
}

// NamingPreview is POST /settings/media/preview: the templates of the
// posted MediaSettings rendered against a fixed sample series.
type NamingPreview struct {
	// VolumePath and ChapterPath are whole example paths under the
	// library root: top folder, series folder, book folder, file.
	VolumePath   string `json:"volumePath"`
	ChapterPath  string `json:"chapterPath"`
	BookFolder   string `json:"bookFolder"`
	SeriesFolder string `json:"seriesFolder"`
	VolumeFile   string `json:"volumeFile"`
	ChapterFile  string `json:"chapterFile"`
}

// SplitBoxBody is POST /series/{id}/books/{bid}/split: how many volumes
// the box holds.
type SplitBoxBody struct {
	Count int `json:"count"`
}

// OrganizeBody is POST /series/{id}/files/organize: which of the
// previewed changes to apply.
type OrganizeBody struct {
	Rename bool `json:"rename"`
	Retag  bool `json:"retag"`
}

// BindingPatch is PUT /series/{id}/bindings/{bid}.
type BindingPatch struct {
	TrustChapters *bool  `json:"trustChapters,omitempty"`
	CollectionID  *int64 `json:"collectionId"`
	State         string `json:"state" tstype:"'active' | 'disabled'"`
}

// ScanBody is POST /series/{id}/import.
type ScanBody struct {
	Path string `json:"path"`
}

// ImportScanBody is POST /library/import/scan (empty root = library path).
type ImportScanBody struct {
	Root string `json:"root"`
}

// ImportFolderBody is POST /library/import.
type ImportFolderBody struct {
	Dir         string `json:"dir"`
	MangaBakaID int64  `json:"mangabakaId"`
}

// ImportFolderResponse confirms one folder import.
type ImportFolderResponse struct {
	SeriesID int64               `json:"seriesId"`
	Scan     *service.ScanResult `json:"scan"`
}

// SetPreferenceBody sets one source preference value (string, bool, or
// string array depending on the preference's valueType).
type SetPreferenceBody struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value" tstype:"unknown"`
}

// ExtensionRepos is the extension repo index URL list (none by default;
// the user adds Mihon-compatible repos).
type ExtensionRepos struct {
	URLs []string `json:"urls"`
	// Repos is the same list with what each repo currently yields on the
	// host (response only).
	Repos []extensions.RepoStatus `json:"repos,omitempty"`
}
