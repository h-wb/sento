package service

import (
	"fmt"
	"strings"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Whether a release belongs to an edition is decided here and nowhere
// else. An edition is what its name says it is, read with the vocabulary
// release names are read with: there is no separate list of traits to
// keep in step with the catalog. Each row below is a facet, something a
// release name or an edition name can state. A facet is one of two kinds,
// and the kind is the whole rule:
//
//   - A content facet changes what is on the pages (omnibus, deluxe,
//     coloured). Release and edition must agree, or it is a different book.
//   - A packaging facet changes only how the same pages are wrapped or
//     produced (box set, digital rip, print scan). It never excludes a
//     release; it only says which of several fitting editions is the
//     better home.
//
// A new term is a new row. If a rule seems to need an exception, the
// facet is probably of the other kind. Edition names this vocabulary does
// not know ("Fullmetal Edition", "Colossal Edition") are handled as they
// are written: see labelFits.
type editionFacet struct {
	name    string
	content bool
	// stated reports whether the release name says so.
	stated func(parse.Result) bool
	// holds reports whether the edition is so.
	holds func(editionView) bool
}

// editionView is an edition with its names read: the catalog's name for
// it and the one it is shown under. Renaming an edition cannot take away
// what the catalog says it is, and can add what the catalog leaves out (a
// "Box Set Edition" whose books are the deluxe ones).
type editionView struct {
	store.Edition
	tags parse.EditionHints
	// phrase is what the catalog's name says beyond this vocabulary:
	// "fullmetal" for "Fullmetal Edition", nothing for "Standard Edition"
	// or "3-in-1 Edition". A local edition is named freely and has none.
	phrase string
}

func viewEdition(c store.Edition) editionView {
	shown, catalog := parse.EditionTags(c.EditionName), parse.EditionTags(c.ProviderEditionName)
	return editionView{Edition: c, phrase: labelPhrase(c.ProviderEditionName), tags: parse.EditionHints{
		Omnibus: shown.Omnibus || catalog.Omnibus, Deluxe: shown.Deluxe || catalog.Deluxe,
		BoxSet: shown.BoxSet || catalog.BoxSet, Colored: shown.Colored || catalog.Colored,
	}}
}

// specified reports whether the edition's name says what kind of edition
// it is. One that does not ("Chapters", a local edition's free name) does
// not contradict a release that does, once something else has established
// the edition.
func (e editionView) specified() bool {
	t := e.tags
	return t.Omnibus || t.Deluxe || t.BoxSet || t.Colored || e.phrase != "" || strings.Contains(strings.ToLower(e.EditionName+" "+e.ProviderEditionName), "standard")
}

// labelPhrase is the distinctive part of an edition name the shared
// vocabulary does not already explain.
func labelPhrase(name string) string {
	n := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "edition"))
	if n == "" || n == "standard" {
		return ""
	}
	if t := parse.EditionTags(n); t.Omnibus || t.Deluxe || t.BoxSet || t.Colored {
		return ""
	}
	return n
}

var editionFacets = []editionFacet{
	{
		name: "omnibus", content: true,
		stated: func(p parse.Result) bool { return p.Edition.Omnibus },
		holds:  func(e editionView) bool { return e.tags.Omnibus },
	},
	{
		name: "deluxe", content: true,
		stated: func(p parse.Result) bool { return p.Edition.Deluxe },
		holds:  func(e editionView) bool { return e.tags.Deluxe },
	},
	{
		name: "colored", content: true,
		stated: func(p parse.Result) bool { return p.Edition.Colored },
		holds:  func(e editionView) bool { return e.tags.Colored },
	},
	{
		// A box is how books are sold, not what they are: the volumes in
		// it are named like any others.
		name:   "box set",
		stated: func(p parse.Result) bool { return p.Edition.BoxSet },
		holds:  func(e editionView) bool { return e.tags.BoxSet },
	},
	{
		// "(Digital)" says how the file was produced.
		name:   "digital",
		stated: func(p parse.Result) bool { return p.Quality.Digital },
		holds:  func(e editionView) bool { return e.HasMedium("digital") },
	},
	{
		name:   "print scan",
		stated: func(p parse.Result) bool { return p.Quality.Scan },
		holds:  func(e editionView) bool { return e.HasMedium("paperback") || e.HasMedium("hardcover") },
	},
}

// labelFits applies an edition name the vocabulary does not know, as it is
// written. A release that names another of the series' editions in full
// ("Fullmetal Edition") is that edition and no other. An edition with a
// name of its own takes only releases that carry it, unless something
// stronger has established the edition.
func labelFits(p parse.Result, e editionView, established bool) bool {
	for _, other := range e.OtherLabels {
		phrase := labelPhrase(other)
		if phrase != "" && phrase != e.phrase && p.Mentions(phrase+" edition") {
			return false
		}
	}
	return e.phrase == "" || established || p.Mentions(e.phrase)
}

// editionFits reports whether a release can be a book of this edition:
// a compatible language and agreement on every content facet. established
// means something stronger than the name already ties the release to this
// edition (a verified source listing): the release then need not restate
// what the edition is, and an edition that does not specify a facet does
// not contradict a release that does.
func editionFits(p parse.Result, c store.Edition, established bool) bool {
	if !languageCompatible(p, c.Language) {
		return false
	}
	e := viewEdition(c)
	for _, f := range editionFacets {
		if !f.content {
			continue
		}
		stated, holds := f.stated(p), f.holds(e)
		switch {
		case stated == holds:
		case !established:
			return false
		case stated && e.specified():
			return false
		}
	}
	return labelFits(p, e, established)
}

