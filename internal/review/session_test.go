package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lum1n/prui/internal/domain"
	"github.com/lum1n/prui/internal/review"
)

func TestSessionPersist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	ref := domain.PRRef{Repo: domain.RepoRef{Owner: "a", Name: "b"}, Number: 3}
	s, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(domain.DraftComment{Body: "hi", Path: "f.go", Anchor: &domain.Anchor{Path: "f.go", Line: 2, Side: domain.SideRight}}); err != nil {
		t.Fatal(err)
	}
	s2, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Draft.Comments) != 1 || s2.Draft.Comments[0].Body != "hi" {
		t.Fatalf("%+v", s2.Draft.Comments)
	}
	id := s2.Draft.Comments[0].ID
	if err := s2.UpdateComment(id, "updated"); err != nil {
		t.Fatal(err)
	}
	s3, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Draft.Comments[0].Body != "updated" {
		t.Fatalf("update: %+v", s3.Draft.Comments)
	}
	if err := s3.RemoveComment(id); err != nil {
		t.Fatal(err)
	}
	s4, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(s4.Draft.Comments) != 0 {
		t.Fatalf("remove left %+v", s4.Draft.Comments)
	}
	drafts := filepath.Join(dir, "prui", "drafts")
	entries, _ := os.ReadDir(drafts)
	if len(entries) != 1 {
		t.Fatalf("expected draft file, got %v", entries)
	}
	if err := review.Clear("host", ref); err != nil {
		t.Fatal(err)
	}
}

func TestSessionAISuggestions(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ref := domain.PRRef{Repo: domain.RepoRef{Owner: "a", Name: "b"}, Number: 4}
	s, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	anchor := &domain.Anchor{Path: "f.go", Line: 7, Side: domain.SideRight}
	err = s.SetAIReview(domain.AIReview{
		Summary: "looks fine",
		Verdict: domain.ActionApprove,
		Suggestions: []domain.AISuggestion{
			{Path: "f.go", Anchor: anchor, Body: "nil check"},
			{Body: "general note"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Draft.Summary != "looks fine" {
		t.Fatalf("summary not prefilled: %q", s.Draft.Summary)
	}
	if s.PendingSuggestions() != 2 {
		t.Fatalf("pending = %d", s.PendingSuggestions())
	}
	first := s.AIReview.Suggestions[0].ID
	second := s.AIReview.Suggestions[1].ID
	if err := s.AcceptSuggestion(first, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateSuggestion(first, "nil check please"); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSuggestion(second); err != nil {
		t.Fatal(err)
	}

	s2, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.Draft.Comments) != 1 {
		t.Fatalf("drafts = %+v", s2.Draft.Comments)
	}
	d := s2.Draft.Comments[0]
	if d.Body != "nil check please" || d.Anchor == nil || d.Anchor.Line != 7 || d.Path != "f.go" {
		t.Fatalf("draft = %+v", d)
	}
	if s2.AIReview == nil || len(s2.AIReview.Suggestions) != 1 || s2.PendingSuggestions() != 0 {
		t.Fatalf("ai review = %+v", s2.AIReview)
	}

	// A rerun keeps accepted suggestions and replaces pending ones.
	if err := s2.SetAIReview(domain.AIReview{Suggestions: []domain.AISuggestion{{Body: "new"}}}); err != nil {
		t.Fatal(err)
	}
	if len(s2.AIReview.Suggestions) != 2 || s2.Draft.Summary != "looks fine" {
		t.Fatalf("rerun = %+v / %q", s2.AIReview.Suggestions, s2.Draft.Summary)
	}

	// Deleting the draft directly sends the suggestion back to pending.
	if err := s2.RemoveComment(d.ID); err != nil {
		t.Fatal(err)
	}
	if s2.PendingSuggestions() != 2 {
		t.Fatalf("pending after draft delete = %d", s2.PendingSuggestions())
	}
	// Removing an accepted suggestion removes its draft.
	id := s2.AIReview.Suggestions[0].ID
	if err := s2.AcceptSuggestion(id, ""); err != nil {
		t.Fatal(err)
	}
	if err := s2.RemoveSuggestion(id); err != nil {
		t.Fatal(err)
	}
	if len(s2.Draft.Comments) != 0 {
		t.Fatalf("draft left after remove: %+v", s2.Draft.Comments)
	}
}

func TestSessionLoadsWithoutAIReview(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	ref := domain.PRRef{Repo: domain.RepoRef{Owner: "a", Name: "b"}, Number: 5}
	s, _ := review.Load("host", ref)
	if err := review.Save(s); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "prui", "drafts"))
	p := filepath.Join(dir, "prui", "drafts", entries[0].Name())
	old := `{"host_name":"host","repo":{"Owner":"a","Name":"b"},"number":5,"draft":{"Summary":"","Action":"COMMENT","Comments":null,"RemoteID":0}}`
	if err := os.WriteFile(p, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, err := review.Load("host", ref)
	if err != nil {
		t.Fatal(err)
	}
	if s2.AIReview != nil || s2.PendingSuggestions() != 0 {
		t.Fatalf("unexpected ai review %+v", s2.AIReview)
	}
}
