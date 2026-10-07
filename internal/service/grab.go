package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// GrabBest greedily grabs eligible candidates by book coverage.
func (s *SearchService) GrabBest(ctx context.Context, seriesID int64, searchLedger string, candidates []ReleaseCandidate) []int64 {
	var queued []int64
	taken := map[string]bool{}
	for _, c := range candidates {
		if len(c.Rejections) > 0 {
			continue
		}
		news := false
		for _, key := range grabKeys(c) {
			if !taken[key] {
				news = true
				break
			}
		}
		if !news {
			continue
		}
		qid, err := s.Grab(ctx, seriesID, searchLedger, c, false)
		if err != nil {
			slog.Warn("grab failed", "series", seriesID, "release", c.Title, "error", err)
			continue
		}
		exists := false
		for _, old := range queued {
			if old == qid {
				exists = true
			}
		}
		if !exists {
			queued = append(queued, qid)
		}
		for _, key := range grabKeys(c) {
			taken[key] = true
		}
	}
	return queued
}

// Grab routes a candidate to its owning provider and records the queue
// item. force bypasses soft rejections; hard gates always hold.
func (s *SearchService) Grab(ctx context.Context, seriesID int64, searchLedger string, c ReleaseCandidate, force bool) (int64, error) {
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

	slots := c.Coverage
	snapshot, _ := json.Marshal(c)
	qid, created, err := s.store.ReserveQueue(ctx, store.QueueItem{SeriesID: seriesID, Protocol: c.Protocol, ProviderID: c.Provider, ReleaseKey: c.cand.ExternalKey(), Title: c.Title, Snapshot: snapshot}, slots)
	if err != nil {
		return 0, err
	}

	if !created {
		return qid, nil
	}
	externalID, err := prov.Grab(ctx, c.cand)
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

// grabKeys are the books a candidate would claim, for de-duplicating
// grabs within one sweep.
func grabKeys(c ReleaseCandidate) []string {
	keys := make([]string, 0, len(c.Coverage))
	for _, sl := range c.Coverage {
		keys = append(keys, store.BookKey(sl.VersionID, sl.BookID))
	}
	return keys
}
