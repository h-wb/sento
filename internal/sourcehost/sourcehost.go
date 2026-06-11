// Package sourcehost defines the protocol-adapter contract for extension
// sources: the host knows how to talk to manga sites (search, chapter
// lists, page images); Sento owns everything stateful around it (queueing,
// pacing, CBZ assembly, import). Suwayomi implements this today; the
// Kotlin sidecar implements the same shape natively.
//
// All identifiers below sourceID are opaque keys minted by the host —
// Suwayomi uses its database ids, a stateless host uses source URLs.
// Sento persists them (bindings, queue items) but never interprets them.
package sourcehost

import (
	"context"
	"io"
)

// Source is one installed extension source.
type Source struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Lang string `json:"lang"`
	NSFW bool   `json:"nsfw"`
}

// Manga is a series entry on a source (binding resolution and the manual
// bind UI).
type Manga struct {
	Key   string `json:"key"`
	Title string `json:"title"`
}

// Chapter is one release entry of a bound manga. Number is the source's
// own recognition, used to corroborate terse names ("1184").
type Chapter struct {
	Key       string  `json:"key"`
	Name      string  `json:"name"`
	Number    float64 `json:"number,omitempty"`
	Scanlator string  `json:"scanlator,omitempty"`
}

// ExtensionInfo describes one extension on a host: available from its
// repos, installed, or both.
type ExtensionInfo struct {
	PkgName          string `json:"pkgName"`
	Name             string `json:"name"`
	Lang             string `json:"lang"`
	NSFW             bool   `json:"nsfw"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	HasUpdate        bool   `json:"hasUpdate,omitempty"`
}

// ExtensionManager is an optional host capability: the sento-host sidecar
// is stateless cattle whose installed extensions Sento reconciles from
// the desired list it stores. Suwayomi manages its own extensions and
// does not implement this.
type ExtensionManager interface {
	Extensions(ctx context.Context) ([]ExtensionInfo, error)
	// InstallExtension installs or updates to the latest repo version.
	InstallExtension(ctx context.Context, pkgName string) error
	UninstallExtension(ctx context.Context, pkgName string) error
}

// Host is the protocol adapter. Implementations must be safe for
// concurrent use; rate limiting against the remote sites is the host's
// concern (the extension runtime owns per-source HTTP behavior).
type Host interface {
	// Name is the stable host key shown as the provider id ("suwayomi").
	Name() string
	Sources(ctx context.Context) ([]Source, error)
	Search(ctx context.Context, sourceID, query string) ([]Manga, error)
	Chapters(ctx context.Context, sourceID, mangaKey string) ([]Chapter, error)
	// Pages lists a chapter's page refs (opaque, ordered).
	Pages(ctx context.Context, sourceID, chapterKey string) ([]string, error)
	// Page streams one page image through the source's HTTP client (auth
	// headers and interceptors live source-side).
	Page(ctx context.Context, sourceID, pageRef string) (io.ReadCloser, error)
}
