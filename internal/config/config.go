package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Prowlarr     ProwlarrConfig     `yaml:"prowlarr"`
	Jackett      JackettConfig      `yaml:"jackett"`
	Transmission TransmissionConfig `yaml:"transmission"`
	Store        StoreConfig        `yaml:"store"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type ProwlarrConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"apiKey"`
}

type JackettConfig struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"apiKey"`
}

type TransmissionConfig struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type StoreConfig struct {
	SQLitePath string `yaml:"sqlitePath"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
		Store:  StoreConfig{SQLitePath: "data/musicx.db"},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if err := LoadOverride(cfg); err != nil {
		return nil, fmt.Errorf("settings override: %w", err)
	}
	applyEnv(cfg)
	return cfg, cfg.Validate()
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("PROWLARR_URL"); v != "" {
		cfg.Prowlarr.URL = v
	}
	if v := os.Getenv("PROWLARR_API_KEY"); v != "" {
		cfg.Prowlarr.APIKey = v
	}
	if v := os.Getenv("JACKETT_URL"); v != "" {
		cfg.Jackett.URL = v
	}
	if v := os.Getenv("JACKETT_API_KEY"); v != "" {
		cfg.Jackett.APIKey = v
	}
	if v := os.Getenv("TRANSMISSION_URL"); v != "" {
		cfg.Transmission.URL = v
	}
	if v := os.Getenv("TRANSMISSION_USER"); v != "" {
		cfg.Transmission.Username = v
	}
	if v := os.Getenv("TRANSMISSION_PASS"); v != "" {
		cfg.Transmission.Password = v
	}
	if v := os.Getenv("MUSIX_SQLITE"); v != "" {
		cfg.Store.SQLitePath = v
	} else if v := os.Getenv("MUSICX_SQLITE"); v != "" {
		cfg.Store.SQLitePath = v
	}
}

func (c *Config) Validate() error {
	if c.Server.Port <= 0 {
		c.Server.Port = 8080
	}
	if c.Prowlarr.URL == "" && c.Jackett.URL == "" {
		return fmt.Errorf("configure at least one of prowlarr or jackett")
	}
	if c.Prowlarr.URL != "" && c.Prowlarr.APIKey == "" {
		return fmt.Errorf("prowlarr.apiKey is required when prowlarr.url is set")
	}
	if c.Jackett.URL != "" && c.Jackett.APIKey == "" {
		return fmt.Errorf("jackett.apiKey is required when jackett.url is set")
	}
	return nil
}

func DefaultPath() string {
	if p := os.Getenv("CONFIG_FILE"); p != "" {
		return p
	}
	if _, err := os.Stat("config/config.yaml"); err == nil {
		return "config/config.yaml"
	}
	return "config/config.yaml.example"
}
