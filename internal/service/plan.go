package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/h-wb/sento/internal/store"
)

// Plans are search observations, never persisted ownership or reservations.
type AcquisitionPlan struct {
	SearchedAt   string     `json:"searchedAt"`
	Complete     bool       `json:"complete"`
	Picks        []PlanPick `json:"picks"`
	Books        []PlanBook `json:"books"`
	Size         int64      `json:"size"`
	UnknownSizes int        `json:"unknownSizes"`
}

type PlanBook struct {
	Book         store.BookRef `json:"book"`
	Label        string        `json:"label"`
	Ownership    BookState     `json:"ownership"`
	Availability string        `json:"availability"`
}

type PlanPick struct {
	Key             string          `json:"key"`
	Books           []store.BookRef `json:"books"`
	AdditionalBooks int             `json:"additionalBooks"`
}

type PlanOutcome struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Message string `json:"message"`
	QueueID int64  `json:"queueId,omitempty"`
}

func candidateKey(c ReleaseCandidate) string {
	data, _ := json.Marshal([]any{c.Provider, c.SiteID, c.GUID, c.VersionID})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func refKey(b store.BookRef) string { return store.BookKey(b.VersionID, b.BookID) }

// BuildPlan greedily maximizes marginal useful coverage, preserving the
// release policy's score components and penalizing redundant book payloads.
// A final pruning pass drops a release whose contribution is fully covered
// by the others. This is deterministic, not a claim of global optimality.
//
// The pack rule (rejected.pack_threshold) holds for a job's share too: a
// pack whose remaining contribution is under half of what it holds is left
// for a person to choose, and its books stay listed as available.
func BuildPlan(candidates []ReleaseCandidate, books []PlanBook, complete bool) AcquisitionPlan {
	plan := AcquisitionPlan{SearchedAt: time.Now().UTC().Format(time.RFC3339), Complete: complete, Picks: []PlanPick{}, Books: append([]PlanBook{}, books...)}
	pool := append([]ReleaseCandidate{}, candidates...)
	sort.Slice(pool, func(i, j int) bool { return candidateKey(pool[i]) < candidateKey(pool[j]) })
	taken := map[string]bool{}
	var chosen []ReleaseCandidate
	used := map[string]bool{}
	for {
		best, bestScore := -1, -int(^uint(0)>>1)
		for i, c := range pool {
			if len(c.Rejections) != 0 || used[candidateKey(c)] {
				continue
			}
			newBooks := 0
			for _, b := range c.Coverage {
				if !taken[refKey(b)] {
					newBooks++
				}
			}
			if newBooks == 0 || belowPackThreshold(newBooks, c.RepresentedBooks) {
				continue
			}
			additional := max(0, c.RepresentedBooks-newBooks)
			// Replace the static coverage term with current useful coverage.
			score := c.Score - 1000*len(c.Coverage) + 1000*newBooks - 150*additional
			// The profile's score decides; the indexer's priority, then
			// the smaller size, only break ties.
			tie := best >= 0 && score == bestScore
			if best < 0 || score > bestScore || tie && (c.IndexerPriority < pool[best].IndexerPriority ||
				c.IndexerPriority == pool[best].IndexerPriority && smallerKnown(c.Size, pool[best].Size)) {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			break
		}
		c := pool[best]
		chosen = append(chosen, c)
		used[candidateKey(c)] = true
		for _, b := range c.Coverage {
			taken[refKey(b)] = true
		}
	}
	for i := len(chosen) - 1; i >= 0; i-- {
		others := map[string]bool{}
		for j, c := range chosen {
			if i != j {
				for _, b := range c.Coverage {
					others[refKey(b)] = true
				}
			}
		}
		if len(chosen[i].Coverage) > 0 && allCovered(chosen[i].Coverage, others) {
			chosen = slices.Delete(chosen, i, i+1)
		}
	}
	clear(taken)
	contents := map[string]bool{}
	for _, c := range chosen {
		pick := PlanPick{Key: candidateKey(c), Books: []store.BookRef{}}
		for _, b := range c.Coverage {
			if !taken[refKey(b)] {
				pick.Books = append(pick.Books, b)
				taken[refKey(b)] = true
			}
		}
		pick.AdditionalBooks = max(0, c.RepresentedBooks-len(pick.Books))
		plan.Picks = append(plan.Picks, pick)
		contentKey := c.ContentKey
		if contentKey == "" {
			contentKey = candidateKey(c)
		}
		if !contents[contentKey] {
			contents[contentKey] = true
			if c.Size > 0 {
				plan.Size += c.Size
			} else {
				plan.UnknownSizes++
			}
		}
	}
	markAvailability(&plan, pool, complete)
	return plan
}

// markAvailability says, for each book of the plan, what the search found
// for it among pool, whether or not the plan selects it.
func markAvailability(plan *AcquisitionPlan, pool []ReleaseCandidate, complete bool) {
	for i := range plan.Books {
		b := &plan.Books[i]
		b.Availability = "not_found"
		if !complete {
			b.Availability = "unknown"
		}
		for _, c := range pool {
			if !slices.ContainsFunc(c.Coverage, func(ref store.BookRef) bool { return refKey(ref) == refKey(b.Book) }) {
				continue
			}
			if len(c.Rejections) == 0 {
				b.Availability = "available"
				break
			}
			hard := slices.ContainsFunc(c.Rejections, func(code string) bool { return hardRejections[code] })
			if hard {
				continue
			}
			if c.Protocol == "torrent" && c.Seeders == 0 && len(c.Rejections) == 1 && c.Rejections[0] == "rejected.seeders" {
				if b.Availability != "outside_profile" {
					b.Availability = "no_seeders"
				}
			} else {
				b.Availability = "outside_profile"
			}
		}
	}
}

func smallerKnown(a, b int64) bool { return a > 0 && (b <= 0 || a < b) }

// belowPackThreshold mirrors the funnel's pack rule for a subset of a
// release: fewer than half of the books it holds would be collected.
func belowPackThreshold(books, represented int) bool {
	return represented > 0 && books*2 < represented
}

func allCovered(books []store.BookRef, set map[string]bool) bool {
	for _, b := range books {
		if !set[refKey(b)] {
			return false
		}
	}
	return true
}
