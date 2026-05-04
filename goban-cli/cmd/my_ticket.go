package cmd

import (
	"fmt"

	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"

	"github.com/spf13/cobra"

	"goban-cli/internal/client"
	"goban-cli/internal/types"
)

// myTicketCmd shows tickets claimed by the current user
var myTicketCmd = &cobra.Command{
	Use:   "my-tickets",
	Short: "Show all tickets you have claimed",
	Long: `Shows all tickets that are currently claimed by your username.

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.
Your username can be provided via --user flag or GOBAN_USER environment variable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := getClient(cmd)
		fmtter := getFormatter(cmd)

		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		userName := getUser(cmd)
		if userName == "" {
			return gerr.NewUserError("username is required", "Use --user flag, GOBAN_USER env var, or 'user' in config.yaml")
		}

		ctx := cmd.Context()

		tickets, classErr := c.SafeListTickets(ctx, client.ListTicketsRequestParams{
			BoardID: boardID,
			View:    "full",
		})
		if classErr != nil {
			return classErr
		}

		var myTickets []types.Ticket
		for _, t := range tickets {
			if t.Assignee == userName {
				myTickets = append(myTickets, t)
			}
		}

		fmt.Println("\nYour Claimed Tickets:")
		fmt.Println("----------------------")
		if len(myTickets) == 0 {
			fmtter.PrintWarning("You don't have any claimed tickets on this board")
			fmt.Println()
			return nil
		}

		fmtter.PrintTickets(myTickets)
		fmt.Printf("\nTotal: %d ticket(s)\n", len(myTickets))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(myTicketCmd)
}
