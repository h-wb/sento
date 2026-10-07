package service

import (
	"context"
	"github.com/h-wb/sento/internal/store"
)

// Calendar uses the same monitoring and fulfillment evaluation as acquisition.
func (l *Library) Calendar(ctx context.Context, from, to string) ([]store.CalendarEntry, error) {
	entries, err := l.store.Calendar(ctx, from, to)
	if err != nil {
		return nil, err
	}
	bySeries := map[int64]map[string]UnitSlot{}
	out := []store.CalendarEntry{}
	for _, e := range entries {
		slots, ok := bySeries[e.SeriesID]
		if !ok {
			view, err := loadLedger(ctx, l.store, e.SeriesID)
			if err != nil {
				return nil, err
			}
			slots = map[string]UnitSlot{}
			for tid, books := range view.Slots {
				for _, book := range books {
					slots[store.BookKey(tid, book.Work.ID)] = book
				}
			}
			bySeries[e.SeriesID] = slots
		}
		book, ok := slots[store.BookKey(e.TargetID, e.WorkID)]
		if !ok || !book.Monitored {
			continue
		}
		e.Owned = len(book.FileIDs) > 0
		out = append(out, e)
	}
	return out, nil
}
