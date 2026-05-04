package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/types"
	"goban-cli/internal/validate"
)

// commentCmd adds a comment to a ticket
var commentCmd = &cobra.Command{
	Use:   "comment [ticket-id]",
	Short: "Add a comment to a ticket",
	Long: `Adds a comment to the specified ticket.

If no ticket ID is provided, uses the active session ticket (set by 'claim').

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.
Your username can be provided via --user flag or GOBAN_USER environment variable.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		// --- 1. INPUT VALIDATION (fast failure, no network call) ---
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		userName := getUser(cmd)
		if userName == "" {
			return gerr.NewUserError("username is required", "Use --user flag, GOBAN_USER env var, or 'user' in config.yaml")
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

		content, _ := cmd.Flags().GetString("message")
		if strings.TrimSpace(content) == "" {
			return gerr.NewUserError("comment message cannot be empty", "Use --message flag to provide comment text")
		}

		ctx := cmd.Context()

		comment, classErr := client.SafeAddComment(ctx, boardID, ticketID, types.AddCommentRequest{Who: userName, Text: content})
		if classErr != nil {
			return classErr
		}

		// Post-mutation verification (CASE 01)
		_, classErr = client.VerifyComment(ctx, ticketID, userName, content)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\n✓ Comment Added (Verified)")
		fmt.Println("----------------")
		fmtter.PrintComment(*comment)
		fmt.Println()

		return nil
	},
}

// listCommentsCmd lists all comments for a ticket
var listCommentsCmd = &cobra.Command{
	Use:   "list-comments <ticket-id>",
	Short: "List all comments on a ticket",
	Long: `Lists all comments for the specified ticket.

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.`,
	Args: cobra.ExactArgs(1),
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

		ctx := cmd.Context()

		comments, classErr := client.SafeListComments(ctx, boardID, ticketID)
		if classErr != nil {
			return classErr
		}

		fmt.Printf("\nComments for ticket %s:\n", ticketID)
		fmt.Println("-----------------------------------")
		fmtter.PrintComments(comments)
		fmt.Println()

		return nil
	},
}

func init() {
	commentCmd.Flags().StringP("message", "m", "", "Comment text to add (required)")
	rootCmd.AddCommand(commentCmd)
	rootCmd.AddCommand(listCommentsCmd)
}
