package service

import (
	"context"
	"encoding/json"
	"github.com/h-wb/sento/internal/provider"
	"log/slog"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// GrabBest uses the same marginal-coverage plan as interactive search.
func (s *SearchService) GrabBest(ctx context.Context, seriesID int64, searchLedger string, candidates []ReleaseCandidate) []int64 {
	plan := BuildPlan(candidates, nil, true)
	var queued []int64
	for _, outcome := range s.SubmitPlan(ctx, seriesID, searchLedger, nil, candidates, plan.Picks) {
		if outcome.QueueID > 0 && !slices.Contains(queued, outcome.QueueID) {
			queued = append(queued, outcome.QueueID)
		}
		if outcome.Status == "failed" {
			slog.Warn("planned grab failed", "series", seriesID, "release", outcome.Title, "error", outcome.Message)
		}
	}
	return queued
}

// SubmitPlan only submits the explicitly selected offers and approved book
// identities. Each offer is evaluated against current policy and ownership
// immediately before reservation. Successful and uncertain jobs keep claims;
// partial failure never silently chooses a different release.
func (s *SearchService) SubmitPlan(ctx context.Context, seriesID int64, ledger string, scope *SearchScope, candidates []ReleaseCandidate, picks []PlanPick) []PlanOutcome {
	outcomes := make([]PlanOutcome, 0, len(picks))
	seen := map[string]bool{}
	for _, pick := range picks {
		if seen[pick.Key] {
			continue
		}
		seen[pick.Key] = true
		outcome := PlanOutcome{Key: pick.Key, Status: "failed", Message: "Release is no longer available for this selection; search again."}
		for _, original := range candidates {
			if candidateKey(original) != pick.Key {
				continue
			}
			outcome.Title = original.Title
			run, err := s.prepare(ctx, seriesID, ledger, scope)
			if err != nil {
				outcome.Message = err.Error()
				break
			}
			fresh, _ := s.process(run, []provider.Candidate{original.cand})
			found := false
			for _, c := range fresh {
				if candidateKey(c) != pick.Key {
					continue
				}
				found = true
				if len(c.Rejections) > 0 {
					outcome.Message = "Release no longer meets the selection or profile: " + strings.Join(c.Rejections, ", ")
					if len(c.Rejections) == 1 && c.Rejections[0] == "rejected.no_coverage" {
						outcome.Status = "skipped"
						outcome.Message = "These books no longer need a download or are already pending."
					}
					break
				}
				var claims []store.BookRef
				for _, ref := range c.Coverage {
					if slices.Contains(pick.Books, ref) {
						claims = append(claims, ref)
					}
				}
				if len(claims) == 0 {
					outcome.Status = "skipped"
					outcome.Message = "The approved books no longer need this release; search again."
					break
				}
				c.RequestedReplacements = pick.RequestedReplacements
				qid, err := s.grabClaims(ctx, seriesID, ledger, c, claims, false)
				if err != nil {
					outcome.Message = err.Error()
				} else {
					outcome.Status = "queued"
					outcome.QueueID = qid
					outcome.Message = "Queued for download"
				}
				break
			}
			if !found {
				outcome.Status = "skipped"
				outcome.Message = "The edition is no longer collected."
			}
			break
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// Grab routes a candidate to its owning provider and records the queue
// item. force bypasses soft rejections; hard gates always hold.
func (s *SearchService) Grab(ctx context.Context, seriesID int64, searchLedger string, c ReleaseCandidate, force bool) (int64, error) {
	return s.grabClaims(ctx, seriesID, searchLedger, c, c.Coverage, force)
}

func (s *SearchService) grabClaims(ctx context.Context, seriesID int64, searchLedger string, c ReleaseCandidate, slots []store.BookRef, force bool) (int64, error) {
	for _, r := range c.Rejections {
		if hardRejections[r] {
			return 0, apperr.Invalidf("candidate is rejected: %s", r)
		}
		if !force {
			return 0, apperr.Invalidf("candidate is rejected: %s (use force to override)", r)
		}
	}
	prov := s.registry.Get(c.Provider)
	if prov == nil {
		return 0, apperr.Unavailablef("provider %q is not configured", c.Provider)
	}

	// Choose the download client before anything is reserved or sent, and
	// keep it with the job: polling and import then ask that client only.
	client := ""
	if router, ok := prov.(provider.ClientRouter); ok {
		var err error
		if client, err = router.ClientFor(ctx, c.cand); err != nil {
			return 0, apperr.Unavailablef("%v", err)
		}
	}

	if err := s.retire(ctx, c.replaces, slots); err != nil {
		return 0, err
	}
	// Only a person's explicit choice grants replacement intent in review.
	// Owned or policy-driven claims and surplus pack contents do not grant it.
	requested := c.RequestedReplacements
	c.RequestedReplacements = nil
	if !scheduled(ctx) {
		for _, ref := range slots {
			if slices.ContainsFunc(requested, func(b store.BookRef) bool { return refKey(b) == refKey(ref) }) {
				c.RequestedReplacements = append(c.RequestedReplacements, ref)
			}
		}
	}
	snapshot, _ := json.Marshal(c)
	qid, created, err := s.store.ReserveQueue(ctx, store.QueueItem{SeriesID: seriesID, Protocol: c.Protocol, ProviderID: c.Provider, ReleaseKey: c.cand.ExternalKey(), DownloadClient: client, Title: c.Title, Snapshot: snapshot}, slots)
	if err != nil {
		return 0, err
	}

	if !created {
		return qid, nil
	}
	externalID, err := prov.Grab(provider.InClient(ctx, client), c.cand)
	if err != nil {
		_ = s.store.DeferQueue(ctx, store.QueueItem{ID: qid}, "Submission result is uncertain: "+err.Error())
		return 0, err
	}
	err = s.store.SetQueueExternalID(ctx, qid, externalID)
	if err == nil {
		_ = s.store.AddDecision(ctx, seriesID, store.DecisionBody{
			Event: "grabbed", Ledger: searchLedger, Title: c.Title,
			Provider: c.Provider, Site: c.Site, Key: externalID,
			Books: slots, Force: force, By: grabbedBy(ctx),
		})
	}
	return qid, err
}

// retire stops tracking the download a search set out to replace, at the
// moment a replacement claims one of its books and not before: a search
// that finds nothing leaves the old job running. Only Sento's record and
// reservations go; the torrent and its files stay in the client.
func (s *SearchService) retire(ctx context.Context, queueID int64, claims []store.BookRef) error {
	if queueID == 0 {
		return nil
	}
	old, err := s.store.GetQueueItem(ctx, queueID)
	if err != nil {
		return err
	}
	switch old.Status {
	case store.QueueImported, store.QueueFailed, store.QueueCancelled:
		return nil
	}
	held := queueBooks(old)
	if !slices.ContainsFunc(claims, func(ref store.BookRef) bool {
		return slices.ContainsFunc(held, func(h store.BookRef) bool { return refKey(h) == refKey(ref) })
	}) {
		return nil
	}
	return s.store.CancelQueue(ctx, queueID)
}

func queueBooks(q *store.QueueItem) []store.BookRef {
	var refs []store.BookRef
	_ = json.Unmarshal(q.Books, &refs)
	return refs
}

type scheduledKey struct{}

// Scheduled marks the work under ctx as Sento's own, not a person's: what
// it grabs is attributed to the schedule, and its searches only fill what
// is missing — quality upgrades wait for a person to ask (Sonarr parity).
func Scheduled(ctx context.Context) context.Context {
	return context.WithValue(ctx, scheduledKey{}, true)
}

// scheduled reports whether Sento's own loops, not a person, started the
// work under ctx.
func scheduled(ctx context.Context) bool {
	v, _ := ctx.Value(scheduledKey{}).(bool)
	return v
}

func grabbedBy(ctx context.Context) string {
	if scheduled(ctx) {
		return "schedule"
	}
	return "you"
}

// SearchAgain searches each target and grabs what Sento chooses, after a
// release failed or was given up: a replacement is under way within
// minutes, not at the next sweep.
func (s *SearchService) SearchAgain(ctx context.Context, targets []SearchTarget) {
	ctx = Scheduled(ctx)
	for _, t := range targets {
		if ctx.Err() != nil {
			return
		}
		result, err := s.FindCandidates(ctx, t.SeriesID, t.Ledger, nil)
		if err != nil {
			slog.Warn("searching again failed", "series", t.SeriesID, "ledger", t.Ledger, "error", err)
			continue
		}
		s.GrabBest(ctx, t.SeriesID, t.Ledger, result.Candidates)
	}
}
