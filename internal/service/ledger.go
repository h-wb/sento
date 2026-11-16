package service

import (
	"context"
	"time"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// ledgerView is loaded once per series and shared by search and UI evaluation.
type ledgerView struct {
	Series   *store.Series
	Versions []store.CollectedVersion
	Editions []store.Edition
	Books    map[int64][]store.Book
	Status   map[int64][]BookStatus
	Profiles map[int64]*profile.Profile
	Files    []store.File
}

// book finds one book's status in the view, or nil.
func (v *ledgerView) book(versionID, bookID int64) *BookStatus {
	for i := range v.Status[versionID] {
		if v.Status[versionID][i].Book.ID == bookID {
			return &v.Status[versionID][i]
		}
	}
	return nil
}

// cardFiles is what a card's profile makes of the files mapped to it:
// the mappings it accepts, each file's score, and per book why a file was
// refused.
type cardFiles struct {
	accepted map[int64][]int64 // book → accepted file ids
	pinned   map[int64]bool    // book → an accepted mapping is pinned
	scores   map[int64]int     // file → score
	refused  map[int64]string  // book → why its file does not count
}

func judgeCardFiles(col store.Edition, prof *profile.Profile, formats []profile.Format, coverage []store.AssetCoverage, byFile map[int64]store.File, present map[int64]bool) cardFiles {
	out := cardFiles{accepted: map[int64][]int64{}, pinned: map[int64]bool{}, scores: map[int64]int{}, refused: map[int64]string{}}
	for _, a := range coverage {
		judgement := judgeMappedFile(col, prof, formats, byFile[a.FileID], a)
		out.scores[a.FileID] = judgement.Score
		if judgement.Reason != "" {
			out.refused[a.BookID] = judgement.Reason
			continue
		}
		// A pin holds whether or not the pinned file is confirmed yet.
		out.pinned[a.BookID] = out.pinned[a.BookID] || a.Pinned
		if a.Verified && present[a.FileID] {
			out.accepted[a.BookID] = append(out.accepted[a.BookID], a.FileID)
		}
	}
	return out
}

func loadLedger(ctx context.Context, st *store.Store, seriesID int64) (*ledgerView, error) {
	series, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	versions, err := st.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	cols, err := st.ListEditions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	works, err := st.SeriesBooksByNumber(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	overrides, err := st.SeriesBookOverrides(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	coverage, err := st.Coverage(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	contents, err := st.Contents(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	pending, err := st.PendingBookStates(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	profiles, err := NewProfiles(st).seriesProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	idx := ledgerIndex{
		series: series, profiles: profiles, today: time.Now().Format("2006-01-02"),
		coverage: map[int64][]store.AssetCoverage{}, overrides: map[int64]map[int64]bool{},
		files: map[int64]store.File{}, present: map[int64]bool{}, unavailable: map[string]bool{},
		contents: map[int64][]store.AssetContent{}, pending: pending,
	}
	for _, a := range coverage {
		idx.coverage[a.VersionID] = append(idx.coverage[a.VersionID], a)
	}
	for _, o := range overrides {
		if idx.overrides[o.VersionID] == nil {
			idx.overrides[o.VersionID] = map[int64]bool{}
		}
		idx.overrides[o.VersionID][o.BookID] = o.Monitored
	}
	// Storage health is per folder: with chapters in their own folder, an
	// unreachable chapter folder says nothing about the volumes, and the
	// other way round. In a shared folder one bad file blocks both.
	for _, f := range files {
		idx.files[f.ID] = f
		idx.present[f.ID] = f.Presence == "present" && !f.NeedsReview
		if f.Presence == "unavailable" || f.Presence == "unknown" {
			switch {
			case series.ChapterPath == "":
				idx.unavailable[LedgerVolume], idx.unavailable[LedgerChapter] = true, true
			case series.InChapterFolder(f.Path):
				idx.unavailable[LedgerChapter] = true
			default:
				idx.unavailable[LedgerVolume] = true
			}
		}
	}
	for _, c := range contents {
		idx.contents[c.BookID] = append(idx.contents[c.BookID], c)
	}
	view := &ledgerView{Series: series, Versions: versions, Editions: cols, Books: map[int64][]store.Book{}, Status: map[int64][]BookStatus{}, Profiles: map[int64]*profile.Profile{}, Files: files}
	for _, w := range works {
		view.Books[w.EditionID] = append(view.Books[w.EditionID], w)
	}
	for _, version := range versions {
		col := editionByID(cols, version.EditionID)
		if col == nil {
			continue
		}
		view.Profiles[version.ID] = profiles.For(version)
		view.Status[version.ID] = idx.cardBooks(version, *col, view.Books[col.ID])
	}
	return view, nil
}

// ledgerIndex is a series' files, mappings and settings, indexed for
// evaluating its cards.
type ledgerIndex struct {
	series      *store.Series
	profiles    cardProfiles
	today       string
	coverage    map[int64][]store.AssetCoverage // card → mappings
	overrides   map[int64]map[int64]bool        // card → book → collected
	files       map[int64]store.File
	present     map[int64]bool                 // file → on disk and confirmed
	unavailable map[string]bool                // ledger → its folder cannot be checked
	contents    map[int64][]store.AssetContent // book → files holding it
	pending     map[string]string              // store.BookKey → reservation phase
}

// cardBooks is where each book of a card stands.
func (idx ledgerIndex) cardBooks(version store.CollectedVersion, col store.Edition, books []store.Book) []BookStatus {
	prof := idx.profiles.For(version)
	judged := judgeCardFiles(col, prof, idx.profiles.Formats, idx.coverage[version.ID], idx.files, idx.present)
	chapter := col.Type == LedgerChapter
	slots := make([]BookStatus, 0, len(books))
	for _, w := range books {
		sl := BookStatus{Book: w, VersionID: version.ID, Availability: availability(w.ReleaseDate, idx.today), FileIDs: judged.accepted[w.ID], OnDiskFileIDs: []int64{}}
		if sl.FileIDs == nil {
			sl.FileIDs = []int64{}
		}
		review := false
		for _, c := range idx.contents[w.ID] {
			f := idx.files[c.FileID]
			if f.Presence == "present" {
				sl.OnDiskFileIDs = append(sl.OnDiskFileIDs, f.ID)
			}
			review = review || !c.Verified || f.NeedsReview
		}
		// Whether the card collects the book, and if not, why: the
		// first refusal from the series down to the book itself.
		inScope := (w.CountType == "main" || version.IncludeExtras) && (!chapter || !w.NumberKnown || w.SequenceNum > version.CutoffNum)
		if v, ok := idx.overrides[version.ID][w.ID]; ok {
			inScope = v
		}
		why := ""
		switch {
		case !idx.series.Monitored:
			why = "Automatic downloads are paused for this series"
		case col.SyncState == "stale":
			why = "Edition is unavailable in the catalog; review it before collecting"
		case w.SyncState == "stale" || w.SyncState == "retired":
			why = "This book is retired or unavailable in the catalog"
		case !version.Monitored:
			why = "Collecting is paused for this edition"
		case !inScope:
			why = "This book is outside what you collect of this edition"
		}
		sl.Monitored = why == ""
		best, known := 0, true
		for i, fid := range sl.FileIDs {
			if i == 0 || judged.scores[fid] > best {
				best = judged.scores[fid]
			}
			known = known && backedFacts(idx.files[fid].Facts, idx.files[fid].Claims).FactsKnown
		}
		sl.CurrentScore = best
		upgrade := prof != nil && prof.UpgradesEnabled && known && !judged.pinned[w.ID] && (prof.UpgradeUntilScore == 0 || best < prof.UpgradeUntilScore)
		sl.State, sl.Reason, sl.Wanted = bookState(bookFacts{
			Collected: sl.Monitored, Why: why,
			Owned: len(sl.FileIDs) > 0, OnDisk: len(sl.OnDiskFileIDs) > 0, Review: review,
			Reserved:     idx.pending[store.BookKey(version.ID, w.ID)] != "",
			Reviewing:    idx.pending[store.BookKey(version.ID, w.ID)] == store.QueueNeedsReview,
			Availability: sl.Availability, AcquireUnknown: version.AcquireUnknown,
			StorageDown: idx.unavailable[col.Type], Upgrade: upgrade, Refused: judged.refused[w.ID],
		})
		slots = append(slots, sl)
	}
	return slots
}

func (l *Library) BooksByVersion(ctx context.Context, seriesID int64) (map[int64][]BookStatus, error) {
	view, err := loadLedger(ctx, l.store, seriesID)
	if err != nil {
		return nil, err
	}
	return view.Status, nil
}
