package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/store"
)

// RemoveOptions is what goes with a series besides its records.
type RemoveOptions struct {
	// DeleteFiles deletes every file Sento has on record for the series,
	// and the folders that leaves empty inside the library.
	DeleteFiles bool `json:"deleteFiles"`
	// RemoveDownloads takes the torrents Sento added for it out of the
	// download client, with their data. Imports are hardlinks: deleting the
	// library files frees no space while the torrent keeps its copy.
	RemoveDownloads bool `json:"removeDownloads"`
}

// RemovedSeries says what removing a series did beyond its records.
type RemovedSeries struct {
	FilesDeleted     int `json:"filesDeleted"`
	DownloadsRemoved int `json:"downloadsRemoved"`
	// Kept lists what was asked for but stayed, and why.
	Kept []string `json:"kept,omitempty"`
}

// RemoveSeries removes a series from Sento, and with opts its files and
// downloads. The records go first: that is the commitment. Disk and client
// clean-up after it is best effort, and what it could not do is reported
// rather than failing a removal that already happened.
func (im *Importer) RemoveSeries(ctx context.Context, id int64, opts RemoveOptions) (*RemovedSeries, error) {
	files, err := im.store.ListFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	jobs, err := im.store.SeriesQueueItems(ctx, id)
	if err != nil {
		return nil, err
	}
	type download struct {
		item   store.QueueItem
		shared bool
	}
	var downloads []download
	if opts.RemoveDownloads {
		seen := map[string]bool{}
		for _, j := range jobs {
			if j.Protocol != provider.ProtocolTorrent || j.ExternalID == "" || seen[j.ExternalID] {
				continue
			}
			seen[j.ExternalID] = true
			shared, err := im.store.DownloadShared(ctx, id, j.Protocol, j.ExternalID)
			if err != nil {
				return nil, err
			}
			downloads = append(downloads, download{j, shared})
		}
	}
	if err := im.store.DeleteSeries(ctx, id); err != nil {
		return nil, err
	}

	out := &RemovedSeries{}
	for _, d := range downloads {
		if d.shared {
			out.Kept = append(out.Kept, d.item.Title+": another series uses this download")
			continue
		}
		prov := im.registry.Get(d.item.ProviderID)
		if prov == nil {
			prov = im.registry.ForProtocol(d.item.Protocol)
		}
		remover, ok := prov.(provider.Remover)
		if !ok {
			out.Kept = append(out.Kept, d.item.Title+": the download client is not configured")
			continue
		}
		if err := remover.Remove(provider.InClient(ctx, d.item.DownloadClient), d.item.ExternalID); err != nil {
			out.Kept = append(out.Kept, d.item.Title+": "+err.Error())
			continue
		}
		out.DownloadsRemoved++
	}
	if opts.DeleteFiles {
		for _, f := range files {
			if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				out.Kept = append(out.Kept, f.Path+": "+err.Error())
				continue
			}
			out.FilesDeleted++
			im.pruneEmptyDirs(filepath.Dir(f.Path))
		}
	}
	return out, nil
}

// pruneEmptyDirs removes dir and its parents while they are empty, inside
// the library and never the library itself: a removed series leaves no
// empty folders behind, and nothing outside the library is touched.
func (im *Importer) pruneEmptyDirs(dir string) {
	if im.libraryRoot == "" {
		return
	}
	root := filepath.Clean(im.libraryRoot)
	for dir = filepath.Clean(dir); dir != root && strings.HasPrefix(dir, root+string(filepath.Separator)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil { // not empty, or not ours to remove
			return
		}
	}
}
