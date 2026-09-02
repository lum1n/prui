// Package ai provides LLM completers for PR summarization.
package ai

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/domain"
)

// Completer generates text completions.
type Completer interface {
	Kind() string
	Complete(ctx context.Context, req Request) (string, error)
}

// Request is a single completion call.
type Request struct {
	System string
	User   string
	Model  string
}

// Options configures Completer construction.
type Options struct {
	Provider   config.AIProvider
	Config     *config.Config
	ActiveHost domain.Host
	HTTPClient *http.Client
}

// New builds a Completer from provider kind.
func New(opts Options) (Completer, error) {
	kind := strings.ToLower(strings.TrimSpace(opts.Provider.Kind))
	switch kind {
	case "claude":
		return newClaude(opts)
	case "copilot":
		return newCopilot(opts)
	case "codex":
		return newCodex(opts.Provider)
	case "opencode":
		return newOpenCode(opts.Provider)
	default:
		return nil, fmt.Errorf("unsupported ai provider kind %q", opts.Provider.Kind)
	}
}

func httpClient(opts Options) *http.Client {
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}
	return &http.Client{Timeout: 120 * time.Second}
}

func resolveToken(envName, fallbackEnv string) (string, error) {
	name := strings.TrimSpace(envName)
	if name == "" {
		name = fallbackEnv
	}
	if name == "" {
		return "", fmt.Errorf("token_env is not set")
	}
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return "", fmt.Errorf("token not set: export %s", name)
	}
	return v, nil
}

func resolveGitHubCred(opts Options) (apiURL string, cred auth.Credentials, host domain.Host, err error) {
	p := opts.Provider
	host = opts.ActiveHost

	if name := strings.TrimSpace(p.GitHubHost); name != "" {
		if opts.Config == nil {
			return "", cred, host, fmt.Errorf("github_host %q set but config is nil", name)
		}
		h, ok := opts.Config.FindHostConfig(name)
		if !ok {
			return "", cred, host, fmt.Errorf("github_host %q not found in hosts", name)
		}
		host = h.ToDomain()
	} else if strings.TrimSpace(p.APIURL) != "" || strings.TrimSpace(p.TokenEnv) != "" {
		api := strings.TrimRight(strings.TrimSpace(p.APIURL), "/")
		if api == "" {
			api = "https://api.github.com"
		}
		host = domain.Host{
			Name:     "copilot",
			Kind:     domain.HostGitHub,
			APIURL:   api,
			BaseURL:  api,
			TokenEnv: p.TokenEnv,
		}
	} else if host.Kind != domain.HostGitHub {
		host = domain.Host{
			Name:     "github",
			Kind:     domain.HostGitHub,
			BaseURL:  "https://github.com",
			APIURL:   "https://api.github.com",
			TokenEnv: "GITHUB_TOKEN",
		}
	}

	if te := strings.TrimSpace(p.TokenEnv); te != "" {
		host.TokenEnv = te
		host.CookieEnv = "" // explicit token_env on provider wins over cookie
	}
	if api := strings.TrimRight(strings.TrimSpace(p.APIURL), "/"); api != "" {
		host.APIURL = api
	}

	cred, err = auth.Resolve(host)
	if err != nil {
		return "", cred, host, err
	}
	apiURL = strings.TrimRight(host.APIURL, "/")
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	return apiURL, cred, host, nil
}
