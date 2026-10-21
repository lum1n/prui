package ui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lum1n/prui/internal/config"
	"github.com/lum1n/prui/internal/diff"
	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/provider"
	"github.com/lum1n/prui/internal/review"
)

const aiTestPatch = `@@ -1,3 +1,3 @@
 package a
-var x = 1
+var x = 2
 func f() {}
`

type fakeReviewHost struct {
	provider.Host
	submitted *domain.DraftReview
}

func (f *fakeReviewHost) Kind() domain.HostKind { return domain.HostGitHub }
func (f *fakeReviewHost) GetFileDiff(_ context.Context, _ domain.PRRef, path string) (*domain.FileDiff, error) {
	return diff.ParseUnified(path, aiTestPatch)
}
func (f *fakeReviewHost) SubmitReview(_ context.Context, _ domain.PRRef, d domain.DraftReview) error {
	f.submitted = &d
	return nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestAIReviewFlow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PRUI_TEST_ANTHROPIC", "k")

	reply := `{"summary":"Bumps x.","verdict":"REQUEST_CHANGES","comments":[` +
		`{"path":"a.go","line":2,"side":"RIGHT","severity":"issue","body":"why 2?"},` +
		`{"path":"a.go","line":3,"side":"RIGHT","severity":"nit","body":"rename f"}]}`
	var gotPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) > 0 {
			gotPrompt = body.Messages[0].Content
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": reply}},
		})
	}))
	defer srv.Close()

	cfg := &config.Config{AI: config.AIConfig{
		Default:   "c",
		Providers: map[string]config.AIProvider{"c": {Kind: "claude", Model: "m", BaseURL: srv.URL, TokenEnv: "PRUI_TEST_ANTHROPIC"}},
	}}
	fh := &fakeReviewHost{}
	repo := domain.RepoRef{Owner: "o", Name: "r"}
	m := NewModel(Options{Config: cfg, Host: domain.Host{Name: "gh", Kind: domain.HostGitHub}, Provider: fh, Repo: repo})
	var tm tea.Model = m
	tm, _ = tm.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := tm.(Model)
	mm.pr = &domain.PullRequest{Ref: domain.PRRef{Repo: repo, Number: 7}, Title: "T", State: "open"}
	mm.files = []domain.FileChange{{Path: "a.go", Status: domain.FileModified}}
	mm.session, _ = review.Load("gh", mm.pr.Ref)
	mm.screen = screenReview

	tm, cmd := mm.Update(key("A"))
	if cmd == nil {
		t.Fatalf("no AI review command; status=%q", tm.(Model).status)
	}
	tm, _ = tm.Update(cmd())
	mm = tm.(Model)
	if mm.screen != screenAIReview {
		t.Fatalf("screen = %v, status=%q", mm.screen, mm.status)
	}
	if !strings.Contains(gotPrompt, "R2 +var x = 2") {
		t.Fatalf("prompt not annotated:\n%s", gotPrompt)
	}
	if n := len(mm.session.AIReview.Suggestions); n != 2 {
		t.Fatalf("suggestions = %d", n)
	}
	if v := mm.View(); !strings.Contains(v, "why 2?") {
		t.Fatalf("view missing suggestion:\n%s", v)
	}

	// Accept the first (cursor moves on), delete the second.
	tm, _ = tm.Update(key("a"))
	tm, _ = tm.Update(key("x"))
	// Add a general comment.
	tm, _ = tm.Update(key("c"))
	tm, _ = tm.Update(key("more"))
	tm, _ = tm.Update(key("ctrl+s"))
	mm = tm.(Model)
	if len(mm.session.Draft.Comments) != 2 || mm.session.PendingSuggestions() != 0 {
		t.Fatalf("drafts=%+v pending=%d", mm.session.Draft.Comments, mm.session.PendingSuggestions())
	}
	if a := mm.session.Draft.Comments[0].Anchor; a == nil || a.Line != 2 || a.Side != domain.SideRight {
		t.Fatalf("accepted anchor %+v", a)
	}

	// Submit: AI verdict preselected, summary carried over.
	tm, _ = tm.Update(key("r"))
	mm = tm.(Model)
	if mm.screen != screenSubmit || mm.availableActions()[mm.submitIdx] != domain.ActionRequestChanges {
		t.Fatalf("screen=%v idx=%d", mm.screen, mm.submitIdx)
	}
	if v := mm.View(); !strings.Contains(v, "Bumps x.") {
		t.Fatalf("submit view missing summary:\n%s", v)
	}
	_, cmd = tm.Update(key("enter"))
	cmd()
	if fh.submitted == nil || fh.submitted.Summary != "Bumps x." || len(fh.submitted.Comments) != 2 ||
		fh.submitted.Action != domain.ActionRequestChanges {
		t.Fatalf("submitted %+v", fh.submitted)
	}
}

func TestAIReviewRequiresConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := NewModel(Options{Host: domain.Host{Name: "gh", Kind: domain.HostGitHub}, Provider: &fakeReviewHost{}})
	m.pr = &domain.PullRequest{Ref: domain.PRRef{Number: 1}, State: "open"}
	m.session, _ = review.Load("gh", m.pr.Ref)
	tm, cmd := m.openAIReview(false)
	if cmd != nil || !strings.Contains(tm.(Model).status, "AI not configured") {
		t.Fatalf("status=%q", tm.(Model).status)
	}
}
