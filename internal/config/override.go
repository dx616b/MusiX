package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type configOverride struct {
	Prowlarr     ProwlarrConfig     `yaml:"prowlarr"`
	Jackett      JackettConfig      `yaml:"jackett"`
	Transmission TransmissionConfig `yaml:"transmission"`
}

func overridePath(cfg *Config) string {
	if p := os.Getenv("SETTINGS_FILE"); p != "" {
		return p
	}
	dir := filepath.Dir(cfg.Store.SQLitePath)
	if dir == "" || dir == "." {
		dir = "data"
	}
	return filepath.Join(dir, "settings.yaml")
}

// LoadOverride merges data/settings.yaml (or SETTINGS_FILE) over the base config.
func LoadOverride(cfg *Config) error {
	path := overridePath(cfg)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var o configOverride
	if err := yaml.Unmarshal(data, &o); err != nil {
		return err
	}
	// UI override replaces integration settings wholesale when the file exists.
	cfg.Prowlarr = o.Prowlarr
	cfg.Jackett = o.Jackett
	cfg.Transmission = o.Transmission
	return nil
}

// OverridePath returns where UI-saved settings are written.
func OverridePath(cfg *Config) string {
	return overridePath(cfg)
}
