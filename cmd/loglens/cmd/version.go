package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Tragidra/logstruct/pkg/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version and commit",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "logstruct %s (commit %s)\n", version.Version, version.Commit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
