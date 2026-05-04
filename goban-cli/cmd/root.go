package cmd

import (
	"errors"
	"fmt"
	"goban/version"
	"os"

	"github.com/spf13/cobra"

	"goban-cli/internal/client"
	"goban-cli/internal/config"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/output"
)

var (
	cfg         *config.Config
	verboseFlag bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "goban-cli",
	Short: "Goban CLI - Interact with your Kanban board from the terminal",
	Long: `Goban CLI is a command-line interface for managing your Goban Kanban boards.

It supports both line-by-line output (colorized by default) and JSON format,
making it suitable for both interactive use and scripting/AI agent integration.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// getClient returns a new API client using the loaded config, with optional server override from flags
func getClient(cmd *cobra.Command) *client.Client {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			// Return a degenerate client — will produce clear errors on first operation
			return &client.Client{}
		}
	}
	// Check for server override from flags or env var
	server := getServer(cmd)
	if server != "" {
		cfg.API.BaseURL = server
	}
	return client.New(cfg)
}

// getFormatter returns a new output formatter using the loaded config
func getFormatter(cmd *cobra.Command) *output.Formatter {
	if cfg == nil {
		var err error
		cfg, err = config.Load()
		if err != nil {
			// Return a degenerate formatter — will produce plain text output (safe fallback)
			return &output.Formatter{}
		}
	}
	// Check for format override from flags
	format := getFormat(cmd)
	if format != "" {
		cfg.Output.Format = format
	}
	return output.New(cfg)
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	// Handle --version / -v before cobra parsing (fast cold start, no config load)
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("goban-cli %s\n", version.Version)
			os.Exit(0)
		}
	}

	err := rootCmd.Execute()
	if err != nil {
		var classErr *gerr.ClassifiedError
		if errors.As(err, &classErr) {
			fmt.Fprintf(os.Stderr, "%s\n", classErr.UserMessage(verboseFlag))
			os.Exit(classErr.Category.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringP("board", "b", "", "Board ID to operate on (also: GOBAN_BOARD env var, config.yaml board_id)")
	rootCmd.PersistentFlags().String("user", "", "Your username for claiming tickets (also: GOBAN_USER env var, config.yaml user)")
	rootCmd.PersistentFlags().String("server", "", "API server URL override (defaults to config or $GOBAN_API_BASE_URL)")
	rootCmd.PersistentFlags().String("format", "", "Output format: 'line', 'json', or 'compact' (defaults to config or 'line')")
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "V", false, "Show detailed error information")
}

// getBoardID returns the board ID from flags, environment variable, or config file.
func getBoardID(cmd *cobra.Command) string {
	board, _ := cmd.Flags().GetString("board")
	if board != "" {
		return board
	}

	board = os.Getenv("GOBAN_BOARD")
	if board != "" {
		return board
	}

	if cfg != nil && cfg.API.BoardID != "" {
		return cfg.API.BoardID
	}

	return ""
}

// getUser returns the username from flags, environment variable, or config file.
func getUser(cmd *cobra.Command) string {
	user, _ := cmd.Flags().GetString("user")
	if user != "" {
		return user
	}

	user = os.Getenv("GOBAN_USER")
	if user != "" {
		return user
	}

	if cfg != nil && cfg.API.User != "" {
		return cfg.API.User
	}

	return ""
}

// getServer returns the server URL from flags, env var, or config
func getServer(cmd *cobra.Command) string {
	server, _ := cmd.Flags().GetString("server")
	if server == "" {
		server = os.Getenv("GOBAN_API_BASE_URL")
	}
	return server
}

// getFormat returns output format from flags or config
func getFormat(cmd *cobra.Command) string {
	format, _ := cmd.Flags().GetString("format")
	if format == "" {
		format = os.Getenv("GOBAN_OUTPUT_FORMAT")
	}
	return format
}
