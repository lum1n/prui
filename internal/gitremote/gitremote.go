package gitremote

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/lum1n/prui/internal/config"
	"github.com/lum1n/prui/internal/domain"
)

var (
	sshGitHub   = regexp.MustCompile(`(?i)^git@([^:]+):([^/]+)/(.+?)(?:\.git)?$`)
	sshGeneric  = regexp.MustCompile(`(?i)^(?:ssh://)?git@([^:/]+)(?::\d+)?[/:](.+)$`)
	ownerRepoPR = regexp.MustCompile(`(?i)^([^/]+)/([^/#]+)(?:#|/pull/|/pull-requests/)(\d+)$`)
)

// Resolved is a repo/PR target inferred from CLI, URL, or git remotes.
type Resolved struct {
	Host domain.Host
	Repo domain.RepoRef
	PR   int // 0 if unset
	URL  string
}

// ParseTarget parses owner/repo#123, a PR URL, or returns an error.
func ParseTarget(raw string, cfg *config.Config) (Resolved, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Resolved{}, fmt.Errorf("empty target")
	}

	if r, err := parseOwnerRepoPR(raw, cfg); err == nil {
		return r, nil
	}
	if r, err := parseURL(raw, cfg); err == nil {
		return r, nil
	}
	return Resolved{}, fmt.Errorf("unrecognized target %q (want owner/repo#123 or a PR URL)", raw)
}

func parseOwnerRepoPR(raw string, cfg *config.Config) (Resolved, error) {
	m := ownerRepoPR.FindStringSubmatch(raw)
	if m == nil {
		parts := strings.Split(raw, "/")
		if len(parts) != 2 || strings.Contains(parts[1], "#") {
			return Resolved{}, fmt.Errorf("not owner/repo")
		}
		host, err := cfg.FindHost("")
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{
			Host: host,
			Repo: domain.RepoRef{Owner: parts[0], Name: strings.TrimSuffix(parts[1], ".git")},
		}, nil
	}
	n, _ := strconv.Atoi(m[3])
	host, err := cfg.FindHost("")
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Host: host,
		Repo: domain.RepoRef{Owner: m[1], Name: m[2]},
		PR:   n,
	}, nil
}

func parseURL(raw string, cfg *config.Config) (Resolved, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return Resolved{}, fmt.Errorf("not a URL")
	}

	host, err := cfg.ResolveRemoteHost(u.Hostname())
	if err != nil {
		return Resolved{}, err
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	switch host.Kind {
	case domain.HostGitHub:
		if len(parts) >= 4 && parts[2] == "pull" {
			n, _ := strconv.Atoi(parts[3])
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[0], Name: parts[1]},
				PR:   n,
				URL:  raw,
			}, nil
		}
		if len(parts) >= 2 {
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[0], Name: parts[1]},
				URL:  raw,
			}, nil
		}
	case domain.HostBitbucketCloud:
		if len(parts) >= 4 && parts[2] == "pull-requests" {
			n, _ := strconv.Atoi(parts[3])
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[0], Name: parts[1]},
				PR:   n,
				URL:  raw,
			}, nil
		}
		if len(parts) >= 2 {
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[0], Name: parts[1]},
				URL:  raw,
			}, nil
		}
	case domain.HostBitbucketDC:
		if len(parts) >= 6 && parts[0] == "projects" && parts[2] == "repos" && parts[4] == "pull-requests" {
			n, _ := strconv.Atoi(parts[5])
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[1], Name: parts[3]},
				PR:   n,
				URL:  raw,
			}, nil
		}
		if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "repos" {
			return Resolved{
				Host: host,
				Repo: domain.RepoRef{Owner: parts[1], Name: parts[3]},
				URL:  raw,
			}, nil
		}
	}
	return Resolved{}, fmt.Errorf("could not parse PR from URL")
}

// FromGitRemote resolves owner/repo from the current git origin and config host map.
func FromGitRemote(cfg *config.Config) (Resolved, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return Resolved{}, fmt.Errorf("git remote: %w", err)
	}
	remote := strings.TrimSpace(string(out))
	return ParseRemote(remote, cfg)
}

// ParseRemote parses a git remote URL into a Resolved target.
func ParseRemote(remote string, cfg *config.Config) (Resolved, error) {
	remote = strings.TrimSpace(remote)

	if m := sshGitHub.FindStringSubmatch(remote); m != nil {
		hostName := m[1]
		owner, name := m[2], strings.TrimSuffix(m[3], ".git")
		host, err := cfg.ResolveRemoteHost(hostName)
		if err != nil {
			return Resolved{}, err
		}
		return Resolved{Host: host, Repo: domain.RepoRef{Owner: owner, Name: name}}, nil
	}

	if strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://") {
		u, err := url.Parse(remote)
		if err != nil {
			return Resolved{}, err
		}
		host, err := cfg.ResolveRemoteHost(u.Hostname())
		if err != nil {
			return Resolved{}, err
		}
		path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		parts := strings.Split(path, "/")
		switch host.Kind {
		case domain.HostBitbucketDC:
			if len(parts) >= 3 && parts[0] == "scm" {
				return Resolved{Host: host, Repo: domain.RepoRef{Owner: parts[1], Name: parts[2]}}, nil
			}
			if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "repos" {
				return Resolved{Host: host, Repo: domain.RepoRef{Owner: parts[1], Name: parts[3]}}, nil
			}
		}
		if len(parts) >= 2 {
			return Resolved{Host: host, Repo: domain.RepoRef{Owner: parts[len(parts)-2], Name: parts[len(parts)-1]}}, nil
		}
	}

	if m := sshGeneric.FindStringSubmatch(remote); m != nil {
		hostName := m[1]
		rest := strings.TrimSuffix(m[2], ".git")
		parts := strings.Split(rest, "/")
		host, err := cfg.ResolveRemoteHost(hostName)
		if err != nil {
			return Resolved{}, err
		}
		if len(parts) >= 2 {
			return Resolved{Host: host, Repo: domain.RepoRef{Owner: parts[len(parts)-2], Name: parts[len(parts)-1]}}, nil
		}
	}

	return Resolved{}, fmt.Errorf("could not parse git remote %q", remote)
}
