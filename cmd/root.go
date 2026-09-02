package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vegard/prui/internal/auth"
	"github.com/vegard/prui/internal/config"
	"github.com/vegard/prui/internal/domain"
	"github.com/vegard/prui/internal/gitremote"
	"github.com/vegard/prui/internal/provider"
	"github.com/vegard/prui/internal/ui"
)

var (
	cfgFile  string
	hostName string
	appCfg   *config.Config

	authHostname string
	authClientID string
)

var rootCmd = &cobra.Command{
	Use:   "prui [owner/repo#number|url]",
	Short: "Terminal UI for reviewing pull requests",
	Long: `prui is a Bubbletea TUI for reviewing PRs on GitHub (cloud/GHE)
and Bitbucket (cloud/Data Center).

With no arguments, prui resolves the current git remote and lists open PRs.
Pass owner/repo#123 or a PR URL to jump straight into review.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTUI,
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Authentication helpers",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show auth status for configured hosts",
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, h := range appCfg.Hosts {
			fmt.Println(auth.StatusLine(h.ToDomain()))
		}
		for name, p := range appCfg.AI.Providers {
			if !strings.EqualFold(p.Kind, "copilot") {
				continue
			}
			api := strings.TrimSpace(p.APIURL)
			if api == "" && p.GitHubHost != "" {
				if hc, ok := appCfg.FindHostConfig(p.GitHubHost); ok {
					api = hc.APIURL
				}
			}
			if api == "" {
				continue
			}
			h := domain.Host{
				Name:     "ai:" + name,
				Kind:     domain.HostGitHub,
				APIURL:   strings.TrimRight(api, "/"),
				BaseURL:  strings.TrimRight(api, "/"),
				TokenEnv: p.TokenEnv,
			}
			fmt.Println(auth.StatusLine(h))
		}
		return nil
	},
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in with device code (stores token; no env export needed)",
	Long: `Log in to GitHub.com or GitHub Enterprise and save a token for prui.

By default this wraps "gh auth login" (device/browser), then saves the token
under ~/.config/prui/credentials.json so you do not need to export GHE_TOKEN.

For a fully native device flow (no gh), set oauth_client_id on the host or
pass --client-id from an OAuth App on that GHE.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		opts, err := resolveLoginOpts()
		if err != nil {
			return err
		}
		return auth.Login(cmd.Context(), opts)
	},
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove a stored prui token for a hostname",
	RunE: func(cmd *cobra.Command, args []string) error {
		hn := strings.TrimSpace(authHostname)
		if hostName != "" {
			h, err := appCfg.FindHost(hostName)
			if err != nil {
				return err
			}
			hn = auth.HostnameOf(h.BaseURL)
			if hn == "" {
				hn = auth.HostnameOf(h.APIURL)
			}
		}
		if hn == "" {
			return fmt.Errorf("pass --hostname or --host")
		}
		if err := auth.DeleteToken(hn); err != nil {
			return err
		}
		fmt.Printf("Removed stored token for %s\n", auth.HostnameOf(hn))
		return nil
	},
}

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Pull request commands",
}

var prListCmd = &cobra.Command{
	Use:   "list [owner/repo]",
	Short: "List open pull requests (non-interactive)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resolved, err := resolveTarget(args)
		if err != nil {
			return err
		}
		host, err := provider.New(resolved.Host)
		if err != nil {
			return err
		}
		prs, err := host.ListPullRequests(cmd.Context(), resolved.Repo, domain.ListOpts{State: "open"})
		if err != nil {
			return err
		}
		for _, p := range prs {
			fmt.Printf("#%d\t%s\t%s\t%s\n", p.Ref.Number, p.Author, p.State, p.Title)
		}
		return nil
	},
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/prui/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&hostName, "host", "", "host name from config")

	authLoginCmd.Flags().StringVar(&authHostname, "hostname", "", "forge hostname (e.g. ghe.example.com)")
	authLoginCmd.Flags().StringVar(&authClientID, "client-id", "", "OAuth App client id for native device flow")
	authLogoutCmd.Flags().StringVar(&authHostname, "hostname", "", "forge hostname")

	authCmd.AddCommand(authStatusCmd, authLoginCmd, authLogoutCmd)
	prCmd.AddCommand(prListCmd)
	rootCmd.AddCommand(authCmd, prCmd)
}

