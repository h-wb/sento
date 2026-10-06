package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
	"time"
)

type UnitSlot struct {
	OnDiskFileIDs []int64 `json:"onDiskFileIds"`
	Reason        string  `json:"reason"`
	CurrentScore  int     `json:"currentScore"`

	Work         store.Work `json:"work"`
	TargetID     int64      `json:"targetId"`
	Monitored    bool       `json:"monitored"`
	Availability string     `json:"availability"`
	State        string     `json:"state"`
	FileIDs      []int64    `json:"fileIds"`
	Wanted       bool       `json:"wanted"`
}

func targetSlots(t store.DownloadTarget, works []store.Work, coverage []store.AssetCoverage, pending map[string]bool, files map[int64]bool, now time.Time, overrides map[int64]bool, chapter bool) []UnitSlot {
	out := make([]UnitSlot, 0, len(works))
	today := now.Format("2006-01-02")
	byWork := map[int64][]store.AssetCoverage{}
	for _, a := range coverage {
		if a.TargetID == t.ID {
			byWork[a.WorkID] = append(byWork[a.WorkID], a)
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
		slot := UnitSlot{Work: w, TargetID: t.ID, Monitored: t.Monitored && (w.CountType == "main" || t.IncludeExtras) && (!chapter || !w.NumberKnown || w.SequenceNum > t.CutoffNum), Availability: availability, FileIDs: []int64{}, OnDiskFileIDs: []int64{}}
		if v, ok := overrides[w.ID]; ok {
			slot.Monitored = t.Monitored && v
		}
		for _, a := range byWork[w.ID] {
			if a.TargetID == t.ID && a.WorkID == w.ID && a.Verified && files[a.FileID] {
				slot.FileIDs = append(slot.FileIDs, a.FileID)
			}
		}
		if w.SyncState == "stale" || w.SyncState == "retired" {
			slot.Monitored = false
		}
		if pending[store.SlotKey("unit", t.ID, w.ID)] {
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
	Targets     []store.DownloadTarget
	Collections []store.Collection
	Works       map[int64][]store.Work
	Slots       map[int64][]UnitSlot
	Profiles    map[int64]*profile.Profile
}

func loadLedger(ctx context.Context, st *store.Store, seriesID int64) (*ledgerView, error) {
	series, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	targets, err := st.ListTargets(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	cols, err := st.ListCollections(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	works, err := st.SeriesWorks(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	overrides, err := st.SeriesUnitOverrides(ctx, seriesID)
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
	pending, err := st.PendingUnits(ctx, seriesID)
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
	coverageByTarget := map[int64][]store.AssetCoverage{}
	for _, a := range coverage {
		coverageByTarget[a.TargetID] = append(coverageByTarget[a.TargetID], a)
	}
	view := &ledgerView{Targets: targets, Collections: cols, Works: map[int64][]store.Work{}, Slots: map[int64][]UnitSlot{}, Profiles: map[int64]*profile.Profile{}}
	byCol := map[int64]store.Collection{}
	for _, c := range cols {
		byCol[c.ID] = c
	}
	for _, w := range works {
		view.Works[w.CollectionID] = append(view.Works[w.CollectionID], w)
	}
	byOverride := map[int64]map[int64]bool{}
	for _, o := range overrides {
		if byOverride[o.TargetID] == nil {
			byOverride[o.TargetID] = map[int64]bool{}
		}
		byOverride[o.TargetID][o.WorkID] = o.Monitored
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
		byContent[c.WorkID] = append(byContent[c.WorkID], c)
	}
	for _, target := range targets {
		col := byCol[target.CollectionID]
		prof := profiles[col.ID]
		if target.ProfileID > 0 {
			prof = byProfile[target.ProfileID]
		}
		view.Profiles[target.ID] = prof
		accepted := []store.AssetCoverage{}
		scores := map[int64]int{}
		reasons := map[int64]string{}
		for _, a := range coverageByTarget[target.ID] {
			f := byFile[a.FileID]
			facts := f.Facts
			reason := ""
			if target.RequiredGroup != "" && !matchGroup(target.RequiredGroup, facts.Group) {
				reason = "Existing copy does not match the required release group"
			}
			if a.Evidence != "manual" && facts.Language != "" && facts.Language != "multi" && baseLang(facts.Language) != baseLang(col.Language) {
				reason = "Existing copy has a different language"
			}
			if a.Evidence != "manual" && !target.AllowUnknown && facts.Language == "" {
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
				reasons[a.WorkID] = reason
				continue
			}
			accepted = append(accepted, a)
		}
		pinnedWorks := map[int64]bool{}
		for _, a := range accepted {
			if a.Pinned {
				pinnedWorks[a.WorkID] = true
			}
		}
		slots := targetSlots(target, view.Works[col.ID], accepted, pending, present, time.Now(), byOverride[target.ID], col.Type == "chapter")
		for i := range slots {
			sl := &slots[i]
			hasReview := false
			for _, c := range byContent[sl.Work.ID] {
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
			case sl.Work.SyncState == "stale" || sl.Work.SyncState == "retired":
				sl.Reason = "This book is retired or unavailable in the catalog"
			case !target.Monitored:
				sl.Reason = "Collecting is paused for this version"
			case !sl.Monitored:
				sl.Reason = "This book is excluded from collection rules"
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
				sl.Reason = reasons[sl.Work.ID]
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
			pinned = pinnedWorks[sl.Work.ID]
			if sl.State == "owned" && sl.Monitored && !storageUnavailable && prof != nil && prof.UpgradesEnabled && known && !pinned && (prof.UpgradeUntilScore == 0 || best < prof.UpgradeUntilScore) {
				sl.State = "upgrade_eligible"
				sl.Wanted = true
				sl.Reason = "Current copy is below the upgrade cutoff"
			}
		}
		view.Slots[target.ID] = slots
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

func (l *Library) TargetSlots(ctx context.Context, seriesID int64) (map[int64][]UnitSlot, error) {
	view, err := loadLedger(ctx, l.store, seriesID)
	if err != nil {
		return nil, err
	}
	return view.Slots, nil
}

func (s *SearchService) prepareTargets(ctx context.Context, run *searchRun) error {
	if err := s.store.CheckFilePresence(ctx, run.seriesID); err != nil {
		return err
	}
	view, err := loadLedger(ctx, s.store, run.seriesID)
	if err != nil {
		return err
	}
	run.collections = view.Collections
	run.targets = view.Targets
	run.units = view.Works
	run.targetProfiles = view.Profiles
	run.targetWanted = map[int64]map[int64]bool{}
	run.ownedScores = map[int64]map[int64]int{}
	run.wanted = map[int64]map[int64]bool{}
	bindings, err := s.store.ListBindings(ctx, run.seriesID)
	if err != nil {
		return err
	}
	run.sourceEditions = map[int64]int64{}
	for _, b := range bindings {
		if b.CollectionID > 0 {
			run.sourceEditions[b.ID] = b.CollectionID
		}
	}
	found := run.target == nil
	for _, t := range view.Targets {
		col := collectionByID(view.Collections, t.CollectionID)
		if col.Type != run.ledger {
			continue
		}
		if run.target != nil && (run.target.CollectionID != t.CollectionID || (run.target.TargetID != 0 && run.target.TargetID != t.ID) || (run.target.TargetID == 0 && !t.IsDefault)) {
			continue
		}
		if run.target != nil && run.target.WorkID == 0 {
			found = true
		}
		wanted := map[int64]bool{}
		run.ownedScores[t.ID] = map[int64]int{}
		for _, sl := range view.Slots[t.ID] {
			selected := sl.Wanted
			if run.target != nil && run.target.AllBooks && run.target.WorkID == 0 && run.target.Number == 0 {
				selected = sl.State != "storage_unavailable" && sl.State != "needs_review" && sl.State != "pending"
			}
			if run.target != nil && (run.target.WorkID != 0 || run.target.Number != 0) {
				selected = run.target.WorkID == sl.Work.ID || (run.target.WorkID == 0 && sl.Work.NumberKnown && run.target.Number == sl.Work.SequenceNum)
				if selected {
					found = true
				}
				if sl.State == "storage_unavailable" || sl.State == "needs_review" || sl.State == "pending" {
					selected = false
				}
			}
			if selected {
				wanted[sl.Work.ID] = true
				if run.wanted[t.CollectionID] == nil {
					run.wanted[t.CollectionID] = map[int64]bool{}
				}
				run.wanted[t.CollectionID][sl.Work.SequenceNum] = true
			}
			if sl.State == "upgrade_eligible" {
				run.ownedScores[t.ID][sl.Work.ID] = sl.CurrentScore
			}
		}
		if len(wanted) > 0 {
			run.targetWanted[t.ID] = wanted
		}
	}
	if !found {
		return apperr.Invalidf("book or version does not belong to this edition")
	}
	return nil
}
