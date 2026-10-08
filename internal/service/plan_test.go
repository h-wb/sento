package service

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

func planCandidate(name string, version int64, numbers ...int64) ReleaseCandidate {
	c := ReleaseCandidate{GUID: name, Provider: "torrent", SiteID: "1", Protocol: "torrent", Seeders: 5, VersionID: version, RepresentedBooks: len(numbers), Size: 100}
	for _, n := range numbers {
		c.Coverage = append(c.Coverage, store.BookRef{BookID: n, VersionID: version, EditionID: version, Ledger: "volume", Number: n * 100})
	}
	c.Score = 1000 * len(numbers)
	c.Key = candidateKey(c)
	return c
}

func TestPlanMarginalCoverageAndIndependentVersions(t *testing.T) {
	a := planCandidate("a", 1, 1, 2, 3, 4, 5)
	a.Score += 100
	b := planCandidate("b", 1, 4, 5, 6, 7, 8)
	c := planCandidate("single", 1, 6)
	d := planCandidate("other version", 2, 1)
	plan := BuildPlan([]ReleaseCandidate{c, b, d, a}, nil, true)
	seen := map[string]bool{}
	for _, p := range plan.Picks {
		for _, ref := range p.Books {
			if seen[refKey(ref)] {
				t.Fatal("overlapping claims")
			}
			seen[refKey(ref)] = true
		}
	}
	if len(seen) != 9 || len(plan.Picks) != 3 {
		t.Fatalf("plan: %+v", plan)
	}
	if plan.Picks[0].Key != a.Key || plan.Picks[1].Key != b.Key || len(plan.Picks[1].Books) != 3 {
		t.Fatalf("marginal plan: %+v", plan.Picks)
	}
	if slices.ContainsFunc(plan.Picks, func(p PlanPick) bool { return p.Key == c.Key }) {
		t.Fatal("redundant single selected")
	}
	reversed := []ReleaseCandidate{a, d, b, c}
	other := BuildPlan(reversed, nil, true)
	x, _ := json.Marshal(plan.Picks)
	y, _ := json.Marshal(other.Picks)
	if string(x) != string(y) {
		t.Fatal("plan depends on arrival order")
	}
}

func TestPlanLeavesInefficientOverlapToAPerson(t *testing.T) {
	a := planCandidate("a", 1, 1, 2, 3, 4, 5)
	a.Score += 100
	b := planCandidate("b", 1, 3, 4, 5, 6)
	books := []PlanBook{{Book: b.Coverage[3]}}
	plan := BuildPlan([]ReleaseCandidate{b, a}, books, true)
	if len(plan.Picks) != 1 || plan.Picks[0].Key != a.Key {
		t.Fatalf("pack adding one of its four books was planned: %+v", plan.Picks)
	}
	if plan.Books[0].Availability != "available" {
		t.Fatalf("book hidden with its only release: %+v", plan.Books)
	}
	single := planCandidate("single", 1, 6)
	plan = BuildPlan([]ReleaseCandidate{b, single, a}, books, true)
	if len(plan.Picks) != 2 || plan.Picks[1].Key != single.Key {
		t.Fatalf("efficient alternative not planned: %+v", plan.Picks)
	}
}

func TestPlanAvailabilityAndUnknownSizes(t *testing.T) {
	ready := planCandidate("ready", 1, 1)
	ready.Size = 0
	dead := planCandidate("dead", 1, 2)
	dead.Seeders = 0
	dead.Rejections = []string{"rejected.seeders"}
	profile := planCandidate("profile", 1, 3)
	profile.Rejections = []string{"rejected.format_score"}
	wrong := planCandidate("wrong", 1, 4)
	wrong.Rejections = []string{"rejected.language"}
	var books []PlanBook
	for _, c := range []ReleaseCandidate{ready, dead, profile, wrong} {
		books = append(books, PlanBook{Book: c.Coverage[0]})
	}
	for _, complete := range []bool{false, true} {
		p := BuildPlan([]ReleaseCandidate{ready, dead, profile, wrong}, books, complete)
		if len(p.Picks) != 1 || p.UnknownSizes != 1 {
			t.Fatalf("plan: %+v", p)
		}
		want := []string{"available", "no_seeders", "outside_profile", "unknown"}
		if complete {
			want[3] = "not_found"
		}
		for i, b := range p.Books {
			if b.Availability != want[i] {
				t.Fatalf("availability: %+v", p.Books)
			}
		}
	}
}

