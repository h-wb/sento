package service

import (
	"context"
	"encoding/json"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
	"testing"
)

func TestReleaseClassificationAndDecisionEvidence(t *testing.T) {
	_, st, id, c, _, target := modelFixture(t)
	ctx := context.Background()
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, target: &TargetSlot{CollectionID: c.ID, TargetID: target.ID, All: true}, titles: []string{"ONE PIECE"}, priorities: map[[2]string]int{{"torrent", "site"}: 5}}
	if err := svc.prepareTargets(ctx, run); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, title, reason, status string
		seeders                     int
	}{
		{"ready", "ONE PIECE Vols. 01-02 [English Digital]", "", "ready", 30},
		{"seeders", "ONE PIECE v01 [English Digital]", "rejected.seeders", "review", 0},
		{"language", "ONE PIECE v01 [French/German Digital]", "rejected.language", "blocked", 30},
		{"unknown", "ONE PIECE 01 [English Digital]", "rejected.numbers_unknown", "blocked", 30},
		{"novel", "ONE PIECE Light Novel v01 [English Digital]", "rejected.content_type", "blocked", 30},
		{"pack", "ONE PIECE (2003-2026) [English Digital]", "rejected.pack_unverified", "blocked", 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, dropped := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", SiteID: "site", Title: tc.title, Torrent: &provider.TorrentRef{GUID: tc.name, Size: 100 << 20, Seeders: tc.seeders}}})
			if dropped != 0 || len(got) != 1 {
				t.Fatalf("classification hidden: %d %d", len(got), dropped)
			}
			candidate := got[0]
			if candidate.Decision.Status != tc.status {
				t.Fatalf("decision=%+v", candidate.Decision)
			}
			if tc.reason != "" && (len(candidate.Rejections) == 0 || candidate.Rejections[0] != tc.reason) {
				t.Fatalf("reasons=%v", candidate.Rejections)
			}
			if candidate.Decision.CanForce != (tc.status == "review") {
				t.Fatalf("force eligibility drift: %+v", candidate.Decision)
			}
			if len(candidate.ParseTrace) == 0 {
				t.Fatal("classification has no evidence")
			}
			if tc.status == "ready" {
				total := 0
				for _, part := range candidate.ScoreParts {
					total += part.Value
				}
				if total != candidate.Score {
					t.Fatalf("score explanation %d != %d", total, candidate.Score)
				}
			}
			if tc.name == "ready" {
				raw, _ := json.Marshal(candidate)
				var wire struct {
					Parse struct {
						Volumes [][2]float64 `json:"volumes"`
					} `json:"parse"`
				}
				if err := json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				if len(wire.Parse.Volumes) != 1 || wire.Parse.Volumes[0] != [2]float64{1, 2} {
					t.Fatalf("range contract: %+v", wire)
				}
			}
		})
	}
}

func TestExtraReleaseDoesNotClaimRegularVolume(t *testing.T) {
	_, st, id, c, works, target := modelFixture(t)
	ctx := context.Background()
	target.IncludeExtras = true
	if _, err := st.SaveTarget(ctx, id, target); err != nil {
		t.Fatal(err)
	}
	extra, err := st.SaveWork(ctx, id, store.Work{CollectionID: c.ID, SequenceNum: 100, SequenceStr: "1 bonus", CountType: "extra", NumberKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, target: &TargetSlot{CollectionID: c.ID, TargetID: target.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err = svc.prepareTargets(ctx, run); err != nil {
		t.Fatal(err)
	}
	out, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE Special v01 [English Digital]", Torrent: &provider.TorrentRef{GUID: "bonus", Size: 100 << 20, Seeders: 30}}})
	if len(out) != 1 || len(out[0].Coverage) != 1 || out[0].Coverage[0].WorkID != extra {
		t.Fatalf("bonus mapped incorrectly: %+v", out)
	}
	if out[0].Coverage[0].WorkID == works[0].ID {
		t.Fatal("special claimed regular book")
	}
}
