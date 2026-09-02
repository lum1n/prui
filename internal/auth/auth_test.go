package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/domain"
)

func TestResolveCookiePreferred(t *testing.T) {
	t.Setenv("GHE_COOKIE", "user_session=abc; _gh_sess=xyz")
	t.Setenv("GHE_TOKEN", "should-not-win")
	cred, err := auth.Resolve(domain.Host{
		Name:      "ghe",
		Kind:      domain.HostGitHub,
		CookieEnv: "GHE_COOKIE",
		TokenEnv:  "GHE_TOKEN",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Cookie != "user_session=abc; _gh_sess=xyz" {
		t.Fatalf("cookie %q", cred.Cookie)
	}
	if cred.Token != "" {
		t.Fatal("expected cookie to win over token")
	}
}

func TestResolveCookieHeaderPrefix(t *testing.T) {
	t.Setenv("BB_COOKIE", "Cookie: JSESSIONID=deadbeef")
	cred, err := auth.Resolve(domain.Host{
		Name:      "bb",
		Kind:      domain.HostBitbucketDC,
		CookieEnv: "BB_COOKIE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Cookie != "JSESSIONID=deadbeef" {
		t.Fatalf("got %q", cred.Cookie)
	}
}

func TestResolveCookieMissing(t *testing.T) {
	t.Setenv("GHE_COOKIE", "")
	_, err := auth.Resolve(domain.Host{
		Name:      "ghe",
		Kind:      domain.HostGitHub,
		BaseURL:   "https://ghe.example.com",
		CookieEnv: "GHE_COOKIE",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCookieRoundTripper(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := auth.WrapClient(srv.Client(), "a=1; b=2")
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got != "a=1; b=2" {
		t.Fatalf("Cookie header %q", got)
	}
}
