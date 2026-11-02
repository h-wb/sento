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
	_, st, id, _, _, version := modelFixture(t)
	ctx := context.Background()
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true}, titles: []string{"ONE PIECE"}, priorities: map[[2]string]int{{"torrent", "site"}: 5}}
	if err := svc.prepareVersions(ctx, run); err != nil {
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
		{"pack", "ONE PIECE (2003-2026) [English Digital]", "rejected.pack_unverified", "review", 30},
		{"bare pack", "ONE PIECE [English Digital]", "rejected.pack_unverified", "review", 30},
		{"explicit complete without units", "ONE PIECE (Complete) [English Digital]", "rejected.pack_unverified", "review", 30},
		{"explicit complete cannot bypass language", "ONE PIECE (Complete) [French/German Digital]", "rejected.language", "blocked", 30},
		{"other work", "ONE PIECE Party [English Digital]", "rejected.series_match", "blocked", 30},
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
			if tc.name == "explicit complete without units" && len(BuildPlan(got, nil, true).Picks) != 0 {
				t.Fatal("unverified complete pack entered the automatic plan")
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
	_, st, id, c, works, version := modelFixture(t)
	ctx := context.Background()
	version.IncludeExtras = true
	if _, err := st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	extra, err := st.SaveBook(ctx, id, store.Book{EditionID: c.ID, SequenceNum: 100, SequenceStr: "1 bonus", CountType: "extra", NumberKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err = svc.prepareVersions(ctx, run); err != nil {
		t.Fatal(err)
	}
	out, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE Special v01 [English Digital]", Torrent: &provider.TorrentRef{GUID: "bonus", Size: 100 << 20, Seeders: 30}}})
	if len(out) != 1 || len(out[0].Coverage) != 1 || out[0].Coverage[0].BookID != extra {
		t.Fatalf("bonus mapped incorrectly: %+v", out)
	}
	if out[0].Coverage[0].BookID == works[0].ID {
		t.Fatal("special claimed regular book")
	}
}

// Edition fit: medium only breaks ties, and only editions the user
// collects compete for a release.
func TestEditionAmbiguity(t *testing.T) {
	paper := store.Edition{ID: 1, Type: "volume", Language: "en", Medium: "paperback", EditionName: "Standard Edition"}
	digital := store.Edition{ID: 2, Type: "volume", Language: "en", Medium: "digital", EditionName: "Standard Edition"}
	french := store.Edition{ID: 3, Type: "volume", Language: "fr", EditionName: "Standard Edition"}
	chapters := store.Edition{ID: 4, Type: "chapter", Language: "en"}
	titles := []string{"ONE PIECE"}
	parsed := func(name string) parse.Result { return parse.ParseAnchored(name, parse.ChannelTorrent, titles) }
	run := func(collected ...int64) *searchRun {
		r := &searchRun{editions: []store.Edition{paper, digital, french, chapters}}
		r.shelf = newShelf(r.editions)
		for _, id := range []int64{1, 2, 3, 4} {
			r.versions = append(r.versions, store.CollectedVersion{ID: id, EditionID: id, Monitored: slices.Contains(collected, id)})
		}
		return r
	}
	cases := []struct {
		name      string
		release   string
		collected []int64
		edition   store.Edition
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
			if !newShelf(nil).fits(p, tc.edition) {
				t.Fatalf("release should fit the edition: %+v", p)
			}
			if got := run(tc.collected...).ambiguousEdition(p, tc.edition); (got != "") != tc.ambiguous {
				t.Fatalf("ambiguous = %v, want %v", got, tc.ambiguous)
			}
		})
	}
	if newShelf(nil).fits(parsed("ONE PIECE v01 [French] (Digital)"), paper) {
		t.Fatal("an explicit other language must stay incompatible")
	}
	if newShelf(nil).fits(parsed("ONE PIECE v01 (Omnibus) [English]"), paper) {
		t.Fatal("an omnibus must not fit a standard edition")
	}
}

// End to end: an ambiguous release is offered for review with its books,
// never as ready, and a stated language makes it ready.
func TestAmbiguousEditionIsReviewable(t *testing.T) {
	_, st, id, c, _, version := modelFixture(t)
	ctx := context.Background()
	french, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true}, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := st.ListVersions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	collecting := false
	for _, other := range versions {
		if other.EditionID == french {
			collecting = true
			other.Monitored = true
			if _, err = st.SaveVersion(ctx, id, other); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !collecting {
		t.Fatal("the French edition has no collected version")
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err = svc.prepareVersions(ctx, run); err != nil {
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
	_, st, id, c, _, version := modelFixture(t)
	ctx := context.Background()
	if _, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: id, EditionName: "Standard Edition", Language: "fr", Monitored: true}, c.ID, 0); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err := svc.prepareVersions(ctx, run); err != nil {
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
	_, st, id, _, works, version := modelFixture(t)
	ctx := context.Background()
	version.Monitored = false
	if _, err := st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, provider.NewRegistry())
	wanted := func(allBooks bool) int {
		t.Helper()
		run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true, AllBooks: allBooks}, titles: []string{"ONE PIECE"}}
		if err := svc.prepareVersions(ctx, run); err != nil {
			t.Fatal(err)
		}
		return len(run.wantedBooks[version.ID])
	}
	if got := wanted(false); got != 0 {
		t.Fatalf("a paused edition wants %d books", got)
	}
	if got := wanted(true); got != len(works) || got == 0 {
		t.Fatalf("all-books scope selected %d of %d", got, len(works))
	}
	// Every release is offered, but the plan proposes only what the
	// edition wants: here, nothing.
	run := &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true, AllBooks: true}, titles: []string{"ONE PIECE"}}
	if err := svc.prepareVersions(ctx, run); err != nil {
		t.Fatal(err)
	}
	offered, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", SiteID: "site", Title: "ONE PIECE v01 [English Digital]", Torrent: &provider.TorrentRef{GUID: "v1", Size: 100 << 20, Seeders: 30}}})
	if len(offered) != 1 || len(offered[0].Coverage) != 1 {
		t.Fatalf("all-books release not offered: %+v", offered)
	}
	plan := run.buildPlan(offered, true)
	if len(plan.Picks) != 0 || len(plan.Books) != len(works) {
		t.Fatalf("all-books plan: %d picks over %d books, want none over %d", len(plan.Picks), len(plan.Books), len(works))
	}
	if plan.Books[0].Availability != "available" {
		t.Errorf("a book on offer but not proposed reads %q", plan.Books[0].Availability)
	}
	// Once a book is wanted again, it is proposed.
	version.Monitored = true
	if _, err := st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	run = &searchRun{seriesID: id, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true, AllBooks: true}, titles: []string{"ONE PIECE"}}
	if err := svc.prepareVersions(ctx, run); err != nil {
		t.Fatal(err)
	}
	offered, _ = svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", SiteID: "site", Title: "ONE PIECE v01 [English Digital]", Torrent: &provider.TorrentRef{GUID: "v1", Size: 100 << 20, Seeders: 30}}})
	if plan := run.buildPlan(offered, true); len(plan.Picks) != 1 {
		t.Fatalf("wanted book not proposed: %d picks", len(plan.Picks))
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

// Release names are evidence about the numbering, weighed by one judge:
// numbers, a color the edition claims and another edition's name say no;
// a word alone leaves it for a person (docs/reviews/edition-rules.md).
func TestEditionRules(t *testing.T) {
	titles := []string{"NAUSICAA"}
	parsed := func(name string) parse.Result { return parse.ParseAnchored(name, parse.ChannelTorrent, titles) }
	edition := func(id int64, medium, name string) store.Edition {
		return store.Edition{ID: id, Type: "volume", Language: "en", Medium: medium, EditionName: name}
	}
	// alone judges an edition with no sibling to tell it from.
	alone := func(p parse.Result, e store.Edition, evidence int64) (fitVerdict, string) {
		return newShelf([]store.Edition{e}).judge(p, e, evidence)
	}
	standard, box, deluxeBox, threeInOne := edition(1, "paperback", "Standard Edition"), edition(2, "paperback", "Box Set Edition"), edition(3, "hardcover", "Deluxe Box Set Edition"), edition(4, "paperback", "3-in-1 Edition")
	color := edition(5, "paperback", "Full Color Edition")
	for _, tc := range []struct {
		name    string
		release string
		edition store.Edition
		want    fitVerdict
	}{
		{"a box of deluxe books takes deluxe volumes that never say box", "NAUSICAA - Deluxe Edition v01 (2012) [English]", deluxeBox, fitYes},
		{"a plain volume may or may not be a deluxe one", "NAUSICAA v01 [English]", deluxeBox, fitUnsure},
		{"and a deluxe volume may be how a standard one is sold", "NAUSICAA - Deluxe Edition v01 [English]", standard, fitUnsure},
		{"a box of standard books takes plain volumes", "NAUSICAA v01 [English]", box, fitYes},
		{"a box never holds a regrouping", "NAUSICAA - Deluxe Edition (2012) [English]", box, fitNo},
		{"a release sold as a box fits the standard edition too", "NAUSICAA Box Set v01-07 [English]", standard, fitYes},
		{"a regrouping is its own numbering", "NAUSICAA v01 [English]", threeInOne, fitUnsure},
		{"the same regrouping by another spelling", "NAUSICAA 3in1 v02 [English]", threeInOne, fitYes},
		{"medium is a variant", "NAUSICAA v01 (Digital) [English]", standard, fitYes},
		{"colorized volumes are a variant", "NAUSICAA v01 (Colored) [English]", standard, fitYes},
		{"a color edition is not plain volumes", "NAUSICAA v01 [English]", color, fitNo},
		{"language still decides first", "NAUSICAA v01 [French]", standard, fitNo},
	} {
		if got, why := alone(parsed(tc.release), tc.edition, 0); got != tc.want {
			t.Errorf("%s: %v (%s), want %v", tc.name, got, why, tc.want)
		}
	}
	// Packaging and color pick between editions that both fit.
	sh := newShelf([]store.Edition{standard, box})
	if best := sh.prefer(parsed("NAUSICAA Box Set v01-07 [English]"), []store.Edition{standard, box}); len(best) != 1 || best[0].ID != box.ID {
		t.Errorf("a release sold as a box should prefer the box set edition: %+v", best)
	}
	if best := sh.prefer(parsed("NAUSICAA v01 [English]"), []store.Edition{standard, box}); len(best) != 2 {
		t.Errorf("no packaging stated: both editions stay: %+v", best)
	}
	// A verified listing settles words, not a color the edition claims;
	// one naming another edition is no.
	for _, tc := range []struct {
		release  string
		edition  store.Edition
		evidence int64
		want     fitVerdict
	}{
		{"NAUSICAA v01", deluxeBox, deluxeBox.ID, fitYes},
		{"NAUSICAA Omnibus v01", deluxeBox, deluxeBox.ID, fitYes},
		{"NAUSICAA v01", color, color.ID, fitNo},
		{"NAUSICAA v01", standard, deluxeBox.ID, fitNo},
	} {
		if got, why := alone(parsed(tc.release), tc.edition, tc.evidence); got != tc.want {
			t.Errorf("%s in %s, listed under %d: %v (%s), want %v", tc.release, tc.edition.EditionName, tc.evidence, got, why, tc.want)
		}
	}
	// A name the vocabulary does not know is a numbering of its own; a
	// release naming another of the series' editions is that one.
	fullmetal := store.Edition{ID: 6, Type: "volume", Language: "en", EditionName: "Fullmetal Edition", ProviderEditionID: "x", ProviderEditionName: "Fullmetal Edition"}
	series := newShelf([]store.Edition{standard, threeInOne, fullmetal})
	for _, tc := range []struct {
		name    string
		release string
		edition store.Edition
		want    fitVerdict
	}{
		{"its own name", "NAUSICAA Fullmetal Edition v01 [English]", fullmetal, fitYes},
		{"without the word edition", "NAUSICAA (Fullmetal) v01 [English]", fullmetal, fitYes},
		{"a plain volume might be it", "NAUSICAA v01 [English]", fullmetal, fitUnsure},
		{"and it is not a plain volume", "NAUSICAA Fullmetal Edition v01 [English]", standard, fitNo},
		{"nor is another regrouping of the series", "NAUSICAA 3-in-1 v01 [English]", standard, fitNo},
		{"a plain volume is still the standard one", "NAUSICAA v01 [English]", standard, fitYes},
	} {
		if got, why := series.judge(parsed(tc.release), tc.edition, 0); got != tc.want {
			t.Errorf("%s: %v (%s), want %v", tc.name, got, why, tc.want)
		}
	}
	if v, _ := series.judge(parsed("NAUSICAA v01"), fullmetal, fullmetal.ID); v != fitYes {
		t.Error("an established edition need not be named by the release")
	}
}

// The rules on real release names (Nyaa, English-translated) against the
// series' MangaBaka editions, as surveyed for edition-rules.md.
func TestEditionRulesOnRealNames(t *testing.T) {
	id := int64(0)
	ed := func(name string, last int64) store.Edition {
		id++
		return store.Edition{ID: id, Type: "volume", Language: "en", EditionName: name, ProviderEditionName: name, ProviderEditionID: "x", LastNumber: last * 100}
	}
	vinlandStandard, vinlandDeluxe := ed("Standard Edition", 15), ed("Deluxe Edition", 8)
	vinland := newShelf([]store.Edition{vinlandStandard, vinlandDeluxe})
	dbStandard, dbThreeInOne, dbVizbig, dbColor := ed("Standard Edition", 26), ed("3-in-1 Edition", 14), ed("VIZBIG Edition", 9), ed("Full Color Edition", 0)
	dragonBall := newShelf([]store.Edition{dbStandard, dbThreeInOne, dbVizbig, dbColor})
	bleachStandard := ed("Standard Edition", 74)
	bleach := newShelf([]store.Edition{bleachStandard, ed("3-in-1 Edition", 25)})
	boysStandard, boysPerfect := ed("Standard Edition", 22), ed("Perfect Edition", 11)
	boys := newShelf([]store.Edition{boysStandard, boysPerfect})
	moonStandard := ed("Standard Edition", 12)
	moon := newShelf([]store.Edition{moonStandard, ed("Eternal Edition", 10), ed("Collection Edition", 12)})
	fmaStandard := ed("Standard Edition", 27)
	fma := newShelf([]store.Edition{fmaStandard, ed("3-in-1 Edition", 9), ed("Fullmetal Edition", 18)})
	nausicaaBox, nausicaaStandard := ed("Box Set Edition", 0), ed("Standard Edition", 7)
	nausicaa := newShelf([]store.Edition{nausicaaBox, nausicaaStandard})
	akiraStandard := ed("Standard Edition", 6)
	akira := newShelf([]store.Edition{akiraStandard})
	for _, tc := range []struct {
		series, release string
		shelf           *shelf
		edition         store.Edition
		want            fitVerdict
	}{
		{"Vinland Saga", "Vinland Saga v14 (2025) (Omnibus Edition) (Digital) (Rillant)", vinland, vinlandStandard, fitUnsure},
		{"Vinland Saga", "Vinland Saga v14 (2025) (Omnibus Edition) (Digital) (Rillant)", vinland, vinlandDeluxe, fitNo},
		{"Dragon Ball", "[Tuxedo Time] Dragon Ball V13 - SoM Kanzenban English", dragonBall, dbStandard, fitUnsure},
		{"Dragon Ball", "[Tuxedo Time] Dragon Ball V13 - SoM Kanzenban English", dragonBall, dbVizbig, fitNo},
		{"Dragon Ball", "Dragon Ball v01-09 (VIZBIG Edition) (Digital)", dragonBall, dbVizbig, fitYes},
		{"Dragon Ball", "Dragon Ball v01-09 (VIZBIG Edition) (Digital)", dragonBall, dbStandard, fitNo},
		{"Dragon Ball", "[Tracksuit] Dragon Ball - Full Color - Vols. 01 - 16", dragonBall, dbColor, fitYes},
		{"Dragon Ball", "[Tracksuit] Dragon Ball - Full Color - Vols. 01 - 16", dragonBall, dbStandard, fitNo},
		{"Bleach", "Bleach - Digital Colored Comics v01-74 (2021) (Colored Council)", bleach, bleachStandard, fitYes},
		// A fan colorization of the original volumes is a variant of the
		// Standard Edition, even where an official color edition exists.
		{"Dragon Ball", "Dragon Ball v13 (Colored) (Fan Colorization)", dragonBall, dbStandard, fitYes},
		{"Dragon Ball", "Dragon Ball v13 (Colored) (Fan Colorization)", dragonBall, dbColor, fitUnsure},
		{"20th Century Boys", "20th Century Boys v01-02 (2018) (The Perfect Edition) (Scan-HQ)", boys, boysStandard, fitNo},
		{"20th Century Boys", "20th Century Boys v01-02 (2018) (The Perfect Edition) (Scan-HQ)", boys, boysPerfect, fitYes},
		{"Sailor Moon", "Sailor Moon Eternal Edition v01-v04 (Digital) (BlackManta-Empire)", moon, moonStandard, fitNo},
		{"Fullmetal Alchemist", "Fullmetal Alchemist (Fullmetal Edition) Volume 01", fma, fmaStandard, fitNo},
		{"Nausicaa", "Nausicaa / Nausicaä of the Valley of the Wind - Deluxe Edition (2012) (c2c) (Trite)", nausicaa, nausicaaBox, fitNo},
	} {
		p := parse.ParseAnchored(tc.release, parse.ChannelTorrent, []string{tc.series, "Nausicaä of the Valley of the Wind"})
		if got, why := tc.shelf.judge(p, tc.edition, 0); got != tc.want {
			t.Errorf("%s in %s: %v (%s), want %v", tc.release, tc.edition.EditionName, got, why, tc.want)
		}
	}
	// What the search offers to add, in the release's own words and numbers.
	for _, tc := range []struct {
		series, release string
		shelf           *shelf
		edition         store.Edition
		want            OtherEdition
	}{
		{"Nausicaa", "Nausicaa / Nausicaä of the Valley of the Wind - Deluxe Edition (2012) (c2c) (Trite)", nausicaa, nausicaaBox, OtherEdition{Name: "Deluxe Edition", Language: "en"}},
		{"Dragon Ball", "Dragon Ball v01-34 (Kanzenban) (Scan)", dragonBall, dbStandard, OtherEdition{Name: "Kanzenban Edition", Language: "en", Volumes: 34}},
		{"Vinland Saga", "Vinland Saga v01-13 (2013-2022) (Omnibus Edition) (Digital) (danke-Empire)", vinland, vinlandStandard, OtherEdition{Name: "Omnibus Edition", Language: "en", Volumes: 13}},
		{"Akira", "Akira (35th Anniversary Edition) (2017) (c2c) (Trite)", akira, akiraStandard, OtherEdition{Name: "35th Anniversary Edition", Language: "en"}},
		{"20th Century Boys", "20th Century Boys v01-02 (2018) (The Perfect Edition) (Scan-HQ)", boys, boysStandard, OtherEdition{Name: "Perfect Edition", Language: "en", Volumes: 2, InLibrary: true}},
	} {
		p := parse.ParseAnchored(tc.release, parse.ChannelTorrent, []string{tc.series, "Nausicaä of the Valley of the Wind"})
		if got := tc.shelf.offer(p, tc.edition); got == nil || *got != tc.want {
			t.Errorf("%s: offers %+v, want %+v", tc.release, got, tc.want)
		}
	}
}

// A catalog box set is one item; the books inside become volumes that
// carry its ISBN and date, and the box itself stops being wanted.
func TestSplitBox(t *testing.T) {
	lib, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	cols, _ := st.ListEditions(ctx, id)
	omni := collectionByName(t, cols, "Omnibus Edition", "en")
	works, _ := st.ListBooks(ctx, omni.ID)
	if len(works) != 1 {
		t.Fatalf("fixture: %d works", len(works))
	}
	box := works[0]
	box.ISBN, box.ReleaseDate = "9781421550640", "2012-11-06"
	if _, err := st.SaveBook(ctx, id, box); err != nil {
		t.Fatal(err)
	}
	// Collect the edition so "wanted" means something.
	versions, _ := st.ListVersions(ctx, id)
	var version store.CollectedVersion
	for _, tg := range versions {
		if tg.EditionID == omni.ID {
			version = tg
		}
	}
	version.Monitored = true
	if _, err := st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	// The fixture's box is itself numbered 1: the split must not mistake it
	// for the first volume.
	volumes, err := lib.SplitBox(ctx, id, box.ID, 2)
	if err != nil || len(volumes) != 2 {
		t.Fatalf("split: %+v %v", volumes, err)
	}
	for i, v := range volumes {
		if v.ID == box.ID || v.SequenceNum != int64(i+1)*100 || v.ISBN != box.ISBN || v.ReleaseDate != box.ReleaseDate || v.CountType != "main" {
			t.Errorf("volume %d = %+v", i+1, v)
		}
	}
	// Again, with one more: only the new volume is added.
	if volumes, err = lib.SplitBox(ctx, id, box.ID, 3); err != nil || len(volumes) != 3 {
		t.Fatalf("second split: %+v %v", volumes, err)
	}
	if all, _ := st.ListBooks(ctx, omni.ID); len(all) != 4 {
		t.Fatalf("edition has %d books, want the box and 3 volumes", len(all))
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	wanted := 0
	for _, sl := range slots[version.ID] {
		if sl.Book.ID == box.ID && (sl.Monitored || sl.Wanted) {
			t.Error("the box itself is still collected")
		}
		if sl.Wanted {
			wanted++
		}
	}
	if wanted != 3 {
		t.Errorf("%d wanted books, want the 3 volumes", wanted)
	}
	if again, _ := st.GetBook(ctx, box.ID); again.CountType != "other" {
		t.Errorf("the box is still counted as a %q book", again.CountType)
	}
	if _, err := lib.SplitBox(ctx, id, box.ID, 1); err == nil {
		t.Error("a box of one volume was accepted")
	}
}

// Volumes and chapters are collected independently: a name is one kind.
func TestLedgerOf(t *testing.T) {
	titles := []string{"HUNTER x HUNTER"}
	for name, want := range map[string]string{
		"Hunter x Hunter v01 (2005) (Digital) (LuCaZ)":                           LedgerVolume,
		"Hunter x Hunter c401":                                                   LedgerChapter,
		"Hunter x Hunter 391-400 (TCB)":                                          "", // bare numbers: resolved elsewhere
		"Hunter x Hunter Vol.5 Ch.34":                                            LedgerChapter,
		"Hunter x Hunter v01 c001-008":                                           LedgerVolume,
		"[VIZ Media] Hunter x Hunter Volumes 01-38 ¦ Chapters 001-410 [Digital]": LedgerVolume,
		"Hunter x Hunter Chapters 12-14":                                         LedgerChapter,
	} {
		p := parse.ParseAnchored(name, parse.ChannelTorrent, titles)
		got := ledgerOf(p)
		if want == "" {
			continue
		}
		if got != want {
			t.Errorf("%q is %q, want %q (volumes %v chapters %v)", name, got, want, p.Volumes, p.Chapters)
		}
		other := LedgerChapter
		if want == LedgerChapter {
			other = LedgerVolume
		}
		if len(bookRanges(p, want)) == 0 || len(bookRanges(p, other)) != 0 {
			t.Errorf("%q should offer %s numbers only", name, want)
		}
	}
}

// A release ruled out as another edition says which edition it is, and
// whether the series has one like it: the search offers to add it if not.
func TestRuledOutReleaseNamesItsEdition(t *testing.T) {
	_, st, sid, _, _, version := modelFixture(t)
	ctx := context.Background()
	svc := NewSearchService(st, provider.NewRegistry())
	run := &searchRun{seriesID: sid, ledger: LedgerVolume, scope: &SearchScope{VersionID: version.ID, All: true}, titles: []string{"ONE PIECE"}}
	if err := svc.prepareVersions(ctx, run); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		title string
		want  *OtherEdition
	}{
		{"ONE PIECE - Deluxe Edition v01 [English]", &OtherEdition{Name: "Deluxe Edition", Language: "en", Volumes: 1}},
		// The series lists an omnibus edition: collecting it is the answer.
		{"ONE PIECE Omnibus v01 [English]", &OtherEdition{Name: "Omnibus Edition", Language: "en", Volumes: 1, InLibrary: true}},
		{"ONE PIECE v01 [English]", nil},
	} {
		got, _ := svc.process(run, []provider.Candidate{{Provider: "torrent", Protocol: "torrent", SiteID: "site", Title: tc.title, Torrent: &provider.TorrentRef{GUID: tc.title, Size: 100 << 20, Seeders: 10}}})
		if len(got) != 1 {
			t.Fatalf("%s: %d candidates", tc.title, len(got))
		}
		if o := got[0].OtherEdition; (o == nil) != (tc.want == nil) || o != nil && *o != *tc.want {
			t.Errorf("%s: other edition %+v, want %+v", tc.title, o, tc.want)
		}
	}
}

// An edition's book count says what it is when its name does not: a
// completed series' English edition in 7 books for 13 regroups it, so a
// release calling itself the omnibus fits; an ongoing series' edition is
// only behind, and named siblings keep a bare "Omnibus" a question.
func TestAnEditionsShapeNamesItsNumbering(t *testing.T) {
	std := func(id, books int64) store.Edition {
		return store.Edition{ID: id, Type: "volume", Language: "en", EditionName: "Standard Edition", ProviderEditionName: "Standard Edition", LastNumber: books * 100}
	}
	named := func(id, books int64, name string) store.Edition {
		e := std(id, books)
		e.EditionName, e.ProviderEditionName = name, name
		return e
	}
	parsed := func(name, title string) parse.Result {
		return parse.ParseAnchored(name, parse.ChannelFile, []string{title})
	}
	punpun := std(1, 7)
	kingdom := std(2, 15)
	db, db3 := std(3, 26), named(4, 14, "3-in-1 Edition")
	for _, tc := range []struct {
		name    string
		shelf   *shelf
		release parse.Result
		edition store.Edition
		want    fitVerdict
	}{
		{"Punpun's English edition is the omnibus", seriesShelf([]store.Edition{punpun}, &store.Series{Status: "completed", FinalVolume: 13}),
			parsed("Goodnight Punpun v01 (2016) (Omnibus Edition) (Digital) (1r0n).cbz", "Goodnight Punpun"), punpun, fitYes},
		{"an ongoing series' edition is only behind", seriesShelf([]store.Edition{kingdom}, &store.Series{Status: "releasing", FinalVolume: 75}),
			parsed("Kingdom v01 (Omnibus Edition).cbz", "Kingdom"), kingdom, fitUnsure},
		{"named siblings keep a bare omnibus a question", seriesShelf([]store.Edition{db, db3}, &store.Series{Status: "completed", FinalVolume: 42}),
			parsed("Dragon Ball v01 (Omnibus).cbz", "Dragon Ball"), db, fitUnsure},
		{"a sibling's own name still wins", seriesShelf([]store.Edition{db, db3}, &store.Series{Status: "completed", FinalVolume: 42}),
			parsed("Dragon Ball 3-in-1 v01.cbz", "Dragon Ball"), db, fitNo},
	} {
		if got, why := tc.shelf.judge(tc.release, tc.edition, 0); got != tc.want {
			t.Errorf("%s: %v (%s), want %v", tc.name, got, why, tc.want)
		}
	}
}
