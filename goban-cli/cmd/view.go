package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"
)

// viewCmd shows full details of a ticket including comments
var viewCmd = &cobra.Command{
	Use:   "view [ticket-id]",
	Short: "View full ticket details",
	Long: `Shows complete details of a ticket including title, description, status, and all comments.

If no ticket ID is provided, uses the active session ticket (set by 'claim').

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		ticketID := ""
		if len(args) > 0 {
			ticketID = args[0]
		} else {
			var err error
			ticketID, err = resolveSessionTicket(cmd)
			if err != nil {
				return err
			}
		}

		if err := validate.ValidateTicketID(ticketID); err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		ctx := cmd.Context()

		ticket, classErr := client.SafeGet(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\nTicket Details:")
		if fmtter.Format() == "compact" {
			fmtter.PrintViewCompact(*ticket)
		} else {
			fmtter.PrintTicket(*ticket)
		}

		// Also show comments if any (CASE 07: use SafeListComments for consistent error formatting)
		comments, commentClassErr := client.SafeListComments(ctx, boardID, ticketID)
		if commentClassErr != nil {
			fmtter.PrintWarning(fmt.Sprintf(
				"Could not load comments (data may be stale): %s", commentClassErr.Message))
		} else if len(comments) > 0 {
			fmt.Println("\nComments:")
			fmt.Println("---------")
			fmtter.PrintComments(comments)
		}

		fmt.Println()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(viewCmd)
}
