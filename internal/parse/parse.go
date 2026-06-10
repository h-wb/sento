// Package parse turns release/file/chapter names into structured facts
// (specs/parser.md). Parse is pure: no I/O, no context — that is what makes
// the regression corpus possible.
package parse

import (
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// ---- pre-clean ----

var archiveExts = map[string]bool{
	".cbz": true, ".cbr": true, ".cb7": true, ".zip": true, ".rar": true,
	".7z": true, ".pdf": true, ".epub": true,
}

func preClean(name string) string {
	// Strip a recognized archive extension (file channel).
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		if archiveExts[strings.ToLower(name[i:])] {
			name = name[:i]
		}
	}
	name = strings.ReplaceAll(name, "_", " ")
	name = width.Fold.String(name) // full-width → ASCII
	name = norm.NFKC.String(name)
	name = strings.Join(strings.Fields(name), " ")
	return name
}

// ---- segmentation ----

type segment struct {
	text  string // inner text, delimiters stripped
	delim byte   // '[', '(', '{', or 0 for free text
	pos   int    // order of appearance
}

var delimPairs = map[byte]byte{'[': ']', '(': ')', '{': '}'}

func segments(s string) []segment {
	var out []segment
	var free strings.Builder
	pos := 0
	flushFree := func() {
		if t := strings.TrimSpace(free.String()); t != "" {
			out = append(out, segment{text: t, delim: 0, pos: pos})
			pos++
		}
		free.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if close, ok := delimPairs[c]; ok {
			if j := strings.IndexByte(s[i+1:], close); j >= 0 {
				flushFree()
				out = append(out, segment{text: strings.TrimSpace(s[i+1 : i+1+j]), delim: c, pos: pos})
				pos++
				i += j + 1
				continue
			}
		}
		free.WriteByte(c)
	}
	flushFree()
	return out
}

// ---- number extraction ----

type span struct{ start, end int }

type extraction struct {
	volumes  []Range
	chapters []Range
	bare     []Range
	version  int
	spans    []span // consumed regions (for title cutting)
}

