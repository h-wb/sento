package service

import (
	"context"
	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
	"maps"
	"time"
)

// BookState is where one book stands for one card.
type BookState string

// bookState derives these states from ownership, reservation phase and policy.
const (
	// StatePending: a download or import already holds the book.
	StatePending BookState = "pending"
	// StateNeedsReview: a file holds the book but its mapping is not
	// confirmed, and no confirmed file does.
	StateNeedsReview BookState = "needs_review"
	// StateOwned: a confirmed file the card's profile accepts holds it.
	StateOwned BookState = "owned"
	// StateUpgradeEligible: owned, and the profile wants a better file.
	StateUpgradeEligible BookState = "upgrade_eligible"
	// StateStorageUnavailable: the library folder cannot be checked.
	StateStorageUnavailable BookState = "storage_unavailable"
	// StateUnmonitored: the card does not collect this book.
	StateUnmonitored BookState = "unmonitored"
	// StateNotReleased: published in the future.
	StateNotReleased BookState = "not_released"
	// StateUnknown: no publication date, and undated books are not searched.
	StateUnknown BookState = "unknown"
	// StateUnsatisfactory: a file on disk holds it, but not one this card
	// accepts; wanted.
	StateUnsatisfactory BookState = "unsatisfactory"
	// StateMissing: wanted, nothing holds it.
	StateMissing BookState = "missing"
)

type BookStatus struct {
	OnDiskFileIDs []int64 `json:"onDiskFileIds"`
	Reason        string  `json:"reason"`
	CurrentScore  int     `json:"currentScore"`

	Book         store.Book `json:"book"`
	VersionID    int64      `json:"versionId"`
	Monitored    bool       `json:"monitored"`
	Availability string     `json:"availability"`
	State        BookState  `json:"state"`
	FileIDs      []int64    `json:"fileIds"`
	Wanted       bool       `json:"wanted"`
}

// bookFacts is everything bookState weighs for one book of one card.
type bookFacts struct {
	// Collected: the card collects the book. Why says why not.
	Collected bool
	Why       string
	Owned     bool // a confirmed, accepted file holds it
	OnDisk    bool // some present file holds it
	Review    bool // a file's mapping to it awaits confirmation
	Reserved  bool // a download or import holds it
	Reviewing bool // its reservation is held by a download awaiting review
	// Availability is released, upcoming or unknown.
	Availability   string
	AcquireUnknown bool
	StorageDown    bool
	// Upgrade: the profile wants a better file than the owned one.
	Upgrade bool
	// Refused says why the file on disk does not count, when it does not.
	Refused string
}

// bookState is where a book stands, why, and whether it is wanted. The
// cases are checked in order; the first that holds decides.
func bookState(f bookFacts) (state BookState, reason string, wanted bool) {
	switch {
	// A download in flight says what is happening to the book; the file
	// awaiting review still shows in Review, and a confirmed download
	// makes the book owned either way.
	case f.Reserved && f.Reviewing && f.Owned:
		return StateOwned, "A verified file is available; the download is awaiting review", false
	case f.Reserved:
		return StatePending, "A download or import already holds this book", false
	case f.Review && !f.Owned:
		return StateNeedsReview, "Existing file mappings need review", false
	case f.Owned && f.StorageDown:
		return StateOwned, "Library storage is unavailable or has not been checked", false
	case f.Owned && !f.Collected:
		return StateOwned, f.Why, false
	case f.Owned && f.Upgrade:
		return StateUpgradeEligible, "The file on disk is below the upgrade cutoff", true
	case f.Owned:
		return StateOwned, "", false
	case f.StorageDown:
		return StateStorageUnavailable, "Library storage is unavailable or has not been checked", false
	case !f.Collected:
		return StateUnmonitored, f.Why, false
	case f.Availability == "upcoming":
		return StateNotReleased, "Publication date is in the future", false
	case f.Availability == "unknown" && !f.AcquireUnknown:
		return StateUnknown, "Publication date is unknown and searching undated books is disabled", false
	case f.OnDisk:
		reason := f.Refused
		if reason == "" {
			reason = "The file on disk is not mapped to this edition"
		}
		return StateUnsatisfactory, reason, true
	default:
		return StateMissing, "", true
	}
}

