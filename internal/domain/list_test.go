package domain

import "testing"

func TestNormalizeListState(t *testing.T) {
	cases := map[string]string{
		"": "open", "OPEN": "open", "ready": "open",
		"draft": "draft", "Drafts": "draft",
		"merged": "merged", "closed": "closed", "all": "all",
	}
	for in, want := range cases {
		if got := NormalizeListState(in); got != want {
			t.Fatalf("NormalizeListState(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPullRequestViewOnly(t *testing.T) {
	cases := []struct {
		state string
		want  bool
	}{
		{"open", false},
		{"OPEN", false},
		{"merged", true},
		{"closed", true},
		{"declined", true},
		{"superseded", true},
	}
	for _, tc := range cases {
		pr := PullRequest{State: tc.state}
		if got := pr.ViewOnly(); got != tc.want {
			t.Fatalf("State %q ViewOnly=%v want %v", tc.state, got, tc.want)
		}
	}
}
