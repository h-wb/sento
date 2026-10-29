package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
)

// Whether a release belongs to an edition is decided here and nowhere
// else (docs/reviews/edition-rules.md).
//
// An edition is a numbering: two copies of "volume 3" are the same book
// when they hold the same pages. What changes the numbering makes an
// edition — the original run, or a regrouping into fewer, bigger books
// ("Deluxe", "Omnibus", "3-in-1", "VIZBIG", "Perfect", … are names for
// that one thing). What does not — medium, a box, a scan or a digital
// rip, colorization of the same volumes, the group — is a variant within
// an edition, ranked by the profile.
//
// Names are evidence, not verdicts. A release and an edition are each
// read into the same shape (an editionClaim): the regroupings they name,
// a name of their own that no vocabulary explains ("Fullmetal Edition"),
// whether they claim color, the highest volume. judge weighs one against
// the other and answers fits, unsure (a person decides: never grabbed
// automatically) or no, with the reason in words. Numbers, color the
// edition claims and a name belonging to another edition of the series
// are strong; a word alone is not.

// regroupings is the vocabulary of regrouped numberings, keyed by the
// canonical name a release and an edition are compared by. Synonyms share
// a key (kanzenban is Japanese for perfect edition).
var regroupings = []struct {
	key     string
	phrases []string
}{
	{"omnibus", []string{"omnibus"}},
	{"deluxe", []string{"deluxe"}},
	{"vizbig", []string{"vizbig", "viz big"}},
	{"perfect", []string{"perfect edition", "kanzenban"}},
	{"colossal", []string{"colossal edition", "colossal"}},
	{"collectors", []string{"collector's edition", "collectors edition", "collector’s edition"}},
	{"all-in-one", []string{"all-in-one", "all in one"}},
	{"eternal", []string{"eternal edition"}},
	{"definitive", []string{"definitive edition"}},
	{"black", []string{"black edition"}},
	{"master", []string{"master edition", "masters edition", "masterpiece edition"}},
	{"ultimate", []string{"ultimate edition"}},
}

// reInOne reads "3-in-1", "3in1", "two in one" as the regrouping "3-in-1".
var reInOne = regexp.MustCompile(`\b(\d|two|three|four)[ -]?in[ -]?(1|one)\b`)

var spelled = map[string]string{"two": "2", "three": "3", "four": "4"}

// plainWords name no numbering of their own: "Standard Edition", "Digital
// Edition", "Box Set Edition" are the original numbering.
var plainWords = map[string]bool{
	"standard": true, "digital": true, "paperback": true, "hardcover": true, "print": true,
	"english": true, "first": true, "second": true, "third": true, "fourth": true, "new": true,
	"revised": true, "box": true, "set": true, "box set": true, "boxset": true, "special": true, "": true,
}

// colorPhrases claim colored pages.
var colorPhrases = []string{"full color", "full colour", "colored", "coloured", "color", "colour", "digital colored comics"}

var reOrdinal = regexp.MustCompile(`^\d+(st|nd|rd|th)$`)

var reEdition = regexp.MustCompile(`(?i)([\p{L}\d'’-]+(?:\s+[\p{L}\d'’-]+)?)\s+edition\b`)

// editionClaim is what a name says about the numbering it belongs to.
type editionClaim struct {
	regroup map[string]bool // regrouping keys named
	own     string          // a name of its own no vocabulary explains ("fullmetal")
	color   bool
	box     bool
}

func (c editionClaim) regrouped() bool { return len(c.regroup) > 0 }

// named reports whether the claim names anything about its numbering.
func (c editionClaim) named() bool { return c.regrouped() || c.own != "" }

// shares reports whether two claims name the same numbering.
func (c editionClaim) shares(o editionClaim) bool {
	for k := range c.regroup {
		if o.regroup[k] {
			return true
		}
	}
	return c.own != "" && c.own == o.own
}

