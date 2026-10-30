package service

import (
	"fmt"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Whether a release belongs to an edition is decided here and nowhere
// else (docs/reviews/edition-rules.md). An edition is a numbering; what a
// name says about it is read by parse.ReadEdition, for release names and
// edition names alike. A release is judged against one edition of a
// series, in view of the series' other editions: the shelf.

// shelf is a series' editions with their names read once. A nil shelf
// judges an edition on its own, with no sibling to tell it from.
type shelf struct {
	editions []store.Edition
	claims   map[int64]parse.EditionClaim
}

func newShelf(eds []store.Edition) *shelf {
	s := &shelf{editions: eds, claims: map[int64]parse.EditionClaim{}}
	for _, e := range eds {
		s.claims[e.ID] = readEditionClaim(e)
	}
	return s
}

// readEditionClaim reads an edition's names: the catalog's and the one it
// is shown under. Renaming cannot take away what the catalog says it is,
// and can add what the catalog leaves out.
func readEditionClaim(e store.Edition) parse.EditionClaim {
	return parse.ReadEdition(e.EditionName + " / " + e.ProviderEditionName)
}

func (s *shelf) claim(e store.Edition) parse.EditionClaim {
	if s != nil {
		if c, ok := s.claims[e.ID]; ok {
			return c
		}
	}
	return readEditionClaim(e)
}

// siblings are e's alternatives: the series' other editions of its type
// and language.
func (s *shelf) siblings(e store.Edition) []store.Edition {
	if s == nil {
		return nil
	}
	var out []store.Edition
	for _, o := range s.editions {
		if o.ID != e.ID && o.Type == e.Type && baseLang(o.Language) == baseLang(e.Language) {
			out = append(out, o)
		}
	}
	return out
}

type fitVerdict int

const (
	fitNo     fitVerdict = iota
	fitUnsure            // a person decides: never grabbed automatically
	fitYes
)

// newVolumeSlack is how many books past an edition's last a release may
// go: the catalog lists a new volume a little after it is out.
const newVolumeSlack = 2 * 100

// judge weighs a release against edition e, answering fits, unsure or no
// with the reason in words. evidence is the edition a verified source
// listing ties the release to (0: none): another edition is no; e itself
// settles words, not numbers, a color e claims, or a sibling's name.
//
// Strong evidence says no: a language mismatch; volume numbers past e's
// last book that a sibling reaches (a catalog listing fewer books than
// exist proves nothing); e claiming color against an uncolored release;
// the release naming a sibling; a regrouping offered to a box of the
// original volumes. Words alone that disagree leave it unsure.
func (s *shelf) judge(p parse.Result, e store.Edition, evidence int64) (fitVerdict, string) {
	if evidence != 0 && evidence != e.ID {
		return fitNo, "A verified source lists this release under another edition."
	}
	if !languageCompatible(p, e.Language) {
		return fitNo, "The stated language does not match this edition."
	}
	ec, r, siblings := s.claim(e), p.Claim, s.siblings(e)
	if last := lastVolume(p); e.Type == LedgerVolume && e.LastNumber > 0 && last > e.LastNumber+newVolumeSlack {
		for _, o := range siblings {
			if o.LastNumber >= last {
				return fitNo, fmt.Sprintf("The release goes to volume %s; %s ends at %s.", hundredths(parse.Num(last)), e.EditionName, hundredths(parse.Num(e.LastNumber)))
			}
		}
	}
	if ec.Color && !r.Color {
		return fitNo, fmt.Sprintf("%s is a color edition; the release does not say it is colored.", e.EditionName)
	}
	if r.Shares(ec) || ec.Own != "" && p.Mentions(ec.Own) {
		return fitYes, ""
	}
	if r.Named() {
		for _, o := range siblings {
			if r.Shares(s.claim(o)) {
				return fitNo, fmt.Sprintf("The release names %s, another edition of this series.", o.EditionName)
			}
		}
		if ec.Box && !ec.Named() {
			return fitNo, fmt.Sprintf("The release is the %s; %s boxes the original volumes.", r.Written, e.EditionName)
		}
	}
	verdict, why := fitYes, ""
	switch {
	case r.Named():
		verdict, why = fitUnsure, fmt.Sprintf("The release calls itself the %s; %s does not. Confirm it is this numbering.", r.Written, e.EditionName)
	case ec.Named():
		verdict, why = fitUnsure, fmt.Sprintf("%s is a numbering of its own; the release does not name it. Confirm it is this one.", e.EditionName)
	case r.Color && !ec.Color && s.anyColor(siblings):
		verdict, why = fitUnsure, "The release is colored and this series has a color edition. Confirm it is this one."
	}
	if verdict == fitUnsure && evidence == e.ID {
		return fitYes, ""
	}
	return verdict, why
}

func (s *shelf) anyColor(eds []store.Edition) bool {
	for _, o := range eds {
		if s.claim(o).Color {
			return true
		}
	}
	return false
}

// fits reports whether a release known only by its name is surely a book
// of e.
func (s *shelf) fits(p parse.Result, e store.Edition) bool {
	v, _ := s.judge(p, e, 0)
	return v == fitYes
}

// lastVolume is the highest volume a release names (hundredths), 0 for none.
func lastVolume(p parse.Result) int64 {
	var last int64
	for _, r := range bookRanges(p, LedgerVolume) {
		last = max(last, int64(r.To))
	}
	return last
}

// prefer narrows editions a release fits by what sets variants apart: a
// release sold as a box belongs to the box set, a colored one to the color
// edition, a digital rip to the digital edition, a scan to the printed one
// — when an edition of that kind is in play. With no such statement, or
// no edition of that kind, every edition stays. Variants never choose
// between languages: a digital rip in an unstated language is not thereby
// English.
func (s *shelf) prefer(p parse.Result, eds []store.Edition) []store.Edition {
	for _, e := range eds {
		if baseLang(e.Language) != baseLang(eds[0].Language) {
			return eds
		}
	}
	for _, f := range []struct {
		stated bool
		holds  func(store.Edition) bool
	}{
		{p.Claim.Box, func(e store.Edition) bool { return s.claim(e).Box }},
		{p.Claim.Color, func(e store.Edition) bool { return s.claim(e).Color }},
		{p.Quality.Digital, func(e store.Edition) bool { return e.HasMedium("digital") }},
		{p.Quality.Scan, func(e store.Edition) bool { return e.HasMedium("paperback") || e.HasMedium("hardcover") }},
	} {
		if !f.stated {
			continue
		}
		var matching []store.Edition
		for _, e := range eds {
			if f.holds(e) {
				matching = append(matching, e)
			}
		}
		if len(matching) > 0 {
			eds = matching
		}
	}
	return eds
}

// OtherEdition is the numbering a release names when no edition of the
// series surely takes it, for the search to offer adding: its name as the
// release writes it, the language, the highest volume it names (0: none).
type OtherEdition struct {
	Name      string `json:"name"`
	Language  string `json:"language"`
	Volumes   int    `json:"volumes"`
	InLibrary bool   `json:"inLibrary"`
}

// offer is what a release that did not surely fit e calls itself, nil when
// it names no numbering. InLibrary when an edition of the series surely
// takes it. A release whose numbers an edition could take is still
// offered: Vinland Saga "Omnibus" volumes are its 2-in-1 Standard Edition,
// which only a person can tell — they confirm them there, or add the
// edition the release names.
func (s *shelf) offer(p parse.Result, e store.Edition) *OtherEdition {
	if !p.Claim.Named() {
		return nil
	}
	o := &OtherEdition{Name: p.Claim.Written, Language: e.Language, Volumes: int(lastVolume(p) / 100)}
	for _, x := range append(s.siblings(e), e) {
		o.InLibrary = o.InLibrary || s.fits(p, x)
	}
	return o
}

// ambiguousEdition names another edition the user collects that a release
// fitting col fits at least as well ("" for none). Editions nobody
// collects do not compete: the user already chose against them.
func (run *searchRun) ambiguousEdition(p parse.Result, col store.Edition) string {
	fits := []store.Edition{col}
	other := ""
	for _, c := range run.editions {
		if c.ID == col.ID || c.Type != col.Type || c.SyncState == "stale" || !run.collected(c.ID) || !run.shelf.fits(p, c) {
			continue
		}
		if other == "" {
			other = c.EditionName
		}
		if baseLang(c.Language) != baseLang(col.Language) {
			return c.EditionName
		}
		fits = append(fits, c)
	}
	if best := run.shelf.prefer(p, fits); len(best) == 1 && best[0].ID == col.ID {
		return ""
	}
	return other
}

func (run *searchRun) collected(editionID int64) bool {
	for _, t := range run.versions {
		if t.EditionID == editionID && t.Monitored {
			return true
		}
	}
	return false
}

// Volumes and chapters are collected independently, so a release or a
// file is one or the other, never both. The name decides:
//
//   - only volume numbers: a volume;
//   - only chapter numbers: chapters;
//   - both, with a single chapter ("Vol.5 Ch.34"): a chapter that notes
//     the volume it belongs to;
//   - both, with several chapters ("v01 c001-008", "Volumes 01-38 ¦
//     Chapters 001-410"): volumes that list the chapters inside them.
//
// bookRanges returns what a name offers the given ledger under that rule:
// its numbers when it is of that kind, nothing otherwise.
func bookRanges(p parse.Result, ledger string) []parse.Range {
	if ledgerOf(p) != ledger {
		return nil
	}
	if ledger == LedgerVolume {
		return p.Volumes
	}
	return p.Chapters
}

// ledgerOf says which kind of book a parsed name is ("" when it carries
// no volume or chapter number).
func ledgerOf(p parse.Result) string {
	switch {
	case len(p.Volumes) == 0 && len(p.Chapters) == 0:
		return ""
	case len(p.Chapters) == 0:
		return LedgerVolume
	case len(p.Volumes) == 0:
		return LedgerChapter
	case len(p.Chapters) == 1 && p.Chapters[0].From == p.Chapters[0].To:
		return LedgerChapter
	}
	return LedgerVolume
}
