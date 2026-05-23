package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const DefaultPath = "/etc/do-droplets-tui/config.json"

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
