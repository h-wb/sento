package parse

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ParseAnchored parses a name in the context of known candidate titles
// (series title + aliases). The best (longest) title occurrence is located
// and removed before number extraction, so numbers inside titles
// ("Mob Psycho 100") never leak into books (parser.md §5.9).
//
// Titles must be plain strings; they are normalized internally.
func ParseAnchored(name string, ch Channel, titles []string) Result {
	res := Result{ContentType: ContentManga, Confidence: 1}
	cleaned := preClean(name)
	res.name = strings.ToLower(cleaned)
	res.Claim = ReadEdition(cleaned)
	res.step("preclean", cleaned)

	folded := foldRunes(cleaned)

	bestLen := 0
	var bestSpan [2]int
	var bestTitle string
	for _, t := range titles {
		tokens := strings.Fields(string(foldRunes(preClean(t))))
		if len(tokens) == 0 {
			continue
		}
		if start, end, ok := findTokenSeq(folded, tokens); ok {
			matchLen := len(strings.Join(tokens, " "))
			if matchLen > bestLen {
				bestLen = matchLen
				bestSpan = [2]int{start, end}
				bestTitle = strings.Join(tokens, " ")
			}
		}
	}

	if bestTitle == "" {
		// No anchor found: fall back to blind parsing.
		res.step("anchor.miss", "no candidate title found in name")
		parseInto(cleaned, ch, &res, "", nil)
		return res
	}

	res.step("anchor.hit", bestTitle)
	res.Anchored = true
	runes := []rune(cleaned)
	for i := bestSpan[0]; i < bestSpan[1] && i < len(runes); i++ {
		runes[i] = ' '
	}
	// Leave a mark where the title ended, so the parse pass knows which
	// words come after it.
	if end := bestSpan[1] - 1; end >= bestSpan[0] && end < len(runes) {
		runes[end] = anchorMark
	}
	// What the name says about its edition is read around the title, not
	// in it: a series called "Perfect Blue Deluxe" claims nothing.
	res.Claim = ReadEdition(strings.ReplaceAll(string(runes), string(anchorMark), " "))
	parseInto(string(runes), ch, &res, bestTitle, titles)
	return res
}

// anchorMark stands in for the last rune of a blanked-out title. A
// private-use code point: it cannot occur in a real name.
const anchorMark = '\uE000'

// Words that end the title region once the title itself has been found:
// after "<title> - Deluxe Edition", what follows describes the release
// ("fixed blacks, reduced file size"), it is not more of a work's name.
// Before such a word, extra text still means a different work ("<title>
// Party - Deluxe Edition").
var editionMarkers = []string{"deluxe", "omnibus", "digital", "colored", "coloured", "full color", "box set", "boxset", "edition"}

// Edition/quality vocabulary that legitimately appears around a title
// ("One Piece - Digital Colored Comics v05") and must not count as
// title leftover.
var leftoverStopwords = map[string]bool{
	"digital": true, "colored": true, "coloured": true, "color": true,
	"colour": true, "full": true, "comics": true, "comic": true,
	"deluxe": true, "omnibus": true, "edition": true, "box": true,
	"set": true, "raw": true, "the": true, "complete": true, "batch": true,
	"cover": true, "stories": true, "translated": true, "archived": true,
	"scanlated": true,
}

// leftoverWords reduces the unexplained title-region text around an anchor
// to meaningful words. Dual-title segments that fuzzy-match another known
// title/alias are dropped ("Only I Level Up / Solo Leveling").
func leftoverWords(leftover string, knownTitles []string) string {
	if strings.TrimSpace(leftover) == "" {
		return ""
	}
	var folded []string
	for _, t := range knownTitles {
		folded = append(folded, strings.Join(strings.Fields(string(foldRunes(preClean(t)))), " "))
	}

	var keep []string
	for _, segment := range strings.FieldsFunc(leftover, func(r rune) bool {
		return r == '/' || r == '|'
	}) {
		words := strings.Fields(string(foldRunes(segment)))
		var meaningful []string
		for _, w := range words {
			if !leftoverStopwords[w] && len(w) > 1 {
				meaningful = append(meaningful, w)
			}
		}
		if len(meaningful) == 0 {
			continue
		}
		joined := strings.Join(meaningful, " ")
		alias := false
		for _, t := range folded {
			if titleSimilar(joined, t) {
				alias = true
				break
			}
		}
		if !alias {
			keep = append(keep, joined)
		}
	}
	return strings.Join(keep, " ")
}

// titleSimilar is a cheap token-overlap check (≥75% of the shorter side).
func titleSimilar(a, b string) bool {
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, w := range tb {
		set[w] = true
	}
	hits := 0
	for _, w := range ta {
		if set[w] {
			hits++
		}
	}
	short := min(len(ta), len(tb))
	return hits*4 >= short*3
}

// foldRunes maps each rune 1:1 (for offset stability) to a lowercase,
// diacritic-free equivalent, or ' ' for non-alphanumerics.
func foldRunes(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, unicode.ToLower(r))
		case r > 127 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			// Strip diacritics: NFD-decompose, keep the base rune.
			base := []rune(norm.NFD.String(string(r)))
			if len(base) > 0 && base[0] < 128 {
				out = append(out, unicode.ToLower(base[0]))
			} else {
				out = append(out, unicode.ToLower(r)) // CJK etc.: keep as-is
			}
		default:
			out = append(out, ' ')
		}
	}
	return out
}

// findTokenSeq locates tokens as a whole-word sequence in folded runes,
// allowing any run of spaces between tokens. Returns rune offsets.
func findTokenSeq(folded []rune, tokens []string) (int, int, bool) {
	n := len(folded)
	for start := 0; start < n; start++ {
		// word boundary at start
		if folded[start] == ' ' || (start > 0 && folded[start-1] != ' ') {
			continue
		}
		pos := start
		ok := true
		for ti, tok := range tokens {
			// skip spaces between tokens
			if ti > 0 {
				skipped := false
				for pos < n && folded[pos] == ' ' {
					pos++
					skipped = true
				}
				if !skipped {
					ok = false
					break
				}
			}
			tr := []rune(tok)
			if pos+len(tr) > n || string(folded[pos:pos+len(tr)]) != tok {
				ok = false
				break
			}
			pos += len(tr)
		}
		if ok && (pos >= n || folded[pos] == ' ') {
			return start, pos, true
		}
	}
	return 0, 0, false
}
