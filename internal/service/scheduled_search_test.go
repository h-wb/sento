package service

import (
	"context"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// upgradeSearchFixture owns volume 1 with a copy the profile scores at 50
// and wants better for (cutoff 100, minimum improvement 10): the book is
// upgrade-eligible, and the other volumes are still missing.
func upgradeSearchFixture(t *testing.T) (*store.Store, int64, store.Edition, []store.Book, store.CollectedVersion) {
	t.Helper()
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, sid, c, ws[0], v)
	group, err := st.AddFormat(ctx, profile.Format{Name: "Preferred group", Conditions: []profile.Condition{{Type: profile.CondGroup, Value: "LuCaZ"}}})
	if err != nil {
		t.Fatal(err)
	}
	premium, err := st.AddFormat(ctx, profile.Format{Name: "Premium", Conditions: []profile.Condition{{Type: profile.CondFlag, Value: "premium"}}})
	if err != nil {
		t.Fatal(err)
	}
	vol, ch := profile.DefaultRules()
	pid, err := st.AddProfile(ctx, profile.Profile{
		Name: "Upgrade", UpgradesEnabled: true, UpgradeUntilScore: 100, MinUpgradeScore: 10,
		MinFormatScore: profile.MinScoreOff,
		FormatScores:   map[int64]int{group: 50, premium: 100}, Volume: vol, Chapter: ch,
	})
	if err != nil {
		t.Fatal(err)
	}
	v.ProfileID = pid
	if _, err = st.SaveVersion(ctx, sid, v); err != nil {
		t.Fatal(err)
	}
	if err = st.SetAssetFacts(ctx, fid, store.AssetFacts{Title: "ONE PIECE v01", Group: "LuCaZ", FactsKnown: true, Protocol: "torrent"}, ""); err != nil {
		t.Fatal(err)
	}
	return st, sid, c, ws, v
}

func upgradeRelease() []provider.Candidate {
	return []provider.Candidate{{
		Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE v01 (Digital) (LuCaZ) (Premium)",
		Torrent: &provider.TorrentRef{GUID: "better", Size: 100 << 20, Seeders: 30},
	}}
}

// A scheduled sweep only fills what is missing: an upgrade-eligible book
// stays out of its plan and cannot be grabbed unattended, while a person's
// search still offers the upgrade (Sonarr parity).
func TestScheduledSearchSkipsUpgradeEligibleBooks(t *testing.T) {
	st, sid, _, ws, v := upgradeSearchFixture(t)
	svc := NewSearchService(st, torrentRegistry(st))

	run, err := svc.prepare(context.Background(), sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !run.wantedBooks[v.ID][ws[0].ID] {
		t.Fatal("person's search should plan the upgrade-eligible book")
	}

	run, err = svc.prepare(Scheduled(context.Background()), sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.wantedBooks[v.ID][ws[0].ID] {
		t.Fatal("scheduled sweep planned an upgrade-eligible book")
	}
	for _, b := range run.planBooks {
		if b.Book.BookID == ws[0].ID {
			t.Fatalf("scheduled plan offers the upgrade-eligible book: %+v", b)
		}
	}
	if !run.wantedBooks[v.ID][ws[1].ID] {
		t.Fatal("scheduled sweep should still plan the missing books")
	}
	cs, _ := svc.process(run, upgradeRelease())
	for _, rc := range cs {
		if rc.VersionID != v.ID {
			continue
		}
		if !slices.Contains(rc.Rejections, "rejected.no_coverage") {
			t.Fatalf("upgrade reachable by the scheduled sweep: %+v", rc.Rejections)
		}
	}
}

// A per-book search is a person's explicit choice: it offers the upgrade
// even when the release does not clear the profile's minimum improvement,
// which a batch search still enforces.
func TestPerBookSearchOffersUpgradesBeyondTheImprovementGate(t *testing.T) {
	st, sid, _, ws, v := upgradeSearchFixture(t)
	svc := NewSearchService(st, torrentRegistry(st))
	ctx := context.Background()
	same := []provider.Candidate{{
		Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE v01 (Digital) (LuCaZ)",
		Torrent: &provider.TorrentRef{GUID: "same-quality", Size: 100 << 20, Seeders: 30},
	}}

	// Batch search: the improvement gate is the second line of defense.
	batch, err := svc.prepare(ctx, sid, LedgerVolume, &SearchScope{VersionID: v.ID, All: true})
	if err != nil {
		t.Fatal(err)
	}
	cs, _ := svc.process(batch, same)
	for _, rc := range cs {
		if rc.VersionID == v.ID && !slices.Contains(rc.Rejections, "rejected.no_upgrade") {
			t.Fatalf("equal-quality release accepted as a batch upgrade: %+v", rc.Rejections)
		}
	}

	// The person named the book: the gate steps aside.
	book, err := svc.prepare(ctx, sid, LedgerVolume, &SearchScope{VersionID: v.ID, BookID: ws[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if !book.wantedBooks[v.ID][ws[0].ID] {
		t.Fatal("per-book search should plan the upgrade-eligible book")
	}
	cs, _ = svc.process(book, same)
	offered := false
	for _, rc := range cs {
		if rc.VersionID != v.ID || slices.Contains(rc.Rejections, "rejected.no_upgrade") {
			t.Fatalf("per-book choice gated by the improvement minimum: %+v", rc.Rejections)
		}
		if len(rc.Coverage) == 1 && rc.Coverage[0].BookID == ws[0].ID {
			offered = true
		}
	}
	if !offered {
		t.Fatal("the upgrade was not offered to the per-book search")
	}
}
