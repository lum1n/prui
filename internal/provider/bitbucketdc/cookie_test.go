package bitbucketdc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider/httputil"
)

func TestCookieValidateAndSwap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok := strings.Contains(r.Header.Get("Cookie"), "S=good")
		switch {
		case r.URL.Path == "/plugins/servlet/applinks/whoami":
			if ok {
				_, _ = w.Write([]byte("jdoe"))
			}
		case r.URL.Path == "/login":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>login</html>"))
		case !ok:
			http.Redirect(w, r, "/login?next=x", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"values":[]}`))
		}
	}))
	t.Cleanup(srv.Close)

	c, err := New(domain.Host{Name: "bb", Kind: domain.HostBitbucketDC, BaseURL: srv.URL}, auth.Credentials{Cookie: "S=old"}, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := domain.RepoRef{Owner: "P", Name: "r"}

	if _, err := c.ListPullRequests(ctx, repo, domain.ListOpts{}); !httputil.IsUnauthorized(err) {
		t.Fatalf("expected auth error for login redirect, got %v", err)
	}
	if _, err := c.ValidateCookie(ctx, "S=bad"); err == nil {
		t.Fatal("expected bad cookie to be rejected")
	}
	user, err := c.ValidateCookie(ctx, "S=good")
	if err != nil || user != "jdoe" {
		t.Fatalf("got %q %v", user, err)
	}
	c.SetCookie("S=good")
	if _, err := c.ListPullRequests(ctx, repo, domain.ListOpts{}); err != nil {
		t.Fatalf("after swap: %v", err)
	}
}

func TestUnauthorizedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	c, _ := New(domain.Host{Name: "bb", Kind: domain.HostBitbucketDC, BaseURL: srv.URL}, auth.Credentials{Cookie: "S=x"}, srv.Client())
	if _, err := c.ListPullRequests(context.Background(), domain.RepoRef{Owner: "P", Name: "r"}, domain.ListOpts{}); !httputil.IsUnauthorized(err) {
		t.Fatalf("expected auth error, got %v", err)
	}
}
