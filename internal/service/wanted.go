package service

import (
	"context"
	"sort"

	"github.com/h-wb/sento/internal/store"
)

// WantedRow is one book the library is still waiting on: a missing,
// monitored slot the ledger marked wanted — never released, storage-blocked
// and unmonitored books do not qualify, and an upgrade-eligible copy is
// here too, with its reason saying so. The library-wide view behind the
// Wanted page.
type WantedRow struct {
	SeriesID     int64      `json:"seriesId"`
	SeriesTitle  string     `json:"seriesTitle"`
	EditionID    int64      `json:"editionId"`
	EditionName  string     `json:"editionName"`
	Language     string     `json:"language"`
	Ledger       string     `json:"ledger"`
	VersionID    int64      `json:"versionId"`
	Book         store.Book `json:"book"`
	Reason       string     `json:"reason"`
	Availability string     `json:"availability"`
	State        string     `json:"state"`
}

// Wanted walks every monitored series and lists the wanted slots of its
// editions, flattened and ordered: series title, then volume books before
// chapter books, then book order.
func (l *Library) Wanted(ctx context.Context) ([]WantedRow, error) {
	series, err := l.store.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	out := []WantedRow{}
	for _, sr := range series {
		if !sr.Monitored {
			continue
		}
		v, err := loadLedger(ctx, l.store, sr.ID)
		if err != nil {
			return nil, err
		}
		byID := map[int64]store.Edition{}
		for _, c := range v.Editions {
			byID[c.ID] = c
		}
		for _, t := range v.Versions {
			for _, sl := range v.Status[t.ID] {
				if !sl.Wanted {
					continue
				}
				col := byID[t.EditionID]
				out = append(out, WantedRow{
					SeriesID: sr.ID, SeriesTitle: sr.Title,
					EditionID: col.ID, EditionName: col.EditionName, Language: col.Language,
					Ledger: col.Type, VersionID: t.ID,
					Book: sl.Book, Reason: sl.Reason, Availability: sl.Availability, State: sl.State,
				})
			}
		}
	}
	ledgerRank := map[string]int{LedgerVolume: 0, LedgerChapter: 1}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SeriesTitle != b.SeriesTitle {
			return a.SeriesTitle < b.SeriesTitle
		}
		if ra, rb := ledgerRank[a.Ledger], ledgerRank[b.Ledger]; ra != rb {
			return ra < rb
		}
		if a.Book.SequenceNum != b.Book.SequenceNum {
			return a.Book.SequenceNum < b.Book.SequenceNum
		}
		return a.Book.ID < b.Book.ID
	})
	return out, nil
}
