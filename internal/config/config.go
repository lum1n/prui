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
	Name       string   `mapstructure:"name"`
	Kind       string   `mapstructure:"kind"`
	BaseURL    string   `mapstructure:"base_url"`
	APIURL     string   `mapstructure:"api_url"`
	TokenEnv   string   `mapstructure:"token_env"`
	CookieEnv  string   `mapstructure:"cookie_env"`
	MatchHosts []string `mapstructure:"match_hosts"` // SSH / alternate hostnames
	Username   string   `mapstructure:"username"`
	CACert     string   `mapstructure:"ca_cert"`
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
		Name:       h.Name,
		Kind:       kind,
		BaseURL:    strings.TrimRight(h.BaseURL, "/"),
		APIURL:     strings.TrimRight(h.APIURL, "/"),
		TokenEnv:   h.TokenEnv,
		CookieEnv:  h.CookieEnv,
		MatchHosts: append([]string(nil), h.MatchHosts...),
		Username:   h.Username,
		CACert:     h.CACert,
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

// MatchHostname finds a configured host for a git/API hostname.
// It checks match_hosts, base_url, and api_url. It does not invent hosts.
func (c *Config) MatchHostname(hostname string) (domain.Host, bool) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if i := strings.IndexByte(hostname, ':'); i >= 0 {
		hostname = hostname[:i] // strip port
	}
	if hostname == "" {
		return domain.Host{}, false
	}

	for _, h := range c.Hosts {
		dh := h.ToDomain()
		for _, m := range dh.MatchHosts {
			if strings.EqualFold(strings.TrimSpace(m), hostname) {
				return dh, true
			}
		}
		for _, cand := range []string{dh.BaseURL, dh.APIURL} {
			if cand == "" {
				continue
			}
			if hostnameOf(cand) == hostname {
				return dh, true
			}
		}
	}
	return domain.Host{}, false
}

// ResolveRemoteHost maps a remote hostname to a configured host.
// Order: explicit hostname match → defaults.host → sole configured host.
func (c *Config) ResolveRemoteHost(hostname string) (domain.Host, error) {
	if host, ok := c.MatchHostname(hostname); ok {
		return host, nil
	}
	// Also accept full URLs via MatchBaseURL for https remotes.
	if host, ok := c.MatchBaseURL("https://" + hostname); ok {
		return host, nil
	}

	if c.Defaults.Host != "" {
		if host, err := c.FindHost(c.Defaults.Host); err == nil {
			return host, nil
		}
	}
	if len(c.Hosts) == 1 {
		return c.Hosts[0].ToDomain(), nil
	}

	var names []string
	for _, h := range c.Hosts {
		names = append(names, h.Name)
	}
	return domain.Host{}, fmt.Errorf(
		"git remote host %q is not mapped to any configured host %v\nAdd it under match_hosts, e.g.:\n\n  match_hosts:\n    - %s\n\nOr pass --host <name>",
		hostname, names, hostname,
	)
}

// MatchBaseURL finds a host whose base or API URL matches the given hostname/URL.
func (c *Config) MatchBaseURL(raw string) (domain.Host, bool) {
	raw = strings.TrimRight(strings.ToLower(raw), "/")
	host := hostnameOf(raw)
	if h, ok := c.MatchHostname(host); ok {
		return h, true
	}
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
			if raw == cand || strings.Contains(raw, hostnameOf(cand)) || strings.Contains(cand, host) {
				return dh, true
			}
		}
	}
	return domain.Host{}, false
}

func hostnameOf(u string) string {
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "ssh://")
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	if i := strings.IndexByte(u, ':'); i >= 0 {
		// host:port — but avoid breaking user@host (already stripped schemes)
		// only strip numeric ports
		port := u[i+1:]
		if port != "" && isAllDigits(port) {
			u = u[:i]
		}
	}
	return strings.ToLower(u)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}
