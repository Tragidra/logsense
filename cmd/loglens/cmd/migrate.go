package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var (
	migrateDB       string
	migratePostgres string
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply schema migrations to the target store",
	Long:  `Opens the target store and applies any pending schema migrations. Useful for Postgres`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger := newLogger()
		logger.Info("loglens: migrate", "store", describeStore(migrateDB, migratePostgres))

		store, err := openStore(context.Background(), migrateDB, migratePostgres, logger)
		if err != nil {
			return err
		}
		defer store.Close()

		logger.Info("loglens: migrate complete")
		return nil
	},
}

func init() {
	migrateCmd.Flags().StringVar(&migrateDB, "db", "./loglens.db", "SQLite database path")
	migrateCmd.Flags().StringVar(&migratePostgres, "postgres", "", "Postgres DSN (alternative to --db)")
	rootCmd.AddCommand(migrateCmd)
}
