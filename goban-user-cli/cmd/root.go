package cmd

import (
	"fmt"
	"goban/version"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/client"
	"goban-user-cli/internal/config"
)

var (
	cfg *config.Config
	cli *client.Client
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "goban-user-cli",
	Short: "Goban User Admin CLI - Manage users via direct database access",
	Long: `Goban User Admin CLI is a dedicated tool for user administration in Goban Kanban.

This tool operates with direct database access and does not require API authentication.
It is intended for use on the host that serves the Goban board, where the executing
user has write permissions to the database file.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		cli, err = client.New(cfg)
		if err != nil {
			return fmt.Errorf("failed to initialize database client: %w", err)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// getClient returns the initialized database client.
func getClient(cmd *cobra.Command) *client.Client {
	if cli == nil {
		fmt.Fprintf(os.Stderr, "Client not initialized\n")
		os.Exit(1)
	}
	return cli
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Handle --version / -v before cobra parsing (fast cold start, no config load)
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("goban-user-cli %s\n", version.Version)
			os.Exit(0)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().String("db-type", "", "Database type: sqlite or postgres (defaults to env var DB_TYPE)")
}

// parseInt64 parses a string to int64, returns -1 on error.
func parseInt64(s string) int64 {
	var result int64
	_, err := fmt.Sscanf(s, "%d", &result)
	if err != nil {
		return -1
	}
	return result
}
