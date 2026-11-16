package service

import (
	"context"
	"sync"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// LibraryFiles makes every change to the files in the library: review,
// organization, upgrades, removal and trash.
type LibraryFiles struct {
	store       *store.Store
	registry    *provider.Registry
	libraryRoot string

	// mu serializes queue passes and file mutations (review, organization,
	// removal and trash), so decisions and disk operations cannot interleave.
	mu sync.Mutex

	organizer libraryOrganizer

	// again holds the series whose download failed since the last
	// TakeSearches: they are searched again within minutes, not at the
	// next sweep.
	again *retrySearches
}

func NewLibraryFiles(st *store.Store, registry *provider.Registry, libraryRoot string) *LibraryFiles {
	return &LibraryFiles{store: st, registry: registry, libraryRoot: libraryRoot, again: &retrySearches{}}
}

// seriesDir returns the folder for a series' books of the given type
// under the current settings, recording it on the series.
func (lf *LibraryFiles) seriesDir(ctx context.Context, series *store.Series, ms MediaSettings, ledger string) (string, error) {
	volumes, chapters := seriesFolders(lf.libraryRoot, series, ms)
	if series.Path != volumes {
		if err := lf.store.SetSeriesPath(ctx, series.ID, volumes); err != nil {
			return "", err
		}
		series.Path = volumes
	}
	separate := ""
	if chapters != volumes {
		separate = chapters
	}
	if series.ChapterPath != separate {
		if err := lf.store.SetSeriesChapterPath(ctx, series.ID, separate); err != nil {
			return "", err
		}
		series.ChapterPath = separate
	}
	if ledger == "chapter" {
		return chapters, nil
	}
	return volumes, nil
}
