package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/parse"
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
			if p, err = l.newProposer(ctx, items[i].SeriesID); err != nil {
				return nil, err
			}
			proposers[items[i].SeriesID] = p
		}
		items[i].Proposal = p.propose(items[i].ItemID)
	}
	return items, nil
}

// proposer holds what one series' proposals are read against.
type proposer struct {
	titles   []string
	editions []store.Edition
	shelf    *shelf
	defaults map[int64]int64 // edition → default version
	books    map[int64][]store.Book
	files    map[int64]store.File
	jobs     map[string]store.QueueItem // by release key
}

func (l *Library) newProposer(ctx context.Context, seriesID int64) (*proposer, error) {
	p := &proposer{defaults: map[int64]int64{}, books: map[int64][]store.Book{}, files: map[int64]store.File{}, jobs: map[string]store.QueueItem{}}
	var err error
	if p.titles, err = l.store.SeriesTitles(ctx, seriesID); err != nil {
		return nil, err
	}
	if p.editions, err = l.store.ListEditions(ctx, seriesID); err != nil {
		return nil, err
	}
	p.shelf = newShelf(p.editions)
	versions, err := l.store.ListVersions(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, v := range versions {
		if v.IsDefault {
			p.defaults[v.EditionID] = v.ID
		}
	}
	for _, e := range p.editions {
		if p.books[e.ID], err = l.store.ListBooks(ctx, e.ID); err != nil {
			return nil, err
		}
	}
	files, err := l.store.ListFiles(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		p.files[f.ID] = f
	}
	jobs, err := l.store.SeriesQueueItems(ctx, seriesID)
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

func (p *proposer) propose(fileID int64) *store.ReviewProposal {
	f, ok := p.files[fileID]
	if !ok || !f.NeedsReview || !f.Unrequested || f.Presence == "unavailable" {
		return nil
	}
	name := f.Facts.Title
	if name == "" {
		name = filepath.Base(f.Path)
	}
	parsed := parse.ParseAnchored(name, parse.ChannelFile, p.titles)
	if parsed.ContentType != parse.ContentManga || parsed.TitleLeftover != "" {
		return nil
	}
	job, fromJob := p.jobs[f.Facts.ReleaseKey]
	fromJob = fromJob && f.Facts.ReleaseKey != ""
	// A name without the series title ("Volume_01.cbz") is only this
	// series' because the download it came in was.
	if !parsed.Anchored && !fromJob {
		return nil
	}
	ledger, evidence := ledgerOf(parsed), "from the file name"
	if ledger == "" {
		ledger, evidence = bareLedger(parsed, job, fromJob)
		if ledger == "" {
			return nil
		}
		if ledger == LedgerVolume {
			parsed.Volumes = parsed.Bare
		} else {
			parsed.Chapters = parsed.Bare
		}
	} else if !parsed.Anchored {
		evidence = "from the file name, inside a download of this series"
	}
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
		if !ambiguous && len(books) > 0 && p.defaults[e.ID] != 0 {
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
	prop := &store.ReviewProposal{Edition: m.edition.EditionName + " · " + m.edition.Language, Evidence: evidence, Label: bookLabel(ledger, m.books)}
	for _, b := range m.books {
		prop.Books = append(prop.Books, store.BookRef{VersionID: p.defaults[m.edition.ID], BookID: b.ID, EditionID: m.edition.ID, Ledger: ledger, Number: b.SequenceNum})
	}
	return prop
}

// bareLedger reads a number with no volume or chapter marker ("067") as
// the kind the download's own name gives that number, or failing that the
// kind of books the download was for. A file outside any download stays
// unread: its number could be either.
func bareLedger(parsed parse.Result, job store.QueueItem, fromJob bool) (string, string) {
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
	return plan[0].Ledger, fmt.Sprintf("numbered %s, inside a download of %ss", rangeText(parsed.Bare), plan[0].Ledger)
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
