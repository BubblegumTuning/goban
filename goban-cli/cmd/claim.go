package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/session"
	"goban-cli/internal/types"
	"goban-cli/internal/validate"
)

// claimCmd claims a ticket for the current user and moves it to In Progress
var targetStatus string // --status / -s flag value

var claimCmd = &cobra.Command{
	Use:   "claim <ticket-id>",
	Short: "Claim a ticket and move it to In Progress",
	Long: `Claims a ticket for your username and automatically moves it to "In Progress" status.

The board ID is optional for v1.1 API (use --board flag or GOBAN_BOARD env var if needed).

Uses the v1.1 API which handles authentication via Bearer token (set via API_TOKEN
in config file or environment). The server automatically assigns the ticket based
on your authenticated user identity.

Use --status/-s to claim and immediately move to a different status:
  goban-cli claim ticket-id -s done    # Claim + move to DONE
  goban-cli claim ticket-id -s review  # Claim + move to REVIEW`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		// boardID is optional for v1.1 API - tickets are globally accessible by ID
		_ = getBoardID(cmd) // unused but available via --board flag or env var

		ticketID := args[0]
		if err := validate.ValidateTicketID(ticketID); err != nil {
			return gerr.NewUserError(err.Error(), "")
		}

		ctx := cmd.Context()

		// First, get the ticket to check its current state (using efficient v1 API)
		ticket, classErr := client.SafeGet(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		// Check if already claimed by someone else (using MatchesColumn for -0 suffix compatibility)
		if ticket.Assignee != "" && ticket.MatchesColumn("inprogress") {
			fmtter.PrintWarning(fmt.Sprintf("Ticket is already claimed and in progress by @%s", ticket.Assignee))
			return nil
		}

		// Claim the ticket using v1.1 API (POST /api/v1/tickets/{id}/claim)
		response, classErr := client.SafeClaim(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		// Validate that we actually got a valid response
		if response.Ticket == nil {
			return gerr.NewVerifyFailedError("Server returned empty response — operation may not have completed", "")
		}

		// Post-mutation verification (CASE 01)
		expectedAssignee := response.Ticket.Assignee
		_, classErr = client.VerifyClaim(ctx, ticketID, expectedAssignee)
		if classErr != nil {
			return classErr
		}

		// Write session file for convenience commands
		boardID := getBoardID(cmd)
		userName := getUser(cmd)
		_ = session.Write(ticketID, boardID, userName)

		fmt.Println("\n✓ Ticket Claimed (Verified)")
		fmt.Println("-----------------")

		// Handle auto-released tickets (array instead of single ID)
		if len(response.AutoReleased) > 0 {
			for _, releasedID := range response.AutoReleased {
				fmt.Printf("Note: Auto-released conflicting ticket [%s]\n", releasedID)
			}
			fmt.Println()
		}

		// Post-claim move if --status specified
		if targetStatus != "" {
			apiStatus, err := validate.ValidateStatus(targetStatus)
			if err != nil {
				return gerr.NewUserError(err.Error(), "")
			}

			fmt.Printf("\nMoving to %s...\n", strings.ToUpper(apiStatus))

			moveResponse, classErr := client.SafeMove(ctx, ticketID, apiStatus, false)
			if classErr != nil {
				return gerr.NewUserError(
					fmt.Sprintf("Claim succeeded but move to %s failed: %s", targetStatus, classErr.Message), "")
			}

			// Verify the move
			expectedColumn := validate.ColumnPrefixFromAPIStatus(apiStatus)
			_, classErr = client.VerifyMove(ctx, ticketID, expectedColumn)
			if classErr != nil {
				return classErr
			}

			fmt.Println("✓ Ticket Moved (Verified)")
			fmt.Println("--------------")
			fmtter.PrintTicket(types.Ticket(*moveResponse))
			fmt.Println()

			// Clear session on terminal states after claim+move
			if apiStatus == "DONE" || apiStatus == "CANCELLED" || apiStatus == "TODO" || apiStatus == "BACKLOG" {
				session.Clear()
			}
			return nil
		}

		fmtter.PrintTicket(*response.Ticket)
		fmt.Println()

		return nil
	},
}

func init() {
	claimCmd.Flags().StringP("status", "s", "",
		"After claiming, move ticket to this status (backlog, todo, inprogress, review, done, cancelled)")
	rootCmd.AddCommand(claimCmd)
}
