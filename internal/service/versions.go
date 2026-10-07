package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
	"time"
)

type BookStatus struct {
	OnDiskFileIDs []int64 `json:"onDiskFileIds"`
	Reason        string  `json:"reason"`
	CurrentScore  int     `json:"currentScore"`

	Book         store.Book `json:"book"`
	VersionID    int64      `json:"versionId"`
	Monitored    bool       `json:"monitored"`
	Availability string     `json:"availability"`
	State        string     `json:"state"`
	FileIDs      []int64    `json:"fileIds"`
	Wanted       bool       `json:"wanted"`
}

func versionBooks(t store.CollectedVersion, works []store.Book, coverage []store.AssetCoverage, pending map[string]bool, files map[int64]bool, now time.Time, overrides map[int64]bool, chapter bool) []BookStatus {
	out := make([]BookStatus, 0, len(works))
	today := now.Format("2006-01-02")
	byBook := map[int64][]store.AssetCoverage{}
	for _, a := range coverage {
		if a.VersionID == t.ID {
			byBook[a.BookID] = append(byBook[a.BookID], a)
		}
	}
	for _, w := range works {
		state := "missing"
		availability := "unknown"
		if w.ReleaseDate != "" {
			availability = "released"
			if w.ReleaseDate > today {
				availability = "upcoming"
			}
		}
		slot := BookStatus{Book: w, VersionID: t.ID, Monitored: t.Monitored && (w.CountType == "main" || t.IncludeExtras) && (!chapter || !w.NumberKnown || w.SequenceNum > t.CutoffNum), Availability: availability, FileIDs: []int64{}, OnDiskFileIDs: []int64{}}
		if v, ok := overrides[w.ID]; ok {
			slot.Monitored = t.Monitored && v
		}
		for _, a := range byBook[w.ID] {
			if a.VersionID == t.ID && a.BookID == w.ID && a.Verified && files[a.FileID] {
				slot.FileIDs = append(slot.FileIDs, a.FileID)
			}
		}
		if w.SyncState == "stale" || w.SyncState == "retired" {
			slot.Monitored = false
		}
		if pending[store.BookKey(t.ID, w.ID)] {
			state = "pending"
		} else if len(slot.FileIDs) > 0 {
			state = "owned"
		} else if availability == "upcoming" {
			state = "not_released"
		} else if !slot.Monitored {
			state = "unmonitored"
		} else if availability == "unknown" && !t.AcquireUnknown {
			state = "unknown"
		}
		slot.State = state
		slot.Wanted = state == "missing"
		out = append(out, slot)
	}
	return out
}