func TestPlanSubmissionRechecksOwnershipAndUsesClaims(t *testing.T) {
	_, st, id, edition, books, version := modelFixture(t)
	ctx := context.Background()
	prov := &payloadProvider{}
	svc := NewSearchService(st, provider.NewRegistry(prov))
	scope := &SearchScope{VersionID: version.ID, AllBooks: true}
	run, err := svc.prepare(ctx, id, "volume", scope)
	if err != nil {
		t.Fatal(err)
	}
	raw := provider.Candidate{Provider: prov.ID(), Protocol: "torrent", SiteID: "1", Title: "ONE PIECE v01-v03 (Digital) [English]", Torrent: &provider.TorrentRef{GUID: "pack", InfoHash: "pack", Seeders: 20, Size: 300 << 20}}
	candidates, _ := svc.process(run, []provider.Candidate{raw})
	if len(candidates) != 1 || len(candidates[0].Rejections) > 0 {
		t.Fatalf("candidates: %+v", candidates)
	}
	claims := []store.BookRef{modelRef(edition, books[1], version)}
	out := svc.SubmitPlan(ctx, id, "volume", scope, candidates, []PlanPick{{Key: candidates[0].Key, Books: claims}})
	if len(out) != 1 || out[0].Status != "queued" {
		t.Fatalf("submission: %+v", out)
	}
	job, err := st.GetQueueItem(ctx, out[0].QueueID)
	if err != nil {
		t.Fatal(err)
	}
	var saved []store.BookRef
	_ = json.Unmarshal(job.Books, &saved)
	var snap ReleaseCandidate
	_ = json.Unmarshal(job.Snapshot, &snap)
	if len(saved) != 1 || saved[0].BookID != books[1].ID || len(snap.Contents) != 3 || len(snap.Coverage) != 3 {
		t.Fatalf("claims or snapshot lost: %s %s", job.Books, job.Snapshot)
	}
	again := svc.SubmitPlan(ctx, id, "volume", scope, candidates, []PlanPick{{Key: candidates[0].Key, Books: claims}})
	if again[0].Status == "queued" {
		t.Fatalf("queued the reserved book again: %+v", again)
	}
}

// A replacement search sees the stalled job's books as wanted, rules out the
// job's own release, and keeps the job until a replacement claims its books.
func TestReplacementKeepsOldDownloadUntilPicked(t *testing.T) {
	_, st, id, edition, books, version := modelFixture(t)
	ctx := context.Background()
	prov := &payloadProvider{}
	svc := NewSearchService(st, provider.NewRegistry(prov))
	offer := func(hash string) provider.Candidate {
		return provider.Candidate{Provider: prov.ID(), Protocol: "torrent", SiteID: "1", Title: "ONE PIECE v01 (Digital) [English]", Torrent: &provider.TorrentRef{GUID: hash, InfoHash: hash, Seeders: 20, Size: 100 << 20}}
	}
	search := func(scope *SearchScope) []ReleaseCandidate {
		t.Helper()
		run, err := svc.prepare(ctx, id, "volume", scope)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := svc.process(run, []provider.Candidate{offer("old"), offer("new")})
		return out
	}
	pick := func(cs []ReleaseCandidate, guid string) ReleaseCandidate {
		t.Helper()
		for _, c := range cs {
			if c.GUID == guid {
				return c
			}
		}
		t.Fatalf("no candidate %q", guid)
		return ReleaseCandidate{}
	}
	scope := &SearchScope{VersionID: version.ID, All: true}
	oldID, err := svc.Grab(ctx, id, "volume", pick(search(scope), "old"), false)
	if err != nil {
		t.Fatal(err)
	}
	if c := pick(search(scope), "new"); !slices.Contains(c.Rejections, "rejected.no_coverage") {
		t.Fatalf("reserved book offered without a replacement scope: %+v", c.Rejections)
	}

	scope.ReplaceQueueID = oldID
	found := search(scope)
	if c := pick(found, "old"); !slices.Contains(c.Rejections, "rejected.previous_download") {
		t.Fatalf("old release not ruled out: %+v", c.Rejections)
	}
	replacement := pick(found, "new")
	want := modelRef(edition, books[0], version)
	if len(replacement.Rejections) > 0 || !slices.Contains(replacement.Coverage, want) {
		t.Fatalf("replacement: %+v %+v", replacement.Rejections, replacement.Coverage)
	}
	if job, _ := st.GetQueueItem(ctx, oldID); job.Status == store.QueueCancelled {
		t.Fatal("searching alone cancelled the old download")
	}
	out := svc.SubmitPlan(ctx, id, "volume", scope, found, []PlanPick{{Key: replacement.Key, Books: []store.BookRef{want}}})
	if len(out) != 1 || out[0].Status != "queued" || out[0].QueueID == oldID {
		t.Fatalf("submission: %+v", out)
	}
	if job, _ := st.GetQueueItem(ctx, oldID); job.Status != store.QueueCancelled {
		t.Fatalf("old download still tracked: %s", job.Status)
	}
}
