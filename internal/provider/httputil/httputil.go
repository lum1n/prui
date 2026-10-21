package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrUnauthorized marks responses that indicate missing or expired credentials:
// HTTP 401, or an HTML login page where an API response was expected.
var ErrUnauthorized = errors.New("unauthorized")

// IsUnauthorized reports whether err is (or wraps) ErrUnauthorized.
func IsUnauthorized(err error) bool { return errors.Is(err, ErrUnauthorized) }

type authError struct{ msg string }

func (e *authError) Error() string { return e.msg }
func (e *authError) Unwrap() error { return ErrUnauthorized }

// loginRedirect reports whether a request was redirected to a login/SSO page.
func loginRedirect(resp *http.Response, origURL string) bool {
	if resp.Request == nil || resp.Request.URL == nil {
		return false
	}
	final := resp.Request.URL.String()
	if final == origURL {
		return false
	}
	p := strings.ToLower(resp.Request.URL.Path)
	return strings.Contains(p, "login") || strings.Contains(p, "saml") || strings.Contains(p, "sso") || strings.Contains(p, "oauth")
}

func isHTML(resp *http.Response) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type"))), "text/html")
}

// DoJSON performs an HTTP request and decodes a JSON response into dest (optional).
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body any, dest any) (int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return resp.StatusCode, &authError{fmt.Sprintf("%s %s: %s (session expired or not logged in)", method, url, resp.Status)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return resp.StatusCode, fmt.Errorf("%s %s: %s (%s)", method, url, resp.Status, msg)
	}
	if isHTML(resp) || loginRedirect(resp, url) {
		return resp.StatusCode, &authError{fmt.Sprintf("%s %s: got a login page instead of JSON (session expired or not logged in)", method, url)}
	}
	if dest != nil && len(data) > 0 && string(data) != "null" {
		if err := json.Unmarshal(data, dest); err != nil {
			return resp.StatusCode, fmt.Errorf("decode json: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// DoRaw performs a request and returns the response body bytes.
func DoRaw(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode == http.StatusUnauthorized || (resp.StatusCode < 300 && loginRedirect(resp, url)) {
		return nil, resp.StatusCode, &authError{fmt.Sprintf("%s %s: %s (session expired or not logged in)", method, url, resp.Status)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return data, resp.StatusCode, fmt.Errorf("%s %s: %s (%s)", method, url, resp.Status, msg)
	}
	return data, resp.StatusCode, nil
}
