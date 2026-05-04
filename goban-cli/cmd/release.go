package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/session"
	"goban-cli/internal/types"
	"goban-cli/internal/validate"
)

// releaseCmd releases a claimed ticket back to TODO.
var releaseCmd = &cobra.Command{
	Use:   "release [ticket-id]",
	Short: "Release your claimed ticket back to TODO",
	Long: `Releases a ticket, clearing the assignee and returning it to TODO.

If no ticket ID is provided, releases the currently active (session) ticket.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clientInstance := getClient(cmd)
		fmtter := getFormatter(cmd)

		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		userName := getUser(cmd)
		if userName == "" {
			return gerr.NewUserError("username is required", "Use --user flag, GOBAN_USER env var, or 'user' in config.yaml")
		}

		// Resolve ticket ID from args, session file, or error
		ticketID := resolveReleaseTicketID(args)
		if err := validate.ValidateTicketID(ticketID); err != nil {
			return gerr.NewUserError(err.Error(), "Specify a ticket ID or claim one first.")
		}

		ctx := cmd.Context()

		// Pre-check: verify ticket is claimed by the current user
		currentTicket, classErr := clientInstance.SafeGet(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		if currentTicket.Assignee == "" {
			fmtter.PrintWarning(fmt.Sprintf("Ticket %s is not currently claimed.", ticketID))
			return nil // Not an error - just informational
		}

		if currentTicket.Assignee != userName {
			return gerr.NewUserError(
				fmt.Sprintf("Ticket is claimed by @%s, not @%s", currentTicket.Assignee, userName), "")
		}

		// Release the ticket using existing API method
		response, classErr := clientInstance.SafeRelease(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		// Verify release (ticket should have empty assignee after release)
		_, classErr = clientInstance.VerifyRelease(ctx, ticketID)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\n✓ Ticket Released (Verified)")
		fmt.Println("--------------")
		fmtter.PrintTicket(types.Ticket(*response))

		// Clear session file if it matches this ticket
		sess, _ := session.Read()
		if sess != nil && sess.TicketID == ticketID {
			session.Clear()
			fmt.Println("\nNote: Session cleared. Use 'claim' to pick up a new ticket.")
		}

		fmt.Println()
		return nil
	},
}

// resolveReleaseTicketID returns the ticket ID from args or session file.
func resolveReleaseTicketID(args []string) string {
	if len(args) > 0 {
		return args[0]
	}

	s, err := session.Read()
	if err == nil && s != nil && s.TicketID != "" {
		return s.TicketID
	}

	return ""
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
