package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

// GrabBest greedily grabs eligible candidates by slot coverage.
func (s *SearchService) GrabBest(ctx context.Context, seriesID int64, searchLedger string, candidates []ReleaseCandidate) []int64 {
	var queued []int64
	taken := map[string]bool{}
	for _, c := range candidates {
		if len(c.Rejections) > 0 {
			continue
		}
		news := false
		for _, key := range grabKeys(searchLedger, c) {
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
		for _, key := range grabKeys(searchLedger, c) {
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

	slots := make([]store.ExpectedSlot, 0, len(c.Covered))
	for _, n := range c.Covered {
		slots = append(slots, store.ExpectedSlot{
			Ledger: searchLedger, CollectionID: c.CollectionID, Number: n,
		})
	}
	if len(c.Coverage) > 0 {
		slots = c.Coverage
	}
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
		_ = s.store.UpdateQueueItem(ctx, qid, store.QueueFailed, err.Error())
		return 0, err
	}
	err = s.store.SetQueueExternalID(ctx, qid, externalID)
	if err == nil {
		_ = s.store.AddDecision(ctx, seriesID, store.DecisionBody{
			Event: "grabbed", Ledger: searchLedger, Title: c.Title,
			Provider: c.Provider, Site: c.Site, Key: externalID,
			Slots: slots, Force: force,
		})
	}
	return qid, err
}

func grabSlotKey(ledger string, c ReleaseCandidate, n int64) string {
	if c.TargetID > 0 {
		return store.SlotKey(ledger, c.TargetID, n)
	}
	return store.SlotKey(ledger, c.CollectionID, n)
}

func grabKeys(ledger string, c ReleaseCandidate) []string {
	keys := []string{}
	if len(c.Coverage) > 0 {
		for _, sl := range c.Coverage {
			keys = append(keys, store.SlotKey("unit", sl.TargetID, sl.WorkID))
		}
		return keys
	}
	for _, n := range c.Covered {
		keys = append(keys, grabSlotKey(ledger, c, n))
	}
	return keys
}
