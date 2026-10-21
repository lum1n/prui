package ui

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lum1n/prui/internal/auth"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider"
	"github.com/lum1n/prui/internal/provider/httputil"
)

type fakeCookieHost struct {
	provider.Host
	cookie string
}

func (f *fakeCookieHost) Cookie() string     { return f.cookie }
func (f *fakeCookieHost) SetCookie(c string) { f.cookie = c }
func (f *fakeCookieHost) ValidateCookie(_ context.Context, c string) (string, error) {
	if c != "S=good" {
		return "", errors.New("cookie rejected")
	}
	return "jdoe", nil
}
func (f *fakeCookieHost) ListPullRequests(context.Context, domain.RepoRef, domain.ListOpts) ([]domain.PullRequest, error) {
	if f.cookie != "S=good" {
		return nil, fmt.Errorf("GET x: %w", httputil.ErrUnauthorized)
	}
	return []domain.PullRequest{{Ref: domain.PRRef{Number: 1}, Title: "t"}}, nil
}
func (f *fakeCookieHost) GetReviewStatus(context.Context, domain.PRRef) (domain.ReviewStatus, error) {
	return domain.ReviewStatus{}, nil
}

func TestCookieLoginFlow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fh := &fakeCookieHost{cookie: "S=expired"}
	m := NewModel(Options{
		Host:     domain.Host{Name: "bb", Kind: domain.HostBitbucketDC, BaseURL: "https://bb.example.com"},
		Provider: fh,
	})
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	// Expired session → dialog opens.
	tm, _ = tm.Update(m.loadPRs()())
	if !tm.(Model).login.active {
		t.Fatal("expected login dialog after auth error")
	}

	// Bad paste keeps dialog open with an error.
	mm := tm.(Model)
	mm.login.input.SetValue("Cookie: S=bad")
	tm, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm, _ = tm.Update(cmd())
	if mm = tm.(Model); !mm.login.active || mm.login.err == "" {
		t.Fatalf("expected rejection, got %+v", mm.login)
	}

	// Good cURL paste → cookie swapped, saved, reload succeeds.
	mm.login.input.SetValue(`curl 'https://bb.example.com/x' -H 'cookie: S=good'`)
	tm, cmd = mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	tm, cmd = tm.Update(cmd())
	if mm = tm.(Model); mm.login.active || fh.cookie != "S=good" {
		t.Fatalf("expected swap, active=%v cookie=%q", mm.login.active, fh.cookie)
	}
	tm, _ = tm.Update(cmd())
	if mm = tm.(Model); len(mm.prs) != 1 || mm.errMsg != "" {
		t.Fatalf("reload failed: prs=%d err=%q", len(mm.prs), mm.errMsg)
	}
	if v := mm.View(); v == "" {
		t.Fatal("empty view")
	}
}

func TestCookieLoginRetriesStoredCookieFirst(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveTestCookie("bb.example.com", "S=good"); err != nil {
		t.Fatal(err)
	}
	fh := &fakeCookieHost{cookie: "S=stale-env"}
	m := NewModel(Options{
		Host:     domain.Host{Name: "bb", Kind: domain.HostBitbucketDC, BaseURL: "https://bb.example.com"},
		Provider: fh,
	})
	tm, cmd := m.Update(m.loadPRs()())
	if tm.(Model).login.active || fh.cookie != "S=good" || cmd == nil {
		t.Fatalf("expected silent retry with stored cookie, active=%v cookie=%q", tm.(Model).login.active, fh.cookie)
	}
}

func TestCookieLoginNotForTokenAuth(t *testing.T) {
	fh := &fakeCookieHost{}
	m := NewModel(Options{Host: domain.Host{Name: "bb", Kind: domain.HostBitbucketDC}, Provider: fh})
	tm, _ := m.Update(loadedPRsMsg{err: fmt.Errorf("x: %w", httputil.ErrUnauthorized)})
	if tm.(Model).login.active {
		t.Fatal("token auth should not auto-open cookie login")
	}
}

func saveTestCookie(host, cookie string) error { return auth.SaveCookie(host, cookie) }
