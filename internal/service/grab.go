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
				outcome.Message = "The edition or version is no longer selected."
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
			Books: slots, Force: force,
		})
	}
	return qid, err
}
