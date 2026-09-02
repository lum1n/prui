package cmd

import (
	"fmt"
	"os"
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

	authCmd.AddCommand(authStatusCmd)
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
		// allow --host override after parse
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
		// Prefer matching config host already done in FromGitRemote; allow default override only when remote unmatched name
		_ = hostName
	}
	return r, nil
}
