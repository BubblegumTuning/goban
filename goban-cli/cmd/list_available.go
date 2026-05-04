package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gobanclient "goban-cli/internal/client"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"

	"goban-cli/internal/types"
)

// listAvailableCmd lists tickets available to claim (not claimed, status = "To Do")
var listAvailableCmd = &cobra.Command{
	Use:   "list-available",
	Short: "List tickets available to claim",
	Long: `Lists all tickets that are available to be claimed.

This command filters for tickets with:
- Status: "To Do"
- Not currently claimed by anyone

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		ctx := cmd.Context()

		tickets, err := client.ListTickets(ctx, boardID)
		if err != nil {
			return gobanclient.Classify(err, "listing tickets for board "+boardID)
		}

		// Filter for available tickets (backlog/todo/review + unclaimed) using
		// the new MatchesColumn() helper for canonical "*-0" suffix support. DRY.
		var available []types.Ticket
		for _, t := range tickets {
			if (t.MatchesColumn("backlog") || t.MatchesColumn("todo") || t.MatchesColumn("review")) && !t.IsClaimed() {
				available = append(available, t)
			}
		}

		fmt.Println("\nAvailable Tickets (Backlog/To Do/Review + Unclaimed):")
		fmt.Println("--------------------------------------")
		if len(available) == 0 {
			fmtter.PrintWarning("No tickets available to claim right now")
			fmt.Println()
			return nil
		}

		fmtter.PrintTickets(available)
		fmt.Println()
		fmt.Printf("\nTotal: %d ticket(s) available\n", len(available))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listAvailableCmd)
}
