// Package version holds build metadata injected at link time.
package version

// Set via -ldflags "-X github.com/h-wb/sento/internal/version.Version=…".
var (
	Version = "0.0.1-dev"
	Commit  = "unknown"
)
