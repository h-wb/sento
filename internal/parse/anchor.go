package parse

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// ParseAnchored parses a name in the context of known candidate titles
// (series title + aliases). The best (longest) title occurrence is located
// and removed before number extraction, so numbers inside titles
// ("Mob Psycho 100") never leak into slots (parser.md §5.9).
//
// Titles must be plain strings; they are normalized internally.
func ParseAnchored(name string, ch Channel, titles []string) Result {
	res := Result{ContentType: ContentManga, Confidence: 1}
	cleaned := preClean(name)
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
		parseInto(cleaned, ch, &res, "")
		return res
	}

	res.step("anchor.hit", bestTitle)
	res.Anchored = true
	runes := []rune(cleaned)
	for i := bestSpan[0]; i < bestSpan[1] && i < len(runes); i++ {
		runes[i] = ' '
	}
	parseInto(string(runes), ch, &res, bestTitle)
	return res
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
