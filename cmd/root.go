// Package cmd defines application CLI commands built on Cobra.
package cmd

import (
	"embed"
	"fmt"
	"os"

	"go-uptime/cli"
	"go-uptime/config"
	"go-uptime/database"
	"go-uptime/server"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"gorm.io/gorm"
)

var (
	cfg                *config.Config
	migrations         embed.FS
	commandsRegistered bool
	rootCmd            = &cobra.Command{
		Use:   "go-uptime",
		Short: "Uptime monitoring application",
		Run: func(cmd *cobra.Command, _ []string) {
			_ = cmd.Help()
		},
	}
)

// Init configures the CLI with embedded migrations. Configuration is loaded lazily when commands run.
func Init(mig embed.FS) {
	migrations = mig
	registerCommands()
}

// ensureConfig loads configuration from the environment on first access.
func ensureConfig() *config.Config {
	if cfg != nil {
		return cfg
	}
	c, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	if c == nil {
		panic("config.Load returned nil config without error")
	}
	cfg = c
	return c
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func registerCommands() {
	if commandsRegistered {
		return
	}
	commandsRegistered = true
	rootCmd.AddCommand(
		serverCmd(),
		dbUsersCreateCmd(),
		dbUsersSeedCmd(),
		dbUsersDeleteAllCmd(),
		dbUsersShowCmd(),
		dbGooseMigrateCmd(),
		dbGormMigrateCmd(),
		dbClearAllTablesCmd(),
		dbClearTableCmd(),
		dbDropAllTablesCmd(),
		dbDropTableCmd(),
		dbExecuteSQLCmd(),
	)
}

func connectDB() *gorm.DB {
	return database.Connect(ensureConfig().Database)
}

func serverCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "server",
		Aliases: []string{"s"},
		Short:   "Start the HTTP server",
		Run: func(_ *cobra.Command, _ []string) {
			server.Run(ensureConfig(), migrations)
		},
	}
}

func dbUsersCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-users-create",
		Short: "Create a user interactively",
		Run: func(_ *cobra.Command, _ []string) {
			cli.UsersCreate(connectDB())
		},
	}
}

func dbUsersSeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-users-seed",
		Short: "Create admin user (admin/admin)",
		Run: func(_ *cobra.Command, _ []string) {
			cli.UsersSeed(connectDB())
		},
	}
}

func dbUsersDeleteAllCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-users-delete-all",
		Short: "Delete all users",
		Run: func(_ *cobra.Command, _ []string) {
			cli.UsersDeleteAll(connectDB())
		},
	}
}

func dbUsersShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-users-show",
		Short: "Show all users",
		Run: func(_ *cobra.Command, _ []string) {
			cli.UsersShow(connectDB())
		},
	}
}

func dbGooseMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-goose-migrate",
		Short: "Apply Goose database migrations",
		Run: func(_ *cobra.Command, _ []string) {
			database.RunGooseMigrations(migrations, ensureConfig().Database)
			fmt.Println("Migrations applied.")
		},
	}
}

func dbGormMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-gorm-migrate",
		Short: "Apply GORM AutoMigrate",
		Run: func(_ *cobra.Command, _ []string) {
			database.RunGormAutoMigrate(ensureConfig().Database)
			fmt.Println("GORM AutoMigrate completed.")
		},
	}
}

func dbClearAllTablesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-clear-all-tables",
		Short: "Clear all database tables",
		Run: func(_ *cobra.Command, _ []string) {
			cli.ClearAllTables(connectDB())
		},
	}
}

func dbClearTableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-clear-table [table]",
		Short: "Clear a database table",
		Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			cli.ClearTable(connectDB(), args[0])
		},
	}
}

func dbDropAllTablesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-drop-all-tables",
		Short: "Drop all database tables",
		Run: func(_ *cobra.Command, _ []string) {
			cli.DropAllTables(connectDB())
		},
	}
}

func dbDropTableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-drop-table [table]",
		Short: "Drop a database table",
		Args:  cobra.ExactArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			cli.DropTable(connectDB(), args[0])
		},
	}
}

func dbExecuteSQLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "db-execute-sql [query...]",
		Short: "Execute SQL and print results as a table",
		Args:  cobra.MinimumNArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			cli.ExecuteSQL(connectDB(), cli.ReadSQLFromArgs(args))
		},
	}
}
