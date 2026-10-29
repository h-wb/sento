package parse

import "strings"

// EditionTags reads an edition's name ("3-in-1 Edition", "Deluxe Box Set")
// with the vocabulary release names are read with, so an edition and a
// release that say the same thing are understood the same way.
func EditionTags(label string) EditionHints {
	return scanTags(strings.ToLower(label)).edition
}

// Name is the cleaned, lowercased name that was parsed.
func (r Result) Name() string { return r.name }

// Mentions reports whether the parsed name contains phrase as whole words.
// phrase must be lowercase.
func (r Result) Mentions(phrase string) bool {
	return phrase != "" && indexWord(r.name, phrase) >= 0
}
