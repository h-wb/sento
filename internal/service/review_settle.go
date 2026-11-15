package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/store"
)

// Reviewed payloads can complete without redownloading or reparsing names that
// the user already corrected. Only assets from this offer can resolve its job.
func (im *Importer) reviewedJobComplete(ctx context.Context, item store.QueueItem) (bool, error) {
	if item.ReleaseKey == "" {
		return false, nil
	}
	if err := im.store.CheckFilePresence(ctx, item.SeriesID); err != nil {
		return false, err
	}
	plan, err := queueBooksErr(&item)
	if err != nil {
		return false, err
	}
	if len(plan) == 0 {
		return false, nil
	}
	view, err := loadLedger(ctx, im.store, item.SeriesID)
	if err != nil {
		return false, err
	}
	files, err := im.store.RecordedFiles(ctx, item.SeriesID)
	if err != nil {
		return false, err
	}
	belongs := map[int64]bool{}
	explicit, waiting := false, false
	for _, f := range files {
		belongs[f.ID] = f.Facts.ReleaseKey == item.ReleaseKey
		if belongs[f.ID] {
			explicit = explicit || f.ReviewAction != ""
			waiting = waiting || f.NeedsReview
		}
	}
	if explicit && !waiting {
		return true, nil
	}
	for _, sl := range plan {
		found := false
		if book := view.book(sl.VersionID, sl.BookID); book != nil {
			for _, fid := range book.FileIDs {
				if belongs[fid] {
					found = true
				}
			}
		}
		if !found {
			return false, nil
		}
	}
	return true, nil
}

// SettleReviewedJobs finishes the series' jobs held for review once no
// file they delivered still waits for it (propose-dont-ask.md §A2): the
// last confirmation closes the job, not another run of the importer.
// History says how many files were confirmed and which planned books no
// file turned out to hold; those are wanted again.
func (im *Importer) SettleReviewedJobs(ctx context.Context, seriesID int64) error {
	jobs, err := im.store.SeriesQueueItems(ctx, seriesID)
	if err != nil {
		return err
	}
	files, err := im.store.RecordedFiles(ctx, seriesID)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if job.Status != store.QueueNeedsReview || job.ReleaseKey == "" {
			continue
		}
		delivered, waiting := map[int64]bool{}, false
		for _, f := range files {
			if f.Facts.ReleaseKey == job.ReleaseKey {
				delivered[f.ID] = true
				waiting = waiting || f.NeedsReview
			}
		}
		// No files at all is an import that never happened (a path the
		// client sees and Sento does not): that still needs a person.
		if waiting || len(delivered) == 0 {
			continue
		}
		plan := queueBooks(&job)
		linked, _, err := im.alreadyImported(ctx, job, plan)
		if err != nil {
			return err
		}
		var missing []string
		var kept []string
		view, err := loadLedger(ctx, im.store, seriesID)
		if err != nil {
			return err
		}
		for _, ref := range plan {
			if !linked[store.BookKey(ref.VersionID, ref.BookID)] {
				book := view.book(ref.VersionID, ref.BookID)
				owned := book != nil && len(book.FileIDs) > 0
				if owned {
					kept = append(kept, im.bookName(ctx, ref))
				} else {
					missing = append(missing, im.bookName(ctx, ref))
				}
			}
		}
		settled, err := im.store.SettleReviewedQueue(ctx, job.ID)
		if err != nil || !settled {
			return err
		}
		if listing := jobListing(job); listing > 0 {
			_ = im.store.ListingWorked(ctx, listing)
		}
		note := fmt.Sprintf("%d %s confirmed in review.", len(delivered), plural(len(delivered), "file"))
		if len(kept) > 0 {
			note += " Kept existing: " + strings.Join(kept, ", ") + "."
		}
		if len(missing) > 0 {
			note += " Not in this download, wanted again: " + strings.Join(missing, ", ") + "."
			im.searchAgain(job)
		}
		if err := im.store.AddDecision(ctx, seriesID, store.DecisionBody{Event: "imported_after_review", Title: job.Title, Provider: job.ProviderID, Key: job.ExternalID, Note: note}); err != nil {
			return err
		}
	}
	return nil
}

// bookName names a planned book for people: "Box Set Edition volume 0".
func (im *Importer) bookName(ctx context.Context, ref store.BookRef) string {
	book, err := im.store.GetBook(ctx, ref.BookID)
	if err != nil {
		return ref.Ledger + " " + mediafile.RenderPlainNum(ref.Number)
	}
	name := ref.Ledger + " " + book.SequenceStr
	if ed, err := im.store.GetEdition(ctx, book.EditionID); err == nil {
		name = ed.EditionName + " " + name
	}
	return name
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