// ledgerView is loaded once per series and shared by search and UI evaluation.
type ledgerView struct {
	Versions []store.CollectedVersion
	Editions []store.Edition
	Books    map[int64][]store.Book
	Status   map[int64][]BookStatus
	Profiles map[int64]*profile.Profile
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
	works, err := st.SeriesBooks(ctx, seriesID)
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
	pending, err := st.PendingBooks(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	profiles, formats, err := NewProfiles(st).EffectiveProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	allProfiles, err := st.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	byProfile := map[int64]*profile.Profile{}
	for i := range allProfiles {
		byProfile[allProfiles[i].ID] = &allProfiles[i]
	}
	coverageByVersion := map[int64][]store.AssetCoverage{}
	for _, a := range coverage {
		coverageByVersion[a.VersionID] = append(coverageByVersion[a.VersionID], a)
	}
	view := &ledgerView{Versions: versions, Editions: cols, Books: map[int64][]store.Book{}, Status: map[int64][]BookStatus{}, Profiles: map[int64]*profile.Profile{}}
	byCol := map[int64]store.Edition{}
	for _, c := range cols {
		byCol[c.ID] = c
	}
	for _, w := range works {
		view.Books[w.EditionID] = append(view.Books[w.EditionID], w)
	}
	byOverride := map[int64]map[int64]bool{}
	for _, o := range overrides {
		if byOverride[o.VersionID] == nil {
			byOverride[o.VersionID] = map[int64]bool{}
		}
		byOverride[o.VersionID][o.BookID] = o.Monitored
	}
	byFile := map[int64]store.File{}
	present := map[int64]bool{}
	storageUnavailable := false
	for _, f := range files {
		byFile[f.ID] = f
		present[f.ID] = f.Presence == "present" && !f.NeedsReview
		if f.Presence == "unavailable" || f.Presence == "unknown" {
			storageUnavailable = true
		}
	}
	byContent := map[int64][]store.AssetContent{}
	for _, c := range contents {
		byContent[c.BookID] = append(byContent[c.BookID], c)
	}
	for _, version := range versions {
		col := byCol[version.EditionID]
		prof := profiles[col.ID]
		if version.ProfileID > 0 {
			prof = byProfile[version.ProfileID]
		}
		view.Profiles[version.ID] = prof
		accepted := []store.AssetCoverage{}
		scores := map[int64]int{}
		reasons := map[int64]string{}
		for _, a := range coverageByVersion[version.ID] {
			f := byFile[a.FileID]
			facts := f.Facts
			reason := ""
			if version.RequiredGroup != "" && !matchGroup(version.RequiredGroup, facts.Group) {
				reason = "Existing copy does not match the required release group"
			}
			if a.Evidence != "manual" && facts.Language != "" && facts.Language != "multi" && baseLang(facts.Language) != baseLang(col.Language) {
				reason = "Existing copy has a different language"
			}
			if a.Evidence != "manual" && !version.AllowUnknown && facts.Language == "" {
				reason = "Existing copy has no confirmed language"
			}
			if a.Evidence == "manual" {
				facts.Language = col.Language
			}
			score := assetScore(facts, prof, formats)
			scores[a.FileID] = score
			if prof != nil && prof.MinFormatScore > profile.MinScoreOff && facts.FactsKnown && score < prof.MinFormatScore {
				reason = "Existing copy does not meet the release profile"
			}
			if reason != "" {
				reasons[a.BookID] = reason
				continue
			}
			accepted = append(accepted, a)
		}
		pinnedBooks := map[int64]bool{}
		for _, a := range accepted {
			if a.Pinned {
				pinnedBooks[a.BookID] = true
			}
		}
		slots := versionBooks(version, view.Books[col.ID], accepted, pending, present, time.Now(), byOverride[version.ID], col.Type == "chapter")
		for i := range slots {
			sl := &slots[i]
			hasReview := false
			for _, c := range byContent[sl.Book.ID] {
				f := byFile[c.FileID]
				if f.Presence == "present" {
					sl.OnDiskFileIDs = append(sl.OnDiskFileIDs, f.ID)
				}
				if !c.Verified || f.NeedsReview {
					hasReview = true
				}
			}
			switch {
			case !series.Monitored:
				sl.Monitored = false
				sl.Reason = "Automatic downloads are paused for this series"
			case col.SyncState == "stale" || col.SyncState == "superseded":
				sl.Monitored = false
				sl.Reason = "Edition is unavailable in the catalog; review it before collecting"
			case sl.Book.SyncState == "stale" || sl.Book.SyncState == "retired":
				sl.Reason = "This book is retired or unavailable in the catalog"
			case !version.Monitored:
				sl.Reason = "Collecting is paused for this version"
			case !sl.Monitored:
				sl.Reason = "This book is excluded by the version's collecting rules"
			}
			if !sl.Monitored {
				sl.Wanted = false
				if len(sl.FileIDs) == 0 && sl.State != "pending" {
					sl.State = "unmonitored"
				}
			}
			if storageUnavailable {
				sl.Wanted = false
				sl.Reason = "Library storage is unavailable or has not been checked"
				if len(sl.FileIDs) == 0 && sl.State != "pending" {
					sl.State = "storage_unavailable"
				}
			}
			if sl.State == "missing" && len(sl.OnDiskFileIDs) > 0 {
				sl.State = "unsatisfactory"
				sl.Reason = reasons[sl.Book.ID]
				if sl.Reason == "" {
					sl.Reason = "Existing copy is not selected for this version"
				}
			}
			if hasReview && len(sl.FileIDs) == 0 {
				sl.State = "needs_review"
				sl.Wanted = false
				sl.Reason = "Existing file mappings need review"
			}
			if sl.State == "pending" {
				sl.Reason = "A download or import already holds this book"
			}
			if sl.State == "not_released" {
				sl.Reason = "Publication date is in the future"
			}
			if sl.State == "unknown" {
				sl.Reason = "Publication date is unknown and searching undated books is disabled"
			}
			best := -int(^uint(0) >> 1)
			known := true
			pinned := false
			for _, fid := range sl.FileIDs {
				best = max(best, scores[fid])
				known = known && byFile[fid].Facts.FactsKnown
			}
			if len(sl.FileIDs) > 0 {
				sl.CurrentScore = best
			}
			pinned = pinnedBooks[sl.Book.ID]
			if sl.State == "owned" && sl.Monitored && !storageUnavailable && prof != nil && prof.UpgradesEnabled && known && !pinned && (prof.UpgradeUntilScore == 0 || best < prof.UpgradeUntilScore) {
				sl.State = "upgrade_eligible"
				sl.Wanted = true
				sl.Reason = "Current copy is below the upgrade cutoff"
			}
		}
		view.Status[version.ID] = slots
	}
	return view, nil
}

func assetScore(f store.AssetFacts, p *profile.Profile, formats []profile.Format) int {
	if p == nil {
		return 0
	}
	score := 0
	input := profile.Input{Title: f.Title, Group: f.Group, Language: f.Language, Protocol: f.Protocol, Site: f.Site, Resolution: f.Resolution, Flags: f.Flags}
	for _, format := range formats {
		if format.Matches(input) {
			score += p.FormatScores[format.ID]
		}
	}
	return score
}

func (l *Library) BooksByVersion(ctx context.Context, seriesID int64) (map[int64][]BookStatus, error) {
	view, err := loadLedger(ctx, l.store, seriesID)
	if err != nil {
		return nil, err
	}
	return view.Status, nil
}

func (s *SearchService) prepareVersions(ctx context.Context, run *searchRun) error {
	if err := s.store.CheckFilePresence(ctx, run.seriesID); err != nil {
		return err
	}
	view, err := loadLedger(ctx, s.store, run.seriesID)
	if err != nil {
		return err
	}
	run.editions = view.Editions
	run.versions = view.Versions
	run.books = view.Books
	run.versionProfiles = view.Profiles
	run.wantedBooks = map[int64]map[int64]bool{}
	run.ownedScores = map[int64]map[int64]int{}
	bindings, err := s.store.ListBindings(ctx, run.seriesID)
	if err != nil {
		return err
	}
	run.sourceEditions = map[int64]int64{}
	for _, b := range bindings {
		if b.EditionID > 0 {
			run.sourceEditions[b.ID] = b.EditionID
		}
	}
	found := run.scope == nil
	for _, t := range view.Versions {
		col := editionByID(view.Editions, t.EditionID)
		if col.Type != run.ledger {
			continue
		}
		if run.scope != nil && run.scope.VersionID != t.ID {
			continue
		}
		if run.scope != nil && run.scope.BookID == 0 {
			found = true
		}
		wanted := map[int64]bool{}
		run.ownedScores[t.ID] = map[int64]int{}
		for _, sl := range view.Status[t.ID] {
			selected := sl.Wanted
			if run.scope != nil && run.scope.AllBooks && run.scope.BookID == 0 {
				selected = sl.State != "storage_unavailable" && sl.State != "needs_review" && sl.State != "pending"
			}
			if run.scope != nil && run.scope.BookID != 0 {
				selected = run.scope.BookID == sl.Book.ID
				if selected {
					found = true
				}
				if sl.State == "storage_unavailable" || sl.State == "needs_review" || sl.State == "pending" {
					selected = false
				}
			}
			if selected {
				wanted[sl.Book.ID] = true
			}
			if sl.State == "upgrade_eligible" {
				run.ownedScores[t.ID][sl.Book.ID] = sl.CurrentScore
			}
		}
		if len(wanted) > 0 {
			run.wantedBooks[t.ID] = wanted
		}
	}
	if !found {
		return apperr.Invalidf("book or version does not belong to this edition")
	}
	return nil
}
