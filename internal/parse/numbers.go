package parse

import (
	"regexp"
	"strconv"
)

type span struct{ start, end int }

type extraction struct {
	volumes  []Range
	chapters []Range
	bare     []Range
	padded   []Range // bare numbers zero-padded to three or more digits
	version  int
	spans    []span // consumed regions (for title cutting)
}

var (
	// Volume markers: v01, vol. 1, Volume 01, Tome 1, Tomo 3, Band 3, Kapitel 4, T01, 第1巻 (ranges allowed).
	reVolMarker = regexp.MustCompile(`(?i)\b(?:vol(?:ume)?s?|t[o0]mes?|tomos?|band|bände|kapitel)\.?\s*(\d+(?:\.\d+)?)(?:\s*(?:[-–~]|à|au|to)\s*(?:(?:v(?:ol(?:ume)?)?|t(?:ome)?|tomo|band|kapitel)\.?\s*)?(\d+(?:\.\d+)?))?|\b[vV](\d+(?:\.\d+)?)(?:\s*[-–~]\s*[vV]?(\d+(?:\.\d+)?))?|\b[tT](\d+)(?:\s*(?:[-–~]|à|au|to)\s*[tT]?(\d+))?\b`)
	reVolList   = regexp.MustCompile(`(?i)\b(?:vol(?:ume)?s?|t[o0]mes?|tomos?|band|kapitel|[tv])\.?\s*(\d+(?:\.\d+)?(?:\s*(?:,|&|et|und|y)\s*(?:[tv]\s*)?\d+(?:\.\d+)?)+)`)
	reVolJP     = regexp.MustCompile(`第?(\d+)(?:\s*[-–~]\s*(\d+))?巻`)
	// Chapter markers: c001, ch. 4, Chapter 1184, Episode 12, #34, 第12話.
	reChMarker = regexp.MustCompile(`(?i)\b(?:ch(?:apitres?|ap(?:ters?)?|s)?|episodes?|ep)\.?\s*(\d+(?:\.\d+)?)([a-z])?(?:\s*[-–~]\s*(?:ch?\.?\s*)?(\d+(?:\.\d+)?))?|\b[cC](\d+(?:\.\d+)?)(?:\s*[-–~]\s*[cC]?(\d+(?:\.\d+)?))?\b|#(\d+(?:\.\d+)?)`)
	reChJP     = regexp.MustCompile(`第?(\d+(?:\.\d+)?)(?:\s*[-–~]\s*(\d+(?:\.\d+)?))?話`)
	// Release-version suffix directly after a number: 34v2.
	reVerSuffix = regexp.MustCompile(`(?i)(\d)\s*v(\d)\b`)
	// Trailing dash-bound chapter: "[Kamizye] Berserk - 375".
	reDashCh = regexp.MustCompile(`\s[-–]\s*(\d+(?:\.\d+)?)\s*$`)
	// Bare numbers / ranges, with an optional alpha sub-chapter suffix
	// ("1150a" → 1150.1, the Mihon a→.1 convention also used by reChMarker).
	reBare = regexp.MustCompile(`(\d+(?:\.\d+)?)([a-i])?(?:\s*[-–~]\s*(\d+(?:\.\d+)?))?`)
	// Season: S2, Season 2.
	reSeason = regexp.MustCompile(`\bS(\d{1,2})\b|(?i)\bseason\s*(\d{1,2})\b`)
)

func rangeFrom(fromStr, toStr string) (Range, bool) {
	from, ok := parseNum(fromStr)
	if !ok {
		return Range{}, false
	}
	to := from
	if toStr != "" {
		if t, ok := parseNum(toStr); ok && t >= from {
			to = t
		}
	}
	return Range{From: from, To: to}, true
}

