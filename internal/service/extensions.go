package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/store"
)

// Sento owns the desired extension list; the sento-extensions sidecar is
// stateless cattle. Installing through the UI records the package here
// and Reconcile re-installs anything a fresh/replaced host is missing —
// on startup, after connection changes, and periodically.

const (
	sourcePrefsKey = "source_prefs"
	repoListKey    = "extension_repos"
)

// storedPrefs is sourceID → preference key → value (JSON-typed).
type storedPrefs map[string]map[string]json.RawMessage

// ExtensionView is one host extension; Desired means an indexer entry
// references it (the entry is what makes an extension wanted).
type ExtensionView struct {
	extensions.ExtensionInfo `tstype:",extends"`
	Desired                  bool `json:"desired"`
}

// ExtensionService manages extensions on hosts that expose management
// (the sidecar). The host accessor is indirect: connections may swap the
// backend at runtime.
type ExtensionService struct {
	store *store.Store
	host  func() extensions.Host
	// managedRepos is the repo list fixed by the deployment's
	// configuration; when set it replaces the stored list and cannot be
	// edited.
	managedRepos []string
}

// ManageRepos fixes the extension repo list (config.Extensions.Repos).
func (e *ExtensionService) ManageRepos(urls []string) { e.managedRepos = urls }

// ReposManaged reports whether the repo list is fixed by configuration.
func (e *ExtensionService) ReposManaged() bool { return len(e.managedRepos) > 0 }

func NewExtensionService(st *store.Store, host func() extensions.Host) *ExtensionService {
	return &ExtensionService{store: st, host: host}
}

// manager returns the current host's management capability, nil when the
// backend doesn't support it (none configured).
func (e *ExtensionService) manager() extensions.ExtensionManager {
	mgr, _ := e.host().(extensions.ExtensionManager)
	return mgr
}

func (e *ExtensionService) HasManager() bool {
	return e.host() != nil && e.manager() != nil
}

// desired is the set of extensions referenced by source indexer entries
// — entries drive the extension lifecycle.
func (e *ExtensionService) desired(ctx context.Context) []string {
	entries, err := e.store.ListIndexers(ctx)
	if err != nil {
		return nil
	}
	var pkgs []string
	for _, ix := range entries {
		if ix.Protocol != store.IndexerSource {
			continue
		}
		var ss SourceSettings
		if json.Unmarshal(ix.Settings, &ss) == nil && ss.PkgName != "" &&
			!slices.Contains(pkgs, ss.PkgName) {
			pkgs = append(pkgs, ss.PkgName)
		}
	}
	slices.Sort(pkgs)
	return pkgs
}

// repoURLs is the stored extension repo list (none by default — the
// user adds Mihon-compatible index URLs).
func (e *ExtensionService) repoURLs(ctx context.Context) []string {
	if e.ReposManaged() {
		return e.managedRepos
	}
	var r struct {
		URLs []string `json:"urls"`
	}
	if raw, err := e.store.GetSetting(ctx, repoListKey); err == nil {
		_ = json.Unmarshal(raw, &r)
	}
	return r.URLs
}

// Repos returns the stored extension repo URLs.
func (e *ExtensionService) Repos(ctx context.Context) []string {
	urls := e.repoURLs(ctx)
	if urls == nil {
		urls = []string{}
	}
	return urls
}

// RepoStatus pairs every stored repo with what the host currently gets
// from it. The stored list is the truth for *which* repos exist; when the
// host cannot be asked, each is reported with that reason rather than
// dropped.
func (e *ExtensionService) RepoStatus(ctx context.Context) []extensions.RepoStatus {
	urls := e.Repos(ctx)
	out := make([]extensions.RepoStatus, 0, len(urls))
	known := map[string]extensions.RepoStatus{}
	hostErr := ""
	if mgr := e.manager(); mgr == nil {
		hostErr = "no extensions sidecar is configured"
	} else if statuses, err := mgr.RepoStatus(ctx); err != nil {
		hostErr = "the extensions sidecar did not answer: " + err.Error()
	} else {
		for _, s := range statuses {
			known[s.URL] = s
		}
	}
	for _, u := range urls {
		s, ok := known[u]
		switch {
		case hostErr != "":
			s = extensions.RepoStatus{URL: u, Error: hostErr}
		case !ok:
			s = extensions.RepoStatus{URL: u, Error: "not loaded on the host yet"}
		}
		out = append(out, s)
	}
	return out
}

