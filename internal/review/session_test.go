package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/review"
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
