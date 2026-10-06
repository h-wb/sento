package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
)

type TargetSummary struct {
	TargetID      int64  `json:"targetId"`
	CollectionID  int64  `json:"collectionId"`
	Total         int    `json:"total"`
	Owned         int    `json:"owned"`
	OnDisk        int    `json:"onDisk"`
	Wanted        int    `json:"wanted"`
	Review        int    `json:"review"`
	ProfileName   string `json:"profileName"`
	ProfileSource string `json:"profileSource"`
}
type EditionOverview struct {
	Targets      []TargetSummary `json:"targets"`
	BooksOnDisk  int             `json:"booksOnDisk"`
	WantedCopies int             `json:"wantedCopies"`
	ReviewFiles  int             `json:"reviewFiles"`
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
	out := &EditionOverview{Targets: []TargetSummary{}}
	unique := map[int64]bool{}
	for _, t := range v.Targets {
		row := TargetSummary{TargetID: t.ID, CollectionID: t.CollectionID, Total: len(v.Slots[t.ID]), ProfileSource: "instance default"}
		c := collectionByID(v.Collections, t.CollectionID)
		if series.ProfileID > 0 {
			row.ProfileSource = "series"
		}
		if c.ProfileID > 0 {
			row.ProfileSource = "edition"
		}
		if t.ProfileID > 0 {
			row.ProfileSource = "this version"
		}
		if p := v.Profiles[t.ID]; p != nil {
			row.ProfileName = p.Name
		}
		for _, sl := range v.Slots[t.ID] {
			if len(sl.FileIDs) > 0 {
				row.Owned++
			}
			if len(sl.OnDiskFileIDs) > 0 {
				row.OnDisk++
				unique[sl.Work.ID] = true
			}
			if sl.Wanted {
				row.Wanted++
				out.WantedCopies++
			}
			if sl.State == "needs_review" || sl.State == "storage_unavailable" {
				row.Review++
			}
		}
		out.Targets = append(out.Targets, row)
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

type UnitPage struct {
	Items []UnitSlot `json:"items"`
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Pages int        `json:"pages"`
}

func (l *Library) TargetPage(ctx context.Context, sid, tid int64, page int, filter string) (*UnitPage, error) {
	v, err := loadLedger(ctx, l.store, sid)
	if err != nil {
		return nil, err
	}
	slots, ok := v.Slots[tid]
	if !ok {
		return nil, apperr.NotFoundf("collected version not found")
	}
	filtered := []UnitSlot{}
	for _, sl := range slots {
		match := filter == "" || filter == "all" || filter == sl.State || filter == "wanted" && sl.Wanted || filter == "on_disk" && len(sl.OnDiskFileIDs) > 0 || filter == "review" && (sl.State == "needs_review" || sl.State == "storage_unavailable" || sl.State == "unsatisfactory")
		if match {
			filtered = append(filtered, sl)
		}
	}
	pages := max(1, (len(filtered)+24)/25)
	page = max(1, min(page, pages))
	start := (page - 1) * 25
	end := min(start+25, len(filtered))
	return &UnitPage{Items: filtered[start:end], Total: len(filtered), Page: page, Pages: pages}, nil
}
