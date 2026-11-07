// Package config loads Sento's configuration from a YAML file with
// environment-variable overrides (SENTO_*).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      Server      `yaml:"server"`
	Database    Database    `yaml:"database"`
	Library     Library     `yaml:"library"`
	Downloads   Downloads   `yaml:"downloads"`
	Extensions  Extensions  `yaml:"extensions"`
	Prowlarr    Prowlarr    `yaml:"prowlarr"`
	QBittorrent QBittorrent `yaml:"qbittorrent"`
	MangaBaka   MangaBaka   `yaml:"mangabaka"`
}

type Library struct {
	Path string `yaml:"path"`
	// PreviousPath bootstraps relocation on the first upgrade from a
	// version that did not record its library root.
	PreviousPath string `yaml:"previous_path"`
	// PathMappings translate download-client paths to local ones
	// ("remote=local"), for setups where qBittorrent runs on another
	// machine whose storage is mounted here (Sonarr's remote path
	// mappings). Unmapped paths pass through.
	PathMappings []string `yaml:"path_mappings"`
}

type Downloads struct {
	// Dir stages in-progress source downloads (page fetches assembled
	// into CBZs) before import.
	Dir string `yaml:"dir"`
	// PageConcurrency caps how many pages of one chapter are fetched in
	// parallel. Keep at or below 4: the sidecar's OkHttp client allows 5
	// concurrent requests per host, and extensions self-throttle on top.
	PageConcurrency int `yaml:"page_concurrency"`
	// PageRatePerSecond caps page requests per second per source (token
	// bucket), pacing in-flight fetches more gently than a fixed sleep.
	// 0 disables the limiter.
	PageRatePerSecond float64 `yaml:"page_rate_per_second"`
}

type Server struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key"`
	StaticDir string `yaml:"static_dir"`
}

// Database selects the backend: a postgres:// URL when set, otherwise
// SQLite at Path.
type Database struct {
	Path string `yaml:"path"`
	URL  string `yaml:"url"`
}

// Extensions points at a sento-extensions sidecar (extensions/), the extension
// runtime serving manga sources.
//
// A URL given here (file or SENTO_EXTENSIONS_URL) is not a first-run seed
// like the other connections: it stays in force and the UI shows it
// read-only. The same goes for Repos (SENTO_EXTENSIONS_REPOS, comma- or
// space-separated), the extension repos the sidecar loads. That lets a
// deployment declare the whole sidecar setup next to the sidecar itself.
type Extensions struct {
	URL   string   `yaml:"url"`
	Repos []string `yaml:"repos"`
}

type Prowlarr struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type QBittorrent struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Category string `yaml:"category"`
}

type MangaBaka struct {
	BaseURL string `yaml:"base_url"`
}

func Default() Config {
	var c Config
	c.Server.Host = "0.0.0.0"
	c.Server.Port = 8484
	c.Server.StaticDir = "web/dist"
	c.Database.Path = "data/sento.db"
	c.Library.Path = "library"
	c.Downloads.Dir = "data/downloads"
	c.Downloads.PageConcurrency = 3
	c.Downloads.PageRatePerSecond = 4
	c.QBittorrent.Category = "sento"
	c.MangaBaka.BaseURL = "https://api.mangabaka.org/v1"
	return c
}

// Load reads path (if it exists), applies env overrides, and validates.
// A missing file is not an error: defaults + env make a runnable config.
func Load(path string) (Config, error) {
	c := Default()

	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &c); err != nil {
			return c, fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return c, err
	}

	applyEnv(&c)
	return c, nil
}

func applyEnv(c *Config) {
	str := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	str("SENTO_HOST", &c.Server.Host)
	str("SENTO_API_KEY", &c.Server.APIKey)
	str("SENTO_STATIC_DIR", &c.Server.StaticDir)
	str("SENTO_DB_PATH", &c.Database.Path)
	str("SENTO_DB_URL", &c.Database.URL)
	str("SENTO_LIBRARY_PATH", &c.Library.Path)
	str("SENTO_LIBRARY_PREVIOUS_PATH", &c.Library.PreviousPath)
	str("SENTO_DOWNLOADS_DIR", &c.Downloads.Dir)
	if v, ok := os.LookupEnv("SENTO_DOWNLOADS_PAGE_CONCURRENCY"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.Downloads.PageConcurrency = n
		}
	}
	if v, ok := os.LookupEnv("SENTO_DOWNLOADS_PAGE_RATE"); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.Downloads.PageRatePerSecond = f
		}
	}
	str("SENTO_EXTENSIONS_URL", &c.Extensions.URL)
	if v, ok := os.LookupEnv("SENTO_EXTENSIONS_REPOS"); ok {
		c.Extensions.Repos = strings.FieldsFunc(v, func(r rune) bool { return r == ',' || unicode.IsSpace(r) })
	}
	str("SENTO_PROWLARR_URL", &c.Prowlarr.URL)
	str("SENTO_PROWLARR_API_KEY", &c.Prowlarr.APIKey)
	str("SENTO_QBITTORRENT_URL", &c.QBittorrent.URL)
	str("SENTO_QBITTORRENT_USERNAME", &c.QBittorrent.Username)
	str("SENTO_QBITTORRENT_PASSWORD", &c.QBittorrent.Password)
	str("SENTO_QBITTORRENT_CATEGORY", &c.QBittorrent.Category)
	str("SENTO_MANGABAKA_BASE_URL", &c.MangaBaka.BaseURL)

	if v, ok := os.LookupEnv("SENTO_PORT"); ok {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
	if v, ok := os.LookupEnv("SENTO_PATH_MAPPINGS"); ok {
		c.Library.PathMappings = nil
		for _, m := range strings.Split(v, ",") {
			if m = strings.TrimSpace(m); m != "" {
				c.Library.PathMappings = append(c.Library.PathMappings, m)
			}
		}
	}
}
