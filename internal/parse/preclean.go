package parse

import (
	"strings"

	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

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