// readClaim reads a lowercased name (a release title, an edition name).
func readClaim(lower string) editionClaim {
	c := editionClaim{regroup: map[string]bool{}}
	has := func(p string) bool { return indexWord(lower, p) }
	for _, r := range regroupings {
		for _, p := range r.phrases {
			if has(p) {
				c.regroup[r.key] = true
			}
		}
	}
	for _, m := range reInOne.FindAllStringSubmatch(lower, -1) {
		n := m[1]
		if s, ok := spelled[n]; ok {
			n = s
		}
		c.regroup[n+"-in-1"] = true
	}
	for _, p := range colorPhrases {
		c.color = c.color || has(p)
	}
	c.box = has("box set") || has("boxset") || has("box edition")
	// "<X> Edition" the vocabulary does not explain is a name of its own.
	for _, m := range reEdition.FindAllStringSubmatch(lower, -1) {
		words := strings.Fields(strings.TrimPrefix(m[1], "the "))
		phrase := words[len(words)-1]
		if len(words) > 1 && reOrdinal.MatchString(words[0]) {
			phrase = strings.Join(words, " ")
		}
		if plainWords[phrase] || readKnown(phrase) {
			continue
		}
		c.own = phrase
	}
	return c
}

// readKnown reports whether a word is already regrouping or color vocabulary.
func readKnown(word string) bool {
	for _, r := range regroupings {
		for _, p := range r.phrases {
			if strings.HasPrefix(p, word) {
				return true
			}
		}
	}
	for _, p := range colorPhrases {
		if strings.HasSuffix(p, word) {
			return true
		}
	}
	return reInOne.MatchString(word)
}

// indexWord reports whether phrase occurs in s as whole words.
func indexWord(s, phrase string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], phrase)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(phrase)
		if (start == 0 || !isWordByte(s[start-1])) && (end == len(s) || !isWordByte(s[end])) {
			return true
		}
		i = start + 1
	}
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
}

// editionView is an edition with its names read: the catalog's name and
// the one it is shown under. Renaming an edition cannot take away what the
// catalog says it is, and can add what the catalog leaves out.
type editionView struct {
	store.Edition
	claim editionClaim
	// others are the claims of the series' other editions of this type.
	others []editionClaim
}

func viewEdition(c store.Edition) editionView {
	claim := readClaim(strings.ToLower(c.EditionName + " · " + c.ProviderEditionName))
	v := editionView{Edition: c, claim: claim}
	for _, o := range c.OtherLabels {
		v.others = append(v.others, readClaim(strings.ToLower(o)))
	}
	return v
}

// original reports whether the edition is the original numbering: its
// names claim no regrouping and no name of their own.
func (e editionView) original() bool { return !e.claim.named() }

// seriesHasColor reports whether another edition of the series claims color.
func (e editionView) seriesHasColor() bool {
	for _, o := range e.others {
		if o.color {
			return true
		}
	}
	return false
}

type fitVerdict int

const (
	fitNo fitVerdict = iota
	fitUnsure
	fitYes
)

// releaseClaim reads what a release name says about its numbering.
func releaseClaim(p parse.Result) editionClaim {
	c := readClaim(p.Name())
	c.color = c.color || p.Edition.Colored
	c.box = c.box || p.Edition.BoxSet
	if p.Edition.Omnibus && !c.regrouped() {
		c.regroup["omnibus"] = true
	}
	if p.Edition.Deluxe {
		c.regroup["deluxe"] = true
	}
	return c
}

// lastVolume is the highest volume a release names (hundredths), 0 for none.
func lastVolume(p parse.Result) int64 {
	var last int64
	for _, r := range bookRanges(p, LedgerVolume) {
		last = max(last, int64(r.To))
	}
	return last
}

// volumeSlack is how far past an edition's last book a release may go:
// the catalog lists a new volume a little after it is out.
const volumeSlack = 200

