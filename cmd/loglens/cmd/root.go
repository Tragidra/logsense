// Package cmd defines the logsense CLI command tree.
//
// Subcommands:
//   - version: print build info
//   - migrate: apply schema migrations to a target store
//   - ui:      serve the read-only Dashboard (local)
package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "logsense",
	Short: "logsense CLI — read-only Dashboard, migrations, version info",
	Long: `logsense is an embeddable Go library for AI-powered log intelligence.
This binary is a thin CLI wrapper that ships:

  logsense ui        — serve the read-only Dashboard against an existing store
  logsense migrate   — apply schema migrations to SQLite or Postgres
  logsense version   — print version and commit

To ingest logs and run analysis, embed the library in your application:

  import "github.com/Tragidra/logsense"`,
	SilenceUsage: true,
}

// Execute runs the root command and exits with a non-zero code on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("logsense: command failed", "err", err)
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
