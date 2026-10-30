package service

import (
	"context"
	"slices"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// fileBooks separates intrinsic contents from the job's selected books.
// Metadata and naming describe the full file. Job claims remain separate;
// version fulfillment is derived from those contents by the existing rules.
func (im *Importer) fileBooks(ctx context.Context, res parse.Result, expected []store.BookRef) (contents, covered []store.BookRef, err error) {
	seen := map[int64]bool{}
	var sh *shelf
	for _, sl := range expected {
		if seen[sl.EditionID] {
			continue
		}
		seen[sl.EditionID] = true
		col, e := im.store.GetEdition(ctx, sl.EditionID)
		if e != nil {
			return nil, nil, e
		}
		if sh == nil {
			eds, e := im.store.ListEditions(ctx, col.SeriesID)
			if e != nil {
				return nil, nil, e
			}
			sh = newShelf(eds)
		}
		if !sh.fits(res, *col) {
			continue
		}
		books, e := im.store.ListBooks(ctx, sl.EditionID)
		if e != nil {
			return nil, nil, e
		}
		matched, ambiguous := resolveBooks(books, bookRanges(res, sl.Ledger), res.Extra)
		if ambiguous {
			return nil, nil, nil
		}
		for _, b := range matched {
			if b.SyncState == "retired" || b.SyncState == "stale" {
				continue
			}
			ref := store.BookRef{BookID: b.ID, EditionID: sl.EditionID, VersionID: sl.VersionID, Ledger: sl.Ledger, Number: b.SequenceNum}
			contents = append(contents, ref)
			for _, requested := range expected {
				if requested.BookID == b.ID && requested.EditionID == sl.EditionID && !slices.Contains(covered, requested) {
					covered = append(covered, requested)
				}
			}
		}
	}
	return contents, covered, nil
}