// judge weighs a release against an edition. established means a verified
// source listing already ties them: words then cannot make it unsure, but
// numbers, a color the edition claims, and a name of another edition
// still say no.
func judge(p parse.Result, c store.Edition, established bool) (fitVerdict, string) {
	if !languageCompatible(p, c.Language) {
		return fitNo, "The stated language does not match this edition."
	}
	e, r := viewEdition(c), releaseClaim(p)
	// Numbers prove another numbering only where another edition of the
	// series reaches them: a catalog that lists fewer books than exist
	// says nothing.
	if last := lastVolume(p); c.Type == LedgerVolume && c.LastNumber > 0 && last > c.LastNumber+volumeSlack && c.OtherLast >= last {
		return fitNo, fmt.Sprintf("The release goes to volume %s; %s ends at %s.", hundredths(parse.Num(last)), c.EditionName, hundredths(parse.Num(c.LastNumber)))
	}
	if e.claim.color && !r.color {
		return fitNo, fmt.Sprintf("%s is a color edition; the release does not say it is colored.", c.EditionName)
	}
	if r.named() && r.shares(e.claim) {
		return fitYes, ""
	}
	if e.claim.own != "" && indexWord(p.Name(), e.claim.own) {
		return fitYes, ""
	}
	if r.named() {
		for i, o := range e.others {
			if r.shares(o) {
				return fitNo, fmt.Sprintf("The release names %s, another edition of this series.", c.OtherLabels[i])
			}
		}
	}
	verdict, why := fitYes, ""
	switch {
	case r.named() && e.claim.box && e.original():
		return fitNo, fmt.Sprintf("The release is %s; %s boxes the original volumes.", claimName(r), c.EditionName)
	case r.named() && e.original():
		verdict, why = fitUnsure, fmt.Sprintf("The release calls itself %s; %s does not. Confirm it is this numbering.", claimName(r), c.EditionName)
	case r.named():
		verdict, why = fitUnsure, fmt.Sprintf("The release calls itself %s, %s does not say so. Confirm it is this numbering.", claimName(r), c.EditionName)
	case !e.original():
		verdict, why = fitUnsure, fmt.Sprintf("%s is a numbering of its own; the release does not name it. Confirm it is this one.", c.EditionName)
	case r.color && !e.claim.color && e.seriesHasColor():
		verdict, why = fitUnsure, "The release is colored and this series has a color edition. Confirm it is this one."
	}
	if established && verdict == fitUnsure {
		return fitYes, ""
	}
	return verdict, why
}

// claimName says what a release calls itself: "a deluxe edition",
// "an omnibus edition".
func claimName(c editionClaim) string {
	article := func(w string) string {
		if strings.ContainsRune("aeiou8", rune(w[0])) {
			return "an " + w
		}
		return "a " + w
	}
	for _, r := range regroupings {
		if c.regroup[r.key] {
			return article(r.key) + " edition"
		}
	}
	for k := range c.regroup {
		return article(k) + " edition"
	}
	return "the " + c.own + " edition"
}

// editionFits reports whether a release is a book of this edition with
// nothing left for a person to confirm.
func editionFits(p parse.Result, c store.Edition, established bool) bool {
	v, _ := judge(p, c, established)
	return v == fitYes
}

// editionCompatible is editionFits for a release known only by its name.
func editionCompatible(p parse.Result, c store.Edition) bool { return editionFits(p, c, false) }

// judgeWithEvidence is judge when a verified source listing may point at
// an edition (evidence is that edition's id, 0 for none): the listing
// settles which edition, and every other one is no.
func judgeWithEvidence(p parse.Result, c store.Edition, evidence int64) (fitVerdict, string) {
	switch {
	case evidence == 0:
		return judge(p, c, false)
	case evidence != c.ID:
		return fitNo, "A verified source lists this release under another edition."
	}
	return judge(p, c, true)
}

// editionCompatibleWithEvidence is judgeWithEvidence answering fits.
func editionCompatibleWithEvidence(p parse.Result, c store.Edition, evidence int64) bool {
	v, _ := judgeWithEvidence(p, c, evidence)
	return v == fitYes
}

