package gitremote_test

import (
	"testing"

	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/gitremote"
)

func testCfg() *config.Config {
	return &config.Config{
		Hosts: []config.HostConfig{
			{Name: "github", Kind: string(domain.HostGitHub), BaseURL: "https://github.com", APIURL: "https://api.github.com/"},
			{Name: "ghe", Kind: string(domain.HostGitHub), BaseURL: "https://ghe.example.com", APIURL: "https://ghe.example.com/api/v3"},
			{Name: "bb", Kind: string(domain.HostBitbucketCloud), BaseURL: "https://bitbucket.org", APIURL: "https://api.bitbucket.org/2.0"},
			{Name: "bbdc", Kind: string(domain.HostBitbucketDC), BaseURL: "https://bitbucket.example.com", APIURL: "https://bitbucket.example.com/rest/api/1.0"},
		},
		Defaults: config.Defaults{Host: "github"},
	}
}

func TestParseTargetOwnerRepo(t *testing.T) {
	r, err := gitremote.ParseTarget("acme/widgets#42", testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo.Owner != "acme" || r.Repo.Name != "widgets" || r.PR != 42 {
		t.Fatalf("%+v", r)
	}
}

func TestParseGitHubURL(t *testing.T) {
	r, err := gitremote.ParseTarget("https://github.com/acme/widgets/pull/7", testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.PR != 7 || r.Host.Kind != domain.HostGitHub {
		t.Fatalf("%+v", r)
	}
}

func TestParseBitbucketDCURL(t *testing.T) {
	r, err := gitremote.ParseTarget("https://bitbucket.example.com/projects/PROJ/repos/repo/pull-requests/9", testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo.Owner != "PROJ" || r.Repo.Name != "repo" || r.PR != 9 {
		t.Fatalf("%+v", r)
	}
	if r.Host.Kind != domain.HostBitbucketDC {
		t.Fatalf("kind %s", r.Host.Kind)
	}
}

func TestParseRemoteSSH(t *testing.T) {
	r, err := gitremote.ParseRemote("git@github.com:acme/widgets.git", testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo.Owner != "acme" || r.Repo.Name != "widgets" {
		t.Fatalf("%+v", r)
	}
}

func TestParseRemoteBBDCSCM(t *testing.T) {
	r, err := gitremote.ParseRemote("https://bitbucket.example.com/scm/proj/repo.git", testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Repo.Owner != "proj" || r.Repo.Name != "repo" {
		t.Fatalf("%+v", r)
	}
}

func TestParseRemoteMatchHosts(t *testing.T) {
	cfg := testCfg()
	for i := range cfg.Hosts {
		if cfg.Hosts[i].Name == "bbdc" {
			cfg.Hosts[i].MatchHosts = []string{"git-ssh.intra.eika.no"}
		}
	}
	r, err := gitremote.ParseRemote("git@git-ssh.intra.eika.no:PROJ/smartspar.git", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Host.Name != "bbdc" || r.Host.Kind != domain.HostBitbucketDC {
		t.Fatalf("host %+v", r.Host)
	}
	if r.Repo.Owner != "PROJ" || r.Repo.Name != "smartspar" {
		t.Fatalf("repo %+v", r.Repo)
	}
}

func TestParseRemoteSoleHostFallback(t *testing.T) {
	cfg := &config.Config{
		Hosts: []config.HostConfig{
			{
				Name: "work-bb", Kind: string(domain.HostBitbucketDC),
				BaseURL: "https://bitbucket.intra.eika.no",
				APIURL:  "https://bitbucket.intra.eika.no/rest/api/1.0",
			},
		},
		Defaults: config.Defaults{Host: "work-bb"},
	}
	r, err := gitremote.ParseRemote("git@git-ssh.intra.eika.no:PROJ/repo.git", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if r.Host.Name != "work-bb" {
		t.Fatalf("expected work-bb, got %+v", r.Host)
	}
}

func TestParseRemoteUnmappedMultiHost(t *testing.T) {
	cfg := testCfg()
	cfg.Defaults.Host = ""
	_, err := gitremote.ParseRemote("git@unknown.example:acme/repo.git", cfg)
	if err == nil {
		t.Fatal("expected error")
	}
}
