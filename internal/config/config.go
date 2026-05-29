package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultPath = "/etc/do-droplets-tui/config.json"

// LocalProfilePath returns the user-local config path (~/.config/do-droplets-tui/config.json).
// Returns "" if the home directory cannot be determined.
func LocalProfilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "do-droplets-tui", "config.json")
}

// ResolvePath returns the config file path to use.
// If explicit is non-empty (--config flag was passed), it is returned as-is.
// Otherwise the local profile (~/.config/do-droplets-tui/config.json) is used
// when it exists, falling back to the system default (/etc/do-droplets-tui/config.json).
func ResolvePath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if local := LocalProfilePath(); local != "" {
		if _, err := os.Stat(local); err == nil {
			return local
		}
	}
	return DefaultPath
}

type Config struct {
	DigitalOcean DigitalOceanConfig `json:"digitalocean"`
	UI           UIConfig           `json:"ui"`
	Spaces       SpacesConfig       `json:"spaces"`
	Inference    InferenceConfig    `json:"inference"`
}

type SpacesConfig struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"` // e.g. "fra1"
}

type InferenceConfig struct {
	ModelAccessKey string `json:"model_access_key"`
	// BaseURL defaults to https://inference.do-ai.run/v1
	BaseURL string `json:"base_url"`
}

type DigitalOceanConfig struct {
	Token string `json:"token"`
}

type UIConfig struct {
	DefaultRegion string `json:"default_region"`
	DefaultSize   string `json:"default_size"`
	DefaultImage  string `json:"default_image"`
	DefaultTags   string `json:"default_tags"` // CSV, e.g. "dev,tui"
	DefaultIPv6   bool   `json:"default_ipv6"`
}

func Load(path string) (Config, error) {
	if path == "" {
		path = DefaultPath
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	// Safe defaults
	if cfg.UI.DefaultRegion == "" {
		cfg.UI.DefaultRegion = "fra1"
	}
	if cfg.UI.DefaultSize == "" {
		cfg.UI.DefaultSize = "s-1vcpu-1gb"
	}
	if cfg.UI.DefaultImage == "" {
		cfg.UI.DefaultImage = "ubuntu-24-04-x64"
	}

	return cfg, nil
}

func EnsureDirFor(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}
