package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LoginOptions configures device / gh login.
type LoginOptions struct {
	// Hostname is the forge host, e.g. "ghe.example.com" or "github.com".
	Hostname string
	// BaseURL is https://hostname (derived from Hostname if empty).
	BaseURL string
	// ClientID enables native GitHub device flow. When empty, login wraps `gh auth login`.
	ClientID string
	// ClientSecret is optional; some GHE OAuth apps require it when polling.
	ClientSecret string
	// Scopes for native device flow.
	Scopes string
	// Out receives user-facing prompts (defaults to stdout).
	Out io.Writer
	// HTTPClient is optional.
	HTTPClient *http.Client
}

// Login runs device-code auth and stores the token for Hostname.
// Prefer wrapping `gh` when ClientID is empty (no OAuth App setup on GHE).
func Login(ctx context.Context, opts LoginOptions) error {
	host := normalizeHostname(opts.Hostname)
	if host == "" && opts.BaseURL != "" {
		host = HostnameOf(opts.BaseURL)
	}
	if host == "" {
		return fmt.Errorf("hostname is required")
	}
	opts.Hostname = host
	if opts.BaseURL == "" {
		opts.BaseURL = "https://" + host
	}
	opts.BaseURL = strings.TrimRight(opts.BaseURL, "/")
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if strings.TrimSpace(opts.Scopes) == "" {
		opts.Scopes = "repo gist read:org read:user"
	}

	if strings.TrimSpace(opts.ClientID) != "" {
		token, err := deviceFlow(ctx, opts)
		if err != nil {
			return err
		}
		if err := SaveToken(host, token); err != nil {
			return err
		}
		fmt.Fprintf(opts.Out, "Logged in to %s (token saved to ~/.config/prui/credentials.json)\n", host)
		return nil
	}

	if err := ghLogin(ctx, host, opts.Out); err != nil {
		return err
	}
	token, err := ghAuthTokenHostname(host)
	if err != nil || token == "" {
		return fmt.Errorf("gh login finished but could not read token for %s: %v\nRun: gh auth token --hostname %s", host, err, host)
	}
	if err := SaveToken(host, token); err != nil {
		return fmt.Errorf("gh login ok but failed to save token: %w", err)
	}
	fmt.Fprintf(opts.Out, "Logged in to %s via gh (token saved for prui)\n", host)
	return nil
}

func ghLogin(ctx context.Context, hostname string, out io.Writer) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh not found in PATH; install GitHub CLI, or set oauth_client_id for native device login")
	}
	fmt.Fprintf(out, "Starting gh device login for %s…\n", hostname)
	args := []string{"auth", "login", "--hostname", hostname, "--git-protocol", "https", "--web"}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// Retry without --web (device code / prompts).
		fmt.Fprintf(out, "Browser login failed or unavailable; trying interactive gh login…\n")
		cmd = exec.CommandContext(ctx, "gh", "auth", "login", "--hostname", hostname, "--git-protocol", "https")
		cmd.Stdin = os.Stdin
		cmd.Stdout = out
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("gh auth login: %w", err)
		}
	}
	return nil
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func deviceFlow(ctx context.Context, opts LoginOptions) (string, error) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	form := url.Values{}
	form.Set("client_id", opts.ClientID)
	form.Set("scope", opts.Scopes)

	codeURL := opts.BaseURL + "/login/device/code"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("device code: HTTP %d: %s", resp.StatusCode, truncate(body, 300))
	}
	var dc deviceCodeResponse
	if err := json.Unmarshal(body, &dc); err != nil {
		return "", fmt.Errorf("device code decode: %w", err)
	}
	if dc.DeviceCode == "" || dc.UserCode == "" {
		return "", fmt.Errorf("device code: incomplete response")
	}
	verify := dc.VerificationURI
	if verify == "" {
		verify = opts.BaseURL + "/login/device"
	}
	interval := dc.Interval
	if interval < 5 {
		interval = 5
	}
	fmt.Fprintf(opts.Out, "\nOpen %s and enter code: %s\nWaiting for authorization…\n", verify, dc.UserCode)

	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
	if dc.ExpiresIn <= 0 {
		deadline = time.Now().Add(15 * time.Minute)
	}
	tokenURL := opts.BaseURL + "/login/oauth/access_token"
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("device code expired")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}

		poll := url.Values{}
		poll.Set("client_id", opts.ClientID)
		poll.Set("device_code", dc.DeviceCode)
		poll.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
		if opts.ClientSecret != "" {
			poll.Set("client_secret", opts.ClientSecret)
		}
		preq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(poll.Encode()))
		if err != nil {
			return "", err
		}
		preq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		preq.Header.Set("Accept", "application/json")
		presp, err := client.Do(preq)
		if err != nil {
			return "", err
		}
		pdata, _ := io.ReadAll(presp.Body)
		presp.Body.Close()

		var tok struct {
			AccessToken string `json:"access_token"`
			Error       string `json:"error"`
			ErrorDesc   string `json:"error_description"`
		}
		_ = json.Unmarshal(pdata, &tok)
		if tok.AccessToken != "" {
			return tok.AccessToken, nil
		}
		switch tok.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5
			continue
		case "expired_token", "access_denied":
			return "", fmt.Errorf("device login: %s (%s)", tok.Error, tok.ErrorDesc)
		default:
			if presp.StatusCode >= 300 {
				return "", fmt.Errorf("device token: HTTP %d: %s", presp.StatusCode, truncate(pdata, 300))
			}
			if tok.Error != "" {
				return "", fmt.Errorf("device login: %s (%s)", tok.Error, tok.ErrorDesc)
			}
			return "", fmt.Errorf("device token: unexpected response: %s", truncate(pdata, 300))
		}
	}
}

func truncate(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
