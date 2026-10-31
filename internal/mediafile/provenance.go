package mediafile

import (
	"slices"
	"strconv"
	"strings"
)

// Provenance is what Sento's line in a ComicInfo's Notes says about the
// file: which series, edition and books it is, and the release it came as
// (docs/domain/metadata.md). Facts only; a rescan adopts the file from it.
type Provenance struct {
	MangaBaka int64
	Edition   string
	// CatalogEdition is MangaBaka's edition id; empty for a local edition.
	CatalogEdition string
	Ledger         string
	// Books are the numbers the file holds, as file names write them.
	Books []string
	// Works are the MangaBaka work ids of those books, when known.
	Works   []string
	Release string
	Via     string
	Site    string
	Flags   []string
}

const provenanceTag = "sento:v1"

// Notes renders the line: "sento:v1" first, then key:value pairs, a value
// quoted when it holds a space or a quote, lists comma-joined.
func (p Provenance) Notes() string {
	parts := []string{provenanceTag}
	add := func(key, value string) {
		if value == "" {
			return
		}
		if strings.ContainsAny(value, " \"\\") || value != strings.TrimSpace(value) {
			value = strconv.Quote(value)
		}
		parts = append(parts, key+":"+value)
	}
	if p.MangaBaka > 0 {
		add("mangabaka", strconv.FormatInt(p.MangaBaka, 10))
	}
	add("edition", p.Edition)
	add("mb-edition", p.CatalogEdition)
	add("ledger", p.Ledger)
	add("books", strings.Join(p.Books, " "))
	add("work", strings.Join(p.Works, ","))
	add("release", p.Release)
	add("via", p.Via)
	add("site", p.Site)
	add("flags", strings.Join(p.Flags, ","))
	return strings.Join(parts, " ")
}

// ReadProvenance reads Sento's line out of a Notes field; ok is false when
// the notes hold none. Unknown keys are skipped.
func ReadProvenance(notes string) (p Provenance, ok bool) {
	at := strings.Index(notes, provenanceTag)
	if at < 0 {
		return p, false
	}
	rest := notes[at+len(provenanceTag):]
	for {
		rest = strings.TrimLeft(rest, " \t\r\n")
		colon := strings.IndexByte(rest, ':')
		if rest == "" || colon < 0 {
			break
		}
		key := rest[:colon]
		if strings.ContainsAny(key, " \t\r\n") {
			// A word without a value: skip it.
			rest = rest[strings.IndexAny(rest, " \t\r\n"):]
			continue
		}
		rest = rest[colon+1:]
		var value string
		if strings.HasPrefix(rest, `"`) {
			quoted, err := strconv.QuotedPrefix(rest)
			if err != nil {
				break
			}
			value, _ = strconv.Unquote(quoted)
			rest = rest[len(quoted):]
		} else {
			end := strings.IndexAny(rest, " \t\r\n")
			if end < 0 {
				end = len(rest)
			}
			value, rest = rest[:end], rest[end:]
		}
		list := func(sep string) []string {
			return slices.DeleteFunc(strings.Split(value, sep), func(s string) bool { return s == "" })
		}
		switch key {
		case "mangabaka":
			p.MangaBaka, _ = strconv.ParseInt(value, 10, 64)
		case "edition":
			p.Edition = value
		case "mb-edition":
			p.CatalogEdition = value
		case "ledger":
			p.Ledger = value
		case "books":
			p.Books = list(" ")
		case "work":
			p.Works = list(",")
		case "release":
			p.Release = value
		case "via":
			p.Via = value
		case "site":
			p.Site = value
		case "flags":
			p.Flags = list(",")
		}
	}
	return p, true
}
