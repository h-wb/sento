package service

import (
	"context"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/torznab"
)

// probeTimeout bounds one remote probe, so a hung service costs a check
// run seconds rather than the request timeout.
const probeTimeout = 10 * time.Second

// runParallel runs probes concurrently — each bounded by timeout when
// positive — and returns their results in probe order.
func runParallel[T any](ctx context.Context, timeout time.Duration, probes []func(context.Context) T) []T {
	out := make([]T, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pctx := ctx
			if timeout > 0 {
				var cancel context.CancelFunc
				pctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			out[i] = probe(pctx)
		}()
	}
	wg.Wait()
	return out
}

// probeSource checks that a source is present on the extensions host.
func probeSource(ctx context.Context, host extensions.Host, sourceID string) error {
	if host == nil {
		return apperr.Unavailablef("no extensions sidecar is configured")
	}
	sources, err := host.Sources(ctx)
	if err != nil {
		return apperr.Upstreamf("%w", err)
	}
	if !sourcePresent(sources, sourceID) {
		return errSourceMissing(sourceID)
	}
	return nil
}

// sourcePresent reports whether sourceID is among the host's sources.
func sourcePresent(sources []extensions.Source, sourceID string) bool {
	for _, s := range sources {
		if s.ID == sourceID {
			return true
		}
	}
	return false
}

// errSourceMissing reports a source the host no longer carries.
func errSourceMissing(sourceID string) error {
	return apperr.Upstreamf("source %s is not present on the host (extension uninstalled?)", sourceID)
}

// probeTorznab pings one parsed torznab entry.
func probeTorznab(ctx context.Context, e provider.IndexerEntry) error {
	return torznab.New(e.URL, e.APIKey).Ping(ctx)
}