var (
	// Volume markers: v01, vol. 1, Volume 01, Tome 1, T01, 第1巻 (ranges allowed).
	reVolMarker = regexp.MustCompile(`(?i)\b(?:vol(?:ume)?|tome)\.?\s*(\d+(?:\.\d+)?)(?:\s*[-–~]\s*(?:v(?:ol(?:ume)?)?\.?\s*)?(\d+(?:\.\d+)?))?|\b[vV](\d+(?:\.\d+)?)(?:\s*[-–~]\s*[vV]?(\d+(?:\.\d+)?))?|\b[tT](\d+)(?:\s*[-–~]\s*[tT]?(\d+))?\b`)
	reVolJP     = regexp.MustCompile(`第(\d+)(?:\s*[-–~]\s*(\d+))?巻`)
	// Chapter markers: c001, ch. 4, Chapter 1184, Episode 12, #34, 第12話.
	reChMarker = regexp.MustCompile(`(?i)\b(?:ch(?:ap(?:ter)?)?|episode|ep)\.?\s*(\d+(?:\.\d+)?)([a-z])?(?:\s*[-–~]\s*(?:ch?\.?\s*)?(\d+(?:\.\d+)?))?|\b[cC](\d+(?:\.\d+)?)(?:\s*[-–~]\s*[cC]?(\d+(?:\.\d+)?))?\b|#(\d+(?:\.\d+)?)`)
	reChJP     = regexp.MustCompile(`第(\d+(?:\.\d+)?)(?:\s*[-–~]\s*(\d+(?:\.\d+)?))?話`)
	// Release-version suffix directly after a number: 34v2.
	reVerSuffix = regexp.MustCompile(`(?i)(\d)\s*v(\d)\b`)
	// Trailing dash-bound chapter: "[Kamizye] Berserk - 375".
	reDashCh = regexp.MustCompile(`\s[-–]\s*(\d+(?:\.\d+)?)\s*$`)
	// Bare numbers / ranges.
	reBare = regexp.MustCompile(`(\d+(?:\.\d+)?)(?:\s*[-–~]\s*(\d+(?:\.\d+)?))?`)
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

	for _, re := range []*regexp.Regexp{reVolJP} {
		for _, m := range re.FindAllStringSubmatchIndex(string(buf), -1) {
			if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 2)); ok {
				consume(m[0:2], r, "volume")
			}
		}
	}
	for _, m := range reChJP.FindAllStringSubmatchIndex(string(buf), -1) {
		if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 2)); ok {
			consume(m[0:2], r, "chapter")
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
		if r, ok := rangeFrom(group(buf, m, 1), group(buf, m, 2)); ok {
			ex.spans = append(ex.spans, span{m[0], m[1]})
			ex.bare = append(ex.bare, r)
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

// ---- main entry points ----

func Parse(name string, ch Channel) Result {
	res := Result{ContentType: ContentManga, Confidence: 1}
	cleaned := preClean(name)
	res.step("preclean", cleaned)
	parseInto(cleaned, ch, &res, "")
	return res
}

// parseInto runs segmentation onward; anchoredTitle, when set, was already
// removed from the input by ParseAnchored.
func parseInto(cleaned string, ch Channel, res *Result, anchoredTitle string) {
	segs := segments(cleaned)

	infos := make([]segInfo, 0, len(segs))
	var freeText string
	freePos := -1

	for _, sg := range segs {
		if sg.delim == 0 {
			// Concatenate free-text runs (rare to have >1 around segments).
			if freePos == -1 {
				freePos = sg.pos
				freeText = sg.text
			} else {
				freeText += " " + sg.text
			}
			continue
		}
		h := classifySegment(sg.text)
		info := segInfo{seg: sg, recognized: h.recognized}
		if h.recognized {
			res.fold(h, sg.text)
		} else {
			// Unrecognized: number-bearing segment or group candidate.
			ex := extractNumbers(sg.text, res)
			if len(ex.volumes)+len(ex.chapters)+len(ex.bare) > 0 {
				info.hasNumbers = true
				res.Volumes = append(res.Volumes, ex.volumes...)
				res.Chapters = append(res.Chapters, ex.chapters...)
				// A bracketed pure range is a chapter range by convention
				// ("[01-270]", parser.md §5.3).
				if len(ex.volumes) == 0 && len(ex.bare) > 0 && isPureRangeSegment(sg.text) {
					res.Chapters = append(res.Chapters, ex.bare...)
					res.step("segment.range→chapters", sg.text)
				} else {
					res.Bare = append(res.Bare, ex.bare...)
				}
				if ex.version > res.Version {
					res.Version = ex.version
				}
			}
		}
		infos = append(infos, info)
	}

	// Free text: season, keywords, numbers, then the title.
	titleEnd := len(freeText)

	if m := reSeason.FindStringSubmatchIndex(freeText); m != nil {
		s := firstGroup([]byte(freeText), m, 1, 2)
		if v, err := strconv.Atoi(s); err == nil {
			res.Season = &v
			res.step("season", freeText[m[0]:m[1]])
			if m[0] < titleEnd {
				titleEnd = m[0]
			}
			freeText = freeText[:m[0]] + strings.Repeat(" ", m[1]-m[0]) + freeText[m[1]:]
		}
	}

	// Edition phrases also occur in free text ("Deluxe Edition",
	// "Digital Colored Comics") — hint-scan without cutting the title.
	lowerFree := strings.ToLower(freeText)
	if strings.Contains(lowerFree, "deluxe") {
		res.Edition.Deluxe = true
	}
	if strings.Contains(lowerFree, "omnibus") {
		res.Edition.Omnibus = true
	}
	if strings.Contains(lowerFree, "box set") || strings.Contains(lowerFree, "boxset") {
		res.Edition.BoxSet = true
	}
	if strings.Contains(lowerFree, "colored") || strings.Contains(lowerFree, "full color") {
		res.Edition.Colored = true
	}

	for _, kw := range []string{"complete", "completed", "batch", "oneshot"} {
		if i := indexWord(lowerFree, kw); i >= 0 {
			if kw == "complete" || kw == "completed" {
				res.Complete = true
			}
			res.step("keyword", kw)
			if i < titleEnd {
				titleEnd = i
			}
		}
	}
	for _, kw := range []string{"extra", "omake", "special"} {
		if indexWord(lowerFree, kw) >= 0 {
			res.Extra = true
			res.step("keyword.extra", kw)
		}
	}

	ex := extractNumbers(freeText, res)
	res.Volumes = append(res.Volumes, ex.volumes...)
	res.Chapters = append(res.Chapters, ex.chapters...)
	if ex.version > res.Version {
		res.Version = ex.version
	}
	for _, sp := range ex.spans {
		if sp.start < titleEnd {
			titleEnd = sp.start
		}
	}
	res.Bare = append(res.Bare, ex.bare...)

	resolveBare(res, ch)
	res.Volumes = mergeRanges(res.Volumes)
	res.Chapters = mergeRanges(res.Chapters)

	// Title.
	if anchoredTitle != "" {
		res.Title = anchoredTitle
	} else {
		title := strings.TrimSpace(freeText[:max(titleEnd, 0)])
		title = strings.TrimRight(title, " -–~:|/+&")
		parts := splitDualTitle(title)
		if len(parts) > 0 {
			res.Title = strings.ToLower(parts[0])
			for _, p := range parts[1:] {
				res.AltTitles = append(res.AltTitles, strings.ToLower(p))
			}
		}
	}

	// Group: last unrecognized number-free segment wins; a leading one is
	// the fallback (parser.md §5.8).
	var leading, trailing string
	for _, info := range infos {
		if info.recognized || info.hasNumbers {
			continue
		}
		if info.seg.pos < freePos || freePos == -1 && info.seg.pos == 0 {
			if leading == "" {
				leading = info.seg.text
			}
		} else {
			trailing = info.seg.text
		}
	}
	switch {
	case trailing != "":
		res.Group = trailing
	case leading != "" && res.Title != "":
		res.Group = leading
	}

	// Implicit whole-series pack: torrent with no numbers at all.
	if ch == ChannelTorrent && len(res.Volumes)+len(res.Chapters)+len(res.Bare) == 0 &&
		!res.Ongoing && res.Title != "" {
		res.Complete = true
		res.step("implicit-pack", "no numbers → whole series")
	}

	scoreConfidence(res, infos)
}

// resolveBare applies the channel priors from parser.md §4 to bare numbers.
func resolveBare(res *Result, ch Channel) {
	if len(res.Bare) == 0 {
		return
	}
	var maxVol Num
	for _, v := range res.Volumes {
		if v.To > maxVol {
			maxVol = v.To
		}
	}
	var remaining []Range
	for _, b := range res.Bare {
		switch {
		case ch == ChannelSource:
			res.Chapters = append(res.Chapters, b)
			res.step("bare→chapter", "source channel")
		case b.From%100 != 0:
			res.Chapters = append(res.Chapters, b)
			res.step("bare→chapter", "decimal")
		case b.From >= 1000*100:
			res.Chapters = append(res.Chapters, b)
			res.step("bare→chapter", "≥1000")
		case maxVol > 0 && b.From > maxVol*2:
			res.Chapters = append(res.Chapters, b)
			res.step("bare→chapter", "far above marker volumes")
		default:
			remaining = append(remaining, b)
		}
	}
	res.Bare = remaining
}

type segInfo struct {
	seg        segment
	recognized bool
	hasNumbers bool
}

func scoreConfidence(res *Result, infos []segInfo) {
	c := 1.0
	for _, info := range infos {
		if !info.recognized && !info.hasNumbers && info.seg.text != res.Group {
			c -= 0.05
		}
	}
	if len(res.Bare) > 0 {
		c -= 0.1
	}
	if res.Title == "" {
		c -= 0.3
	}
	if len(res.Volumes)+len(res.Chapters)+len(res.Bare) == 0 && !res.Complete {
		c -= 0.2
	}
	if c < 0 {
		c = 0
	}
	res.Confidence = c
}

// ---- helpers ----

func isPureRangeSegment(s string) bool {
	s = strings.TrimSpace(s)
	return regexp.MustCompile(`^\d+(?:\.\d+)?\s*[-–~]\s*\d+(?:\.\d+)?$`).MatchString(s)
}

func splitDualTitle(title string) []string {
	for _, sep := range []string{" / ", " | "} {
		if strings.Contains(title, sep) {
			parts := strings.Split(title, sep)
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return parts
		}
	}
	if title == "" {
		return nil
	}
	return []string{title}
}

func indexWord(s, word string) int {
	idx := 0
	for {
		i := strings.Index(s[idx:], word)
		if i < 0 {
			return -1
		}
		i += idx
		before := i == 0 || !isAlnum(s[i-1])
		after := i+len(word) >= len(s) || !isAlnum(s[i+len(word)])
		if before && after {
			return i
		}
		idx = i + len(word)
	}
}
