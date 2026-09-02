package auth

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/vegard/prui/internal/domain"
)

// Credentials holds resolved auth material for a host.
type Credentials struct {
	Token    string
	Cookie   string // raw Cookie header value for session auth
	Username string
}

// HasAuth reports whether any usable credential is present.
func (c Credentials) HasAuth() bool {
	return c.Token != "" || c.Cookie != ""
}

// Resolve loads credentials from cookie_env and/or token_env.
// Cookie auth is preferred when cookie_env is configured and set (typical for GHE / Bitbucket Server).
func Resolve(host domain.Host) (Credentials, error) {
	cred := Credentials{Username: host.Username}

	if host.CookieEnv != "" {
		if v := strings.TrimSpace(os.Getenv(host.CookieEnv)); v != "" {
			cred.Cookie = normalizeCookie(v)
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
			return cred, nil
		}
	}

	if host.Kind == domain.HostGitHub && isGitHubDotCom(host) && host.CookieEnv == "" {
		token, err := ghAuthToken()
		if err == nil && token != "" {
			cred.Token = token
			return cred, nil
		}
	}

	switch {
	case host.CookieEnv != "" && envName != "":
		return cred, fmt.Errorf("auth not set: export %s (cookie) or %s (token) for host %q", host.CookieEnv, envName, host.Name)
	case host.CookieEnv != "":
		return cred, fmt.Errorf("cookie not set: export %s (host %q)", host.CookieEnv, host.Name)
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

func isGitHubDotCom(host domain.Host) bool {
	base := strings.ToLower(host.BaseURL)
	api := strings.ToLower(host.APIURL)
	return strings.Contains(base, "github.com") || api == "" || strings.Contains(api, "api.github.com")
}

func ghAuthToken() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
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
		return fmt.Sprintf("%s (%s): ok cookie %s", host.Name, host.Kind, mask(cred.Cookie))
	}
	user := cred.Username
	if user == "" {
		user = "(token)"
	}
	return fmt.Sprintf("%s (%s): ok token %s as %s", host.Name, host.Kind, mask(cred.Token), user)
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
