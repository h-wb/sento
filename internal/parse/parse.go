// Package parse turns release/file/chapter names into structured facts
// (docs/spec/parser.md). Parse is pure: no I/O, no context — that is what makes
// the regression corpus possible.
//
// Files follow the pipeline: preclean.go → segment.go → tags.go (segment
// classification) → numbers.go (extraction) → parse.go (orchestration);
// anchor.go is the title-aware entry point, num.go the number domain.
package parse

import (
	"regexp"
	"strconv"
	"strings"
)

func Parse(name string, ch Channel) Result {
	res := Result{ContentType: ContentManga, Confidence: 1}
	cleaned := preClean(name)
	res.name = strings.ToLower(cleaned)
	res.Claim = ReadEdition(cleaned)
	res.step("preclean", cleaned)
	parseInto(cleaned, ch, &res, "", nil)
	return res
}

// parseInto runs segmentation onward; anchoredTitle, when set, was already
// removed from the input by ParseAnchored (knownTitles are its aliases,
// used to excuse dual-title leftovers).
func parseInto(cleaned string, ch Channel, res *Result, anchoredTitle string, knownTitles []string) {
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
			// Mixed segments can contain both tags and explicit numbering. Bare
			// numbers in metadata (years, resolution, 3-in-1) are not book numbers.
			if h.year == nil && h.yearRange == nil && h.version == 0 {
				ex := extractNumbers(sg.text, res)
				res.Volumes = append(res.Volumes, ex.volumes...)
				res.Chapters = append(res.Chapters, ex.chapters...)
				if ex.version > res.Version {
					res.Version = ex.version
				}
			}
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

	// Where the anchored title ended, when it sits in the free text.
	anchorEnd := strings.IndexRune(freeText, anchorMark)
	if anchorEnd >= 0 {
		freeText = strings.Replace(freeText, string(anchorMark), strings.Repeat(" ", len(string(anchorMark))), 1)
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
	res.fold(scanLanguages(lowerFree, freeTextLangWords), freeText)
	res.fold(scanLanguages(freeText, sceneLangCodes), freeText)
	// Edition/quality tags also live in free text ("Deluxe Edition",
	// "Digital Colored Comics", scene-style "...RAW") — fold them through the
	// same vocabulary scanner as bracketed segments. Hint-only: it never cuts
	// the title (that's the keyword/number passes below).
	res.fold(scanTags(lowerFree), freeText)

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
		if i := indexWord(lowerFree, kw); i >= 0 {
			res.Extra = true
			res.step("keyword.extra", kw)
			// Trim the marker out of the title, but only when title text
			// precedes it ("One Piece Special" → "one piece"). A leading
			// marker is likely part of the title itself ("Special A").
			if (i > 0 || anchoredTitle != "") && i < titleEnd {
				titleEnd = i
			}
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
		if anchorEnd >= 0 && anchorEnd < len(lowerFree) {
			for _, marker := range editionMarkers {
				if i := indexWord(lowerFree[anchorEnd:], marker); i >= 0 && anchorEnd+i < titleEnd {
					titleEnd = anchorEnd + i
				}
			}
		}
		leftover := strings.TrimSpace(freeText[:max(titleEnd, 0)])
		res.TitleLeftover = leftoverWords(leftover, knownTitles)
		if res.TitleLeftover != "" {
			res.step("anchor.leftover", res.TitleLeftover)
		}
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
	// the fallback (parser.md §5.8). Curly braces carry notes ("{repack}",
	// "{HD}"), so one only names the group when nothing else can.
	var leading, trailing, note string
	for _, info := range infos {
		if info.recognized || info.hasNumbers {
			continue
		}
		switch {
		case info.seg.pos < freePos || freePos == -1 && info.seg.pos == 0:
			if leading == "" {
				leading = info.seg.text
			}
		case info.seg.delim == '{':
			note = info.seg.text
		default:
			trailing = info.seg.text
		}
	}
	switch {
	case trailing != "":
		res.Group = trailing
	case note != "":
		res.Group = note
	case leading != "" && res.Title != "":
		res.Group = leading
	}

	if res.Ongoing {
		res.Complete = false
	}
	// Implicit whole-series pack: torrent with no numbers at all AND a year
	// range ("Solo Leveling (2021-2026)"). A bare numberless title is more
	// likely a one-shot/spin-off than a pack.
	if ch == ChannelTorrent && len(res.Volumes)+len(res.Chapters)+len(res.Bare) == 0 &&
		!res.Ongoing && !res.Complete && res.Title != "" && res.YearRange != nil {
		res.Complete = true
		res.CompleteInferred = true
		res.step("implicit-pack", "no numbers + year range → whole series")
	}

	if ch == ChannelTorrent {
		detectMedia(cleaned, res)
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
	// Whatever stays in res.Bare is deliberately unresolved: nothing
	// downstream reads it for book coverage (only the implicit-pack count
	// and scoreConfidence look at it). An ambiguous number conservatively
	// matches no book rather than guessing an axis.
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

var rePureRange = regexp.MustCompile(`^\d+(?:\.\d+)?\s*[-–~]\s*\d+(?:\.\d+)?$`)

// isPureRangeSegment reports whether s is exactly "N-M" (e.g. a bracketed
// "[01-270]"), which the orchestrator reads as a chapter range.
func isPureRangeSegment(s string) bool {
	return rePureRange.MatchString(strings.TrimSpace(s))
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
