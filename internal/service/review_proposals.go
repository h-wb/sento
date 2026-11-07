package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

// ReviewItems is the review inbox with a proposal on every file whose own
// name resolves it (docs/reviews/propose-dont-ask.md §A1). Proposals are
// read against today's editions and recorded only when accepted.
func (l *Library) ReviewItems(ctx context.Context, seriesID int64) ([]store.ReviewItem, error) {
	items, err := l.store.ReviewItems(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	proposers := map[int64]*proposer{}
	for i := range items {
		if items[i].Kind != "file" {
			continue
		}
		p, ok := proposers[items[i].SeriesID]
		if !ok {
			if p, err = newProposer(ctx, l.store, items[i].SeriesID); err != nil {
				return nil, err
			}
			proposers[items[i].SeriesID] = p
		}
		// Pack-level review actions group a download's files together, so
		// each question says which release it arrived in.
		if f, exists := p.files[items[i].ItemID]; exists && f.Facts.ReleaseKey != "" {
			items[i].ReleaseKey = f.Facts.ReleaseKey
			if job, fromJob := p.jobs[f.Facts.ReleaseKey]; fromJob {
				items[i].Release = job.Title
			}
		}
		items[i].Proposal = p.propose(items[i].ItemID)
		items[i].Hint = p.hint(items[i].ItemID)
	}
	return items, nil
}

// proposer holds what one series' proposals are read against.
type proposer struct {
	titles   []string
	editions []store.Edition
	shelf    *shelf
	versions map[int64][]store.CollectedVersion
	profiles cardProfiles
	books    map[int64][]store.Book
	files    map[int64]store.File
	jobs     map[string]store.QueueItem // by release key
	parses   map[int64]parse.Result     // each file's own name, parsed once
	packs    map[string][]parse.Range   // a download's bare numbers, by release key
	stats    map[int64]os.FileInfo      // review: each archive as it was fingerprinted
}

// newProposer reads everything one series' proposals are read against, for
// review's per-file questions and the pack-level actions alike.
func newProposer(ctx context.Context, st *store.Store, seriesID int64) (*proposer, error) {
	p := &proposer{versions: map[int64][]store.CollectedVersion{}, books: map[int64][]store.Book{}, files: map[int64]store.File{}, jobs: map[string]store.QueueItem{}, parses: map[int64]parse.Result{}, packs: map[string][]parse.Range{}}
	var err error
	if p.titles, err = st.SeriesTitles(ctx, seriesID); err != nil {
		return nil, err
	}
	if p.editions, err = st.ListEditions(ctx, seriesID); err != nil {
		return nil, err
	}
	series, err := st.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	p.shelf = seriesShelf(p.editions, series)
	versions, err := st.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		p.versions[v.EditionID] = append(p.versions[v.EditionID], v)
	}
	if p.profiles, err = NewProfiles(st).seriesProfiles(ctx, seriesID); err != nil {
		return nil, err
	}
	for _, e := range p.editions {
		if p.books[e.ID], err = st.ListBooks(ctx, e.ID); err != nil {
			return nil, err
		}
	}
	files, err := st.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		p.files[f.ID] = f
	}
	jobs, err := st.SeriesQueueItems(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if j.ReleaseKey != "" {
			p.jobs[j.ReleaseKey] = j
		}
	}
	return p, nil
}

// fileParse reads a file's own name once; every question review asks
// about the file reads the same parse.
func (p *proposer) fileParse(f store.File) parse.Result {
	if parsed, ok := p.parses[f.ID]; ok {
		return parsed
	}
	name := f.Facts.Title
	if name == "" {
		name = filepath.Base(f.Path)
	}
	parsed := parse.ParseAnchored(name, parse.ChannelFile, p.titles)
	p.parses[f.ID] = parsed
	return parsed
}

// readsName says whether review reads a file's own name at all: a file
// that is not manga, a different work that merely contains the title, or
// nothing tying it to the series says nothing about its books. The job
// of a name it does read is returned with it.
func (p *proposer) readsName(f store.File) (parsed parse.Result, job store.QueueItem, fromJob, ok bool) {
	if !f.NeedsReview || !f.Unrequested || f.Presence == "unavailable" {
		return parse.Result{}, store.QueueItem{}, false, false
	}
	parsed = p.fileParse(f)
	if parsed.ContentType != parse.ContentManga || parsed.TitleLeftover != "" {
		return parse.Result{}, store.QueueItem{}, false, false
	}
	// A name without the series title ("Volume_01.cbz") is only this
	// series' because the download it came in was.
	job, fromJob = p.jobs[f.Facts.ReleaseKey]
	fromJob = fromJob && f.Facts.ReleaseKey != ""
	if !parsed.Anchored && !fromJob {
		return parse.Result{}, store.QueueItem{}, false, false
	}
	return parsed, job, fromJob, true
}

