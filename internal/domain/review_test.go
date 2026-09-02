package domain_test

import (
	"testing"

	"github.com/lum1n/prui/internal/domain"
)

func TestAuthorLogin(t *testing.T) {
	if got := domain.AuthorLogin("Ada Lovelace (alovelace)"); got != "alovelace" {
		t.Fatalf("got %q", got)
	}
	if got := domain.AuthorLogin("alovelace"); got != "alovelace" {
		t.Fatalf("got %q", got)
	}
}

func TestReviewStatusNormalize(t *testing.T) {
	st := domain.ReviewStatus{
		ViewerLogin:   "me",
		ViewerAliases: []string{"Me Person"},
		Reviewers: []domain.Reviewer{
			{Login: "a", Name: "Alice", Decision: domain.DecisionApproved},
			{Login: "b", Name: "Bob", Decision: domain.DecisionChangesRequested},
			{Login: "me", Name: "Me Person", Decision: domain.DecisionApproved},
		},
	}
	st.Normalize()
	if len(st.Approvers) != 2 || len(st.ChangeRequesters) != 1 {
		t.Fatalf("%+v", st)
	}
	if st.Approvers[0] != "Alice (a)" {
		t.Fatalf("approver label %q", st.Approvers[0])
	}
	if st.ViewerDecision != domain.DecisionApproved {
		t.Fatalf("viewer %q", st.ViewerDecision)
	}
	// Alias-only match
	st2 := domain.ReviewStatus{
		ViewerAliases: []string{"h8010ch"},
		Reviewers: []domain.Reviewer{
			{Login: "h8010ch", Name: "Vegard", Decision: domain.DecisionApproved},
		},
	}
	st2.Normalize()
	if st2.ViewerDecision != domain.DecisionApproved {
		t.Fatal("expected alias match")
	}
}
