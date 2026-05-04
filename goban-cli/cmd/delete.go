package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <ticket-id>",
	Short: "Delete a ticket",
	Long:  `Deletes the specified ticket.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)

		// --- 1. INPUT VALIDATION (fast failure, no network call) ---
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		ticketID := args[0]
		if err := validate.ValidateTicketID(ticketID); err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Printf("This will permanently delete ticket %s. Use --force to confirm.\n", ticketID)
			return nil
		}

		ctx := cmd.Context()

		classErr := client.SafeDelete(ctx, boardID, ticketID)
		if classErr != nil {
			return classErr
		}

		// Post-mutation verification (CASE 01)
		_, classErr = client.VerifyDelete(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		fmt.Printf("\n✓ Ticket %s deleted (Verified)\n", ticketID)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolP("force", "f", false, "Force deletion without confirmation")
	rootCmd.AddCommand(deleteCmd)
}
