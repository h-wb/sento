package parse

import (
	"regexp"
	"slices"
	"strings"
)

// An edition, for a downloader, is a numbering: which pages "volume 3"
// holds (docs/reviews/edition-rules.md). Release names and edition names
// say things about it in one vocabulary, read here and nowhere else:
//
//   - a regrouping: the story cut into fewer, bigger books. "Deluxe",
//     "Omnibus", "3-in-1", "VIZBIG", "Perfect Edition", "Kanzenban", … are
//     names for that one thing; synonyms share a key.
//   - a name of its own the vocabulary does not explain ("Fullmetal
//     Edition", "35th Anniversary Edition"): a numbering only that name
//     identifies.
//   - color, and a box: variants of a numbering, not numberings.

// EditionClaim is what one name says about the numbering it belongs to.
type EditionClaim struct {
	// Regroup holds the regrouping keys named, sorted ("deluxe", "3-in-1").
	Regroup []string `json:"regroup,omitempty"`
	// Own is a lowercase name of its own ("fullmetal", "35th anniversary").
	Own string `json:"own,omitempty"`
	// Written is the edition's name as the source writes it ("Deluxe
	// Edition", "Kanzenban Edition"); empty when the name names none.
	Written string `json:"written,omitempty"`
	Color   bool   `json:"color,omitempty"`
	// ColorEdition: the name names a color edition ("Full Color", "Color
	// Edition") — a numbering of its own, like Viz's Dragon Ball Full
	// Color. Colored alone ("(Colored)", "Digital Colored Comics") is a
	// colorization of the original volumes, a variant.
	ColorEdition bool `json:"colorEdition,omitempty"`
	Box          bool `json:"box,omitempty"`
}

// Named reports whether the claim names a numbering: a regrouping or a
// name of its own.
func (c EditionClaim) Named() bool { return len(c.Regroup) > 0 || c.Own != "" || c.ColorEdition }

// Shares reports whether two claims name the same numbering.
func (c EditionClaim) Shares(o EditionClaim) bool {
	for _, k := range c.Regroup {
		if slices.Contains(o.Regroup, k) {
			return true
		}
	}
	return c.Own != "" && c.Own == o.Own || c.ColorEdition && o.ColorEdition
}

// regroupings is the regrouping vocabulary: canonical key, then the
// phrases that name it. A phrase that would also be an ordinary title word
// ("perfect", "black") only counts followed by "edition".
var regroupings = []struct {
	key     string
	phrases []string
}{
	{"omnibus", []string{"omnibus"}},
	{"deluxe", []string{"deluxe"}},
	{"vizbig", []string{"vizbig", "viz big"}},
	{"perfect", []string{"perfect edition", "kanzenban"}},
	{"colossal", []string{"colossal edition"}},
	{"collectors", []string{"collector's edition", "collectors edition", "collector’s edition"}},
	{"all-in-one", []string{"all-in-one", "all in one"}},
	{"eternal", []string{"eternal edition"}},
	{"definitive", []string{"definitive edition"}},
	{"black", []string{"black edition"}},
	{"master", []string{"master edition", "masters edition", "masterpiece edition"}},
	{"ultimate", []string{"ultimate edition"}},
	{"n-in-1", []string{"in1 edition"}},
}

// reInOne reads "3-in-1", "3in1", "2 in 1", "two in one" as "3-in-1".
var reInOne = regexp.MustCompile(`\b(\d|two|three|four)[ -]?in[ -]?(1|one)\b`)

var spelled = map[string]string{"two": "2", "three": "3", "four": "4"}

var colorPhrases = []string{"digital colored comics", "full color", "full colour", "colored", "coloured", "color", "colour"}

var boxPhrases = []string{"box set", "boxset", "box edition"}

// colorEditionPhrases name a color edition. "Digital Colored Comics" says
// "full color" too, but colorizes the original volumes: it is a variant.
var colorEditionPhrases = []string{"full color", "full colour", "full-color", "color edition", "colour edition", "complete color"}

// plainEditions name no numbering of their own: "Standard Edition",
// "Digital Edition", "Box Set Edition", "Limited Edition".
var plainEditions = map[string]bool{
	"standard": true, "digital": true, "paperback": true, "hardcover": true, "print": true,
	"english": true, "first": true, "second": true, "third": true, "fourth": true, "new": true,
	"revised": true, "special": true, "limited": true, "set": true, "box": true,
}

// reEditionName finds "<name> Edition", the name one word, after an
// optional ordinal ("35th Anniversary").
var reEditionName = regexp.MustCompile(`(?i)(?:\b(\d+(?:st|nd|rd|th))\s+)?([\p{L}\d'’-]+)\s+edition\b`)

// ReadEdition reads what a name says about its numbering.
func ReadEdition(name string) EditionClaim {
	lower := strings.ToLower(name)
	var c EditionClaim
	type span struct{ from, to int }
	var spans []span
	first := span{-1, -1}
	found := func(at, length int, regroup bool) {
		spans = append(spans, span{at, at + length})
		if regroup && (first.from < 0 || at < first.from) {
			first = span{at, at + length}
		}
	}
	for _, r := range regroupings {
		for _, p := range r.phrases {
			for _, at := range wordIndexes(lower, p) {
				found(at, len(p), true)
				if !slices.Contains(c.Regroup, r.key) {
					c.Regroup = append(c.Regroup, r.key)
				}
			}
		}
	}
	for _, m := range reInOne.FindAllStringSubmatchIndex(lower, -1) {
		n := lower[m[2]:m[3]]
		if s, ok := spelled[n]; ok {
			n = s
		}
		found(m[0], m[1]-m[0], true)
		if key := n + "-in-1"; !slices.Contains(c.Regroup, key) {
			c.Regroup = append(c.Regroup, key)
		}
	}
	for _, p := range colorPhrases {
		for _, at := range wordIndexes(lower, p) {
			c.Color = true
			found(at, len(p), false)
		}
	}
	if len(wordIndexes(lower, "digital colored comics")) == 0 {
		for _, p := range colorEditionPhrases {
			for _, at := range wordIndexes(lower, p) {
				c.ColorEdition = true
				found(at, len(p), true)
			}
		}
	}
	for _, p := range boxPhrases {
		for _, at := range wordIndexes(lower, p) {
			c.Box = true
			found(at, len(p), false)
		}
	}
	slices.Sort(c.Regroup)
	explained := func(from, to int) bool {
		for _, s := range spans {
			if from < s.to && s.from < to {
				return true
			}
		}
		return false
	}
	for _, m := range reEditionName.FindAllStringSubmatchIndex(name, -1) {
		word := lower[m[4]:m[5]]
		if plainEditions[word] || explained(m[4], m[5]) {
			continue
		}
		start := m[4]
		if m[2] >= 0 {
			start = m[2]
		}
		c.Own = strings.ToLower(name[start:m[5]])
		c.Written = name[start:m[1]]
		return c
	}
	if first.from >= 0 {
		c.Written = name[first.from:first.to]
		if !strings.HasSuffix(strings.ToLower(c.Written), "edition") {
			c.Written += " Edition"
		}
	}
	return c
}

// wordIndexes is every whole-word occurrence of phrase in s.
func wordIndexes(s, phrase string) []int {
	var out []int
	for from := 0; from < len(s); {
		i := indexWord(s[from:], phrase)
		if i < 0 {
			break
		}
		out = append(out, from+i)
		from += i + len(phrase)
	}
	return out
}
