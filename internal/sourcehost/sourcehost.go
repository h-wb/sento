// Package sourcehost defines the protocol-adapter contract for extension
// sources: the host knows how to talk to manga sites (search, chapter
// lists, page images); Sento owns everything stateful around it (queueing,
// pacing, CBZ assembly, import). The Kotlin sidecar (host/) implements
// it; any other extension runtime could.
//
// All identifiers below sourceID are opaque keys minted by the host —
// Sento persists them (bindings, queue items) but never interprets them.
package sourcehost

import (
	"context"
	"encoding/json"
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
// the desired list it stores.
type ExtensionManager interface {
	Extensions(ctx context.Context) ([]ExtensionInfo, error)
	// InstallExtension installs or updates to the latest repo version.
	InstallExtension(ctx context.Context, pkgName string) error
	UninstallExtension(ctx context.Context, pkgName string) error
}

// Preference is one ConfigurableSource setting (login, mirror, quality…)
// in the typed form the host serializes from the extension's preference
// screen. Values are JSON: string, bool, or string array per ValueType.
type Preference struct {
	Key         string          `json:"key"`
	Type        string          `json:"type" tstype:"'EditTextPreference' | 'ListPreference' | 'CheckBoxPreference' | 'SwitchPreferenceCompat' | 'MultiSelectListPreference'"`
	Title       string          `json:"title,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	ValueType   string          `json:"valueType" tstype:"'String' | 'Boolean' | 'Set<String>'"`
	Default     json.RawMessage `json:"default,omitempty" tstype:"unknown"`
	Current     json.RawMessage `json:"current,omitempty" tstype:"unknown"`
	Entries     []string        `json:"entries,omitempty"`
	EntryValues []string        `json:"entryValues,omitempty"`
}

// PreferenceManager is an optional host capability paired with
// ExtensionManager: Sento stores the values users set and replays them
// onto fresh hosts during reconciliation.
type PreferenceManager interface {
	SourcePreferences(ctx context.Context, sourceID string) ([]Preference, error)
	SetSourcePreference(ctx context.Context, sourceID, key string, value json.RawMessage) error
}

// Host is the protocol adapter. Implementations must be safe for
// concurrent use; rate limiting against the remote sites is the host's
// concern (the extension runtime owns per-source HTTP behavior).
type Host interface {
	// Name is the stable host key shown as the provider id ("host").
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
