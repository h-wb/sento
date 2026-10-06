package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
	"os"
	"time"
)

type UnitSlot struct {
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
	for _, w := range works {
		state := "missing"
		availability := "unknown"
		if w.ReleaseDate != "" {
			availability = "released"
			if w.ReleaseDate > today {
				availability = "upcoming"
			}
		}
		slot := UnitSlot{Work: w, TargetID: t.ID, Monitored: t.Monitored && w.CountType == "main" && (!chapter || !w.NumberKnown || w.SequenceNum > t.CutoffNum), Availability: availability, FileIDs: []int64{}}
		if v, ok := overrides[w.ID]; ok {
			slot.Monitored = t.Monitored && v
		}
		for _, a := range coverage {
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
func (s *SearchService) prepareTargets(ctx context.Context, run *searchRun) error {
	targets, err := s.store.ListTargets(ctx, run.seriesID)
	if err != nil {
		return err
	}
	if run.target != nil && run.target.TargetID != 0 {
		valid := false
		for _, t := range targets {
			if t.ID == run.target.TargetID && t.CollectionID == run.target.CollectionID {
				valid = true
			}
		}
		if !valid {
			return apperr.Invalidf("version does not belong to this edition")
		}
	}
	bindings, e := s.store.ListBindings(ctx, run.seriesID)
	if e != nil {
		return e
	}
	run.sourceEditions = map[int64]int64{}
	for _, b := range bindings {
		if b.CollectionID > 0 {
			run.sourceEditions[b.ID] = b.CollectionID
		}
	}
	cols, err := s.store.ListCollections(ctx, run.seriesID)
	if err != nil {
		return err
	}
	run.collections = cols
	run.targets = targets
	run.units = map[int64][]store.Work{}
	run.targetWanted = map[int64]map[int64]bool{}
	run.targetProfiles = map[int64]*profile.Profile{}
	run.ownedScores = map[int64]map[int64]int{}
	coverage, err := s.store.Coverage(ctx, run.seriesID)
	if err != nil {
		return err
	}
	coverage, err = eligibleCoverage(ctx, s.store, targets, run.collections, coverage)
	if err != nil {
		return err
	}
	allfiles, err := s.store.ListFiles(ctx, run.seriesID)
	if err != nil {
		return err
	}
	files := map[int64]bool{}
	for _, f := range allfiles {
		_, e := os.Stat(f.Path)
		files[f.ID] = e == nil && !f.NeedsReview
	}
	pending, err := s.store.PendingUnits(ctx, run.seriesID)
	if err != nil {
		return err
	}
	for _, c := range run.collections {
		if c.Type != run.ledger {
			continue
		}
		works, e := s.store.ListWorks(ctx, c.ID)
		if e != nil {
			return e
		}
		run.units[c.ID] = works
	}
	for _, t := range targets {
		works, ok := run.units[t.CollectionID]
		if !ok {
			continue
		}
		if run.target != nil && (run.target.CollectionID != t.CollectionID || (run.target.TargetID != 0 && run.target.TargetID != t.ID)) {
			continue
		}
		if run.target != nil && run.target.TargetID == 0 && !t.IsDefault {
			continue
		}
		prof := run.profileFor(t.CollectionID)
		if t.ProfileID > 0 {
			p, e := s.store.GetProfile(ctx, t.ProfileID)
			if e != nil {
				return e
			}
			prof = p
		}
		run.targetProfiles[t.ID] = prof
		wanted := map[int64]bool{}
		run.ownedScores[t.ID] = map[int64]int{}
		overrides, e := s.store.UnitOverrides(ctx, t.ID)
		if e != nil {
			return e
		}
		col := collectionByID(run.collections, t.CollectionID)
		for _, slot := range targetSlots(t, works, coverage, pending, files, time.Now(), overrides, col.Type == "chapter") {
			selected := slot.Wanted
			if slot.State == "owned" && slot.Monitored && !pending[store.SlotKey("unit", t.ID, slot.Work.ID)] && prof != nil && prof.UpgradesEnabled {
				best, eligible, e := ownedUpgrade(ctx, s.store, prof, run.formats, t, slot, coverage)
				if e != nil {
					return e
				}
				run.ownedScores[t.ID][slot.Work.ID] = best
				selected = eligible
			}

			if run.target != nil && (run.target.WorkID != 0 || run.target.Number != 0) {
				selected = run.target.WorkID == slot.Work.ID || (run.target.WorkID == 0 && slot.Work.NumberKnown && run.target.Number == slot.Work.SequenceNum)
			}
			if selected {
				wanted[slot.Work.ID] = true
			}
		}
		if len(wanted) > 0 {
			run.targetWanted[t.ID] = wanted
		}
	}
	if run.target != nil && run.target.WorkID != 0 {
		found := false
		for _, ws := range run.units {
			for _, w := range ws {
				if w.ID == run.target.WorkID && w.CollectionID == run.target.CollectionID {
					found = true
				}
			}
		}
		if !found {
			return apperr.Invalidf("unit does not belong to this collection")
		}
	}
	return nil
}
func (l *Library) TargetSlots(ctx context.Context, seriesID int64) (map[int64][]UnitSlot, error) {
	targets, err := l.store.ListTargets(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	coverage, err := l.store.Coverage(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	pending, err := l.store.PendingUnits(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	fs, err := l.store.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files := map[int64]bool{}
	for _, f := range fs {
		_, e := os.Stat(f.Path)
		files[f.ID] = e == nil && !f.NeedsReview
	}
	cols, err := l.store.ListCollections(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	coverage, err = eligibleCoverage(ctx, l.store, targets, cols, coverage)
	if err != nil {
		return nil, err
	}
	profiles, formats, err := NewProfiles(l.store).EffectiveProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	out := map[int64][]UnitSlot{}
	for _, t := range targets {
		ws, e := l.store.ListWorks(ctx, t.CollectionID)
		if e != nil {
			return nil, e
		}
		overrides, e := l.store.UnitOverrides(ctx, t.ID)
		if e != nil {
			return nil, e
		}
		col, e := l.store.GetCollection(ctx, t.CollectionID)
		if e != nil {
			return nil, e
		}
		out[t.ID] = targetSlots(t, ws, coverage, pending, files, time.Now(), overrides, col.Type == "chapter")
		prof := profiles[col.ID]
		if t.ProfileID > 0 {
			prof, err = l.store.GetProfile(ctx, t.ProfileID)
			if err != nil {
				return nil, err
			}
		}
		if prof != nil && prof.UpgradesEnabled {
			for i := range out[t.ID] {
				sl := &out[t.ID][i]
				if sl.State != "owned" || !sl.Monitored {
					continue
				}
				_, eligible, e := ownedUpgrade(ctx, l.store, prof, formats, t, *sl, coverage)
				if e != nil {
					return nil, e
				}
				if eligible {
					sl.State = "upgrade_eligible"
					sl.Wanted = true
				}
			}
		}
	}
	return out, nil
}

func ownedUpgrade(ctx context.Context, st *store.Store, prof *profile.Profile, formats []profile.Format, t store.DownloadTarget, slot UnitSlot, coverage []store.AssetCoverage) (int, bool, error) {
	pinned := false
	for _, a := range coverage {
		if a.TargetID == t.ID && a.WorkID == slot.Work.ID && a.Pinned {
			pinned = true
		}
	}
	best := -int(^uint(0) >> 1)
	for _, fid := range slot.FileIDs {
		facts, e := st.FileFacts(ctx, fid)
		if e != nil {
			return 0, false, e
		}
		if !facts.FactsKnown {
			return 0, false, nil
		}
		for _, a := range coverage {
			if a.FileID == fid && a.TargetID == t.ID && a.WorkID == slot.Work.ID && a.Evidence == "manual" {
				c, e := st.GetCollection(ctx, t.CollectionID)
				if e != nil {
					return 0, false, e
				}
				facts.Language = c.Language
			}
		}
		score := 0
		input := profile.Input{Title: facts.Title, Group: facts.Group, Language: facts.Language, Protocol: facts.Protocol, Site: facts.Site, Resolution: facts.Resolution, Flags: facts.Flags}
		for _, f := range formats {
			if f.Matches(input) {
				score += prof.FormatScores[f.ID]
			}
		}
		best = max(best, score)
	}
	return best, !pinned && (prof.UpgradeUntilScore == 0 || best < prof.UpgradeUntilScore), nil
}

func eligibleCoverage(ctx context.Context, st *store.Store, targets []store.DownloadTarget, cols []store.Collection, coverage []store.AssetCoverage) ([]store.AssetCoverage, error) {
	out := []store.AssetCoverage{}
	facts := map[int64]store.AssetFacts{}
	for _, a := range coverage {
		var t *store.DownloadTarget
		for i := range targets {
			if targets[i].ID == a.TargetID {
				t = &targets[i]
				break
			}
		}
		if t == nil {
			continue
		}
		c := collectionByID(cols, t.CollectionID)
		if c == nil {
			continue
		}
		f, ok := facts[a.FileID]
		if !ok {
			var err error
			f, err = st.FileFacts(ctx, a.FileID)
			if err != nil {
				return nil, err
			}
			facts[a.FileID] = f
		}
		if t.RequiredGroup != "" && !matchGroup(t.RequiredGroup, f.Group) {
			continue
		}
		if a.Evidence != "manual" && f.Language != "" && f.Language != "multi" && baseLang(f.Language) != baseLang(c.Language) {
			continue
		}
		if a.Evidence != "manual" && !t.AllowUnknown && f.Language == "" {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}
