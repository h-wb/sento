// Package config loads Sento's configuration from a YAML file with
// environment-variable overrides (SENTO_*).
package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    Server    `yaml:"server"`
	Database  Database  `yaml:"database"`
	Library   Library   `yaml:"library"`
	Suwayomi  Suwayomi  `yaml:"suwayomi"`
	Prowlarr  Prowlarr  `yaml:"prowlarr"`
	MangaBaka MangaBaka `yaml:"mangabaka"`
}

type Library struct {
	Path string `yaml:"path"`
}

type Server struct {
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key"`
	StaticDir string `yaml:"static_dir"`
}

type Database struct {
	Path string `yaml:"path"`
}

type Suwayomi struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Prowlarr struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
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
	str("SENTO_LIBRARY_PATH", &c.Library.Path)
	str("SENTO_SUWAYOMI_URL", &c.Suwayomi.URL)
	str("SENTO_SUWAYOMI_USERNAME", &c.Suwayomi.Username)
	str("SENTO_SUWAYOMI_PASSWORD", &c.Suwayomi.Password)
	str("SENTO_PROWLARR_URL", &c.Prowlarr.URL)
	str("SENTO_PROWLARR_API_KEY", &c.Prowlarr.APIKey)
	str("SENTO_MANGABAKA_BASE_URL", &c.MangaBaka.BaseURL)

	if v, ok := os.LookupEnv("SENTO_PORT"); ok {
		if p, err := strconv.Atoi(v); err == nil {
			c.Server.Port = p
		}
	}
}
