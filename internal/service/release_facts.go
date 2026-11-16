package service

import (
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
)

// parseFlags reads the boolean parse facts named in profile.Flags.
func parseFlags(p parse.Result) map[string]bool {
	return map[string]bool{
		"digital":  p.Quality.Digital,
		"scan":     p.Quality.Scan,
		"web":      p.Quality.Web,
		"premium":  p.Quality.Premium,
		"colored":  p.Edition.Colored,
		"omnibus":  p.Edition.Omnibus,
		"deluxe":   p.Edition.Deluxe,
		"boxset":   p.Edition.BoxSet,
		"official": p.Edition.Official,
		"raw":      p.Edition.Raw,
		"complete": p.Complete,
	}
}

func languageCompatible(p parse.Result, want string) bool {
	if want == "" {
		return true
	}
	if len(p.Languages) > 0 {
		for _, lang := range p.Languages {
			if profile.BaseLang(lang) == profile.BaseLang(want) {
				return true
			}
		}
		return false
	}
	return p.Language == "" || p.Language == "multi" || profile.BaseLang(p.Language) == profile.BaseLang(want)
}