// parseFile reads a file's own name against the series: what it claims
// to hold (ledger, ranges), or why it cannot be read at all.
func (p *proposer) parseFile(fileID int64) (parsed parse.Result, ledger, evidence string, job store.QueueItem, fromJob, ok bool) {
	f, exists := p.files[fileID]
	if !exists {
		return parse.Result{}, "", "", store.QueueItem{}, false, false
	}
	parsed, job, fromJob, ok = p.readsName(f)
	if !ok {
		return parse.Result{}, "", "", store.QueueItem{}, false, false
	}
	ledger, evidence = ledgerOf(parsed), "from the file name"
	if ledger == "" {
		ledger, evidence = p.bareLedger(parsed, job, fromJob)
		if ledger == "" {
			return parse.Result{}, "", "", store.QueueItem{}, false, false
		}
		if ledger == LedgerVolume {
			parsed.Volumes = parsed.Bare
		} else {
			parsed.Chapters = parsed.Bare
		}
	} else if !parsed.Anchored {
		evidence = "from the file name, inside a download of this series"
	}
	return parsed, ledger, evidence, job, fromJob, true
}

// hint is what the file's own name says it holds even when no edition
// claims it yet, so review can prefill the numbers.
func (p *proposer) hint(fileID int64) *store.FileHint {
	parsed, ledger, _, _, _, ok := p.parseFile(fileID)
	if !ok || ledger == "" {
		return nil
	}
	ranges := bookRanges(parsed, ledger)
	if len(ranges) == 0 {
		return nil
	}
	h := &store.FileHint{Ledger: ledger}
	for _, r := range ranges {
		s := hundredths(r.From)
		if r.To != r.From {
			s += "–" + hundredths(r.To)
		}
		h.Numbers = append(h.Numbers, s)
	}
	return h
}

func (p *proposer) propose(fileID int64) *store.ReviewProposal {
	parsed, ledger, evidence, _, _, ok := p.parseFile(fileID)
	if !ok {
		return nil
	}
	f := p.files[fileID]
	ranges := bookRanges(parsed, ledger)
	var fits []store.Edition
	for _, e := range p.editions {
		if e.Type == ledger && p.shelf.fits(parsed, e) {
			fits = append(fits, e)
		}
	}
	type match struct {
		edition store.Edition
		books   []store.Book
	}
	var found []match
	for _, e := range p.shelf.prefer(parsed, fits) {
		books, ambiguous := resolveBooks(p.books[e.ID], ranges, parsed.Extra)
		if !ambiguous && len(books) > 0 && everyNumberHeld(books, ranges) && len(p.versions[e.ID]) > 0 {
			found = append(found, match{e, books})
		}
	}
	// Among editions that hold the books, the one you collect.
	if len(found) > 1 {
		var collected []match
		for _, m := range found {
			if m.edition.Monitored {
				collected = append(collected, m)
			}
		}
		found = collected
	}
	if len(found) != 1 {
		return nil
	}
	m := found[0]
	version := p.proposedVersion(m.edition, backedFacts(f.Facts, f.Claims))
	if version == nil {
		return nil
	}
	name := m.edition.EditionName
	if len(p.versions[m.edition.ID]) > 1 {
		if prof := p.profiles.For(*version); prof != nil {
			name += " · " + prof.Name
		} else if !version.IsDefault {
			name += " · " + version.Name
		}
	}
	prop := &store.ReviewProposal{Edition: name + " · " + m.edition.Language, Evidence: evidence, Label: bookLabel(ledger, m.books)}
	for _, b := range m.books {
		prop.Books = append(prop.Books, store.BookRef{VersionID: version.ID, BookID: b.ID, EditionID: m.edition.ID, Ledger: ledger, Number: b.SequenceNum})
	}
	return prop
}

// proposedVersion picks a collected card only when its profile accepts the
// file and it is the sole eligible card. Overlapping profiles are a choice
// for the person reviewing, never a reason to silently use the default.
func (p *proposer) proposedVersion(edition store.Edition, facts store.AssetFacts) *store.CollectedVersion {
	versions := p.versions[edition.ID]
	var collected []store.CollectedVersion
	for _, v := range versions {
		if v.Monitored {
			collected = append(collected, v)
		}
	}
	if len(collected) == 0 {
		for _, v := range versions {
			if v.IsDefault {
				collected = append(collected, v)
			}
		}
	}
	var eligible []store.CollectedVersion
	for _, v := range collected {
		prof := p.profiles.For(v)
		judgement := assetJudgement(facts, prof, p.profiles.Formats)
		if judgement.Broken != "" {
			continue
		}
		if prof != nil && facts.FactsKnown && prof.MinFormatScore > profile.MinScoreOff && judgement.Score < prof.MinFormatScore {
			continue
		}
		eligible = append(eligible, v)
	}
	if len(eligible) != 1 {
		return nil
	}
	return &eligible[0]
}

