package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/h-wb/sento/internal/mediafile"
	"github.com/h-wb/sento/internal/parse"
	"github.com/h-wb/sento/internal/store"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// fileHash identifies a book by its pages (mediafile.Fingerprint), so a
// download and its processed, tagged library copy are the same asset.
func fileHash(path string) (string, error) { return mediafile.Fingerprint(path) }

// assetPath is dir/name with an unsafe or overlong name tamed and, when
// hash is given, " [hash]" before the extension.
func assetPath(dir, name, hash string) string {
	ext := filepath.Ext(name)
	base := mediafile.Sanitize(strings.TrimSuffix(filepath.Base(name), ext))
	for len([]byte(base)) > 120 {
		r := []rune(base)
		base = string(r[:len(r)-1])
	}
	if base == "" {
		base = "book"
	}
	if hash != "" {
		base += " [" + hash + "]"
	}
	return filepath.Join(dir, base+ext)
}
func verifyDestination(path, hash string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	got, err := fileHash(path)
	if err != nil {
		return err
	}
	if got != hash {
		return fmt.Errorf("destination collision: existing asset has different content: %s", path)
	}
	return nil
}
func (im *Importer) normalizeExpected(ctx context.Context, item store.QueueItem) ([]store.BookRef, error) {
	var slots []store.BookRef
	if err := json.Unmarshal(item.Books, &slots); err != nil {
		return nil, err
	}
	if len(slots) == 0 {
		return nil, fmt.Errorf("queue item has no expected books")
	}
	for i := range slots {
		sl := &slots[i]
		col, err := im.store.GetEdition(ctx, sl.EditionID)
		if err != nil {
			return nil, err
		}
		if col.SeriesID != item.SeriesID || col.Type != sl.Ledger {
			return nil, fmt.Errorf("invalid edition mapping")
		}
		w, err := im.store.GetBook(ctx, sl.BookID)
		if err != nil {
			return nil, err
		}
		t, err := im.store.GetVersion(ctx, sl.VersionID)
		if err != nil {
			return nil, err
		}
		if w.EditionID != sl.EditionID || t.EditionID != sl.EditionID {
			return nil, fmt.Errorf("book/version mapping does not belong to this edition")
		}
	}
	return slots, nil
}
func (im *Importer) recordAsset(ctx context.Context, item store.QueueItem, path string, slots []store.BookRef, parsed any, contents ...[]store.BookRef) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	links := make([]store.BookLink, 0, len(slots))
	for _, sl := range slots {
		links = append(links, store.BookLink{Ledger: sl.Ledger, EditionID: sl.EditionID, Number: sl.Number, VersionID: sl.VersionID, BookID: sl.BookID})
	}
	var snap ReleaseCandidate
	_ = json.Unmarshal(item.Snapshot, &snap)
	facts := store.AssetFacts{Title: snap.Title, Site: snap.Site, Protocol: item.Protocol, FactsKnown: snap.Title != "", Flags: candidateFlags(snap), Language: snap.Parse.Language, Group: snap.Parse.Group, Format: strings.TrimPrefix(filepath.Ext(path), "."), FormatScore: snap.FormatScore, Resolution: snap.Parse.Quality.Resolution, Revision: snap.Parse.Version, Digital: snap.Parse.Quality.Digital, ProfileID: snap.ProfileID, ReleaseKey: item.ReleaseKey}
	if facts.Group == "" {
		facts.Group = snap.Scanlator
	}
	facts.Sources = map[string]string{"language": "offer", "group": "offer", "quality": "offer"}
	claims := facts
	if actual, ok := parsed.(parse.Result); ok {
		// An offer's claims remain available for review; file facts are specific to these bytes.
		facts.Sources = map[string]string{"language": "filename", "group": "filename", "quality": "filename"}
		facts.Title = filepath.Base(path)
		facts.Group = actual.Group
		facts.Language = actual.Language
		facts.Flags = candidateFlags(ReleaseCandidate{Parse: actual})
		facts.Resolution = actual.Quality.Resolution
		facts.Revision = actual.Version
		facts.Digital = actual.Quality.Digital
		facts.FactsKnown = true
	}
	hash, err := fileHash(path)
	if err != nil {
		return err
	}
	review := false
	for _, sl := range slots {
		version, e := im.store.GetVersion(ctx, sl.VersionID)
		if e != nil {
			return e
		}
		if version.RequiredGroup != "" && !matchGroup(version.RequiredGroup, facts.Group) {
			review = true
		}
	}
	var contentBooks []store.BookRef
	for _, refs := range contents {
		contentBooks = append(contentBooks, refs...)
	}
	_, err = im.store.AddFile(ctx, store.File{ContentBooks: contentBooks, SeriesID: item.SeriesID, Path: path, Size: fi.Size(), Provenance: item.Protocol, Facts: facts, Claims: claims, Hash: hash, NeedsReview: review}, parsed, links)
	if err == nil && review {
		return fmt.Errorf("file's observed release group differs from the requested copy; confirm its facts in review")
	}
	return err
}

// installAsset never replaces an existing pathname, even if another actor
// creates it between the hash check and installation.
func installAsset(src, dest, hash string, alwaysCopy bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if !alwaysCopy {
		if err := os.Link(src, dest); err == nil {
			return nil
		} else if os.IsExist(err) {
			return verifyDestination(dest, hash)
		}
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".sento-install-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err = io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Link(name, dest); os.IsExist(err) {
		return verifyDestination(dest, hash)
	}
	if err == nil {
		return nil
	}
	// Filesystems without hardlinks still get exclusive, non-overwriting copies.
	staged, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = staged.Close() }()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return verifyDestination(dest, hash)
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, staged)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(dest)
	}
	return copyErr
}

// Unmatched archives are retained for human mapping without claiming coverage.
func (im *Importer) importReviewFile(ctx context.Context, item store.QueueItem, root, src string, parsed any, copy bool) error {
	hash, err := fileHash(src)
	if err != nil {
		return err
	}
	dest := assetPath(filepath.Join(root, "review"), filepath.Base(src), hash)
	if err = installAsset(src, dest, hash, copy); err != nil {
		return err
	}
	info, err := os.Stat(dest)
	if err != nil {
		return err
	}
	var snap ReleaseCandidate
	_ = json.Unmarshal(item.Snapshot, &snap)
	claims := store.AssetFacts{Title: snap.Title, Language: snap.Parse.Language, Group: snap.Parse.Group, ReleaseKey: item.ReleaseKey, Sources: map[string]string{"language": "offer", "group": "offer"}}
	facts := store.AssetFacts{Title: filepath.Base(src), Format: strings.TrimPrefix(filepath.Ext(src), "."), Protocol: item.Protocol, ReleaseKey: item.ReleaseKey, Sources: map[string]string{"language": "filename", "group": "filename", "quality": "filename"}}
	if p, ok := parsed.(parse.Result); ok {
		facts.Language = p.Language
		facts.Group = p.Group
		facts.Flags = candidateFlags(ReleaseCandidate{Parse: p})
		facts.FactsKnown = true
		facts.Resolution = p.Quality.Resolution
		facts.Revision = p.Version
		facts.Digital = p.Quality.Digital
	}
	_, err = im.store.AddFile(ctx, store.File{SeriesID: item.SeriesID, Path: dest, Size: info.Size(), Provenance: item.Protocol, NeedsReview: true, Unrequested: true, Facts: facts, Claims: claims, Hash: hash}, parsed, nil)
	return err
}
