package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"

	"goban-cli/internal/types"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new ticket",
	Long:  `Creates a new ticket on the specified board.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		// --- 1. INPUT VALIDATION (fast failure, no network call) ---
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		title, _ := cmd.Flags().GetString("title")
		if err := validate.ValidateTitle(title); err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		description, _ := cmd.Flags().GetString("description")
		columnStr, _ := cmd.Flags().GetString("column")

		// Map user-friendly column names to API column IDs (default to "todo")
		var column string
		if columnStr == "" {
			column = "todo"
		} else {
			apiStatus, err := validate.ValidateStatus(columnStr)
			if err != nil {
				return gerr.NewUserError(err.Error(), "")
			}
			column = validate.ColumnPrefixFromAPIStatus(apiStatus)
		}

		ctx := cmd.Context()

		idempKey, _ := cmd.Flags().GetString("idempotency-key")
	parents, _ := cmd.Flags().GetStringSlice("parents")
	ticket, classErr := client.SafeCreate(ctx, boardID, types.CreateTicketRequest{Title: title, Description: description, Column: column, BoardID: boardID, IdempotencyKey: idempKey, Parents: parents})
		if classErr != nil {
			return classErr
		}

		// Post-mutation verification (CASE 01)
		_, classErr = client.VerifyCreate(ctx, ticket.ID, title)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\n✓ Ticket Created (Verified)")
		fmtter.PrintTicket(*ticket)
		fmt.Println()
		return nil
	},
}

func init() {
	createCmd.Flags().StringP("title", "t", "", "Ticket title (required)")
	createCmd.Flags().StringP("description", "d", "", "Ticket description")
	createCmd.Flags().StringP("column", "c", "todo", "Initial column (backlog, todo, inprogress, review, done)")
	createCmd.Flags().StringP("idempotency-key", "k", "", "Idempotency key — duplicate creates with the same key return the existing ticket")
	createCmd.Flags().StringSliceP("parents", "p", nil, "Parent ticket IDs (comma-separated or repeated)")
	rootCmd.AddCommand(createCmd)
}
