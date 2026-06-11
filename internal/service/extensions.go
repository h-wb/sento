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

	"github.com/h-wb/sento/internal/sourcehost"
	"github.com/h-wb/sento/internal/store"
)

// Sento owns the desired extension list; the sento-host sidecar is
// stateless cattle. Installing through the UI records the package here
// and Reconcile re-installs anything a fresh/replaced host is missing —
// on startup, after connection changes, and periodically.

const (
	extensionsKey  = "extensions"
	sourcePrefsKey = "source_prefs"
	repoListKey    = "extension_repos"
)

type desiredExtensions struct {
	Packages []string `json:"packages"`
}

// storedPrefs is sourceID → preference key → value (JSON-typed).
type storedPrefs map[string]map[string]json.RawMessage

// ExtensionView is one host extension plus Sento's desired flag.
type ExtensionView struct {
	sourcehost.ExtensionInfo `tstype:",extends"`
	Desired                  bool `json:"desired"`
}

// ExtensionService manages extensions on hosts that expose management
// (the sidecar). The host accessor is indirect: connections may swap the
// backend at runtime.
type ExtensionService struct {
	store *store.Store
	host  func() sourcehost.Host
}

func NewExtensionService(st *store.Store, host func() sourcehost.Host) *ExtensionService {
	return &ExtensionService{store: st, host: host}
}

// manager returns the current host's management capability, nil when the
// backend doesn't support it (none configured).
func (e *ExtensionService) manager() sourcehost.ExtensionManager {
	mgr, _ := e.host().(sourcehost.ExtensionManager)
	return mgr
}

func (e *ExtensionService) HasManager() bool {
	return e.host() != nil && e.manager() != nil
}

func (e *ExtensionService) desired(ctx context.Context) []string {
	var d desiredExtensions
	if raw, err := e.store.GetSetting(ctx, extensionsKey); err == nil {
		_ = json.Unmarshal(raw, &d)
	}
	return d.Packages
}

func (e *ExtensionService) saveDesired(ctx context.Context, packages []string) error {
	slices.Sort(packages)
	raw, err := json.Marshal(desiredExtensions{Packages: packages})
	if err != nil {
		return err
	}
	return e.store.PutSetting(ctx, extensionsKey, raw)
}

// repoURLs is the stored extension repo list (none by default — the
// user adds Mihon-compatible index URLs).
func (e *ExtensionService) repoURLs(ctx context.Context) []string {
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

// SetRepos stores the repo list and pushes it to the host.
func (e *ExtensionService) SetRepos(ctx context.Context, urls []string) error {
	mgr := e.manager()
	if mgr == nil {
		return fmt.Errorf("the configured source backend does not manage extensions")
	}
	cleaned := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("repo url must start with http:// or https://: %s", u)
		}
		if !slices.Contains(cleaned, u) {
			cleaned = append(cleaned, u)
		}
	}
	if err := mgr.SetRepos(ctx, cleaned); err != nil {
		return err
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
func (e *ExtensionService) reconcileRepos(ctx context.Context, mgr sourcehost.ExtensionManager) error {
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
	if !missing {
		return nil
	}
	slog.Info("reconciling extension repos onto host", "count", len(want))
	return mgr.SetRepos(ctx, want)
}

// List merges the host's extension catalog with the desired flags.
func (e *ExtensionService) List(ctx context.Context) ([]ExtensionView, error) {
	mgr := e.manager()
	if mgr == nil {
		return nil, fmt.Errorf("the configured source backend does not manage extensions")
	}
	infos, err := mgr.Extensions(ctx)
	if err != nil {
		return nil, err
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

// Install installs (or updates) on the host and records the package as
// desired so reconciliation survives host replacement.
func (e *ExtensionService) Install(ctx context.Context, pkgName string) error {
	mgr := e.manager()
	if mgr == nil {
		return fmt.Errorf("the configured source backend does not manage extensions")
	}
	if err := mgr.InstallExtension(ctx, pkgName); err != nil {
		return err
	}
	desired := e.desired(ctx)
	if !slices.Contains(desired, pkgName) {
		desired = append(desired, pkgName)
	}
	return e.saveDesired(ctx, desired)
}

func (e *ExtensionService) Uninstall(ctx context.Context, pkgName string) error {
	mgr := e.manager()
	if mgr == nil {
		return fmt.Errorf("the configured source backend does not manage extensions")
	}
	if err := mgr.UninstallExtension(ctx, pkgName); err != nil {
		return err
	}
	desired := slices.DeleteFunc(e.desired(ctx), func(p string) bool { return p == pkgName })
	return e.saveDesired(ctx, desired)
}

// Reconcile installs every desired package the host is missing, then
// replays stored source preferences. It never uninstalls extras (manual
// host-side installs are not Sento's to remove). No-op when the backend
// has no manager.
func (e *ExtensionService) Reconcile(ctx context.Context) error {
	mgr := e.manager()
	if mgr == nil {
		return nil
	}
	var errs []error
	// Repos first: installs resolve through them.
	if err := e.reconcileRepos(ctx, mgr); err != nil {
		errs = append(errs, err)
	}
	desired := e.desired(ctx)
	if len(desired) > 0 {
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
		for _, pkg := range desired {
			if installed[pkg] {
				continue
			}
			slog.Info("reconciling extension onto host", "pkg", pkg)
			if err := mgr.InstallExtension(ctx, pkg); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
			}
		}
	}
	if err := e.reconcilePrefs(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// --- source preferences (ConfigurableSource) ---

func (e *ExtensionService) prefManager() sourcehost.PreferenceManager {
	mgr, _ := e.host().(sourcehost.PreferenceManager)
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
func (e *ExtensionService) Preferences(ctx context.Context, sourceID string) ([]sourcehost.Preference, error) {
	mgr := e.prefManager()
	if mgr == nil {
		return nil, fmt.Errorf("the configured source backend does not manage source preferences")
	}
	return mgr.SourcePreferences(ctx, sourceID)
}

// SetPreference pushes one value to the host and records it so
// reconciliation replays it onto fresh hosts.
func (e *ExtensionService) SetPreference(ctx context.Context, sourceID, key string, value json.RawMessage) error {
	mgr := e.prefManager()
	if mgr == nil {
		return fmt.Errorf("the configured source backend does not manage source preferences")
	}
	if err := mgr.SetSourcePreference(ctx, sourceID, key, value); err != nil {
		return err
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
