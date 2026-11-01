package parse

import (
	"strings"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

// archiveExts is the *parser's* strip list: any extension that can wrap a
// book's pages or text, so preClean removes it before matching names. It is
// wider than mediafile.IsArchive on purpose — .7z/.cb7 cannot be rewritten
// by Process, but a name ending in them is still a book file, and the parse
// must see the bare title. mediafile keeps the narrower rewriteable set.
var archiveExts = map[string]bool{
	".cbz": true, ".cbr": true, ".cb7": true, ".zip": true, ".rar": true,
	".7z": true, ".pdf": true, ".epub": true,
}

// preClean canonicalizes a name before any pattern runs: every later stage
// assumes single spaces, ASCII-width characters and composed Unicode.
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
