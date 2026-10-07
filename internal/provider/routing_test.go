package provider

import (
	"context"
	"errors"
	"testing"
)

// Not knowing which client an indexer uses must stop the grab: falling
// back to the default would send a release meant for one client (a VPN
// one, say) to another.
func TestClientRoutingFailsClosed(t *testing.T) {
	clients := []TorrentClient{{ID: "open"}, {ID: "vpn"}}
	entries := func(list []IndexerEntry, err error) func(context.Context) ([]IndexerEntry, error) {
		return func(context.Context) ([]IndexerEntry, error) { return list, err }
	}
	ctx := context.Background()

	p := &TorrentProvider{clients: clients, entries: entries(nil, errors.New("database unavailable"))}
	if got, err := p.clientFor(ctx, "2"); err == nil {
		t.Errorf("failed lookup chose %q", got.ID)
	}
	p = &TorrentProvider{clients: clients, entries: entries([]IndexerEntry{{ID: 2, DownloadClient: "vpn"}}, nil)}
	if got, err := p.clientFor(ctx, "9"); err == nil {
		t.Errorf("unknown indexer chose %q", got.ID)
	}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "vpn" {
		t.Errorf("named client: got %q, %v", got.ID, err)
	}
	p = &TorrentProvider{clients: clients, entries: entries([]IndexerEntry{{ID: 2}}, nil)}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "open" {
		t.Errorf("indexer naming no client should use the default: got %q, %v", got.ID, err)
	}
	p = &TorrentProvider{clients: clients[:1], entries: entries(nil, errors.New("database unavailable"))}
	if got, err := p.clientFor(ctx, "2"); err != nil || got.ID != "open" {
		t.Errorf("a single client leaves nothing to choose: got %q, %v", got.ID, err)
	}
}
