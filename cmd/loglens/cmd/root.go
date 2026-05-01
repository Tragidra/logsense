// Package cmd defines the logstruct CLI command tree.
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
	Use:   "logstruct",
	Short: "logstruct CLI — read-only Dashboard, migrations, version info",
	Long: `logstruct is an embeddable Go library for AI-powered log intelligence.
This binary is a thin CLI wrapper that ships:

  logstruct ui        — serve the read-only Dashboard against an existing store
  logstruct migrate   — apply schema migrations to SQLite or Postgres
  logstruct version   — print version and commit

To ingest logs and run analysis, embed the library in your application:

  import "github.com/Tragidra/logstruct"`,
	SilenceUsage: true,
}

// Execute runs the root command and exits with a non-zero code on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("logstruct: command failed", "err", err)
		os.Exit(1)
	}
}

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
