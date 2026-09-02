package ai

import "testing"

func TestGitHubAuthHeader(t *testing.T) {
	if got := githubAuthHeader("gho_abc"); got != "Bearer gho_abc" {
		t.Fatalf("got %q", got)
	}
	if got := githubAuthHeader("github_pat_x"); got != "Bearer github_pat_x" {
		t.Fatalf("got %q", got)
	}
	if got := githubAuthHeader("ghp_abc"); got != "token ghp_abc" {
		t.Fatalf("got %q", got)
	}
}
