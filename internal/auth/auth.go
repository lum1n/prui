package auth

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/lum1n/prui/internal/domain"
)

// Credentials holds resolved auth material for a host.
type Credentials struct {
	Token    string
	Cookie   string // raw Cookie header value for session auth
	Username string
	Source   string // env | store | gh | cookie | stored-cookie
}

// HasAuth reports whether any usable credential is present.
func (c Credentials) HasAuth() bool {
	return c.Token != "" || c.Cookie != ""
}

// Resolve loads credentials from cookie_env, token_env, stored login, or gh.
// Order: cookie (if set) → token_env → ~/.config/prui/credentials.json → gh auth token.
// Bitbucket DC falls back to a cookie saved from the TUI session login.
func Resolve(host domain.Host) (Credentials, error) {
	cred := Credentials{Username: host.Username}

	if host.CookieEnv != "" {
		if v := strings.TrimSpace(os.Getenv(host.CookieEnv)); v != "" {
			cred.Cookie = normalizeCookie(v)
			cred.Source = "cookie"
			return cred, nil
		}
	}

	envName := host.TokenEnv
	if envName == "" && host.CookieEnv == "" {
		switch host.Kind {
		case domain.HostGitHub:
			envName = "GITHUB_TOKEN"
		case domain.HostBitbucketCloud, domain.HostBitbucketDC:
			envName = "BITBUCKET_TOKEN"
		}
	}

	if envName != "" {
		if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
			cred.Token = v
			cred.Source = "env"
			return cred, nil
		}
	}

	if SupportsCookieLogin(host) {
		if c, err := LoadCookie(Hostname(host)); err == nil && c != "" {
			cred.Cookie = c
			cred.Source = "stored-cookie"
			return cred, nil
		}
	}

	if host.Kind == domain.HostGitHub {
		hn := githubHostname(host)
		if tok, err := LoadToken(hn); err == nil && tok != "" {
			cred.Token = tok
			cred.Source = "store"
			return cred, nil
		}
		if tok, err := ghAuthTokenHostname(hn); err == nil && tok != "" {
			cred.Token = tok
			cred.Source = "gh"
			return cred, nil
		}
		return cred, fmt.Errorf("auth not set for %q (%s); run: prui auth login --hostname %s", host.Name, hn, hn)
	}

	switch {
	case host.CookieEnv != "" && envName != "":
		return cred, fmt.Errorf("auth not set: export %s (cookie) or %s (token) for host %q", host.CookieEnv, envName, host.Name)
	case host.CookieEnv != "":
		return cred, fmt.Errorf("cookie not set: export %s or log in from the TUI (host %q)", host.CookieEnv, host.Name)
	case envName != "":
		return cred, fmt.Errorf("token not set: export %s (host %q)", envName, host.Name)
	default:
		return cred, fmt.Errorf("no auth configured for host %q (set cookie_env or token_env)", host.Name)
	}
}

// normalizeCookie accepts either a raw Cookie header value or a full "Cookie: …" line.
func normalizeCookie(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "cookie:") {
		v = strings.TrimSpace(v[7:])
	}
	return v
}

// SupportsCookieLogin reports whether the host accepts browser session cookies
// that can be refreshed from the TUI.
func SupportsCookieLogin(host domain.Host) bool {
	return host.Kind == domain.HostBitbucketDC
}

// Hostname returns the forge hostname for host (base URL, else API URL).
func Hostname(host domain.Host) string {
	if h := HostnameOf(host.BaseURL); h != "" {
		return h
	}
	return HostnameOf(host.APIURL)
}

func isGitHubDotCom(host domain.Host) bool {
	return githubHostname(host) == "github.com" || githubHostname(host) == "api.github.com"
}

func githubHostname(host domain.Host) string {
	for _, cand := range []string{host.BaseURL, host.APIURL} {
		h := HostnameOf(cand)
		if h == "" {
			continue
		}
		if h == "api.github.com" {
			return "github.com"
		}
		// Strip GHE API path hosts that are still the enterprise hostname.
		return h
	}
	return "github.com"
}

func ghAuthToken() (string, error) {
	return ghAuthTokenHostname("github.com")
}

func ghAuthTokenHostname(hostname string) (string, error) {
	hostname = normalizeHostname(hostname)
	if hostname == "" || hostname == "github.com" || hostname == "api.github.com" {
		cmd := exec.Command("gh", "auth", "token")
		out, err := cmd.Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	cmd := exec.Command("gh", "auth", "token", "--hostname", hostname)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// StatusLine returns a short auth status string for a host.
func StatusLine(host domain.Host) string {
	cred, err := Resolve(host)
	if err != nil {
		return fmt.Sprintf("%s (%s): missing — %v", host.Name, host.Kind, err)
	}
	if cred.Cookie != "" {
		src := "env"
		if cred.Source == "stored-cookie" {
			src = "saved"
		}
		return fmt.Sprintf("%s (%s): ok cookie (%s) %s", host.Name, host.Kind, src, mask(cred.Cookie))
	}
	user := cred.Username
	if user == "" {
		user = "(token)"
	}
	src := cred.Source
	if src == "" {
		src = "token"
	}
	return fmt.Sprintf("%s (%s): ok %s %s as %s", host.Name, host.Kind, src, mask(cred.Token), user)
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "…" + s[len(s)-4:]
}

// CookieRoundTripper injects a Cookie header on every request.
type CookieRoundTripper struct {
	Cookie string
	Base   http.RoundTripper
}

// RoundTrip implements http.RoundTripper.
func (t *CookieRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Cookie", t.Cookie)
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// WrapClient returns an HTTP client that sends the Cookie header when set.
func WrapClient(base *http.Client, cookie string) *http.Client {
	if cookie == "" {
		return base
	}
	if base == nil {
		base = &http.Client{}
	}
	out := *base
	out.Transport = &CookieRoundTripper{Cookie: cookie, Base: base.Transport}
	return &out
}
