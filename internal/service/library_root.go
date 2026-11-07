package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/h-wb/sento/internal/store"
)

// PrepareLibraryRoot runs before the server or scheduler starts. Relocation
// changes references only: the operator moves the files while Sento is stopped.
func PrepareLibraryRoot(ctx context.Context, st *store.Store, root, previous string) error {
	if root == "" {
		return fmt.Errorf("library path is empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if previous != "" {
		previous, err = filepath.Abs(previous)
		if err != nil {
			return err
		}
	}
	return st.ReconcileLibraryRoot(ctx, root, previous, func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("relocated file %q is unavailable: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("relocated file %q is not a regular file", path)
		}
		return nil
	})
}
