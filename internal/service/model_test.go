package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func modelFixture(t *testing.T) (*Library, *store.Store, int64, store.Edition, []store.Book, store.CollectedVersion) {
	t.Helper()
	lib, st := setup(t)
	ctx := context.Background()
	id, err := lib.AddSeries(ctx, AddSeriesRequest{MangaBakaID: 377})
	if err != nil {
		t.Fatal(err)
	}
	cs, err := st.ListEditions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	c := collectionByName(t, cs, "Standard Edition", "en")
	ws, err := st.ListBooks(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	ts, err := st.ListVersions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range ts {
		if version.EditionID == c.ID && version.IsDefault {
			return lib, st, id, c, ws, version
		}
	}
	t.Fatal("missing default target")
	return nil, nil, 0, store.Edition{}, nil, store.CollectedVersion{}
}
func ownedModelFile(t *testing.T, st *store.Store, id int64, c store.Edition, w store.Book, version store.CollectedVersion) int64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ONE PIECE v01.cbz")
	if err := os.WriteFile(path, []byte("test archive"), 0600); err != nil {
		t.Fatal(err)
	}
	fid, err := st.AddFile(context.Background(), store.File{SeriesID: id, Path: path, Size: 12}, nil, []store.BookLink{{Ledger: c.Type, EditionID: c.ID, Number: w.SequenceNum, VersionID: version.ID, BookID: w.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return fid
}
func modelRef(c store.Edition, w store.Book, version store.CollectedVersion) store.BookRef {
	return store.BookRef{Ledger: c.Type, EditionID: c.ID, Number: w.SequenceNum, VersionID: version.ID, BookID: w.ID}
}

func TestModelRefreshKeepsIdentityAndCoverage(t *testing.T) {
	lib, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	p := testProvider()
	p.collections[0].EditionName = "Publisher renamed this"
	p.works["uuid-standard"][0].SequenceNum = 150
	p.works["uuid-standard"][0].SequenceStr = "1.5"
	p.works["uuid-standard"] = append(p.works["uuid-standard"], metadata.Work{ProviderID: "same-number-new-isbn", SequenceNum: 150, SequenceStr: "1.5", CountType: "main", ISBN: "another", ReleaseDate: "2001-01-01"})
	if err := st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	w, err := st.GetBook(ctx, ws[0].ID)
	if err != nil || w.SequenceNum != 150 {
		t.Fatalf("provider numbering correction lost identity: %+v %v", w, err)
	}
	current, err := st.GetEdition(ctx, c.ID)
	if err != nil || current.EditionName != "Publisher renamed this" {
		t.Fatalf("rename lost edition ID: %+v %v", current, err)
	}
	cv, err := st.Coverage(ctx, id)
	if err != nil || len(cv) != 1 || cv[0].BookID != ws[0].ID || cv[0].FileID != fid {
		t.Fatalf("coverage changed: %+v %v", cv, err)
	}
	units, err := st.ListBooks(ctx, c.ID)
	if err != nil || len(units) != 4 {
		t.Fatalf("distinct provider works collapsed: %+v %v", units, err)
	}
	slots, err := lib.BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	owned := 0
	for _, s := range slots[version.ID] {
		if s.State == "owned" {
			owned++
			if s.Book.ID != ws[0].ID {
				t.Fatal("ownership leaked to same-number revision")
			}
		}
	}
	if owned != 1 {
		t.Fatal(slots)
	}
}

func TestModelLocalCatalogLinkIsExplicitAndPreservesEdits(t *testing.T) {
	_, st, id, c, _, _ := modelFixture(t)
	ctx := context.Background()
	local, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: id, EditionName: c.EditionName, Language: c.Language, Publisher: c.Publisher, Medium: c.Medium, Monitored: true}, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	p := testProvider()
	if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	lc, _ := st.GetEdition(ctx, local)
	if !lc.Custom || len(lc.Collections) != 0 {
		t.Fatal("refresh silently adopted local edition")
	}
	label := "My local publication"
	if err = st.EditEdition(ctx, id, local, store.EditionMetadataPatch{EditionName: &label}); err != nil {
		t.Fatal(err)
	}
	before, _ := st.ListBooks(ctx, local)
	preview, err := st.PreviewCatalogLink(ctx, id, local, c.ID)
	if err != nil || len(preview.Mapping) != 3 {
		t.Fatalf("bad link preview: %+v %v", preview, err)
	}
	if err = st.LinkCatalog(ctx, id, local, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	lc, _ = st.GetEdition(ctx, local)
	after, _ := st.ListBooks(ctx, local)
	if lc.EditionName != label || len(lc.Collections) != 1 || lc.Collections[0].ProviderID != "uuid-standard" || after[0].ID != before[0].ID {
		t.Fatalf("link changed local identity: %+v %+v", lc, after)
	}
	if lc.Custom || lc.SyncState != "linked" {
		t.Fatalf("linked edition still counts as local: %+v", lc)
	}
	if _, err = st.GetEdition(ctx, c.ID); err == nil {
		t.Fatal("unused catalog row left behind")
	}
	editions, _ := st.ListEditions(ctx, id)
	for _, e := range editions {
		if e.ID != local && len(e.Collections) > 0 && e.Collections[0].ProviderID == "uuid-standard" {
			t.Fatalf("refresh recreated the catalog edition: %+v", e)
		}
	}
	// A linked edition is a catalog edition: it goes stale with its entry.
	if err = st.ReconcileMetadata(ctx, id, p.series, nil, nil); err != nil {
		t.Fatal(err)
	}
	if lc, _ = st.GetEdition(ctx, local); lc.SyncState != "stale" {
		t.Fatalf("linked edition did not go stale: %s", lc.SyncState)
	}
}

func TestModelCatalogLinkIsSuggestedAndCompletesTheEdition(t *testing.T) {
	_, st, id, c, _, _ := modelFixture(t)
	ctx := context.Background()
	local, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: id, EditionName: "Ma collection", Language: c.Language, Monitored: true}, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	books, _ := st.ListBooks(ctx, local)
	if err = st.DeleteBook(ctx, id, books[2].ID); err != nil {
		t.Fatal(err)
	}
	suggested := func() []store.LinkSuggestion {
		t.Helper()
		out, err := st.LinkSuggestions(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	got := suggested()
	if len(got) != 1 || got[0].LocalID != local || got[0].CatalogID != c.ID || got[0].Matched != 2 {
		t.Fatalf("suggestions: %+v", got)
	}
	review, err := st.ReviewItems(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range review {
		found = found || (item.Kind == "edition" && item.ItemID == local && item.RelatedID == c.ID)
	}
	if !found {
		t.Fatalf("suggestion missing from review: %+v", review)
	}
	if err = st.DeclineCatalogLink(ctx, id, local, c.ID); err != nil {
		t.Fatal(err)
	}
	// The next-best catalog edition may be offered instead; this one is not.
	for _, again := range suggested() {
		if again.CatalogID == c.ID {
			t.Fatalf("declined link suggested again: %+v", again)
		}
	}

	// Declining only silences the prompt; the link itself stays available.
	if err = st.LinkCatalog(ctx, id, local, c.ID); err != nil {
		t.Fatal(err)
	}
	linked, _ := st.GetEdition(ctx, local)
	after, _ := st.ListBooks(ctx, local)
	if linked.EditionName != c.EditionName || linked.Publisher != c.Publisher || linked.Medium != c.Medium {
		t.Fatalf("catalog descriptors not adopted: %+v", linked)
	}
	if len(after) != 3 || after[0].ID != books[0].ID {
		t.Fatalf("catalog-only book missing or local ids changed: %+v", after)
	}
	for _, b := range after {
		if b.ProviderID == "" {
			t.Fatalf("book left without a catalog reference: %+v", b)
		}
	}
	// The next refresh finds everything in place and adds nothing.
	p := testProvider()
	if err = st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	if again, _ := st.ListBooks(ctx, local); len(again) != 3 {
		t.Fatalf("refresh duplicated books: %+v", again)
	}
	if got = suggested(); len(got) != 0 {
		t.Fatalf("linked edition still suggested: %+v", got)
	}
}

func TestModelTargetsReserveOnceAndFreezeCombinedPlan(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	other := version
	other.ID = 0
	other.Name = "Another group"
	other.RequiredGroup = "Other"
	oid, err := st.SaveVersion(ctx, id, other)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = oid
	q := store.QueueItem{SeriesID: id, Protocol: "torrent", ProviderID: "torrent", ReleaseKey: "same-offer", Title: "ONE PIECE v01", Snapshot: store.JSONText(`{"title":"original"}`)}
	first, created, err := st.ReserveQueue(ctx, q, []store.BookRef{modelRef(c, ws[0], version)})
	if err != nil || !created {
		t.Fatalf("first reservation: %v %v", created, err)
	}
	second, created, err := st.ReserveQueue(ctx, q, []store.BookRef{modelRef(c, ws[0], other)})
	if err != nil || created || second != first {
		t.Fatalf("same offer did not share job: %d %v %v", second, created, err)
	}
	frozen, err := st.BeginImport(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	var plan []store.BookRef
	if json.Unmarshal(frozen.Books, &plan) != nil || len(plan) != 2 {
		t.Fatalf("combined plan lost: %s", frozen.Books)
	}
	if _, _, err = st.ReserveQueue(ctx, q, []store.BookRef{modelRef(c, ws[1], version)}); err == nil {
		t.Fatal("importing plan must not change")
	}
	otherOffer := q
	otherOffer.ReleaseKey = "another-offer"
	if _, _, err = st.ReserveQueue(ctx, otherOffer, []store.BookRef{modelRef(c, ws[0], version)}); err == nil {
		t.Fatal("double reservation allowed")
	}
	if _, err = st.SaveBook(ctx, id, ws[0]); err == nil {
		t.Fatal("changed unit while download pending")
	}
	if err = st.UpdateQueueItem(ctx, first, store.QueueFailed, "test failure"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = st.ReserveQueue(ctx, otherOffer, []store.BookRef{modelRef(c, ws[0], version)}); err != nil {
		t.Fatal("failed job did not release claims:", err)
	}
}

func TestModelFractionalZeroExtrasAndUnknownDates(t *testing.T) {
	_, st, id, c, _, version := modelFixture(t)
	ctx := context.Background()
	for _, w := range []store.Book{{SequenceStr: "0", SequenceNum: 0, NumberKnown: true, CountType: "main"}, {SequenceStr: "1.5", SequenceNum: 150, NumberKnown: true, CountType: "main"}, {SequenceStr: "Special", SequenceNum: 100, NumberKnown: true, CountType: "extra"}, {SequenceStr: "Artbook", NumberKnown: false, CountType: "other"}} {
		w.EditionID = c.ID
		if _, err := st.SaveBook(ctx, id, w); err != nil {
			t.Fatal(err)
		}
	}
	ws, _ := st.ListBooks(ctx, c.ID)
	extra := store.Book{}
	for _, w := range ws {
		if w.SequenceStr == "Special" {
			extra = w
		}
	}
	if err := st.SetBookMonitored(ctx, id, version.ID, extra.ID, true); err != nil {
		t.Fatal(err)
	}
	version.AcquireUnknown = false
	if _, err := st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	slots, err := NewLibrary(st, nil).BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]BookStatus{}
	for _, s := range slots[version.ID] {
		seen[s.Book.SequenceStr] = s
	}
	if !seen["0"].Monitored || seen["0"].Wanted || seen["0"].State != "unknown" || !seen["1.5"].Book.NumberKnown || !seen["Special"].Monitored || seen["Artbook"].Book.NumberKnown {
		t.Fatalf("unit semantics: %+v", seen)
	}
}

func TestModelCoverageReviewPinAndVersionDeletion(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	other := version
	other.ID = 0
	other.Name = "Retained"
	tid, err := st.SaveVersion(ctx, id, other)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = tid
	if err = st.MapFile(ctx, id, fid, []store.BookRef{modelRef(c, ws[0], version), modelRef(c, ws[1], other)}); err != nil {
		t.Fatal(err)
	}
	if err = st.PinCoverage(ctx, id, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.DeleteFile(ctx, fid); err == nil {
		t.Fatal("pinned copy deleted")
	}
	if err = st.DeleteVersion(ctx, id, other.ID); err != nil {
		t.Fatal(err)
	}
	cv, _ := st.Coverage(ctx, id)
	fs, _ := st.ListFiles(ctx, id)
	if len(cv) != 1 || len(fs) != 1 {
		t.Fatalf("target deletion lost shared file: %+v %+v", cv, fs)
	}
	if err = st.EnsureModels(ctx, id); err != nil {
		t.Fatal(err)
	}
	cv, _ = st.Coverage(ctx, id)
	if len(cv) != 1 {
		t.Fatal("replaced coverage was unexpectedly restored")
	}
}

func TestModelObservationReviewIsBoundedAndIdempotent(t *testing.T) {
	_, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	cs, _ := st.ListEditions(ctx, id)
	chapter := chapterCollection(t, cs)
	before, _ := st.ListBooks(ctx, chapter.ID)
	o := store.BookObservation{EditionID: chapter.ID, Source: "site", ExternalKey: "huge-number", Label: "99999.5", Number: 9999950}
	for i := 0; i < 2; i++ {
		if err := st.ObserveBook(ctx, id, o); err != nil {
			t.Fatal(err)
		}
	}
	same, _ := st.ListBooks(ctx, chapter.ID)
	if len(same) != len(before) {
		t.Fatal("observation expanded library before review")
	}
	obs, _ := st.ListObservations(ctx, id)
	if len(obs) != 1 {
		t.Fatal("duplicate observations")
	}
	if err := st.ResolveObservation(ctx, id, obs[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := st.ResolveObservation(ctx, id, obs[0].ID, true); err == nil {
		t.Fatal("accepted observation twice")
	}
	after, _ := st.ListBooks(ctx, chapter.ID)
	if len(after) != len(before)+1 {
		t.Fatal("acceptance expanded gaps instead of adding one unit")
	}
}

func TestModelQualifyingAssetReusedAcrossVersions(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	other := version
	other.ID = 0
	other.Name = "Second"
	tid, err := st.SaveVersion(ctx, id, other)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = tid
	ownedModelFile(t, st, id, c, ws[0], version)
	slots, err := NewLibrary(st, nil).BooksByVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if slots[version.ID][0].State != "owned" || slots[other.ID][0].State != "owned" {
		t.Fatal("qualifying asset was not reused")
	}
	version.Name = "Renamed default"
	if _, err = st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	if err = st.EnsureModels(ctx, id); err != nil {
		t.Fatal("renaming default broke model:", err)
	}
}

func TestModelAtomicInstallCannotClobber(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "incoming")
	dst := filepath.Join(dir, "existing")
	if err := os.WriteFile(src, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, _ := fileHash(src)
	for _, copy := range []bool{false, true} {
		if err := installAsset(src, dst, hash, copy); err == nil {
			t.Fatal("different bytes accepted at occupied destination")
		}
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "retained" {
		t.Fatal("retained asset overwritten")
	}
	// A free path is used as named; one held by a different book gets the
	// fingerprint instead of being replaced; the setting forces it always.
	free, err := placeAsset(dir, "book.cbz", hash, false)
	if err != nil || filepath.Base(free) != "book.cbz" {
		t.Fatalf("free path = %q, %v", free, err)
	}
	taken, err := placeAsset(dir, filepath.Base(dst), hash, false)
	if err != nil || taken == dst || !strings.Contains(filepath.Base(taken), "["+hash+"]") {
		t.Fatalf("occupied path = %q, %v", taken, err)
	}
	if always, _ := placeAsset(dir, "book.cbz", hash, true); !strings.Contains(always, "["+hash+"]") {
		t.Fatalf("fingerprint missing from %q", always)
	}
}

func TestModelUpgradeReevaluatesFactsAndRespectsPins(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	f := profile.Format{Name: "Preferred group", Conditions: []profile.Condition{{Type: profile.CondGroup, Value: "LuCaZ", Required: true}}}
	formatID, err := st.AddFormat(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	p := profile.Profile{Name: "Upgrade test", UpgradesEnabled: true, UpgradeUntilScore: 100, FormatScores: map[int64]int{formatID: 100}}
	pid, err := st.AddProfile(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	version.ProfileID = pid
	if _, err = st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	if err = st.SetAssetFacts(ctx, fid, store.AssetFacts{Title: "ONE PIECE v01", Group: "LuCaZ", FactsKnown: true, FormatScore: -100, Protocol: "torrent"}, ""); err != nil {
		t.Fatal(err)
	}
	svc := NewSearchService(st, torrentRegistry(st))
	run, err := svc.prepare(ctx, id, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.wantedBooks[version.ID][ws[0].ID] {
		t.Fatal("stale stored score used instead of current profile")
	}
	p.FormatScores[formatID] = 50
	p.ID = pid
	if err = st.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	run, err = svc.prepare(ctx, id, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !run.wantedBooks[version.ID][ws[0].ID] {
		t.Fatal("eligible upgrade not searched")
	}
	if err = st.PinCoverage(ctx, id, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	run, err = svc.prepare(ctx, id, LedgerVolume, nil)
	if err != nil {
		t.Fatal(err)
	}
	if run.wantedBooks[version.ID][ws[0].ID] {
		t.Fatal("pinned copy selected for upgrade")
	}
}

func TestModelPackStaysOneAssetAcrossUnitsAndVersions(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	other := version
	other.ID = 0
	other.Name = "Second version"
	tid, err := st.SaveVersion(ctx, id, other)
	if err != nil {
		t.Fatal(err)
	}
	other.ID = tid
	slots := []store.BookRef{modelRef(c, ws[0], version), modelRef(c, ws[1], version), modelRef(c, ws[0], other)}
	raw, _ := json.Marshal(slots)
	dir := t.TempDir()
	name := "ONE PIECE v01-v02 (Digital).cbz"
	writeCBZ(t, filepath.Join(dir, name), "one unsplit pack")
	im := NewImporter(st, nil, t.TempDir(), nil)
	if err = im.importFiles(ctx, store.QueueItem{SeriesID: id, Protocol: "torrent", Books: raw}, dir, []string{name}); err != nil {
		t.Fatal(err)
	}
	fs, err := st.ListFiles(ctx, id)
	if err != nil || len(fs) != 1 {
		t.Fatalf("pack became multiple assets: %+v %v", fs, err)
	}
	cv, err := st.Coverage(ctx, id)
	if err != nil || len(cv) != 4 {
		t.Fatalf("pack coverage lost: %+v %v", cv, err)
	}
	// Still one archive with the same pages; tagged at series level only,
	// since no single number describes two volumes.
	if want, _ := fileHash(filepath.Join(dir, name)); fs[0].Hash != want {
		t.Fatal("pack pages changed")
	}
	if _, info := readBook(t, fs[0].Path); info == nil || info.Series != "ONE PIECE" || info.Number != "" || info.Volume != "" {
		t.Fatalf("pack ComicInfo = %+v", info)
	}
}

func TestModelDuplicateMergePreservesTargetPoliciesAndCoverage(t *testing.T) {
	_, st, id, c, _, _ := modelFixture(t)
	ctx := context.Background()
	duplicate, err := st.AddCustomEdition(ctx, store.Edition{SeriesID: id, EditionName: c.EditionName, Language: c.Language, Publisher: c.Publisher, Medium: c.Medium, Monitored: true}, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.EnsureModels(ctx, id); err != nil {
		t.Fatal(err)
	}
	local, _ := st.GetEdition(ctx, duplicate)
	ws, _ := st.ListBooks(ctx, duplicate)
	ts, _ := st.ListVersions(ctx, id)
	var version store.CollectedVersion
	for _, v := range ts {
		if v.EditionID == duplicate && v.IsDefault {
			version = v
		}
	}
	version.RequiredGroup = "LuCaZ"
	if _, err = st.SaveVersion(ctx, id, version); err != nil {
		t.Fatal(err)
	}
	fid := ownedModelFile(t, st, id, *local, ws[0], version)
	if err = st.PinCoverage(ctx, id, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = st.MergeEditions(ctx, id, duplicate, c.ID); err != nil {
		t.Fatal(err)
	}
	retained, err := st.GetVersion(ctx, version.ID)
	if err != nil || retained.EditionID != c.ID || retained.RequiredGroup != "LuCaZ" || retained.IsDefault {
		t.Fatalf("merge lost version policy: %+v %v", retained, err)
	}
	cv, err := st.Coverage(ctx, id)
	pinned := false
	for _, a := range cv {
		if a.FileID == fid && a.VersionID == version.ID && a.Pinned {
			pinned = true
		}
	}
	if err != nil || len(cv) != 2 || !pinned {
		t.Fatalf("merge lost coverage or pin: %+v %v", cv, err)
	}
	w, _ := st.GetBook(ctx, cv[0].BookID)
	if w.EditionID != c.ID {
		t.Fatal("ownership points to removed edition")
	}
}

func TestModelLayoutSurvivesDisplayRenameAndRequiresEvidence(t *testing.T) {
	_, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	cs, _ := st.ListEditions(ctx, id)
	omni := collectionByName(t, cs, "Omnibus Edition", "en")
	label := "My favorite books"
	if err := st.EditEdition(ctx, id, omni.ID, store.EditionMetadataPatch{EditionName: &label}); err != nil {
		t.Fatal(err)
	}
	renamed, _ := st.GetEdition(ctx, omni.ID)
	if renamed.ProviderEditionName != "Omnibus Edition" {
		t.Fatal("the catalog's name was lost when the display name changed")
	}
	plain := parse.ParseAnchored("ONE PIECE v01 (Digital)", parse.ChannelTorrent, []string{"ONE PIECE"})
	sh := newShelf(cs)
	if sh.fits(plain, *renamed) {
		t.Fatal("plain standard release satisfied renamed omnibus")
	}
	if v, _ := sh.judge(plain, *renamed, omni.ID); v != fitYes {
		t.Fatal("verified listing did not establish edition")
	}
	plain.Language = "fr"
	if v, _ := sh.judge(plain, *renamed, omni.ID); v == fitYes {
		t.Fatal("listing evidence bypassed contradictory language")
	}
}

func TestModelEditingOneUnitFieldDoesNotFreezeCatalogDates(t *testing.T) {
	_, st, id, _, ws, _ := modelFixture(t)
	ctx := context.Background()
	w := ws[1]
	w.SequenceStr = "Local label"
	if _, err := st.SaveBook(ctx, id, w); err != nil {
		t.Fatal(err)
	}
	p := testProvider()
	p.works["uuid-standard"][1].ReleaseDate = "2020-02-01"
	if err := st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	current, _ := st.GetBook(ctx, w.ID)
	if current.SequenceStr != "Local label" || current.ReleaseDate != "2020-02-01" {
		t.Fatalf("local edit froze unrelated provider fields: %+v", current)
	}
}

func TestModelNewChapterEditionDoesNotBorrowSeriesNumbering(t *testing.T) {
	lib, st, id, _, _, _ := modelFixture(t)
	ctx := context.Background()
	local, err := lib.CreateEdition(ctx, id, CreateEditionParams{EditionName: "French chapters", Language: "fr", Type: "chapter", Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	works, err := st.ListBooks(ctx, local.ID)
	if err != nil || len(works) != 0 {
		t.Fatalf("new edition fabricated global chapter units: %d %v", len(works), err)
	}
	source := "series_count"
	if err = st.EditEdition(ctx, id, local.ID, store.EditionMetadataPatch{NumberingSource: &source}); err != nil {
		t.Fatal(err)
	}
	if err = st.EnsureModels(ctx, id); err != nil {
		t.Fatal(err)
	}
	works, _ = st.ListBooks(ctx, local.ID)
	if len(works) != 1184 {
		t.Fatal("explicit count numbering not applied")
	}
}

func TestModelFileReviewPreservesPinsInOtherVersions(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	if err := st.PinCoverage(ctx, id, store.AssetCoverage{FileID: fid, VersionID: version.ID, BookID: ws[0].ID, Pinned: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.MapFile(ctx, id, fid, []store.BookRef{modelRef(c, ws[0], version), modelRef(c, ws[1], version)}); err != nil {
		t.Fatal(err)
	}
	cv, _ := st.Coverage(ctx, id)
	if len(cv) != 2 {
		t.Fatal(cv)
	}
	for _, a := range cv {
		if a.BookID == ws[0].ID && !a.Pinned {
			t.Fatal("review silently removed a retained pin")
		}
	}
}

func TestCatalogIdentityCorrectionRequiresCoverageReview(t *testing.T) {
	_, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	fid := ownedModelFile(t, st, id, c, ws[0], version)
	p := testProvider()
	for i := range p.collections {
		if p.collections[i].ProviderID == "uuid-standard" {
			p.collections[i].Language = "fr"
		}
	}
	if err := st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	coverage, err := st.Coverage(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range coverage {
		if a.FileID == fid && a.Verified {
			t.Fatal("language correction retained unreviewed ownership")
		}
	}
	files, err := st.ListFiles(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.ID == fid && !f.NeedsReview {
			t.Fatal("asset missing review flag")
		}
	}
}

// What an edition is follows the catalog: when MangaBaka files a
// collection under another edition, the edition here changes with it.
func TestEditionFollowsTheCatalogName(t *testing.T) {
	_, st, id, c, _, _ := modelFixture(t)
	ctx := context.Background()
	p := testProvider()
	p.collections[0].EditionName = "Colossal Edition"
	if err := st.ReconcileMetadata(ctx, id, p.series, p.collections, p.works); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEdition(ctx, c.ID)
	if err != nil || got.EditionName != "Colossal Edition" || got.ProviderEditionName != "Colossal Edition" {
		t.Fatalf("edition did not follow the catalog: %+v %v", got, err)
	}
}

func TestCalendarSeparatesVersionsAndDuplicateNumbers(t *testing.T) {
	lib, st, id, c, ws, version := modelFixture(t)
	ctx := context.Background()
	ownedModelFile(t, st, id, c, ws[0], version)
	other := store.CollectedVersion{EditionID: c.ID, Name: "Another group", Monitored: true, AcquireUnknown: true}
	if tid, err := st.SaveVersion(ctx, id, other); err != nil {
		t.Fatal(err)
	} else {
		other.ID = tid
	}
	entries, err := lib.Calendar(ctx, "1900-01-01", "2100-01-01")
	if err != nil {
		t.Fatal(err)
	}
	foundOwned, foundMissing := false, false
	for _, e := range entries {
		if e.BookID != ws[0].ID {
			continue
		}
		if e.VersionID == version.ID {
			foundOwned = e.Owned
		}
		if e.VersionID == other.ID {
			foundMissing = !e.Owned
		}
	}
	if !foundOwned || !foundMissing {
		t.Fatalf("calendar conflated versions: %+v", entries)
	}
}

func fixtureLinks(t *testing.T, st *store.Store, sid, cid int64, numbers ...int64) []store.BookLink {
	t.Helper()
	versions, err := st.ListVersions(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	works, err := st.ListBooks(context.Background(), cid)
	if err != nil {
		t.Fatal(err)
	}
	var tid int64
	for _, version := range versions {
		if version.EditionID == cid && version.IsDefault {
			tid = version.ID
		}
	}
	var links []store.BookLink
	for _, n := range numbers {
		for _, w := range works {
			if w.SequenceNum == n {
				links = append(links, store.BookLink{Ledger: "volume", EditionID: cid, Number: n, BookID: w.ID, VersionID: tid})
			}
		}
	}
	return links
}

// booksOf returns the books of an edition's default collected version, in
// the ledger's order.
func booksOf(t *testing.T, lib *Library, st *store.Store, sid, cid int64) []BookStatus {
	t.Helper()
	ctx := context.Background()
	versions, err := st.ListVersions(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	all, err := lib.BooksByVersion(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range versions {
		if version.EditionID == cid && version.IsDefault {
			return all[version.ID]
		}
	}
	t.Fatalf("edition %d has no default version", cid)
	return nil
}