// extractNumbers pulls marker-bound numbers out of text, blanking consumed
// spans, then collects bare numbers from what remains.
func extractNumbers(text string, res *Result) extraction {
	var ex extraction
	buf := []byte(text)

	consume := func(loc []int, r Range, axis string) {
		ex.spans = append(ex.spans, span{loc[0], loc[1]})
		for i := loc[0]; i < loc[1]; i++ {
			buf[i] = ' '
		}
		switch axis {
		case "volume":
			ex.volumes = append(ex.volumes, r)
		case "chapter":
			ex.chapters = append(ex.chapters, r)
		}
		res.step("number."+axis, text[loc[0]:loc[1]])
	}

	// Version suffix first (34v2) so the v2 isn't read as a volume: blank
	// from after the chapter digit through the end of the match.
	if m := reVerSuffix.FindStringSubmatchIndex(string(buf)); m != nil {
		v, _ := strconv.Atoi(string(buf[m[4]:m[5]]))
		ex.version = v
		for i := m[3]; i < m[1]; i++ {
			buf[i] = ' '
		}
		res.step("version.suffix", text[m[0]:m[1]])
	}

	for _, m := range reVolJP.FindAllStringSubmatchIndex(string(buf), -1) {
		if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 2)); ok {
			consume(m[0:2], r, "volume")
		}
	}
	for _, m := range reChJP.FindAllStringSubmatchIndex(string(buf), -1) {
		if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 2)); ok {
			consume(m[0:2], r, "chapter")
		}
	}

	// Explicit lists keep their gaps: “Tomes 2, 4 et 7” is not 2–7.
	for _, m := range reVolList.FindAllStringSubmatchIndex(string(buf), -1) {
		body := group(buf, m, 1)
		for _, n := range reBare.FindAllStringSubmatch(body, -1) {
			if r, ok := rangeFrom(n[1], ""); ok {
				consume(m[0:2], r, "volume")
			}
		}
	}

	for _, m := range reVolMarker.FindAllStringSubmatchIndex(string(buf), -1) {
		from := firstGroup(buf, m, 1, 3, 5)
		to := firstGroup(buf, m, 2, 4, 6)
		if r, ok := rangeFrom(from, to); ok {
			consume(m[0:2], r, "volume")
		}
	}

	for _, m := range reChMarker.FindAllStringSubmatchIndex(string(buf), -1) {
		from := firstGroup(buf, m, 1, 4, 6)
		to := firstGroup(buf, m, 3, 5)
		if r, ok := rangeFrom(from, to); ok {
			// alpha sub-chapter suffix (ch 4a)
			if a := group(buf, m, 2); a != "" && r.From == r.To {
				add := alphaSuffixNum(a[0])
				r.From += add
				r.To += add
			}
			consume(m[0:2], r, "chapter")
		}
	}

	// Trailing dash-bound chapter ("Berserk - 375") — only if nothing
	// marker-bound consumed that region already.
	if m := reDashCh.FindStringSubmatchIndex(string(buf)); m != nil {
		if r, ok := rangeFrom(group(buf, m, 1), ""); ok {
			consume(m[0:2], r, "chapter")
		}
	}

	// Bare numbers from the remainder.
	for _, m := range reBare.FindAllStringSubmatchIndex(string(buf), -1) {
		if !cleanBoundary(buf, m[0], m[1]) {
			continue
		}
		if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 3)); ok {
			// Alpha sub-chapter suffix ("1150a" → 1150.1); cleanBoundary
			// already rejected j–z glue, so only a–i reach here.
			if a := group(buf, m, 2); a != "" && r.From == r.To {
				add := alphaSuffixNum(a[0])
				r.From += add
				r.To += add
			}
			ex.spans = append(ex.spans, span{m[0], m[1]})
			ex.bare = append(ex.bare, r)
			// "001" keeps its padding as a fact: three or more digits
			// behind a leading zero is the chapter-pack convention.
			if digits := group(buf, m, 1); len(digits) >= 3 && digits[0] == '0' && digits[1] != '.' {
				ex.padded = append(ex.padded, r)
			}
			res.step("number.bare", text[m[0]:m[1]])
		}
	}

	return ex
}

func group(buf []byte, m []int, n int) string {
	if 2*n+1 >= len(m) || m[2*n] < 0 {
		return ""
	}
	return string(buf[m[2*n]:m[2*n+1]])
}

func firstGroup(buf []byte, m []int, ns ...int) string {
	for _, n := range ns {
		if g := group(buf, m, n); g != "" {
			return g
		}
	}
	return ""
}

// cleanBoundary rejects digit runs glued to letters ("S2", "1r0n", "mp3").
func cleanBoundary(buf []byte, start, end int) bool {
	if start > 0 {
		c := buf[start-1]
		if isAlnum(c) || c == '.' || c == '#' {
			return false
		}
	}
	if end < len(buf) {
		c := buf[end]
		if isAlnum(c) || c == '.' {
			return false
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