func initConfig() {
	var err error
	appCfg, err = config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveLoginOpts() (auth.LoginOptions, error) {
	opts := auth.LoginOptions{
		Hostname: authHostname,
		ClientID: authClientID,
	}
	if hostName != "" {
		hc, ok := appCfg.FindHostConfig(hostName)
		if !ok {
			return opts, fmt.Errorf("host %q not found", hostName)
		}
		h := hc.ToDomain()
		opts.Hostname = auth.HostnameOf(h.BaseURL)
		if opts.Hostname == "" {
			opts.Hostname = auth.HostnameOf(h.APIURL)
		}
		opts.BaseURL = h.BaseURL
		if opts.BaseURL == "" {
			opts.BaseURL = "https://" + opts.Hostname
		}
		if opts.ClientID == "" {
			opts.ClientID = hc.OAuthClientID
		}
		if opts.ClientSecret == "" {
			opts.ClientSecret = hc.OAuthClientSecret
		}
	}
	if opts.Hostname == "" {
		for _, p := range appCfg.AI.Providers {
			if !strings.EqualFold(p.Kind, "copilot") {
				continue
			}
			if p.GitHubHost != "" {
				if hc, ok := appCfg.FindHostConfig(p.GitHubHost); ok {
					opts.Hostname = auth.HostnameOf(hc.BaseURL)
					if opts.Hostname == "" {
						opts.Hostname = auth.HostnameOf(hc.APIURL)
					}
					opts.BaseURL = hc.BaseURL
					if opts.ClientID == "" {
						opts.ClientID = firstNonEmpty(authClientID, p.OAuthClientID, hc.OAuthClientID)
					}
					if opts.ClientSecret == "" {
						opts.ClientSecret = firstNonEmpty(p.OAuthClientSecret, hc.OAuthClientSecret)
					}
					break
				}
			}
			if api := strings.TrimSpace(p.APIURL); api != "" {
				opts.Hostname = auth.HostnameOf(api)
				opts.BaseURL = "https://" + opts.Hostname
				if opts.ClientID == "" {
					opts.ClientID = firstNonEmpty(authClientID, p.OAuthClientID)
				}
				opts.ClientSecret = firstNonEmpty(opts.ClientSecret, p.OAuthClientSecret)
				break
			}
		}
	}
	if opts.Hostname == "" {
		return opts, fmt.Errorf("pass --hostname ghe.example.com (or --host name from config)")
	}
	if opts.ClientID == "" && authClientID != "" {
		opts.ClientID = authClientID
	}
	return opts, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func runTUI(cmd *cobra.Command, args []string) error {
	resolved, err := resolveTarget(args)
	if err != nil {
		return err
	}
	p, err := provider.New(resolved.Host)
	if err != nil {
		return err
	}
	return ui.Run(ui.Options{
		Config:   appCfg,
		Host:     resolved.Host,
		Provider: p,
		Repo:     resolved.Repo,
		PRNumber: resolved.PR,
	})
}

func resolveTarget(args []string) (gitremote.Resolved, error) {
	if len(args) == 1 {
		raw := args[0]
		r, err := gitremote.ParseTarget(raw, appCfg)
		if err != nil {
			return gitremote.Resolved{}, err
		}
		if hostName != "" {
			h, err := appCfg.FindHost(hostName)
			if err != nil {
				return gitremote.Resolved{}, err
			}
			r.Host = h
		}
		return r, nil
	}

	r, err := gitremote.FromGitRemote(appCfg)
	if err != nil {
		return gitremote.Resolved{}, fmt.Errorf("%w\nPass owner/repo#123 or a PR URL", err)
	}
	if hostName != "" {
		h, err := appCfg.FindHost(hostName)
		if err != nil {
			return gitremote.Resolved{}, err
		}
		r.Host = h
	} else if r.Host.Name == "" || (hostName == "" && appCfg.Defaults.Host != "") {
		_ = hostName
	}
	return r, nil
}
