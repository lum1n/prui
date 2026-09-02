package gitremote

import (
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/domain"
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
		// owner/repo without PR
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

	host, ok := cfg.MatchBaseURL(u.Scheme + "://" + u.Host)
	if !ok {
		// infer kind from hostname
		host = inferHostFromHostname(u.Host, cfg)
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")

	switch host.Kind {
	case domain.HostGitHub:
		// /owner/repo/pull/123
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
		// /workspace/repo/pull-requests/123
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
		// /projects/KEY/repos/slug/pull-requests/123
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

func inferHostFromHostname(hostname string, cfg *config.Config) domain.Host {
	h := strings.ToLower(hostname)
	if strings.Contains(h, "github") {
		host, err := cfg.FindHost("github")
		if err == nil {
			return host
		}
		return domain.Host{
			Name:    "github",
			Kind:    domain.HostGitHub,
			BaseURL: "https://" + hostname,
			APIURL:  "https://api.github.com/",
		}
	}
	if strings.Contains(h, "bitbucket.org") {
		host, err := cfg.FindHost("bitbucket")
		if err == nil {
			return host
		}
		return domain.Host{
			Name:    "bitbucket",
			Kind:    domain.HostBitbucketCloud,
			BaseURL: "https://bitbucket.org",
			APIURL:  "https://api.bitbucket.org/2.0",
		}
	}
	// on-prem guess: prefer configured DC, else github enterprise shape
	for _, hc := range cfg.Hosts {
		dh := hc.ToDomain()
		if dh.Kind == domain.HostBitbucketDC && strings.Contains(strings.ToLower(dh.BaseURL), h) {
			return dh
		}
		if dh.Kind == domain.HostGitHub && strings.Contains(strings.ToLower(dh.BaseURL), h) {
			return dh
		}
	}
	return domain.Host{
		Name:    hostname,
		Kind:    domain.HostGitHub,
		BaseURL: "https://" + hostname,
		APIURL:  "https://" + hostname + "/api/v3",
	}
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
		host, ok := cfg.MatchBaseURL("https://" + hostName)
		if !ok {
			host = inferHostFromHostname(hostName, cfg)
		}
		return Resolved{Host: host, Repo: domain.RepoRef{Owner: owner, Name: name}}, nil
	}

	if strings.HasPrefix(remote, "http://") || strings.HasPrefix(remote, "https://") {
		u, err := url.Parse(remote)
		if err != nil {
			return Resolved{}, err
		}
		host, ok := cfg.MatchBaseURL(u.Scheme + "://" + u.Host)
		if !ok {
			host = inferHostFromHostname(u.Host, cfg)
		}
		path := strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/")
		parts := strings.Split(path, "/")
		switch host.Kind {
		case domain.HostBitbucketDC:
			// /scm/project/repo.git or /projects/KEY/repos/slug
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
		host, ok := cfg.MatchBaseURL("https://" + hostName)
		if !ok {
			host = inferHostFromHostname(hostName, cfg)
		}
		if len(parts) >= 2 {
			return Resolved{Host: host, Repo: domain.RepoRef{Owner: parts[len(parts)-2], Name: parts[len(parts)-1]}}, nil
		}
	}

	return Resolved{}, fmt.Errorf("could not parse git remote %q", remote)
}