// editionCompatible is editionFits for a release known only by its name.
func editionCompatible(p parse.Result, c store.Edition) bool { return editionFits(p, c, false) }

// editionCompatibleWithEvidence is editionFits when a verified source
// listing may point at an edition (evidence is that edition's id, 0 for
// none): the listing settles which edition, and only that one fits.
func editionCompatibleWithEvidence(p parse.Result, c store.Edition, evidence int64) bool {
	if evidence == 0 {
		return editionFits(p, c, false)
	}
	return evidence == c.ID && editionFits(p, c, true)
}

// preferByPackaging narrows editions a release fits by its packaging
// facets: a digital rip belongs to the digital edition, a scan to the
// printed one, a release sold as a box to the box set — when an edition
// of that kind is in play. With no such statement, or no edition of that
// kind, every edition stays. Packaging never chooses between languages: a
// digital rip in an unstated language is not thereby English.
func preferByPackaging(p parse.Result, cols []store.Edition) []store.Edition {
	for _, c := range cols {
		if baseLang(c.Language) != baseLang(cols[0].Language) {
			return cols
		}
	}
	for _, f := range editionFacets {
		if f.content || !f.stated(p) {
			continue
		}
		var matching []store.Edition
		for _, c := range cols {
			if f.holds(viewEdition(c)) {
				matching = append(matching, c)
			}
		}
		if len(matching) > 0 {
			cols = matching
		}
	}
	return cols
}

// ambiguousEdition reports whether a release that fits col fits another
// edition the user collects at least as well. Catalog editions nobody
// collects do not compete: the user already chose against them.
func (run *searchRun) ambiguousEdition(p parse.Result, col store.Edition) bool {
	fits := []store.Edition{col}
	otherLanguage := false
	for _, c := range run.editions {
		if c.ID == col.ID || c.Type != col.Type || c.SyncState == "stale" {
			continue
		}
		collected := false
		for _, t := range run.versions {
			collected = collected || (t.EditionID == c.ID && t.Monitored)
		}
		if !collected || !editionCompatible(p, c) {
			continue
		}
		if baseLang(c.Language) != baseLang(col.Language) {
			otherLanguage = true
			continue
		}
		fits = append(fits, c)
	}
	best := preferByPackaging(p, fits)
	return otherLanguage || len(best) != 1 || best[0].ID != col.ID
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

// editionConflict says, in words, why editionFits turned a release away
// from c, and whether the release states an edition of its own. "" when
// the name does not show it (a source listing placed it elsewhere).
func editionConflict(p parse.Result, c store.Edition) (why string, statesOwn bool) {
	e := viewEdition(c)
	for _, f := range editionFacets {
		if !f.content {
			continue
		}
		switch stated, holds := f.stated(p), f.holds(e); {
		case stated && !holds:
			return fmt.Sprintf("The release is a %s edition; %s is not.", f.name, c.EditionName), true
		case holds && !stated:
			return fmt.Sprintf("%s is a %s edition; the release does not say it is.", c.EditionName, f.name), false
		}
	}
	for _, other := range e.OtherLabels {
		phrase := labelPhrase(other)
		if phrase != "" && phrase != e.phrase && p.Mentions(phrase+" edition") {
			return fmt.Sprintf("The release names %s, another edition of this series.", other), false
		}
	}
	if e.phrase != "" && !p.Mentions(e.phrase) {
		return fmt.Sprintf("The release name does not mention %q.", e.phrase), false
	}
	return "", false
}

// statedFacet is the content facet a release states that c does not hold
// ("deluxe" for a Deluxe release ruled out of a box set), "" for none.
func statedFacet(p parse.Result, c store.Edition) string {
	e := viewEdition(c)
	for _, f := range editionFacets {
		if f.content && f.stated(p) && !f.holds(e) {
			return f.name
		}
	}
	return ""
}

// holdsFacet reports whether any of eds in language is a facet edition.
func holdsFacet(eds []store.Edition, facet, language string) bool {
	for _, c := range eds {
		if baseLang(c.Language) != baseLang(language) {
			continue
		}
		for _, f := range editionFacets {
			if f.name == facet && f.holds(viewEdition(c)) {
				return true
			}
		}
	}
	return false
}

// facetEditionName is what an edition of that facet is usually called.
func facetEditionName(facet string) string {
	return strings.ToUpper(facet[:1]) + facet[1:] + " Edition"
}

// OtherEdition names the edition a release is when it was ruled out of
// another one: "Deluxe Edition" in en. InLibrary says whether the series
// already has an edition like it; when not, the search offers to add it.
type OtherEdition struct {
	Name      string `json:"name"`
	Language  string `json:"language"`
	InLibrary bool   `json:"inLibrary"`
}

// otherEdition is the edition a release states that c is not, nil when
// the release names none (a source listing placed it elsewhere).
func otherEdition(p parse.Result, c store.Edition, eds []store.Edition) *OtherEdition {
	facet := statedFacet(p, c)
	if facet == "" {
		return nil
	}
	return &OtherEdition{Name: facetEditionName(facet), Language: c.Language, InLibrary: holdsFacet(eds, facet, c.Language)}
}
