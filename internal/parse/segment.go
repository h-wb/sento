package parse

import "strings"

// A segment is one delimited run of a cleaned name: "[Group] Title (2020)"
// yields a '['-segment, free text, and a '('-segment. Appearance order is
// kept — group election depends on it (parser.md §5.8).
type segment struct {
	text  string // inner text, delimiters stripped
	delim byte   // '[', '(', '{', or 0 for free text
	pos   int    // order of appearance
}

var delimPairs = map[byte]byte{'[': ']', '(': ')', '{': '}'}

// segments splits a cleaned name into delimited segments and free text.
// Flat scan, no nesting: a segment ends at the first matching closer, and
// an opener with no closer degrades to free text ("Berserk [incomplete").
// Free-text runs between segments are emitted separately, not merged —
// parseInto concatenates them and keeps the first run's position.
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
