package service

import (
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/profile"
	"github.com/h-wb/sento/internal/store"
)

func editionByID(cols []store.Edition, id int64) *store.Edition {
	for i := range cols {
		if cols[i].ID == id {
			return &cols[i]
		}
	}
	return nil
}

// parseFlags reads the boolean parse facts named in profile.Flags.
func parseFlags(p parse.Result) map[string]bool {
	flags := make(map[string]bool, len(profile.Flags))
	for _, f := range profile.Flags {
		switch f {
		case "digital":
			flags[f] = p.Quality.Digital
		case "scan":
			flags[f] = p.Quality.Scan
		case "web":
			flags[f] = p.Quality.Web
		case "premium":
			flags[f] = p.Quality.Premium
		case "colored":
			flags[f] = p.Edition.Colored
		case "omnibus":
			flags[f] = p.Edition.Omnibus
		case "deluxe":
			flags[f] = p.Edition.Deluxe
		case "boxset":
			flags[f] = p.Edition.BoxSet
		case "official":
			flags[f] = p.Edition.Official
		case "raw":
			flags[f] = p.Edition.Raw
		case "complete":
			flags[f] = p.Complete
		}
	}
	return flags
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
