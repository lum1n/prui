package auth_test

import (
	"testing"

	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/domain"
)

func TestParseCookieInput(t *testing.T) {
	want := "BITBUCKETSESSIONID=abc; JSESSIONID=def"
	cases := map[string]string{
		"raw":         want,
		"header":      "Cookie: " + want,
		"header-lc":   "cookie:   " + want + "  ",
		"curl-H":      `curl 'https://bb.example.com/rest/api/1.0/x' \  -H 'accept: */*' \  -H 'cookie: ` + want + `' \  -H 'user-agent: x'`,
		"curl-H-dq":   `curl "https://bb.example.com/x" -H "Cookie: ` + want + `"`,
		"curl-b":      `curl 'https://bb.example.com/x' -H 'accept: */*' -b '` + want + `' --compressed`,
		"curl-cookie": `curl https://bb.example.com/x --cookie "` + want + `"`,
		"fetch":       `fetch("https://bb.example.com/x", {"headers": {"accept": "*/*", "cookie": "` + want + `"}, "method": "GET"});`,
		"headers":     "GET /x HTTP/1.1\nHost: bb.example.com\nCookie: " + want + "\nAccept: */*",
	}
	for name, in := range cases {
		got, err := auth.ParseCookieInput(in)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v", name, got, err)
		}
	}
	for _, bad := range []string{"", "   ", "curl https://bb.example.com/x -H 'accept: */*'", "notacookie"} {
		if got, err := auth.ParseCookieInput(bad); err == nil {
			t.Errorf("%q: expected error, got %q", bad, got)
		}
	}
}

func TestResolveStoredCookie(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("BB_COOKIE", "")
	t.Setenv("BITBUCKET_TOKEN", "")
	host := domain.Host{Name: "bb", Kind: domain.HostBitbucketDC, BaseURL: "https://bb.example.com", CookieEnv: "BB_COOKIE"}

	if _, err := auth.Resolve(host); err == nil {
		t.Fatal("expected error without cookie")
	}
	if err := auth.SaveCookie("bb.example.com", "Cookie: S=1"); err != nil {
		t.Fatal(err)
	}
	cred, err := auth.Resolve(host)
	if err != nil || cred.Cookie != "S=1" || cred.Source != "stored-cookie" {
		t.Fatalf("got %+v %v", cred, err)
	}

	t.Setenv("BB_COOKIE", "S=env")
	if cred, _ := auth.Resolve(host); cred.Cookie != "S=env" {
		t.Fatalf("env cookie should win, got %q", cred.Cookie)
	}

	t.Setenv("BB_COOKIE", "")
	if err := auth.DeleteCookie("bb.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Resolve(host); err == nil {
		t.Fatal("expected error after delete")
	}

	// Stored cookies only apply to Bitbucket DC.
	_ = auth.SaveCookie("ghe.example.com", "S=1")
	gh := domain.Host{Name: "ghe", Kind: domain.HostGitHub, BaseURL: "https://ghe.example.com", CookieEnv: "BB_COOKIE"}
	if cred, _ := auth.Resolve(gh); cred.Cookie != "" {
		t.Fatalf("unexpected cookie for GitHub host: %q", cred.Cookie)
	}
}
