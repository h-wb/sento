package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/h-wb/sento/internal/sourcehost"
	"github.com/h-wb/sento/internal/store"
)

// Sento owns the desired extension list; the sento-host sidecar is
// stateless cattle. Installing through the UI records the package here
// and Reconcile re-installs anything a fresh/replaced host is missing —
// on startup, after connection changes, and periodically.

const extensionsKey = "extensions"

type desiredExtensions struct {
	Packages []string `json:"packages"`
}

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
// backend doesn't support it (Suwayomi, or none configured).
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

// Reconcile installs every desired package the host is missing. It never
// uninstalls extras (manual host-side installs are not Sento's to
// remove). No-op when the backend has no manager.
func (e *ExtensionService) Reconcile(ctx context.Context) error {
	mgr := e.manager()
	if mgr == nil {
		return nil
	}
	desired := e.desired(ctx)
	if len(desired) == 0 {
		return nil
	}
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
	var errs []error
	for _, pkg := range desired {
		if installed[pkg] {
			continue
		}
		slog.Info("reconciling extension onto host", "pkg", pkg)
		if err := mgr.InstallExtension(ctx, pkg); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", pkg, err))
		}
	}
	return errors.Join(errs...)
}
