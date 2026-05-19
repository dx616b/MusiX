package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dx616b/musicx/internal/config"
	"github.com/dx616b/musicx/internal/jackett"
	"github.com/dx616b/musicx/internal/prowlarr"
	"github.com/dx616b/musicx/internal/search"
	"github.com/dx616b/musicx/internal/transmission"
	"gopkg.in/yaml.v3"
)

// Public is returned by GET /api/settings (secrets masked).
type Public struct {
	ConfigPath   string                 `json:"configPath"`
	OverridePath string                 `json:"overridePath"`
	Prowlarr     IntegrationPublic      `json:"prowlarr"`
	Jackett      IntegrationPublic      `json:"jackett"`
	Transmission TransmissionPublic     `json:"transmission"`
}

type IntegrationPublic struct {
	URL       string `json:"url"`
	APIKeySet bool   `json:"apiKeySet"`
	APIKey    string `json:"apiKey,omitempty"` // masked when set
}

type TransmissionPublic struct {
	URL         string `json:"url"`
	Username    string `json:"username"`
	PasswordSet bool   `json:"passwordSet"`
}

// Update is the PUT /api/settings body. Empty apiKey/password keeps the stored value.
type Update struct {
	Prowlarr     *IntegrationUpdate     `json:"prowlarr,omitempty"`
	Jackett      *IntegrationUpdate     `json:"jackett,omitempty"`
	Transmission *TransmissionUpdate    `json:"transmission,omitempty"`
}

type IntegrationUpdate struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

type TransmissionUpdate struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Manager struct {
	mu           sync.RWMutex
	configPath   string
	overridePath string
	cfg          *config.Config

	search *search.Service
	tx     *transmission.Client
	pr     **prowlarr.Prowlarr
}

func NewManager(cfgPath string, cfg *config.Config, searchSvc *search.Service, tx *transmission.Client, pr **prowlarr.Prowlarr) *Manager {
	return &Manager{
		configPath:   cfgPath,
		overridePath: config.OverridePath(cfg),
		cfg:          cloneConfig(cfg),
		search:       searchSvc,
		tx:           tx,
		pr:           pr,
	}
}

func (m *Manager) Get() Public {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toPublic()
}

func (m *Manager) Update(u Update) (Public, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	next := cloneConfig(m.cfg)
	if u.Prowlarr != nil {
		next.Prowlarr.URL = strings.TrimSpace(u.Prowlarr.URL)
		if next.Prowlarr.URL == "" {
			next.Prowlarr.APIKey = ""
		} else if k := strings.TrimSpace(u.Prowlarr.APIKey); k != "" {
			next.Prowlarr.APIKey = k
		}
	}
	if u.Jackett != nil {
		next.Jackett.URL = strings.TrimSpace(u.Jackett.URL)
		if next.Jackett.URL == "" {
			next.Jackett.APIKey = ""
		} else if k := strings.TrimSpace(u.Jackett.APIKey); k != "" {
			next.Jackett.APIKey = k
		}
	}
	if u.Transmission != nil {
		next.Transmission.URL = strings.TrimSpace(u.Transmission.URL)
		next.Transmission.Username = strings.TrimSpace(u.Transmission.Username)
		if p := u.Transmission.Password; p != "" {
			next.Transmission.Password = p
		}
	}

	if err := next.Validate(); err != nil {
		return Public{}, err
	}
	if err := m.saveOverride(next); err != nil {
		return Public{}, err
	}
	if err := m.applyLocked(next); err != nil {
		return Public{}, err
	}
	m.cfg = next
	return m.toPublic(), nil
}

func (m *Manager) saveOverride(cfg *config.Config) error {
	payload := struct {
		Prowlarr     config.ProwlarrConfig     `yaml:"prowlarr"`
		Jackett      config.JackettConfig      `yaml:"jackett"`
		Transmission config.TransmissionConfig `yaml:"transmission"`
	}{
		Prowlarr:     cfg.Prowlarr,
		Jackett:      cfg.Jackett,
		Transmission: cfg.Transmission,
	}
	if err := os.MkdirAll(filepath.Dir(m.overridePath), 0o755); err != nil {
		return fmt.Errorf("settings dir: %w", err)
	}
	data, err := yaml.Marshal(&payload)
	if err != nil {
		return err
	}
	return os.WriteFile(m.overridePath, data, 0o600)
}

func (m *Manager) applyLocked(cfg *config.Config) error {
	var pr *prowlarr.Prowlarr
	if cfg.Prowlarr.URL != "" {
		pr = prowlarr.New(cfg.Prowlarr.URL, cfg.Prowlarr.APIKey)
	}
	var jk *jackett.Jackett
	if cfg.Jackett.URL != "" {
		jk = jackett.New(cfg.Jackett.URL, cfg.Jackett.APIKey)
	}
	m.search.Prowlarr = pr
	m.search.Jackett = jk
	*m.pr = pr
	m.tx.Configure(cfg.Transmission.URL, cfg.Transmission.Username, cfg.Transmission.Password)
	return nil
}

func (m *Manager) toPublic() Public {
	return Public{
		ConfigPath:   m.configPath,
		OverridePath: m.overridePath,
		Prowlarr: IntegrationPublic{
			URL:       m.cfg.Prowlarr.URL,
			APIKeySet: m.cfg.Prowlarr.APIKey != "",
			APIKey:    maskSecret(m.cfg.Prowlarr.APIKey),
		},
		Jackett: IntegrationPublic{
			URL:       m.cfg.Jackett.URL,
			APIKeySet: m.cfg.Jackett.APIKey != "",
			APIKey:    maskSecret(m.cfg.Jackett.APIKey),
		},
		Transmission: TransmissionPublic{
			URL:         m.cfg.Transmission.URL,
			Username:    m.cfg.Transmission.Username,
			PasswordSet: m.cfg.Transmission.Password != "",
		},
	}
}

func cloneConfig(c *config.Config) *config.Config {
	if c == nil {
		return &config.Config{}
	}
	cp := *c
	return &cp
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
