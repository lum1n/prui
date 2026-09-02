// Package version holds build-time identity for prui.
package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

// Set via -ldflags at release build time. The source default is the latest
// tag so `go build` / `go install .` still report a real version. `go install
// github.com/lum1n/prui/cmd/prui@vX.Y.Z` overwrites this from module build info.
var (
	Version = "0.1.0"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	applyBuildInfo(debug.ReadBuildInfo())
}

func applyBuildInfo(info *debug.BuildInfo, ok bool) {
	if !ok || info == nil {
		return
	}
	if v := moduleVersion(info.Main.Version); v != "" {
		Version = v
	}

	var rev, vcsTime string
	dirty := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		case "vcs.time":
			vcsTime = s.Value
		}
	}
	if unset(Commit) && rev != "" {
		Commit = rev
		if dirty {
			Commit += "-dirty"
		}
	}
	if unset(Date) && vcsTime != "" {
		if t, err := time.Parse(time.RFC3339, vcsTime); err == nil {
			Date = t.UTC().Format("2006-01-02T15:04:05Z")
		} else {
			Date = vcsTime
		}
	}
}

func unset(s string) bool {
	switch s {
	case "", "dev", "none", "unknown":
		return true
	default:
		return false
	}
}

func moduleVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "(devel)" {
		return ""
	}
	v = strings.TrimPrefix(v, "v")
	// Ignore Go VCS pseudo-versions so -ldflags / source defaults win for local builds.
	if isPseudoVersion(v) {
		return ""
	}
	return v
}

func isPseudoVersion(v string) bool {
	// e.g. 0.0.0-20260902130116-634951a3c2ca or 1.2.3-0.20260902130116-634951a3c2ca
	i := strings.LastIndex(v, "-")
	if i <= 0 {
		return false
	}
	rest := v[:i]
	j := strings.LastIndex(rest, "-")
	if j < 0 {
		return false
	}
	ts := rest[j+1:]
	if len(ts) != 14 {
		return false
	}
	for _, c := range ts {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// String returns a human-readable version line.
func String() string {
	if unset(Commit) {
		return Version
	}
	short := Commit
	if len(short) > 7 {
		short = short[:7]
	}
	return fmt.Sprintf("%s (%s)", Version, short)
}

// Full returns version, commit, and build date.
func Full() string {
	return fmt.Sprintf("prui %s\ncommit: %s\nbuilt:  %s", Version, Commit, Date)
}
