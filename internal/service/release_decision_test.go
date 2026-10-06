package service

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
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

// Edition fit: medium only breaks ties, and only editions the user
// collects compete for a release.
func TestEditionAmbiguity(t *testing.T) {
	paper := store.Collection{ID: 1, Type: "volume", Language: "en", Medium: "paperback", EditionTraits: []string{"standard"}}
	digital := store.Collection{ID: 2, Type: "volume", Language: "en", Medium: "digital", EditionTraits: []string{"standard"}}
	french := store.Collection{ID: 3, Type: "volume", Language: "fr", EditionTraits: []string{"standard"}}
	chapters := store.Collection{ID: 4, Type: "chapter", Language: "en"}
	titles := []string{"ONE PIECE"}
	parsed := func(name string) parse.Result { return parse.ParseAnchored(name, parse.ChannelTorrent, titles) }
	run := func(collected ...int64) *searchRun {
		r := &searchRun{collections: []store.Collection{paper, digital, french, chapters}}
		for _, id := range []int64{1, 2, 3, 4} {
			r.targets = append(r.targets, store.DownloadTarget{ID: id, CollectionID: id, Monitored: slices.Contains(collected, id)})
		}
		return r
	}
	cases := []struct {
		name      string
		release   string
		collected []int64
		edition   store.Collection
		ambiguous bool
	}{
		{"digital rip fits the only collected edition, a paperback", "ONE PIECE v01 (Digital) (1r0n)", []int64{1}, paper, false},
		{"uncollected digital edition does not compete", "ONE PIECE v01 (Digital) (1r0n)", []int64{1, 4}, paper, false},
		{"digital rip goes to the digital edition when both are collected", "ONE PIECE v01 (Digital) (1r0n)", []int64{1, 2}, digital, false},
		{"and needs review for the paperback one", "ONE PIECE v01 (Digital) (1r0n)", []int64{1, 2}, paper, true},
		{"untagged medium stays ambiguous between both", "ONE PIECE v01 [English]", []int64{1, 2}, paper, true},
		{"unknown language fits English and French", "ONE PIECE v01 (Digital) (1r0n)", []int64{1, 3}, paper, true},
		{"medium never settles a language question", "ONE PIECE v01 (Digital) (1r0n)", []int64{2, 3}, digital, true},
		{"a language tag settles it", "ONE PIECE v01 [English] (Digital)", []int64{1, 3}, paper, false},
		{"searching an uncollected edition still sees collected rivals", "ONE PIECE v01 [English]", []int64{1}, digital, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parsed(tc.release)
			if !editionCompatible(p, tc.edition) {
				t.Fatalf("release should fit the edition: %+v", p)
			}
			if got := run(tc.collected...).ambiguousEdition(p, tc.edition); got != tc.ambiguous {
				t.Fatalf("ambiguous = %v, want %v", got, tc.ambiguous)
			}
		})
	}
	if editionCompatible(parsed("ONE PIECE v01 [French] (Digital)"), paper) {
		t.Fatal("an explicit other language must stay incompatible")
	}
	if editionCompatible(parsed("ONE PIECE v01 (Omnibus) [English]"), paper) {
		t.Fatal("an omnibus must not fit a standard edition")
	}
}

