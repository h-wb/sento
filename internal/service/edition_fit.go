package service

import (
	"slices"

	"github.com/h-wb/sento/internal/metadata"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Whether a release belongs to an edition is decided here and nowhere
// else, from one table. Each row is a facet: something a release name or
// an edition can state. A facet is one of two kinds, and the kind is the
// whole rule:
//
//   - A content facet changes what is on the pages (omnibus, deluxe,
//     coloured). Release and edition must agree, or it is a different book.
//   - A packaging facet changes only how the same pages are wrapped or
//     produced (box set, digital rip, print scan). It never excludes a
//     release; it only says which of several fitting editions is the
//     better home.
//
// A new trait is a new row. If a rule seems to need an exception, the
// facet is probably of the other kind.
type editionFacet struct {
	name    string
	content bool
	// stated reports whether the release name says so.
	stated func(parse.Result) bool
	// holds reports whether the edition is so.
	holds func(editionView) bool
	// unspecified reports whether the edition says nothing either way
	// (content facets only): absence of evidence is not a contradiction
	// once something else has established the edition.
	unspecified func(editionView) bool
}

// editionView is an edition with its traits and variant resolved: the
// stored ones, or what its name implies when none are stored.
type editionView struct {
	store.Edition
	traits  []string
	variant string
}

func viewEdition(c store.Edition) editionView {
	v := editionView{Edition: c, traits: c.EditionTraits, variant: c.ContentVariant}
	if v.traits == nil {
		v.traits = metadata.EditionTraits(c.EditionName)
	}
	if v.variant == "" {
		v.variant = metadata.ContentVariant(c.EditionName)
	}
	return v
}

func trait(name string) func(editionView) bool {
	return func(e editionView) bool { return slices.Contains(e.traits, name) }
}

func noTraits(e editionView) bool { return len(e.traits) == 0 }

var editionFacets = []editionFacet{
	{
		name: "omnibus", content: true,
		stated: func(p parse.Result) bool { return p.Edition.Omnibus },
		holds:  trait("omnibus"), unspecified: noTraits,
	},
	{
		name: "deluxe", content: true,
		stated: func(p parse.Result) bool { return p.Edition.Deluxe },
		holds:  trait("deluxe"), unspecified: noTraits,
	},
	{
		name: "colored", content: true,
		stated:      func(p parse.Result) bool { return p.Edition.Colored },
		holds:       func(e editionView) bool { return e.variant == "colored" },
		unspecified: func(e editionView) bool { return e.variant == "unknown" },
	},
	{
		// A box is how books are sold, not what they are: the volumes in
		// it are named like any others.
		name:   "box set",
		stated: func(p parse.Result) bool { return p.Edition.BoxSet },
		holds:  trait("boxset"),
	},
	{
		// "(Digital)" says how the file was produced.
		name:   "digital",
		stated: func(p parse.Result) bool { return p.Quality.Digital },
		holds:  func(e editionView) bool { return e.Medium == "digital" },
	},
	{
		name:   "print scan",
		stated: func(p parse.Result) bool { return p.Quality.Scan },
		holds:  func(e editionView) bool { return e.Medium != "" && e.Medium != "digital" },
	},
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
		case stated && !f.unspecified(e):
			return false
		}
	}
	return true
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
