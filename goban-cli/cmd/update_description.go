package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"

	"goban-cli/internal/types"
)

// updateDescriptionCmd updates a ticket's description
var updateDescriptionCmd = &cobra.Command{
	Use:   "update-description <ticket-id>",
	Short: "Update a ticket's description",
	Long:  `Updates the description of the specified ticket.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		// --- 1. INPUT VALIDATION (fast failure, no network call) ---
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		ticketID := args[0]
		if err := validate.ValidateTicketID(ticketID); err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		description, _ := cmd.Flags().GetString("description")
		if description == "" {
			return gerr.NewUserError("--description is required", "")
		}

		ctx := cmd.Context()

		ticket, classErr := client.SafeUpdate(ctx, boardID, ticketID, types.UpdateTicketRequest{Description: description})
		if classErr != nil {
			return classErr
		}

		// Post-mutation verification (CASE 01)
		_, classErr = client.VerifyUpdate(ctx, ticketID, "description", description)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\n✓ Description Updated (Verified)")
		fmtter.PrintTicket(*ticket)
		fmt.Println()
		return nil
	},
}

func init() {
	updateDescriptionCmd.Flags().StringP("description", "d", "", "New description text")
	rootCmd.AddCommand(updateDescriptionCmd)
}
