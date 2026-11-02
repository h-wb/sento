package service

import "github.com/h-wb/sento/internal/store"

// preferredFileOwners chooses a placed file independently for each collected
// version's book. A shared asset may win several versions; incompatible
// profiles may choose different assets for the same underlying book.
func preferredFileOwners(versions []store.CollectedVersion, editions []store.Edition, coverage []store.AssetCoverage, files map[int64]store.File, profiles cardProfiles) map[string]int64 {
	byVersion := map[int64][]store.AssetCoverage{}
	present := map[int64]bool{}
	for id, f := range files {
		present[id] = f.Presence == "present" && !f.NeedsReview
	}
	for _, c := range coverage {
		byVersion[c.VersionID] = append(byVersion[c.VersionID], c)
	}
	placed := map[string]int64{}
	for _, version := range versions {
		col := editionByID(editions, version.EditionID)
		if col == nil {
			continue
		}
		cov := byVersion[version.ID]
		judged := judgeCardFiles(*col, profiles.For(version), profiles.Formats, cov, files, present)
		pins := map[int64]map[int64]bool{}
		for _, c := range cov {
			if pins[c.BookID] == nil {
				pins[c.BookID] = map[int64]bool{}
			}
			pins[c.BookID][c.FileID] = c.Pinned
		}
		for bookID, candidates := range judged.accepted {
			var chosen int64
			for _, id := range candidates {
				pinned, currentPinned := pins[bookID][id], pins[bookID][chosen]
				if chosen == 0 || pinned && !currentPinned || pinned == currentPinned && (judged.scores[id] > judged.scores[chosen] || judged.scores[id] == judged.scores[chosen] && id < chosen) {
					chosen = id
				}
			}
			placed[store.BookKey(version.ID, bookID)] = chosen
		}
	}
	return placed
}
