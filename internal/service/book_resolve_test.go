package service

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// An upgrade is measured against the copy on disk, not against zero: a
// release scoring the same as the installed one is not an upgrade.
func TestUpgradeComparesAgainstTheInstalledCopy(t *testing.T) {
	_, st, sid, c, ws, v := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, sid, c, ws[0], v)
	formatID, err := st.AddFormat(ctx, profile.Format{Name: "Preferred", Conditions: []profile.Condition{{Type: profile.CondGroup, Value: "LuCaZ"}}})
	if err != nil {
		t.Fatal(err)
	}
	vol, ch := profile.DefaultRules()
	pid, err := st.AddProfile(ctx, profile.Profile{
		Name: "Upgrade", UpgradesEnabled: true, UpgradeUntilScore: 100, MinUpgradeScore: 10,
		MinFormatScore: profile.MinScoreOff, FormatScores: map[int64]int{formatID: 50}, Volume: vol, Chapter: ch,
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
	svc := NewSearchService(st, torrentRegistry(st))
	run, err := svc.prepare(ctx, sid, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := run.ownedScores[v.ID][ws[0].ID]; got != 50 {
		t.Fatalf("installed copy's score = %d, want 50", got)
	}
	cs, _ := svc.process(run, []provider.Candidate{{
		Provider: "torrent", Protocol: "torrent", Title: "ONE PIECE v01 (Digital) (LuCaZ)",
		Torrent: &provider.TorrentRef{GUID: "same-quality", Size: 100 << 20, Seeders: 30},
	}})
	seen := false
	for _, rc := range cs {
		if rc.VersionID != v.ID {
			continue
		}
		seen = true
		if !slices.Contains(rc.Rejections, "rejected.no_upgrade") {
			t.Fatalf("equal-quality release accepted as an upgrade: score %d, rejections %v", rc.FormatScore, rc.Rejections)
		}
	}
	if !seen {
		t.Fatal("the release was not evaluated for the version")
	}
}

// A file name's number cannot choose between books that share it: a bonus
// numbered like a volume is not confirmed by the volume's file, and two
// revisions of one volume go to review.
func TestScanDoesNotConfirmBooksThatOnlyShareANumber(t *testing.T) {
	_, st, sid, c, ws, _ := modelFixture(t)
	ctx := context.Background()
	parsed := parse.ParseAnchored("ONE PIECE v01 (Digital).cbz", parse.ChannelFile, []string{"ONE PIECE"})
	sc := NewScanner(st, t.TempDir())

	bonus := ws[0]
	bonus.ID, bonus.ProviderID, bonus.CountType, bonus.SequenceStr = 0, "", "extra", "Bonus 1"
	if _, err := st.SaveBook(ctx, sid, bonus); err != nil {
		t.Fatal(err)
	}
	links := sc.linksFor(ctx, newShelf([]store.Edition{c}), parsed, []store.Edition{c}, nil)
	if len(links) != 1 || links[0].BookID != ws[0].ID {
		t.Fatalf("volume file should confirm only the volume, got %+v", links)
	}

	revision := ws[0]
	revision.ID, revision.ProviderID, revision.SequenceStr = 0, "", "1 (revised)"
	if _, err := st.SaveBook(ctx, sid, revision); err != nil {
		t.Fatal(err)
	}
	if links := sc.linksFor(ctx, newShelf([]store.Edition{c}), parsed, []store.Edition{c}, nil); len(links) != 0 {
		t.Fatalf("two revisions share the number; the file needs review, got %+v", links)
	}
}

type discoveryHost struct {
	fakeExtHost
	calls int
}

func (h *discoveryHost) Sources(context.Context) ([]extensions.Source, error) {
	return []extensions.Source{{ID: "reader", Name: "Reader", Lang: "en"}}, nil
}

func (h *discoveryHost) Chapters(context.Context, string, string) ([]extensions.Chapter, error) {
	h.calls++
	return []extensions.Chapter{{Key: "one", Name: "Chapter 1", Number: 1}}, nil
}

// Discovery does not wait for a wanted book: an empty chapter edition asks
// its trusted source which chapters exist, and the same search then
// offers them.
func TestEmptyChapterEditionDiscoversItsFirstBooks(t *testing.T) {
	lib, st, sid, _, _, _ := modelFixture(t)
	ctx := context.Background()
	c, err := lib.CreateEdition(ctx, sid, CreateEditionParams{EditionName: "Scanlation chapters", Language: "en", Type: "chapter", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	if err = st.EditEdition(ctx, sid, c.ID, store.EditionMetadataPatch{DiscoverChapters: &yes}); err != nil {
		t.Fatal(err)
	}
	iid, err := st.AddIndexer(ctx, store.Indexer{Name: "Reader", Protocol: store.IndexerSource, Enabled: true, UseChapters: true, Settings: []byte(`{"sourceId":"reader"}`)})
	if err != nil {
		t.Fatal(err)
	}
	bid, err := st.AddBinding(ctx, store.Binding{SeriesID: sid, SourceID: fmt.Sprint(iid), SourceMangaKey: "one-piece", State: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.SetBindingEdition(ctx, sid, bid, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.SetBindingTrust(ctx, sid, bid, true); err != nil {
		t.Fatal(err)
	}
	versions, err := st.ListVersions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	var vid int64
	for _, v := range versions {
		if v.EditionID == c.ID {
			vid = v.ID
		}
	}
	host := &discoveryHost{}
	reg := provider.NewRegistry(provider.NewSource(host, t.TempDir(), provider.DownloadConfig{}, IndexerEntries(st, store.IndexerSource)))
	result, err := NewSearchService(st, reg).FindCandidates(ctx, sid, LedgerChapter, &SearchScope{VersionID: vid, All: true})
	if err != nil {
		t.Fatal(err)
	}
	if host.calls == 0 {
		t.Fatalf("the source was never asked for its chapters: %v", result.BindingNotes)
	}
	books, err := st.ListBooks(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 1 || books[0].SequenceNum != 100 {
		t.Fatalf("chapter 1 was not added to the edition: %+v", books)
	}
	offered := false
	for _, rc := range result.Candidates {
		for _, ref := range rc.Coverage {
			offered = offered || ref.BookID == books[0].ID
		}
	}
	if !offered {
		t.Fatalf("the discovered chapter was not offered by the same search: %d candidates, notes %v", len(result.Candidates), result.BindingNotes)
	}
}
