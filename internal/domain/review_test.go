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
		ViewerLogin: "me",
		Reviewers: []domain.Reviewer{
			{Login: "a", Decision: domain.DecisionApproved},
			{Login: "b", Decision: domain.DecisionChangesRequested},
			{Login: "me", Decision: domain.DecisionApproved},
		},
	}
	st.Normalize()
	if len(st.Approvers) != 2 || len(st.ChangeRequesters) != 1 {
		t.Fatalf("%+v", st)
	}
	if st.ViewerDecision != domain.DecisionApproved {
		t.Fatalf("viewer %q", st.ViewerDecision)
	}
	if !st.HasReviews() {
		t.Fatal("expected HasReviews")
	}
}
