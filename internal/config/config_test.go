package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lum1n/prui/internal/config"
	"github.com/lum1n/prui/internal/domain"
)

func TestLoadWithHosts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
hosts:
  - name: ghe
    kind: github
    base_url: https://ghe.example.com
    api_url: https://ghe.example.com/api/v3
    cookie_env: GHE_COOKIE
    ca_cert: /tmp/ca.pem
defaults:
  host: ghe
ui:
  theme: dark
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := cfg.FindHost("")
	if err != nil {
		t.Fatal(err)
	}
	if h.Kind != domain.HostGitHub || h.CACert != "/tmp/ca.pem" || h.CookieEnv != "GHE_COOKIE" {
		t.Fatalf("%+v", h)
	}
	matched, ok := cfg.MatchBaseURL("https://ghe.example.com/org/repo")
	if !ok || matched.Name != "ghe" {
		t.Fatalf("match: %+v %v", matched, ok)
	}
}

func TestLoadAIConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
hosts:
  - name: github
    kind: github
    base_url: https://github.com
    api_url: https://api.github.com/
ai:
  default: claude
  providers:
    claude:
      kind: claude
      model: claude-test
      token_env: ANTHROPIC_API_KEY
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AIConfigured() {
		t.Fatal("expected AI configured")
	}
	p, err := cfg.DefaultAIProvider()
	if err != nil || p.Kind != "claude" || p.Model != "claude-test" {
		t.Fatalf("%+v %v", p, err)
	}
	if cfg.AI.MaxContextBytes != 120000 || cfg.AI.TimeoutSec != 120 {
		t.Fatalf("defaults: %+v", cfg.AI)
	}
}
