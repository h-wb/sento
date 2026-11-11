package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
)

type VersionSummary struct {
	VersionID     int64  `json:"versionId"`
	EditionID     int64  `json:"editionId"`
	Total         int    `json:"total"`
	Owned         int    `json:"owned"`
	OnDisk        int    `json:"onDisk"`
	Wanted        int    `json:"wanted"`
	Review        int    `json:"review"`
	ProfileName   string `json:"profileName"`
	ProfileSource string `json:"profileSource"`
}
type EditionOverview struct {
	Versions     []VersionSummary `json:"versions"`
	BooksOnDisk  int              `json:"booksOnDisk"`
	WantedCopies int              `json:"wantedCopies"`
	ReviewFiles  int              `json:"reviewFiles"`
}

func (l *Library) EditionOverview(ctx context.Context, sid int64) (*EditionOverview, error) {
	v, err := loadLedger(ctx, l.store, sid)
	if err != nil {
		return nil, err
	}
	series, err := l.store.GetSeries(ctx, sid)
	if err != nil {
		return nil, err
	}
	out := &EditionOverview{Versions: []VersionSummary{}}
	unique := map[int64]bool{}
	for _, t := range v.Versions {
		row := VersionSummary{VersionID: t.ID, EditionID: t.EditionID, Total: len(v.Status[t.ID]), ProfileSource: "instance default"}
		if series.ProfileID > 0 {
			row.ProfileSource = "series"
		}
		if t.ProfileID > 0 {
			row.ProfileSource = "card"
		}
		if p := v.Profiles[t.ID]; p != nil {
			row.ProfileName = p.Name
		}
		for _, sl := range v.Status[t.ID] {
			if len(sl.FileIDs) > 0 {
				row.Owned++
			}
			if len(sl.OnDiskFileIDs) > 0 {
				row.OnDisk++
				unique[sl.Book.ID] = true
			}
			if sl.Wanted {
				row.Wanted++
				out.WantedCopies++
			}
			if sl.State == "needs_review" || sl.State == "storage_unavailable" {
				row.Review++
			}
		}
		out.Versions = append(out.Versions, row)
	}
	out.BooksOnDisk = len(unique)
	files, err := l.store.ListFiles(ctx, sid)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.NeedsReview {
			out.ReviewFiles++
		}
	}
	return out, nil
}

// SeriesProgress is how much of what a series collects is in the library:
// copies whose card holds an accepted file, of the copies its cards
// collect that are out. The Library draws it under each poster.
type SeriesProgress struct {
	SeriesID  int64 `json:"seriesId"`
	Owned     int   `json:"owned"`
	Collected int   `json:"collected"`
}

// Progress counts, for every series, the copies its cards own and collect.
// A file a card refuses, or a download still running, is not owned yet.
func (l *Library) Progress(ctx context.Context) ([]SeriesProgress, error) {
	series, err := l.store.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SeriesProgress, 0, len(series))
	for _, sr := range series {
		v, err := loadLedger(ctx, l.store, sr.ID)
		if err != nil {
			return nil, err
		}
		row := SeriesProgress{SeriesID: sr.ID}
		for _, t := range v.Versions {
			for _, sl := range v.Status[t.ID] {
				switch sl.State {
				case StateUnmonitored, StateNotReleased:
					continue
				case StateOwned, StateUpgradeEligible:
					row.Owned++
				}
				row.Collected++
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// VersionBooks lists every book of one collected version with its ledger
// state. The list is not paged: the ledger is evaluated whole either way,
// and the Editions view needs all of it to draw the overview grid, count
// each state and select ranges.
func (l *Library) VersionBooks(ctx context.Context, sid, tid int64) ([]BookStatus, error) {
	v, err := loadLedger(ctx, l.store, sid)
	if err != nil {
		return nil, err
	}
	slots, ok := v.Status[tid]
	if !ok {
		return nil, apperr.NotFoundf("card not found")
	}
	return slots, nil
}
