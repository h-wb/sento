package server

import (
	"github.com/h-wb/sento/internal/ledger"
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

// SeriesDetail is GET /series/{id}.
type SeriesDetail struct {
	Series        *store.Series        `json:"series"`
	Collections   []store.Collection   `json:"collections"`
	ChapterLedger *store.ChapterLedger `json:"chapterLedger"`
}

// SlotsResponse is GET /series/{id}/slots: volume slots keyed by
// collection id, plus the chapter ledger slots.
type SlotsResponse struct {
	Volumes  map[string][]ledger.VolumeSlot `json:"volumes"`
	Chapters []ledger.ChapterSlot           `json:"chapters,omitempty"`
}

// SearchBody is POST /series/{id}/search.
type SearchBody struct {
	Ledger string              `json:"ledger"`
	DryRun bool                `json:"dryRun"`
	Slot   *service.TargetSlot `json:"slot"`
	Grab   string              `json:"grab"` // GUID of one candidate
	Force  bool                `json:"force"`
	Stream bool                `json:"stream"`
}

// SearchResponse is the non-streaming POST /series/{id}/search reply.
type SearchResponse struct {
	service.SearchResult `tstype:",extends"`
	Queued               []int64 `json:"queued"`
}

// ChapterLedgerPatch is PUT /series/{id}/chapter-ledger (nil = keep).
type ChapterLedgerPatch struct {
	Monitored *bool   `json:"monitored"`
	Language  *string `json:"language"`
	CutoffNum *int64  `json:"cutoffNum"`
}

// SeriesPatch is PUT /series/{id}: series-level monitoring (gates the
// scheduler's automatic searches).
type SeriesPatch struct {
	Monitored bool `json:"monitored"`
}

// CollectionPatch is PUT /series/{id}/collections/{cid}.
type CollectionPatch struct {
	Monitored bool `json:"monitored"`
}

// CreateCollectionBody is POST /series/{id}/collections — a custom
// edition. Slot structure clones from basedOn (language variants) or
// generates volumes 1..N (editions MangaBaka doesn't know).
type CreateCollectionBody struct {
	EditionName string `json:"editionName"`
	Language    string `json:"language"`
	Publisher   string `json:"publisher"`
	Medium      string `json:"medium"`
	BasedOn     int64  `json:"basedOn"` // collection id; 0 = from scratch
	Volumes     int    `json:"volumes"` // only when basedOn is 0
	Monitored   bool   `json:"monitored"`
}

// MergeCollectionBody is POST /series/{id}/collections/{cid}/merge.
type MergeCollectionBody struct {
	Into int64 `json:"into"`
}

// MergeCollectionResponse lists volume numbers (hundredths) whose files
// have no matching slot in the target edition.
type MergeCollectionResponse struct {
	Unmatched []int64 `json:"unmatched"`
}

// IndexerSettingPatch is PUT /settings/indexers/{provider}/{sid} (nil = keep).
type IndexerSettingPatch struct {
	Enabled     *bool `json:"enabled"`
	UseVolumes  *bool `json:"useVolumes"`
	UseChapters *bool `json:"useChapters"`
	Priority    *int  `json:"priority"`
}

// OverridePatch is PUT /series/{id}/indexers/{provider}/{sid}; all-nil
// clears the override (inherit globals).
type OverridePatch struct {
	Enabled     *bool `json:"enabled"`
	UseVolumes  *bool `json:"useVolumes"`
	UseChapters *bool `json:"useChapters"`
}

// NamingPreview is POST /settings/media/preview: the templates of the
// posted MediaSettings rendered against a fixed sample series.
type NamingPreview struct {
	SeriesFolder string `json:"seriesFolder"`
	VolumeFile   string `json:"volumeFile"`
	ChapterFile  string `json:"chapterFile"`
}

// BindingPatch is PUT /series/{id}/bindings/{bid}.
type BindingPatch struct {
	State string `json:"state" tstype:"'active' | 'disabled'"`
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