// preferByPackaging narrows editions a release fits by what sets variants
// apart: a digital rip belongs to the digital edition, a scan to the
// printed one, a release sold as a box to the box set, a colored one to
// the color edition — when an edition of that kind is in play. With no
// such statement, or no edition of that kind, every edition stays.
// Packaging never chooses between languages: a digital rip in an unstated
// language is not thereby English.
func preferByPackaging(p parse.Result, cols []store.Edition) []store.Edition {
	for _, c := range cols {
		if baseLang(c.Language) != baseLang(cols[0].Language) {
			return cols
		}
	}
	r := releaseClaim(p)
	for _, f := range []struct {
		stated bool
		holds  func(editionView) bool
	}{
		{r.box, func(e editionView) bool { return e.claim.box }},
		{r.color, func(e editionView) bool { return e.claim.color }},
		{p.Quality.Digital, func(e editionView) bool { return e.HasMedium("digital") }},
		{p.Quality.Scan, func(e editionView) bool { return e.HasMedium("paperback") || e.HasMedium("hardcover") }},
	} {
		if !f.stated {
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

// OtherEdition is the edition a release names that the series has none
// of, when the search should offer to add it: its name in the release's
// own words, the language, and the highest volume the release names (0
// when it names none).
type OtherEdition struct {
	Name      string `json:"name"`
	Language  string `json:"language"`
	Volumes   int    `json:"volumes"`
	InLibrary bool   `json:"inLibrary"`
}

// otherEdition is what a release that did not surely fit c calls itself,
// nil when it names no numbering. InLibrary when an edition of the series
// surely takes it (one that shares its name). A release whose numbers an
// edition could take is still offered: Vinland Saga "Omnibus" volumes are
// its 2-in-1 Standard Edition, which only a person can tell — they can
// confirm them there, or add the edition the release names.
func otherEdition(p parse.Result, title string, c store.Edition, eds []store.Edition) *OtherEdition {
	r := releaseClaim(p)
	if !r.named() {
		return nil
	}
	o := &OtherEdition{Name: editionNameIn(title, r), Language: c.Language, Volumes: int(lastVolume(p) / 100)}
	for _, e := range eds {
		if e.Type != c.Type || baseLang(e.Language) != baseLang(c.Language) {
			continue
		}
		if v, _ := judge(p, e, false); v == fitYes {
			o.InLibrary = true
		}
	}
	return o
}

// editionNameIn takes the edition's name from the release title as it is
// written ("Deluxe Edition", "The Perfect Edition" → "Perfect Edition",
// "Kanzenban" → "Kanzenban Edition").
func editionNameIn(title string, r editionClaim) string {
	for _, m := range reEdition.FindAllStringSubmatch(title, -1) {
		words := strings.Fields(m[1])
		if strings.EqualFold(words[0], "the") {
			words = words[1:]
		}
		phrase := strings.Join(words, " ")
		lower := strings.ToLower(phrase)
		if !plainWords[lower] && (r.own != "" && strings.Contains(lower, r.own) || len(readClaim(lower+" edition").regroup) > 0) {
			if len(words) > 1 && !readKnown(strings.ToLower(words[0])) && !reOrdinal.MatchString(strings.ToLower(words[0])) {
				phrase = words[len(words)-1]
			}
			return phrase + " Edition"
		}
	}
	lower := strings.ToLower(title)
	for _, rg := range regroupings {
		if !r.regroup[rg.key] {
			continue
		}
		for _, ph := range rg.phrases {
			if i := strings.Index(lower, ph); i >= 0 {
				return strings.TrimSuffix(title[i:i+len(ph)], " Edition") + " Edition"
			}
		}
	}
	for k := range r.regroup {
		return k + " Edition"
	}
	return strings.ToUpper(r.own[:1]) + r.own[1:] + " Edition"
}

// editionConflict says, in words, why a release is not (or not surely)
// of edition c; "" when it fits.
func editionConflict(p parse.Result, c store.Edition) string {
	_, why := judge(p, c, false)
	return why
}
