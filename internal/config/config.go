package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"github.com/vegard/prui/internal/domain"
)

// Config is the loaded application configuration.
type Config struct {
	Hosts    []HostConfig `mapstructure:"hosts"`
	Defaults Defaults     `mapstructure:"defaults"`
	UI       UIConfig     `mapstructure:"ui"`
}

// HostConfig is one forge endpoint in YAML.
type HostConfig struct {
	Name      string `mapstructure:"name"`
	Kind      string `mapstructure:"kind"`
	BaseURL   string `mapstructure:"base_url"`
	APIURL    string `mapstructure:"api_url"`
	TokenEnv  string `mapstructure:"token_env"`
	CookieEnv string `mapstructure:"cookie_env"`
	Username  string `mapstructure:"username"`
	CACert    string `mapstructure:"ca_cert"`
}

// Defaults holds default host selection.
type Defaults struct {
	Host string `mapstructure:"host"`
}

// UIConfig controls TUI presentation.
type UIConfig struct {
	Diff  string `mapstructure:"diff"`  // unified | split
	Theme string `mapstructure:"theme"` // dark | light
}

// Load reads config from file, env, and defaults.
func Load(cfgFile string) (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("PRUI")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	v.SetDefault("ui.diff", "unified")
	v.SetDefault("ui.theme", "dark")

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		dir, err := ConfigDir()
		if err != nil {
			return nil, err
		}
		v.AddConfigPath(dir)
		v.AddConfigPath(".")
		v.SetConfigName("config")
		v.SetConfigType("yaml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	if len(cfg.Hosts) == 0 {
		cfg.Hosts = defaultHosts()
	}
	if cfg.UI.Diff == "" {
		cfg.UI.Diff = "unified"
	}
	if cfg.UI.Theme == "" {
		cfg.UI.Theme = "dark"
	}
	return &cfg, nil
}

func defaultHosts() []HostConfig {
	return []HostConfig{
		{
			Name:     "github",
			Kind:     string(domain.HostGitHub),
			BaseURL:  "https://github.com",
			APIURL:   "https://api.github.com/",
			TokenEnv: "GITHUB_TOKEN",
		},
		{
			Name:     "bitbucket",
			Kind:     string(domain.HostBitbucketCloud),
			BaseURL:  "https://bitbucket.org",
			APIURL:   "https://api.bitbucket.org/2.0",
			TokenEnv: "BITBUCKET_TOKEN",
		},
	}
}

// ConfigDir returns ~/.config/prui (or XDG).
func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "prui"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "prui"), nil
}

// DraftsDir returns the directory for persisted draft reviews.
func DraftsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "drafts"), nil
}

// ToDomain converts a host config entry to a domain.Host.
func (h HostConfig) ToDomain() domain.Host {
	kind := domain.HostKind(h.Kind)
	switch kind {
	case domain.HostGitHub, domain.HostBitbucketCloud, domain.HostBitbucketDC:
	default:
		kind = domain.HostGitHub
	}
	return domain.Host{
		Name:      h.Name,
		Kind:      kind,
		BaseURL:   strings.TrimRight(h.BaseURL, "/"),
		APIURL:    strings.TrimRight(h.APIURL, "/"),
		TokenEnv:  h.TokenEnv,
		CookieEnv: h.CookieEnv,
		Username:  h.Username,
		CACert:    h.CACert,
	}
}

// FindHost returns a host by name, or the default, or the first entry.
func (c *Config) FindHost(name string) (domain.Host, error) {
	if name == "" {
		name = c.Defaults.Host
	}
	if name != "" {
		for _, h := range c.Hosts {
			if h.Name == name {
				return h.ToDomain(), nil
			}
		}
		return domain.Host{}, fmt.Errorf("host %q not found in config", name)
	}
	if len(c.Hosts) == 0 {
		return domain.Host{}, fmt.Errorf("no hosts configured")
	}
	return c.Hosts[0].ToDomain(), nil
}

// MatchBaseURL finds a host whose base or API URL matches the given hostname/URL.
func (c *Config) MatchBaseURL(raw string) (domain.Host, bool) {
	raw = strings.TrimRight(strings.ToLower(raw), "/")
	for _, h := range c.Hosts {
		dh := h.ToDomain()
		candidates := []string{
			strings.ToLower(dh.BaseURL),
			strings.ToLower(dh.APIURL),
		}
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			if raw == cand || strings.Contains(raw, hostnameOf(cand)) || strings.Contains(cand, hostnameOf(raw)) {
				return dh, true
			}
		}
	}
	return domain.Host{}, false
}

func hostnameOf(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}