// SetRepos stores the repo list and pushes it to the host.
func (e *ExtensionService) SetRepos(ctx context.Context, urls []string) error {
	if e.ReposManaged() {
		return apperr.Invalidf("extension repos are set by this deployment's configuration (SENTO_EXTENSIONS_REPOS) and cannot be changed here")
	}
	mgr := e.manager()
	if mgr == nil {
		return apperr.Unavailablef("the configured source backend does not manage extensions")
	}
	cleaned := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return apperr.Invalidf("repo url must start with http:// or https://: %s", u)
		}
		if !slices.Contains(cleaned, u) {
			cleaned = append(cleaned, u)
		}
	}
	if err := mgr.SetRepos(ctx, cleaned); err != nil {
		return apperr.Upstreamf("pushing repos to host: %w", err)
	}
	raw, err := json.Marshal(struct {
		URLs []string `json:"urls"`
	}{cleaned})
	if err != nil {
		return err
	}
	return e.store.PutSetting(ctx, repoListKey, raw)
}

// reconcileRepos pushes the stored repo list when the host differs (a
// fresh host knows only its env-provided extras).
func (e *ExtensionService) reconcileRepos(ctx context.Context, mgr extensions.ExtensionManager) error {
	want := e.repoURLs(ctx)
	if len(want) == 0 {
		return nil
	}
	have, err := mgr.Repos(ctx)
	if err != nil {
		return fmt.Errorf("listing host repos: %w", err)
	}
	missing := false
	for _, u := range want {
		if !slices.Contains(have, u) {
			missing = true
			break
		}
	}
	// A managed list is the whole list: repos the host has beyond it go.
	if !missing && (!e.ReposManaged() || len(have) == len(want)) {
		return nil
	}
	slog.Info("reconciling extension repos onto host", "count", len(want))
	return mgr.SetRepos(ctx, want)
}

// List merges the host's extension catalog with the desired flags.
func (e *ExtensionService) List(ctx context.Context) ([]ExtensionView, error) {
	mgr := e.manager()
	if mgr == nil {
		return nil, apperr.Unavailablef("the configured source backend does not manage extensions")
	}
	infos, err := mgr.Extensions(ctx)
	if err != nil {
		return nil, apperr.Upstreamf("listing host extensions: %w", err)
	}
	want := map[string]bool{}
	for _, p := range e.desired(ctx) {
		want[p] = true
	}
	views := make([]ExtensionView, 0, len(infos))
	for _, info := range infos {
		views = append(views, ExtensionView{ExtensionInfo: info, Desired: want[info.PkgName]})
	}
	return views, nil
}

// Install installs (or updates) the extension on the host. Persistence
// of "wanted" comes from the indexer entry that references it.
func (e *ExtensionService) Install(ctx context.Context, pkgName string) error {
	mgr := e.manager()
	if mgr == nil {
		return apperr.Unavailablef("the configured source backend does not manage extensions")
	}
	if err := mgr.InstallExtension(ctx, pkgName); err != nil {
		return apperr.Upstreamf("installing %s: %w", pkgName, err)
	}
	return nil
}

// Sources lists the sources one installed extension provides (the Add
// Indexer picker).
func (e *ExtensionService) Sources(ctx context.Context, pkgName string) ([]extensions.Source, error) {
	mgr := e.manager()
	if mgr == nil {
		return nil, apperr.Unavailablef("the configured source backend does not manage extensions")
	}
	sources, err := mgr.ExtensionSources(ctx, pkgName)
	if err != nil {
		return nil, apperr.Upstreamf("listing %s sources: %w", pkgName, err)
	}
	return sources, nil
}

// DeleteIndexerEntry deletes the entry and garbage-collects its extension
// when it was the last reference (source entries drive the lifecycle).
func (e *ExtensionService) DeleteIndexerEntry(ctx context.Context, id int64) error {
	var pkg string
	if ix, err := e.store.GetIndexer(ctx, id); err == nil && ix.Protocol == store.IndexerSource {
		var ss SourceSettings
		if json.Unmarshal(ix.Settings, &ss) == nil {
			pkg = ss.PkgName
		}
	}
	if err := e.store.DeleteIndexer(ctx, id); err != nil {
		return err
	}
	e.CleanupExtension(ctx, pkg)
	return nil
}

// CleanupExtension uninstalls the package when no indexer entry
// references it anymore (called after entry deletion). Best-effort.
func (e *ExtensionService) CleanupExtension(ctx context.Context, pkgName string) {
	if pkgName == "" || slices.Contains(e.desired(ctx), pkgName) {
		return
	}
	mgr := e.manager()
	if mgr == nil {
		return
	}
	slog.Info("uninstalling extension (no indexer entry references it)", "pkg", pkgName)
	if err := mgr.UninstallExtension(ctx, pkgName); err != nil {
		slog.Warn("extension cleanup failed", "pkg", pkgName, "error", err)
	}
}

