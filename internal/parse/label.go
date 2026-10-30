package parse

// Mentions reports whether the parsed name contains phrase as whole words.
// phrase must be lowercase.
func (r Result) Mentions(phrase string) bool {
	return phrase != "" && indexWord(r.name, phrase) >= 0
}
