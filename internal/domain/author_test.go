package domain_test

import (
	"testing"

	"github.com/vegard/prui/internal/domain"
)

func TestFormatAuthor(t *testing.T) {
	cases := []struct {
		login, name, want string
	}{
		{"alice", "Alice Andersen", "Alice Andersen (alice)"},
		{"alice", "alice", "alice"},
		{"alice", "", "alice"},
		{"", "Alice", "Alice"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := domain.FormatAuthor(tc.login, tc.name); got != tc.want {
			t.Fatalf("FormatAuthor(%q,%q)=%q want %q", tc.login, tc.name, got, tc.want)
		}
	}
}
