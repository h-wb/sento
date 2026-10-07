package service

import (
	"context"
	"strconv"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// maxBoxVolumes bounds a split: no box holds more books than this.
const maxBoxVolumes = 100

// SplitBox records that one catalog item is a box of several volumes.
// Catalogs list a box set as the single thing you buy (one entry, one
// ISBN); releases and readers deal in the books inside it. The split adds
// volumes 1..count to the same edition, carrying the box's ISBN and
// publication date, moves the box out of the main run ("other") and stops
// collecting it in every version of the edition. The box entry stays, with its catalog
// link. Running it again only adds what
// is missing.
func (l *Library) SplitBox(ctx context.Context, seriesID, workID int64, count int) ([]store.Work, error) {
	if count < 2 || count > maxBoxVolumes {
		return nil, apperr.Invalidf("a box holds between 2 and %d volumes", maxBoxVolumes)
	}
	box, err := l.store.GetWork(ctx, workID)
	if err != nil {
		return nil, err
	}
	col, err := l.store.GetCollection(ctx, box.CollectionID)
	if err != nil {
		return nil, err
	}
	if col.SeriesID != seriesID {
		return nil, store.ErrNotFound
	}
	if col.Type != "volume" {
		return nil, apperr.Invalidf("only a volume can be a box of volumes")
	}
	existing, err := l.store.ListWorks(ctx, col.ID)
	if err != nil {
		return nil, err
	}
	byNumber := map[int64]store.Work{}
	for _, w := range existing {
		if w.ID != box.ID && w.CountType == "main" && w.NumberKnown {
			byNumber[w.SequenceNum] = w
		}
	}

	volumes := make([]store.Work, 0, count)
	for n := 1; n <= count; n++ {
		number := int64(n) * 100
		w, ok := byNumber[number]
		if !ok {
			id, err := l.store.SaveWork(ctx, seriesID, store.Work{
				CollectionID: col.ID, SequenceNum: number, SequenceStr: strconv.Itoa(n),
				CountType: "main", NumberKnown: true, ReleaseDate: box.ReleaseDate, ISBN: box.ISBN,
			})
			if err != nil {
				return nil, err
			}
			saved, err := l.store.GetWork(ctx, id)
			if err != nil {
				return nil, err
			}
			w = *saved
		}
		volumes = append(volumes, w)
	}

	// The box is a container, not a book: out of the main run, so it never
	// competes with a volume for a number, and explicitly not collected.
	if box.CountType == "main" {
		box.CountType = "other"
		if _, err := l.store.SaveWork(ctx, seriesID, *box); err != nil {
			return nil, err
		}
	}
	targets, err := l.store.ListTargets(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if t.CollectionID != col.ID {
			continue
		}
		if err := l.store.BulkMonitor(ctx, seriesID, t.ID, []int64{box.ID}, false); err != nil {
			return nil, err
		}
	}
	return volumes, nil
}
