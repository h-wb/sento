package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
)

type payloadArchive struct {
	name, path string
	review     bool
}

// Only an explicit volume/chapter folder marker may inherit the series
// name from the job. Bare numbers, parts and ranges are left for review.
var imageBookFolder = regexp.MustCompile(`(?i)^(?:v(?:ol(?:ume)?)?|t(?:ome)?|ch(?:apter|apitre)?|c)\.?\s*\d+(?:\.\d+)?$`)

func payloadArchives(ctx context.Context, dir string, files, titles []string) (archives []payloadArchive, cleanup func(), err error) {
	cleanup = func() {}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, cleanup, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, cleanup, err
	}
	images := map[string][]string{}
	withArchive := map[string]bool{}
	seen := map[string]bool{}
	for _, rel := range files {
		if !filepath.IsLocal(rel) {
			return nil, cleanup, fmt.Errorf("invalid payload path")
		}
		rel = filepath.Clean(rel)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		path, e := filepath.EvalSymlinks(filepath.Join(root, rel))
		if e != nil {
			return nil, cleanup, e
		}
		if !strings.HasPrefix(path, root+string(filepath.Separator)) {
			return nil, cleanup, fmt.Errorf("payload path escapes its download folder")
		}
		folder := filepath.Dir(rel)
		if mediafile.IsBook(rel) {
			withArchive[folder] = true
		}
		if mediafile.IsArchive(rel) {
			archives = append(archives, payloadArchive{name: rel, path: path})
		}
		if mediafile.IsPageImage(rel) {
			images[folder] = append(images[folder], path)
		}
	}
	if len(images) == 0 {
		return archives, cleanup, nil
	}
	stage, err := os.MkdirTemp("", "sento-image-import-*")
	if err != nil {
		return nil, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(stage) }
	folders := make([]string, 0, len(images))
	for folder := range images {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	for i, folder := range folders {
		if withArchive[folder] {
			continue
		}
		name := filepath.Base(folder)
		if folder == "." {
			name = filepath.Base(root)
		}
		if imageBookFolder.MatchString(name) && len(titles) > 0 {
			name = titles[0] + " " + name
		}
		res := parse.ParseAnchored(name, parse.ChannelFile, titles)
		// Keep ambiguous folders as review archives; the normal importer
		// checks title, numbering, kind and edition before linking anything.
		ranges := append(append([]parse.Range{}, res.Volumes...), res.Chapters...)
		review := len(ranges) != 1 || ranges[0].From != ranges[0].To
		path := filepath.Join(stage, fmt.Sprintf("%d.cbz", i))
		if err := mediafile.WriteImageArchive(ctx, path, images[folder]); err != nil {
			return nil, cleanup, err
		}
		archives = append(archives, payloadArchive{name: name + ".cbz", path: path, review: review})
	}
	return archives, cleanup, nil
}