// Reconcile converges the host onto the indexer entries: repos pushed
// first (installs resolve through them), missing extensions installed,
// unreferenced ones removed (abandoned Add-Indexer picks, deleted
// entries while the host was down), stored source preferences replayed.
// No-op when the backend has no manager.
func (e *ExtensionService) Reconcile(ctx context.Context) error {
	mgr := e.manager()
	if mgr == nil {
		return nil
	}
	var errs []error
	if err := e.reconcileRepos(ctx, mgr); err != nil {
		errs = append(errs, err)
	}
	desired, complete := e.desiredSet(ctx)
	infos, err := mgr.Extensions(ctx)
	if err != nil {
		return fmt.Errorf("listing host extensions: %w", err)
	}
	installed := map[string]bool{}
	for _, info := range infos {
		if info.InstalledVersion != "" {
			installed[info.PkgName] = true
		}
	}
	for pkg := range desired {
		if installed[pkg] {
			continue
		}
		slog.Info("reconciling extension onto host", "pkg", pkg)
		if err := mgr.InstallExtension(ctx, pkg); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
		}
	}
	// Garbage-collect only when every source entry declares its package —
	// otherwise an unbackfilled entry's extension would be torn down.
	if complete {
		for pkg := range installed {
			if desired[pkg] {
				continue
			}
			slog.Info("uninstalling unreferenced extension", "pkg", pkg)
			if err := mgr.UninstallExtension(ctx, pkg); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
			}
		}
	}
	if err := e.reconcilePrefs(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// desiredSet is the entry-referenced packages as a set, plus whether
// every source entry declares its package (GC safety).
func (e *ExtensionService) desiredSet(ctx context.Context) (map[string]bool, bool) {
	set := map[string]bool{}
	complete := true
	entries, err := e.store.ListIndexers(ctx)
	if err != nil {
		return set, false
	}
	for _, ix := range entries {
		if ix.Protocol != store.IndexerSource {
			continue
		}
		var ss SourceSettings
		if json.Unmarshal(ix.Settings, &ss) != nil || ss.PkgName == "" {
			complete = false
			continue
		}
		set[ss.PkgName] = true
	}
	return set, complete
}

// --- source preferences (ConfigurableSource) ---

func (e *ExtensionService) prefManager() extensions.PreferenceManager {
	mgr, _ := e.host().(extensions.PreferenceManager)
	return mgr
}

func (e *ExtensionService) storedSourcePrefs(ctx context.Context) storedPrefs {
	prefs := storedPrefs{}
	if raw, err := e.store.GetSetting(ctx, sourcePrefsKey); err == nil {
		_ = json.Unmarshal(raw, &prefs)
	}
	return prefs
}

// Preferences proxies the host's preference form for one source.
func (e *ExtensionService) Preferences(ctx context.Context, sourceID string) ([]extensions.Preference, error) {
	mgr := e.prefManager()
	if mgr == nil {
		return nil, apperr.Unavailablef("the configured source backend does not manage source preferences")
	}
	prefs, err := mgr.SourcePreferences(ctx, sourceID)
	if err != nil {
		return nil, apperr.Upstreamf("source %s preferences: %w", sourceID, err)
	}
	return prefs, nil
}

// SetPreference pushes one value to the host and records it so
// reconciliation replays it onto fresh hosts.
func (e *ExtensionService) SetPreference(ctx context.Context, sourceID, key string, value json.RawMessage) error {
	mgr := e.prefManager()
	if mgr == nil {
		return apperr.Unavailablef("the configured source backend does not manage source preferences")
	}
	if err := mgr.SetSourcePreference(ctx, sourceID, key, value); err != nil {
		return apperr.Upstreamf("setting %s on source %s: %w", key, sourceID, err)
	}
	prefs := e.storedSourcePrefs(ctx)
	if prefs[sourceID] == nil {
		prefs[sourceID] = map[string]json.RawMessage{}
	}
	prefs[sourceID][key] = value
	raw, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	return e.store.PutSetting(ctx, sourcePrefsKey, raw)
}

// reconcilePrefs replays stored values that differ from the host's
// current ones (a fresh host starts at the extension defaults).
func (e *ExtensionService) reconcilePrefs(ctx context.Context) error {
	mgr := e.prefManager()
	if mgr == nil {
		return nil
	}
	var errs []error
	for sourceID, values := range e.storedSourcePrefs(ctx) {
		current, err := mgr.SourcePreferences(ctx, sourceID)
		if err != nil {
			errs = append(errs, fmt.Errorf("source %s preferences: %w", sourceID, err))
			continue
		}
		currentByKey := map[string]json.RawMessage{}
		for _, p := range current {
			currentByKey[p.Key] = p.Current
		}
		for key, want := range values {
			if jsonEqual(currentByKey[key], want) {
				continue
			}
			slog.Info("reconciling source preference onto host", "source", sourceID, "key", key)
			if err := mgr.SetSourcePreference(ctx, sourceID, key, want); err != nil {
				errs = append(errs, fmt.Errorf("source %s pref %s: %w", sourceID, key, err))
			}
		}
	}
	return errors.Join(errs...)
}

// jsonEqual compares two JSON values structurally.
func jsonEqual(a, b json.RawMessage) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}
