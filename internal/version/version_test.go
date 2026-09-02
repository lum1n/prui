package version_test

import (
	"runtime/debug"
	"testing"

	"github.com/lum1n/prui/internal/version"
)

func TestString(t *testing.T) {
	if version.String() == "" {
		t.Fatal("empty version")
	}
}

func TestFull(t *testing.T) {
	s := version.Full()
	if s == "" || s[:4] != "prui" {
		t.Fatalf("got %q", s)
	}
}

func TestModuleVersionViaBuildInfo(t *testing.T) {
	// Ensure package init does not panic when build info is present.
	_, _ = debug.ReadBuildInfo()
}
