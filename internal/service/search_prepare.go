package service

import (
	"cmp"
	"context"
	"slices"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func (s *SearchService) prepareVersions(ctx context.Context, run *searchRun) error {
	if err := CheckFilePresence(ctx, s.store, run.seriesID); err != nil {
		return err
	}
	view, err := loadLedger(ctx, s.store, run.seriesID)
	if err != nil {
		return err
	}
	run.editions = view.Editions
	run.shelf = seriesShelf(view.Editions, view.Series)
	run.versions = view.Versions
	run.books = view.Books
	run.versionProfiles = view.Profiles
	run.wantedBooks = map[int64]map[int64]bool{}
	run.wanted = map[string]bool{}
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
			// A scheduled search only fills what is missing or
			// unsatisfactory: an upgrade-eligible book keeps its copy
			// until a person searches for a better one, so unattended
			// sweeps never hunt quality upgrades (Sonarr parity).
			if run.scheduled && sl.State == StateUpgradeEligible {
				continue
			}
			// A book reserved by the download being replaced is still wanted.
			if sl.Wanted && sl.State != StatePending || sl.State == StatePending && run.replacing[store.BookKey(t.ID, sl.Book.ID)] {
				run.wanted[store.BookKey(t.ID, sl.Book.ID)] = true
			}
			selected := sl.Wanted
			if run.scope != nil && run.scope.AllBooks && run.scope.BookID == 0 {
				selected = sl.State != StateStorageUnavailable && sl.State != StateNeedsReview && sl.State != StatePending
			}
			if run.scope != nil && run.scope.BookID != 0 {
				selected = run.scope.BookID == sl.Book.ID
				if selected {
					found = true
				}
				if sl.State == StateStorageUnavailable || sl.State == StateNeedsReview || sl.State == StatePending {
					selected = false
				}
			}
			if sl.State == StatePending && run.replacing[store.BookKey(t.ID, sl.Book.ID)] &&
				(run.scope.BookID == 0 || run.scope.BookID == sl.Book.ID) {
				selected = true
			}
			if selected {
				wanted[sl.Book.ID] = true
				run.planBooks = append(run.planBooks, PlanBook{
					Book:  store.BookRef{VersionID: t.ID, BookID: sl.Book.ID, EditionID: col.ID, Ledger: col.Type, Number: sl.Book.SequenceNum},
					Label: sl.Book.SequenceStr, Ownership: sl.State,
				})
			}
			if sl.State == StateUpgradeEligible {
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

func (s *SearchService) prepare(ctx context.Context, seriesID int64, searchLedger string, version *SearchScope) (*searchRun, error) {
	if s.registry.Empty() {
		return nil, apperr.Unavailablef("no search providers configured")
	}
	titles, err := s.store.SeriesTitles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	sites, err := s.effectiveSites(ctx, seriesID, searchLedger)
	if err != nil {
		return nil, err
	}
	run := &searchRun{
		seriesID: seriesID, ledger: searchLedger, scope: version,
		scheduled: scheduled(ctx),
		titles:    titles, sites: sites,
		allowed: map[string]map[string]bool{}, priorities: map[[2]string]int{},
	}
	for _, st := range sites {
		if run.allowed[st.providerID] == nil {
			run.allowed[st.providerID] = map[string]bool{}
		}
		if st.usable {
			run.allowed[st.providerID][st.entryID] = true
			run.searched = append(run.searched, st.providerID+"/"+st.name)
		}
		run.priorities[[2]string{st.providerID, st.entryID}] = st.eff.Priority
	}
	if run.blocked, err = s.store.BlockedKeys(ctx); err != nil {
		return nil, err
	}
	if run.profiles, err = s.profiles.seriesProfiles(ctx, seriesID); err != nil {
		return nil, err
	}
	if version != nil && version.ReplaceQueueID > 0 {
		old, err := s.store.GetQueueItem(ctx, version.ReplaceQueueID)
		if err != nil {
			return nil, err
		}
		if old.SeriesID != seriesID {
			return nil, apperr.Invalidf("that download belongs to another series")
		}
		run.replaceQueueID = old.ID
		run.excludeRelease = old.ProviderID + "/" + cmp.Or(old.ReleaseKey, old.ExternalID)
		run.replacing = map[string]bool{}
		for _, ref := range queueBooks(old) {
			run.replacing[refKey(ref)] = true
		}
	}
	if err = s.prepareVersions(ctx, run); err != nil {
		return nil, err
	}
	var languages []string
	for _, v := range run.versions {
		if col := editionByID(run.editions, v.EditionID); col != nil && !slices.Contains(languages, profile.BaseLang(col.Language)) {
			languages = append(languages, profile.BaseLang(col.Language))
		}
	}
	if run.queryTitles, err = s.store.QueryTitles(ctx, seriesID, languages...); err != nil {
		return nil, err
	}
	if version != nil {
		for _, v := range run.versions {
			if v.ID == version.VersionID {
				if col := editionByID(run.editions, v.EditionID); col != nil {
					run.titles, err = s.store.SeriesTitles(ctx, seriesID, profile.BaseLang(col.Language))
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}
	return run, nil
}
