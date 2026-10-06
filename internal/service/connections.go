package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/config"
	"github.com/h-wb/sento/internal/extensions"
	"github.com/h-wb/sento/internal/provider"
	"github.com/h-wb/sento/internal/qbittorrent"
	"github.com/h-wb/sento/internal/store"
)

// Connection settings are the service endpoints Sento talks to. They live
// in the settings table (editable in the UI, applied without restart);
// yaml/env values seed the first run only — once saved, the DB wins.

const connectionsKey = "connections"

// ProwlarrConn is remembered for the Add Indexer "browse Prowlarr"
// assist — it is not a search connection (indexer entries are).
type ProwlarrConn struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

type QBittorrentConn struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
	Category string `json:"category"`
}

// ExtensionsConn points at a sento-extensions sidecar (extensions/), the extension
// runtime. Empty URL = no extension sources.
type ExtensionsConn struct {
	URL string `json:"url"`
}

type ConnectionSettings struct {
	Prowlarr    ProwlarrConn    `json:"prowlarr"`
	QBittorrent QBittorrentConn `json:"qbittorrent"`
	Extensions  ExtensionsConn  `json:"extensions"`
}

// DefaultConnections seeds settings from the file/env config.
func DefaultConnections(cfg config.Config) ConnectionSettings {
	return ConnectionSettings{
		Prowlarr: ProwlarrConn{URL: cfg.Prowlarr.URL, APIKey: cfg.Prowlarr.APIKey},
		QBittorrent: QBittorrentConn{
			URL: cfg.QBittorrent.URL, Username: cfg.QBittorrent.Username,
			Password: cfg.QBittorrent.Password, Category: cfg.QBittorrent.Category,
		},
		Extensions: ExtensionsConn{URL: cfg.Extensions.URL},
	}
}

func ValidateConnections(cs ConnectionSettings) error {
	for name, u := range map[string]string{
		"prowlarr": cs.Prowlarr.URL, "qbittorrent": cs.QBittorrent.URL,
		"extensions": cs.Extensions.URL,
	} {
		if u != "" && !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return apperr.Invalidf("%s url must start with http:// or https://", name)
		}
	}
	return nil
}

// Connections owns the live service clients: it loads settings, builds
// providers into the shared registry, and rebuilds on save. The registry
// pointer never changes, so search/import/sites services follow along.
type Connections struct {
	store    *store.Store
	cfg      config.Config
	registry *provider.Registry

	mu   sync.RWMutex
	host extensions.Host // nil when no source backend is configured
}

func NewConnections(st *store.Store, cfg config.Config) *Connections {
	return &Connections{store: st, cfg: cfg, registry: provider.NewRegistry()}
}

func (c *Connections) Registry() *provider.Registry { return c.registry }

// Host is the current source backend (binding endpoints).
func (c *Connections) Host() extensions.Host {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.host
}

// Load returns the stored settings, falling back to file/env defaults.
func (c *Connections) Load(ctx context.Context) ConnectionSettings {
	if raw, err := c.store.GetSetting(ctx, connectionsKey); err == nil {
		var cs ConnectionSettings
		if json.Unmarshal(raw, &cs) == nil {
			// Settings saved before the sidecar was renamed keep it under
			// "sourceHost".
			var legacy struct {
				SourceHost ExtensionsConn `json:"sourceHost"`
			}
			if cs.Extensions.URL == "" && json.Unmarshal(raw, &legacy) == nil {
				cs.Extensions = legacy.SourceHost
			}
			return cs
		}
	}
	return DefaultConnections(c.cfg)
}

// Save persists and applies new settings.
func (c *Connections) Save(ctx context.Context, cs ConnectionSettings) error {
	if err := ValidateConnections(cs); err != nil {
		return err
	}
	raw, err := json.Marshal(cs)
	if err != nil {
		return err
	}
	if err := c.store.PutSetting(ctx, connectionsKey, raw); err != nil {
		return err
	}
	c.Apply(cs)
	return nil
}

// Apply rebuilds clients and providers from settings. In-flight source
// downloads lose their job entries on a swap; the queue importer's State
// poll restarts them against the new backend.
func (c *Connections) Apply(cs ConnectionSettings) {
	host := buildHost(cs)

	var qb *qbittorrent.Client
	if cs.QBittorrent.URL != "" {
		qb = qbittorrent.New(cs.QBittorrent.URL, cs.QBittorrent.Username, cs.QBittorrent.Password)
	}

	c.registry.Replace(
		provider.NewSource(host, c.cfg.Downloads.Dir, provider.DownloadConfig{
			PageConcurrency:   c.cfg.Downloads.PageConcurrency,
			PageRatePerSecond: c.cfg.Downloads.PageRatePerSecond,
		}, IndexerEntries(c.store, store.IndexerSource)),
		provider.NewTorrent(IndexerEntries(c.store, store.IndexerTorznab), qb, cs.QBittorrent.Category),
	)
	c.mu.Lock()
	c.host = host
	c.mu.Unlock()
}

func buildHost(cs ConnectionSettings) extensions.Host {
	if cs.Extensions.URL == "" {
		return nil
	}
	return extensions.NewClient(cs.Extensions.URL)
}

// ConnectionTest reports one service check ("" error = healthy).
type ConnectionTest struct {
	Service string `json:"service"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// TestConnections probes every configured service in the given settings
// (not the saved ones — the UI tests before saving).
func TestConnections(ctx context.Context, cs ConnectionSettings) []ConnectionTest {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	type probe struct {
		service string
		run     func() (string, error)
	}
	var probes []probe
	if cs.QBittorrent.URL != "" {
		qb := qbittorrent.New(cs.QBittorrent.URL, cs.QBittorrent.Username, cs.QBittorrent.Password)
		probes = append(probes, probe{"qbittorrent", func() (string, error) {
			return qb.Version(ctx)
		}})
	}
	if cs.Extensions.URL != "" {
		hc := extensions.NewClient(cs.Extensions.URL)
		probes = append(probes, probe{"extensions", func() (string, error) {
			sources, err := hc.Sources(ctx)
			detail := fmt.Sprintf("%d sources", len(sources))
			if len(sources) == 1 {
				detail = "1 source"
			}
			return detail, err
		}})
	}

	results := make([]ConnectionTest, len(probes))
	var wg sync.WaitGroup
	for i, p := range probes {
		wg.Add(1)
		go func(i int, p probe) {
			defer wg.Done()
			detail, err := p.run()
			results[i] = ConnectionTest{Service: p.service, OK: err == nil, Detail: detail}
			if err != nil {
				results[i].Error = err.Error()
			}
		}(i, p)
	}
	wg.Wait()
	return results
}