// availability is released, upcoming or unknown, as of today.
func availability(releaseDate, today string) string {
	switch {
	case releaseDate == "":
		return "unknown"
	case releaseDate > today:
		return "upcoming"
	default:
		return "released"
	}
}

// ledgerView is loaded once per series and shared by search and UI evaluation.
type ledgerView struct {
	Series   *store.Series
	Versions []store.CollectedVersion
	Editions []store.Edition
	Books    map[int64][]store.Book
	Status   map[int64][]BookStatus
	Profiles map[int64]*profile.Profile
}

// cardFiles is what a card's profile makes of the files mapped to it:
// the mappings it accepts, each file's score, and per book why a file was
// refused.
type cardFiles struct {
	accepted map[int64][]int64 // book → accepted file ids
	pinned   map[int64]bool    // book → an accepted mapping is pinned
	scores   map[int64]int     // file → score
	refused  map[int64]string  // book → why its file does not count
}

func judgeCardFiles(col store.Edition, prof *profile.Profile, formats []profile.Format, coverage []store.AssetCoverage, byFile map[int64]store.File, present map[int64]bool) cardFiles {
	out := cardFiles{accepted: map[int64][]int64{}, pinned: map[int64]bool{}, scores: map[int64]int{}, refused: map[int64]string{}}
	for _, a := range coverage {
		facts := backedFacts(byFile[a.FileID].Facts, byFile[a.FileID].Claims)
		reason := ""
		if a.Evidence != "manual" && facts.Language != "" && facts.Language != "multi" && baseLang(facts.Language) != baseLang(col.Language) {
			reason = "The file on disk is in another language"
		}
		if a.Evidence == "manual" {
			facts.Language = col.Language
			// Confirming a file as this explicit color edition also confirms color,
			// even when its bare filename ("Volume 1.cbz") cannot state it.
			if parse.ReadEdition(col.EditionName).Color {
				facts.Flags = maps.Clone(facts.Flags)
				if facts.Flags == nil {
					facts.Flags = map[string]bool{}
				}
				facts.Flags["colored"] = true
			}
		}
		judgement := assetJudgement(facts, prof, formats)
		if judgement.Broken != "" {
			reason = "The file on disk " + judgement.Broken
		}
		out.scores[a.FileID] = judgement.Score
		if prof != nil && prof.MinFormatScore > profile.MinScoreOff && facts.FactsKnown && judgement.Score < prof.MinFormatScore {
			reason = "The file on disk is below the profile’s minimum score"
		}
		if reason != "" {
			out.refused[a.BookID] = reason
			continue
		}
		// A pin holds whether or not the pinned file is confirmed yet.
		out.pinned[a.BookID] = out.pinned[a.BookID] || a.Pinned
		if a.Verified && present[a.FileID] {
			out.accepted[a.BookID] = append(out.accepted[a.BookID], a.FileID)
		}
	}
	return out
}

