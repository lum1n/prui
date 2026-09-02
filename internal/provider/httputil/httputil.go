package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return resp.StatusCode, fmt.Errorf("%s %s: %s (%s)", method, url, resp.Status, msg)
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 400 {
			msg = msg[:400] + "…"
		}
		return data, resp.StatusCode, fmt.Errorf("%s %s: %s (%s)", method, url, resp.Status, msg)
	}
	return data, resp.StatusCode, nil
}
