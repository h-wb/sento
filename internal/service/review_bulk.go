package service

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// Bulk file review: one decision for a whole pack of files instead of one
// dialog per file. Both actions stay explicit — nothing here ever accepts
// a proposal on its own (propose-dont-ask.md §A1).

// ResolveFileReviews declines files in bulk: each one is marked reviewed
// and unmapped, exactly as saving its mapping with no books would, but in
// one transaction. A file that no longer waits for review is skipped with
// its reason, never overwritten. Afterwards the download the pack came in
// settles like any other reviewed import.
func (im *Importer) ResolveFileReviews(ctx context.Context, seriesID int64, fileIDs []int64) (*store.ReviewResolution, error) {
	if len(fileIDs) == 0 || len(fileIDs) > 500 {
		return nil, apperr.Invalidf("select between 1 and 500 files")
	}
	im.mu.Lock()
	defer im.mu.Unlock()
	res, err := im.store.DeclineFiles(ctx, seriesID, fileIDs)
	if err != nil {
		return nil, err
	}
	if len(res.Resolved) > 0 {
		if err = im.SettleReviewedJobs(ctx, seriesID); err != nil {
			slog.Warn("settling reviewed jobs failed", "series", seriesID, "error", err)
		}
	}
	return res, nil
}

// ReleaseFileMatch is one file's outcome of matching a whole release at
// once: the books its own numbers resolved to, or why it was left for
// individual review.
type ReleaseFileMatch struct {
	FileID int64 `json:"fileId"`
	// Books is empty when the file was left; Note then says why.
	Books []store.BookRef `json:"books"`
	// Label names the resolved books in words ("volume 3").
	Label string `json:"label,omitempty"`
	// Note says why the file was left for individual review, or what to
	// check on a mapped one (an organize failure).
	Note string `json:"note,omitempty"`
}

// MatchReleaseFiles applies one edition choice to a whole pack: every file
// is mapped by its own parsed numbers onto the chosen collected version,
// through the same upgrade path as one file's review, and organized like a
// confirmed import. Files whose numbers do not resolve — an unreadable
// name, the other ledger, numbers the edition does not hold, a number two
// books share — are left for individual review rather than guessed.
func (im *Importer) MatchReleaseFiles(ctx context.Context, seriesID, versionID int64, fileIDs []int64) ([]ReleaseFileMatch, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	out, err := releaseFileMatches(ctx, im.store, seriesID, versionID, fileIDs)
	if err != nil {
		return nil, err
	}
	for i := range out {
		m := &out[i]
		if len(m.Books) == 0 {
			continue
		}
		if _, err = im.applyUpgrades(ctx, seriesID, m.FileID, m.Books, false); err != nil {
			m.Books, m.Label, m.Note = []store.BookRef{}, "", err.Error()
			continue
		}
		// A failed move preserves the mapping and is reported per file.
		if err = im.organizeReviewedFile(ctx, seriesID, m.FileID); err != nil {
			m.Note = fmt.Sprintf("mapping saved, but organizing the file failed: %v", err)
		}
	}
	if err = im.SettleReviewedJobs(ctx, seriesID); err != nil {
		slog.Warn("settling reviewed jobs failed", "series", seriesID, "error", err)
	}
	return out, nil
}

// PreviewReleaseFiles uses the same resolver as submission, without changing
// mappings or files. The browser displays these decisions instead of deriving
// book identity from display numbers a second time.
func (l *Library) PreviewReleaseFiles(ctx context.Context, seriesID, versionID int64, fileIDs []int64) ([]ReleaseFileMatch, error) {
	return releaseFileMatches(ctx, l.store, seriesID, versionID, fileIDs)
}

func releaseFileMatches(ctx context.Context, st *store.Store, seriesID, versionID int64, fileIDs []int64) ([]ReleaseFileMatch, error) {
	if len(fileIDs) == 0 || len(fileIDs) > 500 {
		return nil, apperr.Invalidf("select between 1 and 500 files")
	}
	version, err := st.GetVersion(ctx, versionID)
	if err != nil {
		return nil, err
	}
	edition, err := st.GetEdition(ctx, version.EditionID)
	if err != nil {
		return nil, err
	}
	if edition.SeriesID != seriesID {
		return nil, store.ErrNotFound
	}
	p, err := newProposer(ctx, st, seriesID)
	if err != nil {
		return nil, err
	}
	out := make([]ReleaseFileMatch, 0, len(fileIDs))
	for _, fid := range slices.Compact(slices.Sorted(slices.Values(fileIDs))) {
		m := ReleaseFileMatch{FileID: fid, Books: []store.BookRef{}}
		slots, books, note, ok := p.matchSlots(fid, *edition, version.ID)
		if !ok {
			m.Note = note
			out = append(out, m)
			continue
		}
		m.Books = slots
		m.Label = bookLabel(edition.Type, books)
		out = append(out, m)
	}
	return out, nil
}

// matchSlots reads one file's own numbers against a chosen edition and
// version. ok is false when the file must stay for individual review; note
// then says why, in words for the person reviewing.
func (p *proposer) matchSlots(fileID int64, edition store.Edition, versionID int64) (slots []store.BookRef, books []store.Book, note string, ok bool) {
	f, exists := p.files[fileID]
	if !exists {
		return nil, nil, "File is no longer available", false
	}
	if !f.NeedsReview {
		return nil, nil, "File is no longer waiting for review", false
	}
	if f.Presence == "unavailable" {
		return nil, nil, "Storage unavailable; recheck this file first", false
	}
	if !f.Unrequested {
		return nil, nil, "This file already claims books; review it individually", false
	}
	parsed, ledger, _, _, _, parsedOK := p.parseFile(fileID)
	if !parsedOK {
		return nil, nil, "The file's own name does not say which books it holds", false
	}
	if ledger != edition.Type {
		return nil, nil, fmt.Sprintf("Holds %ss; this edition collects %ss", ledger, edition.Type), false
	}
	ranges := bookRanges(parsed, ledger)
	books, ambiguous := resolveBooks(p.books[edition.ID], ranges, parsed.Extra)
	switch {
	case ambiguous:
		return nil, nil, "Its number is shared by two books of this edition", false
	case len(books) == 0 || !everyNumberHeld(books, ranges):
		return nil, nil, fmt.Sprintf("Its numbers are not %ss of this edition", edition.Type), false
	}
	for _, b := range books {
		slots = append(slots, store.BookRef{VersionID: versionID, BookID: b.ID, EditionID: edition.ID, Ledger: ledger, Number: b.SequenceNum})
	}
	return slots, books, "", true
}
