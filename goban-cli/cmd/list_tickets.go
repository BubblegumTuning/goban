package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"

	"goban-cli/internal/client"
)

// listTicketsCmd lists all tickets on a board using v1.1 API with query parameters
var listTicketsCmd = &cobra.Command{
	Use:   "list-tickets",
	Short: "List all tickets (uses v1.1 API)",
	Long: `Lists all tickets using the v1.1 API with filtering options.

View Modes:
  (default)     Show TODO, IN_PROGRESS, REVIEW columns (lightweight view)
  --backlog     Include BACKLOG column in results  
  --full        Show ALL columns including DONE and CANCELLED`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := getClient(cmd)
		fmtter := getFormatter(cmd)

		// Build query parameters based on flags
		var params client.ListTicketsRequestParams

		// Include board_id if specified (filters results by board)
		boardID := getBoardID(cmd)
		if boardID != "" {
			if err := validate.ValidateBoardID(boardID); err != nil {
				return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
			}
			params.BoardID = boardID
		}

		if fullBoard {
			params.View = "full" // Show all columns including DONE/CANCELLED
		} else if includeBacklog {
			params.Include = "backlog" // Include BACKLOG column
		}

		ctx := cmd.Context()

		tickets, classErr := c.SafeListTickets(ctx, params)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\nTickets:")
		fmt.Println("--------")
		fmtter.PrintTickets(tickets)
		fmt.Println()

		return nil
	},
}

var (
	includeBacklog bool // --backlog flag: include backlog column
	fullBoard      bool // --full flag: show all columns including done/cancelled
)

func init() {
	listTicketsCmd.Flags().BoolVar(&includeBacklog, "backlog", false, "Include BACKLOG column in results")
	listTicketsCmd.Flags().BoolVar(&fullBoard, "full", false, "Show ALL columns including DONE and CANCELLED")
	rootCmd.AddCommand(listTicketsCmd)
}