// bareLedger reads a number with no volume or chapter marker ("067") as
// the kind the download's own name gives that number, or failing that the
// kind of books the download was planned for — and the plan only decides
// when the download's own files bear that reading out: an unnumbered
// completed pack says nothing about its numbering, so its numbers as a
// whole must be books of the planned kind. A file outside any download
// stays unread: its number could be either.
func (p *proposer) bareLedger(parsed parse.Result, job store.QueueItem, fromJob bool) (string, string) {
	if !fromJob || len(parsed.Bare) == 0 {
		return "", ""
	}
	var snap ReleaseCandidate
	_ = json.Unmarshal(job.Snapshot, &snap)
	release := snap.Parse
	inVol, inCh := covers(release.Volumes, parsed.Bare), covers(release.Chapters, parsed.Bare)
	switch {
	case inVol && !inCh:
		return LedgerVolume, fmt.Sprintf("numbered %s, within the volumes the download names", rangeText(parsed.Bare))
	case inCh && !inVol:
		return LedgerChapter, fmt.Sprintf("numbered %s, within the chapters the download names", rangeText(parsed.Bare))
	case len(release.Volumes)+len(release.Chapters) > 0:
		return "", ""
	}
	var plan []store.BookRef
	if json.Unmarshal(job.Books, &plan) != nil || len(plan) == 0 {
		return "", ""
	}
	// A number zero-padded to three or more digits ("001") is written the
	// chapter way; a download of volumes cannot make it a volume number.
	if plan[0].Ledger == LedgerVolume && len(parsed.BarePadded) > 0 {
		return "", ""
	}
	if !p.packReadsAs(plan, job.ReleaseKey) {
		return "", ""
	}
	return plan[0].Ledger, fmt.Sprintf("numbered %s, inside a download of %ss", rangeText(parsed.Bare), plan[0].Ledger)
}

// packNumbers gathers the bare numbers every file of one download waiting
// for review carries: the distribution the release's own name does not
// give (an unnumbered completed pack).
func (p *proposer) packNumbers(key string) []parse.Range {
	if numbers, ok := p.packs[key]; ok {
		return numbers
	}
	var numbers []parse.Range
	for _, f := range p.files {
		if f.Facts.ReleaseKey != key {
			continue
		}
		if parsed, _, _, ok := p.readsName(f); ok {
			numbers = append(numbers, parsed.Bare...)
		}
	}
	p.packs[key] = numbers
	return numbers
}

// packReadsAs says whether the numbers of the download's files fit the
// kind of books the download was planned for: every one of them must
// stand for a book of the plan's editions. A pack numbered past what
// that kind holds (200 files against a 15-volume edition) is another
// numbering's chapters, not this plan's books.
func (p *proposer) packReadsAs(plan []store.BookRef, key string) bool {
	var books []store.Book
	seen := map[int64]bool{}
	for _, ref := range plan {
		if !seen[ref.EditionID] {
			seen[ref.EditionID] = true
			books = append(books, p.books[ref.EditionID]...)
		}
	}
	for _, r := range p.packNumbers(key) {
		if !numberHolds(books, r) {
			return false
		}
	}
	return true
}

// covers says whether every number in want falls inside one of ranges.
func covers(ranges, want []parse.Range) bool {
	if len(ranges) == 0 {
		return false
	}
	for _, w := range want {
		in := false
		for _, r := range ranges {
			in = in || (w.From >= r.From && w.To <= r.To)
		}
		if !in {
			return false
		}
	}
	return true
}

func rangeText(ranges []parse.Range) string {
	var parts []string
	for _, r := range ranges {
		s := hundredths(r.From)
		if r.To != r.From {
			s += "–" + hundredths(r.To)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func hundredths(n parse.Num) string {
	if n%100 == 0 {
		return fmt.Sprint(n / 100)
	}
	return strings.TrimRight(fmt.Sprintf("%d.%02d", n/100, n%100), "0")
}

// bookLabel names a run of books: "chapter 67", "volumes 1–7".
func bookLabel(ledger string, books []store.Book) string {
	if len(books) == 1 {
		return ledger + " " + books[0].SequenceStr
	}
	return fmt.Sprintf("%ss %s–%s", ledger, books[0].SequenceStr, books[len(books)-1].SequenceStr)
}
