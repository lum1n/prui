package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/lum1n/prui/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print prui version",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(version.Full())
		fmt.Println()
	},
}

func init() {
	rootCmd.Version = version.String()
	rootCmd.SetVersionTemplate("{{.Version}}\n")
	rootCmd.AddCommand(versionCmd)
}
