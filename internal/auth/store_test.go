package auth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/domain"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := auth.SaveToken("ghe.example.com", "gho_secret"); err != nil {
		t.Fatal(err)
	}
	tok, err := auth.LoadToken("https://GHE.Example.com/api/v3")
	if err != nil || tok != "gho_secret" {
		t.Fatalf("got %q %v", tok, err)
	}
	path := filepath.Join(dir, "prui", "credentials.json")
	if _, err := filepath.Glob(path); err != nil {
		t.Fatal(err)
	}
	cred, err := auth.Resolve(domain.Host{
		Name:    "ghe",
		Kind:    domain.HostGitHub,
		BaseURL: "https://ghe.example.com",
		APIURL:  "https://ghe.example.com/api/v3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "gho_secret" || cred.Source != "store" {
		t.Fatalf("%+v", cred)
	}
	if err := auth.DeleteToken("ghe.example.com"); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceFlow(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "dev",
				"user_code":        "ABCD-1234",
				"verification_uri": "http://example/device",
				"expires_in":       900,
				"interval":         1,
			})
		case r.URL.Path == "/login/oauth/access_token":
			n := polls.Add(1)
			if n < 2 {
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "gho_device"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	var buf strings.Builder
	err := auth.Login(context.Background(), auth.LoginOptions{
		Hostname:   "ghe.test",
		BaseURL:    srv.URL,
		ClientID:   "client",
		Out:        &buf,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ABCD-1234") {
		t.Fatalf("prompt: %s", buf.String())
	}
	tok, err := auth.LoadToken("ghe.test")
	if err != nil || tok != "gho_device" {
		t.Fatalf("%q %v", tok, err)
	}
}

func TestHostnameOf(t *testing.T) {
	if got := auth.HostnameOf("https://ghe.example.com/api/v3"); got != "ghe.example.com" {
		t.Fatal(got)
	}
}

// silence unused in case build tags change
var _ = io.Discard