func loadLedger(ctx context.Context, st *store.Store, seriesID int64) (*ledgerView, error) {
	series, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	versions, err := st.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	cols, err := st.ListEditions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	works, err := st.SeriesBooks(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	overrides, err := st.SeriesBookOverrides(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	coverage, err := st.Coverage(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	contents, err := st.Contents(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	pending, err := st.PendingBookStates(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	profiles, err := NewProfiles(st).seriesProfiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	idx := ledgerIndex{
		series: series, profiles: profiles, today: time.Now().Format("2006-01-02"),
		coverage: map[int64][]store.AssetCoverage{}, overrides: map[int64]map[int64]bool{},
		files: map[int64]store.File{}, present: map[int64]bool{}, unavailable: map[string]bool{},
		contents: map[int64][]store.AssetContent{}, pending: pending,
	}
	for _, a := range coverage {
		idx.coverage[a.VersionID] = append(idx.coverage[a.VersionID], a)
	}
	for _, o := range overrides {
		if idx.overrides[o.VersionID] == nil {
			idx.overrides[o.VersionID] = map[int64]bool{}
		}
		idx.overrides[o.VersionID][o.BookID] = o.Monitored
	}
	// Storage health is per folder: with chapters in their own folder, an
	// unreachable chapter folder says nothing about the volumes, and the
	// other way round. In a shared folder one bad file blocks both.
	for _, f := range files {
		idx.files[f.ID] = f
		idx.present[f.ID] = f.Presence == "present" && !f.NeedsReview
		if f.Presence == "unavailable" || f.Presence == "unknown" {
			switch {
			case series.ChapterPath == "":
				idx.unavailable[LedgerVolume], idx.unavailable[LedgerChapter] = true, true
			case series.InChapterFolder(f.Path):
				idx.unavailable[LedgerChapter] = true
			default:
				idx.unavailable[LedgerVolume] = true
			}
		}
	}
	for _, c := range contents {
		idx.contents[c.BookID] = append(idx.contents[c.BookID], c)
	}
	view := &ledgerView{Series: series, Versions: versions, Editions: cols, Books: map[int64][]store.Book{}, Status: map[int64][]BookStatus{}, Profiles: map[int64]*profile.Profile{}}
	for _, w := range works {
		view.Books[w.EditionID] = append(view.Books[w.EditionID], w)
	}
	for _, version := range versions {
		col := editionByID(cols, version.EditionID)
		if col == nil {
			continue
		}
		view.Profiles[version.ID] = profiles.For(version)
		view.Status[version.ID] = idx.cardBooks(version, *col, view.Books[col.ID])
	}
	return view, nil
}

// ledgerIndex is a series' files, mappings and settings, indexed for
// evaluating its cards.
type ledgerIndex struct {
	series      *store.Series
	profiles    cardProfiles
	today       string
	coverage    map[int64][]store.AssetCoverage // card → mappings
	overrides   map[int64]map[int64]bool        // card → book → collected
	files       map[int64]store.File
	present     map[int64]bool                 // file → on disk and confirmed
	unavailable map[string]bool                // ledger → its folder cannot be checked
	contents    map[int64][]store.AssetContent // book → files holding it
	pending     map[string]string              // store.BookKey → reservation phase
}

// cardBooks is where each book of a card stands.
func (idx ledgerIndex) cardBooks(version store.CollectedVersion, col store.Edition, books []store.Book) []BookStatus {
	prof := idx.profiles.For(version)
	judged := judgeCardFiles(col, prof, idx.profiles.Formats, idx.coverage[version.ID], idx.files, idx.present)
	chapter := col.Type == LedgerChapter
	slots := make([]BookStatus, 0, len(books))
	for _, w := range books {
		sl := BookStatus{Book: w, VersionID: version.ID, Availability: availability(w.ReleaseDate, idx.today), FileIDs: judged.accepted[w.ID], OnDiskFileIDs: []int64{}}
		if sl.FileIDs == nil {
			sl.FileIDs = []int64{}
		}
		review := false
		for _, c := range idx.contents[w.ID] {
			f := idx.files[c.FileID]
			if f.Presence == "present" {
				sl.OnDiskFileIDs = append(sl.OnDiskFileIDs, f.ID)
			}
			review = review || !c.Verified || f.NeedsReview
		}
		// Whether the card collects the book, and if not, why: the
		// first refusal from the series down to the book itself.
		inScope := (w.CountType == "main" || version.IncludeExtras) && (!chapter || !w.NumberKnown || w.SequenceNum > version.CutoffNum)
		if v, ok := idx.overrides[version.ID][w.ID]; ok {
			inScope = v
		}
		why := ""
		switch {
		case !idx.series.Monitored:
			why = "Automatic downloads are paused for this series"
		case col.SyncState == "stale":
			why = "Edition is unavailable in the catalog; review it before collecting"
		case w.SyncState == "stale" || w.SyncState == "retired":
			why = "This book is retired or unavailable in the catalog"
		case !version.Monitored:
			why = "Collecting is paused for this edition"
		case !inScope:
			why = "This book is outside what you collect of this edition"
		}
		sl.Monitored = why == ""
		best, known := 0, true
		for i, fid := range sl.FileIDs {
			if i == 0 || judged.scores[fid] > best {
				best = judged.scores[fid]
			}
			known = known && backedFacts(idx.files[fid].Facts, idx.files[fid].Claims).FactsKnown
		}
		sl.CurrentScore = best
		upgrade := prof != nil && prof.UpgradesEnabled && known && !judged.pinned[w.ID] && (prof.UpgradeUntilScore == 0 || best < prof.UpgradeUntilScore)
		sl.State, sl.Reason, sl.Wanted = bookState(bookFacts{
			Collected: sl.Monitored, Why: why,
			Owned: len(sl.FileIDs) > 0, OnDisk: len(sl.OnDiskFileIDs) > 0, Review: review,
			Reserved:     idx.pending[store.BookKey(version.ID, w.ID)] != "",
			Reviewing:    idx.pending[store.BookKey(version.ID, w.ID)] == store.QueueNeedsReview,
			Availability: sl.Availability, AcquireUnknown: version.AcquireUnknown,
			StorageDown: idx.unavailable[col.Type], Upgrade: upgrade, Refused: judged.refused[w.ID],
		})
		slots = append(slots, sl)
	}
	return slots
}

// backedFacts is what a file is judged on: its own facts, with the
// release's claims backing the fields the facts do not carry. Organizing
// and importing name a file after its books ("Volume 1.cbz"), which drops
// the digital/group/resolution evidence the release itself carried; the
// claims are that evidence, so a good copy is not forgotten. A field the
// facts do carry always wins over the offer, and a file without claims
// (a folder scan) keeps scoring from its own name alone.
func backedFacts(facts, claims store.AssetFacts) store.AssetFacts {
	if !claims.FactsKnown && claims.Title == "" {
		return facts
	}
	if facts.Group == "" && facts.Sources["group"] != "manual" {
		facts.Group = claims.Group
	}
	if facts.Language == "" && facts.Sources["language"] != "manual" {
		facts.Language = claims.Language
	}
	if facts.Site == "" {
		facts.Site = claims.Site
	}
	if facts.Protocol == "" {
		facts.Protocol = claims.Protocol
	}
	if facts.Resolution == 0 {
		facts.Resolution = claims.Resolution
	}
	if claims.Digital && !facts.Digital {
		facts.Digital = true
	}
	var backed []string
	for flag, set := range claims.Flags {
		if set && !facts.Flags[flag] {
			backed = append(backed, flag)
		}
	}
	if len(backed) > 0 {
		flags := maps.Clone(facts.Flags)
		if flags == nil {
			flags = map[string]bool{}
		}
		for _, flag := range backed {
			flags[flag] = true
		}
		facts.Flags = flags
	}
	facts.FactsKnown = facts.FactsKnown || claims.FactsKnown
	return facts
}

func assetJudgement(f store.AssetFacts, p *profile.Profile, formats []profile.Format) profile.Judgement {
	return p.Judge(profile.Input{Title: f.Title, Group: f.Group, Language: f.Language, Protocol: f.Protocol, Site: f.Site, Resolution: f.Resolution, Flags: f.Flags}, formats)
}

func (l *Library) BooksByVersion(ctx context.Context, seriesID int64) (map[int64][]BookStatus, error) {
	view, err := loadLedger(ctx, l.store, seriesID)
	if err != nil {
		return nil, err
	}
	return view.Status, nil
}

func (s *SearchService) prepareVersions(ctx context.Context, run *searchRun) error {
	if err := s.store.CheckFilePresence(ctx, run.seriesID); err != nil {
		return err
	}
	view, err := loadLedger(ctx, s.store, run.seriesID)
	if err != nil {
		return err
	}
	run.editions = view.Editions
	run.shelf = seriesShelf(view.Editions, view.Series)
	run.versions = view.Versions
	run.books = view.Books
	run.versionProfiles = view.Profiles
	run.wantedBooks = map[int64]map[int64]bool{}
	run.wanted = map[string]bool{}
	run.ownedScores = map[int64]map[int64]int{}
	bindings, err := s.store.ListBindings(ctx, run.seriesID)
	if err != nil {
		return err
	}
	run.sourceEditions = map[int64]int64{}
	for _, b := range bindings {
		if b.EditionID > 0 {
			run.sourceEditions[b.ID] = b.EditionID
		}
	}
	found := run.scope == nil
	for _, t := range view.Versions {
		col := editionByID(view.Editions, t.EditionID)
		if col.Type != run.ledger {
			continue
		}
		if run.scope != nil && run.scope.VersionID != t.ID {
			continue
		}
		if run.scope != nil && run.scope.BookID == 0 {
			found = true
		}
		wanted := map[int64]bool{}
		run.ownedScores[t.ID] = map[int64]int{}
		for _, sl := range view.Status[t.ID] {
			// A scheduled search only fills what is missing or
			// unsatisfactory: an upgrade-eligible book keeps its copy
			// until a person searches for a better one, so unattended
			// sweeps never hunt quality upgrades (Sonarr parity).
			if run.scheduled && sl.State == StateUpgradeEligible {
				continue
			}
			// A book reserved by the download being replaced is still wanted.
			if sl.Wanted && sl.State != "pending" || sl.State == "pending" && run.replacing[store.BookKey(t.ID, sl.Book.ID)] {
				run.wanted[store.BookKey(t.ID, sl.Book.ID)] = true
			}
			selected := sl.Wanted
			if run.scope != nil && run.scope.AllBooks && run.scope.BookID == 0 {
				selected = sl.State != "storage_unavailable" && sl.State != "needs_review" && sl.State != "pending"
			}
			if run.scope != nil && run.scope.BookID != 0 {
				selected = run.scope.BookID == sl.Book.ID
				if selected {
					found = true
				}
				if sl.State == "storage_unavailable" || sl.State == "needs_review" || sl.State == "pending" {
					selected = false
				}
			}
			// A book reserved by the download being replaced is still wanted.
			if sl.State == "pending" && run.replacing[store.BookKey(t.ID, sl.Book.ID)] &&
				(run.scope.BookID == 0 || run.scope.BookID == sl.Book.ID) {
				selected = true
			}
			if selected {
				wanted[sl.Book.ID] = true
				run.planBooks = append(run.planBooks, PlanBook{
					Book:  store.BookRef{VersionID: t.ID, BookID: sl.Book.ID, EditionID: col.ID, Ledger: col.Type, Number: sl.Book.SequenceNum},
					Label: sl.Book.SequenceStr, Ownership: sl.State,
				})
			}
			if sl.State == "upgrade_eligible" {
				run.ownedScores[t.ID][sl.Book.ID] = sl.CurrentScore
			}
		}
		if len(wanted) > 0 {
			run.wantedBooks[t.ID] = wanted
		}
	}
	if !found {
		return apperr.Invalidf("book or version does not belong to this edition")
	}
	return nil
}

// SaveVersion saves a way of collecting an edition, as a person sets it:
// an edition is collected once per release profile (two of its cards with
// one profile would want the same releases for the same books), and
// release restrictions live in the profile, never here. A card beside the
// first is named after its profile: the name is its folder.
func (l *Library) SaveVersion(ctx context.Context, seriesID int64, t store.CollectedVersion) (int64, error) {
	taken, err := l.store.ProfileTaken(ctx, t.EditionID, t.ID, t.ProfileID)
	if err != nil {
		return 0, err
	}
	if taken {
		return 0, apperr.Conflictf("this edition is already collected with that release profile")
	}
	if !t.IsDefault && t.ProfileID > 0 {
		p, err := l.store.GetProfile(ctx, t.ProfileID)
		if err != nil {
			return 0, err
		}
		t.Name = p.Name
	}
	return l.store.SaveVersion(ctx, seriesID, t)
}
