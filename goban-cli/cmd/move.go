package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/session"
	"goban-cli/internal/types"
	"goban-cli/internal/validate"
)

var (
	forceMove   bool
	noAutoClaim bool
)

// moveCmd moves a ticket to a different status using v1.1 API
var moveCmd = &cobra.Command{
	Use:   "move <ticket-id> <status>",
	Short: "Move a ticket to a different status",
	Long: `Moves a ticket to a different status (To Do, In Progress, or Done) using the v1.1 API.

The board ID can be provided via --board flag or GOBAN_BOARD environment variable.

Status options:
 - "Backlog" or "backlog"
 - "To Do" or "todo"
 - "In Progress" or "inprogress" or "ip"
 - "Review" or "review"
 - "Done" or "done"
 - "Cancelled" or "cancelled"` + "\n\n" + `The --force flag is required when moving tickets out of terminal states (DONE/CANCELLED).

Unclaimed tickets are automatically claimed before moving. Use --no-auto-claim to disable this behavior.`,
	Args: cobra.ExactArgs(2),
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

		targetStatus, err := validate.ValidateStatus(args[1])
		if err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		ctx := cmd.Context()

		// Fetch current ticket to check its status (for warning on terminal state moves)
		currentTicket, classErr := client.SafeGet(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		// Auto-claim if unclaimed and --no-auto-claim not set
		userName := getUser(cmd)
		if currentTicket.Assignee == "" && !noAutoClaim && userName != "" {
			fmt.Fprintf(cmd.OutOrStderr(), "\nAuto-claiming unclaimed ticket %s...\n", ticketID)

			response, classErr := client.SafeClaim(ctx, ticketID)
			if classErr != nil {
				return classErr
			}

			if response.Ticket == nil {
				return gerr.NewVerifyFailedError("Server returned empty claim response", "")
			}

			// Verify the claim succeeded
			expectedAssignee := response.Ticket.Assignee
			_, classErr = client.VerifyClaim(ctx, ticketID, expectedAssignee)
			if classErr != nil {
				return classErr
			}

			fmt.Fprintf(cmd.OutOrStderr(), "✓ Auto-claimed as @%s\n", expectedAssignee)

			// Update currentTicket for subsequent terminal state checks
			currentTicket = response.Ticket

			// Write session file after successful auto-claim
			_ = session.Write(ticketID, boardID, userName)
		} else if currentTicket.Assignee != "" && !noAutoClaim {
			// Ticket claimed by someone else — proceed; server will return 403 if forbidden
			if currentTicket.Assignee != userName {
				fmt.Fprintf(cmd.OutOrStderr(), "\n⚠️  Ticket is already claimed by @%s\n", currentTicket.Assignee)
			}
		}

		// Check if moving from a terminal state using MatchesColumn helper (handles "done-0"/"done")
		isTerminalState := currentTicket.MatchesColumn("done") || currentTicket.MatchesColumn("cancelled")
		if isTerminalState && !forceMove {
			fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠️  WARNING: Moving ticket from [%s] column\n", currentTicket.Column)
			fmt.Fprintf(cmd.ErrOrStderr(), "This may indicate an accidental reset of completed work.\n")
			fmt.Fprintf(cmd.ErrOrStderr(), "\nUse --force flag if this move is intentional:\n")
			fmt.Fprintf(cmd.ErrOrStderr(), " goban-cli move %s %s --force\n", ticketID, args[1])
			return gerr.NewUserError("blocked: moving from terminal state requires --force flag", "")
		}

		// Use v1.1 API: POST /api/v1/tickets/{id}/move with {target_status, force} body
		response, classErr := client.SafeMove(ctx, ticketID, targetStatus, forceMove)
		if classErr != nil {
			return classErr
		}

		// Post-mutation verification (CASE 01)
		_, classErr = client.VerifyMove(ctx, ticketID, validate.ColumnPrefixFromAPIStatus(targetStatus))
		if classErr != nil {
			return classErr
		}

		// Clear session on terminal/non-working states
		if targetStatus == "DONE" || targetStatus == "CANCELLED" || targetStatus == "TODO" || targetStatus == "BACKLOG" {
			session.Clear()
		}

		fmt.Println("\n✓ Ticket Moved (Verified)")
		fmt.Println("--------------")
		// MoveResponse is a type alias for types.Ticket (server returns ticket directly)
		fmtter.PrintTicket(types.Ticket(*response))
		fmt.Println()

		return nil
	},
}

func init() {
	moveCmd.Flags().BoolVar(&forceMove, "force", false, "Force move from terminal states (done/cancelled)")
	moveCmd.Flags().BoolVar(&noAutoClaim, "no-auto-claim", false, "Do not auto-claim unclaimed tickets; fail with 403 instead")
	rootCmd.AddCommand(moveCmd)
}