// End to end: an ambiguous release is offered for review with its books,
// never as ready, and a stated language makes it ready.
func TestAmbiguousEditionIsReviewable(t *testing.T) {
	_, st, id, c, _, target := modelFixture(t)
	ctx := context.Background()
	french, err := st.AddCustomCollection(ctx, store.Collection{SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true}, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := st.ListTargets(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	collecting := false
	for _, other := range targets {
		if other.CollectionID == french {
			collecting = true
			other.Monitored = true
			if _, err = st.SaveTarget(ctx, id, other); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !collecting {
		t.Fatal("the French edition has no collected version")
	}
	target.AllowUnknown = true
	if _, err = st.SaveTarget(ctx, id, target); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, target: &TargetSlot{CollectionID: c.ID, TargetID: target.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err = svc.prepareTargets(ctx, run); err != nil {
		t.Fatal(err)
	}
	search := func(title string) ReleaseCandidate {
		t.Helper()
		got, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: title, Torrent: &provider.TorrentRef{GUID: title, Size: 100 << 20, Seeders: 30}}})
		if len(got) != 1 {
			t.Fatalf("%q: %d candidates", title, len(got))
		}
		return got[0]
	}
	untagged := search("ONE PIECE v01 (Digital) (1r0n)")
	if untagged.Decision.Status != "review" || !untagged.Decision.CanForce || untagged.Decision.CanDownload ||
		!slices.Equal(untagged.Rejections, []string{"rejected.edition_ambiguous"}) || len(untagged.Coverage) != 1 {
		t.Fatalf("untagged release: %+v rejections=%v coverage=%v", untagged.Decision, untagged.Rejections, untagged.Coverage)
	}
	if tagged := search("ONE PIECE v01 [English] (Digital) (1r0n)"); tagged.Decision.Status != "ready" {
		t.Fatalf("tagged release: %+v", tagged.Decision)
	}
	if other := search("ONE PIECE v01 [French] (Digital)"); other.Decision.Status != "blocked" || other.Rejections[0] != "rejected.language" {
		t.Fatalf("other language: %+v %v", other.Decision, other.Rejections)
	}
}

// A name without a language takes it from the evidence around it, in
// order: indexer section, English-only publisher, indexer default.
func TestLanguageEvidenceForUntaggedReleases(t *testing.T) {
	_, st, id, c, _, target := modelFixture(t)
	ctx := context.Background()
	if _, err := st.AddCustomCollection(ctx, store.Collection{SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true}, c.ID, 0); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, target: &TargetSlot{CollectionID: c.ID, TargetID: target.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err := svc.prepareTargets(ctx, run); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, title      string
		category, deflt  string
		language, source string
		status           string
	}{
		{"indexer section", "ONE PIECE v01 (Digital) (1r0n)", "en", "fr", "en", "indexer category", "ready"},
		{"publisher tag", "ONE PIECE v01 (Digital) (VIZ)", "", "fr", "en", "publisher", "ready"},
		{"indexer default", "ONE PIECE v01 (Digital) (1r0n)", "", "fr", "fr", "indexer default", "blocked"},
		{"the name wins", "ONE PIECE v01 [English] (Digital)", "ja", "fr", "en", "release name", "ready"},
		{"no evidence", "ONE PIECE v01 (Digital) (1r0n)", "", "", "", "unknown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: tc.title, Torrent: &provider.TorrentRef{
				GUID: tc.name, Size: 100 << 20, Seeders: 30, CategoryLanguage: tc.category, DefaultLanguage: tc.deflt,
			}}})
			if len(got) != 1 {
				t.Fatalf("%d candidates", len(got))
			}
			if got[0].Parse.Language != tc.language || got[0].Decision.LanguageSource != tc.source {
				t.Fatalf("language %q from %q, want %q from %q", got[0].Parse.Language, got[0].Decision.LanguageSource, tc.language, tc.source)
			}
			if tc.status != "" && got[0].Decision.Status != tc.status {
				t.Fatalf("status %q, want %q (%v)", got[0].Decision.Status, tc.status, got[0].Rejections)
			}
		})
	}
}

// An edition-wide search covers the wanted books; AllBooks widens it to
// every book, so a paused edition can still be browsed.
func TestEditionSearchScope(t *testing.T) {
	_, st, id, c, works, target := modelFixture(t)
	ctx := context.Background()
	target.Monitored = false
	if _, err := st.SaveTarget(ctx, id, target); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	wanted := func(allBooks bool) int {
		t.Helper()
		run := &searchRun{seriesID: id, ledger: LedgerVolume, target: &TargetSlot{CollectionID: c.ID, TargetID: target.ID, All: true, AllBooks: allBooks}, titles: []string{"ONE PIECE"}}
		if err := svc.prepareTargets(ctx, run); err != nil {
			t.Fatal(err)
		}
		return len(run.targetWanted[target.ID])
	}
	if got := wanted(false); got != 0 {
		t.Fatalf("a paused edition wants %d books", got)
	}
	if got := wanted(true); got != len(works) || got == 0 {
		t.Fatalf("all-books scope selected %d of %d", got, len(works))
	}
}

// The raw catalog documents supply what the extracted columns do not.
func TestApplyCatalog(t *testing.T) {
	info := mediafile.ComicInfo{Summary: "Series summary"}
	applyCatalog(&info,
		`{"authors":["Yoshihiro Togashi"],"artists":["Yoshihiro Togashi"],"genres":["action","slice_of_life"],"tags":["Survival","Rivals"],"content_rating":"suggestive"}`,
		`{"sub_title":"The Day of Departure","description":{"desc":" Gon decides to become a Hunter. "}}`)
	want := mediafile.ComicInfo{
		Title: "The Day of Departure", Summary: "Gon decides to become a Hunter.",
		Writer: "Yoshihiro Togashi", Penciller: "Yoshihiro Togashi",
		Genre: "Action, Slice Of Life", Tags: "Survival, Rivals", AgeRating: "Teen",
	}
	if info.Title != want.Title || info.Summary != want.Summary || info.Writer != want.Writer || info.Penciller != want.Penciller ||
		info.Genre != want.Genre || info.Tags != want.Tags || info.AgeRating != want.AgeRating {
		t.Fatalf("got %+v\nwant %+v", info, want)
	}
	// Nothing known: nothing overwritten.
	kept := mediafile.ComicInfo{Summary: "Series summary"}
	applyCatalog(&kept, "", "")
	if kept.Summary != "Series summary" || kept.Writer != "" {
		t.Fatalf("empty documents changed the info: %+v", kept)
	}
	if y, m, d, ok := splitDate("2005-04-05"); !ok || y != "2005" || m != "4" || d != "5" {
		t.Fatalf("splitDate = %s %s %s %v", y, m, d, ok)
	}
}
