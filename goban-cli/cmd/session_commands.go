package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/session"
	"goban-cli/internal/types"
)

// resolveSessionTicket reads the session file and returns ticket ID.
// Returns error if no active session exists.
func resolveSessionTicket(cmd *cobra.Command) (string, error) {
	s, err := session.Read()
	if err != nil {
		return "", gerr.NewUserError("Failed to read session file: "+err.Error(), "")
	}
	if s == nil || s.TicketID == "" {
		return "", gerr.NewUserError(
			"No active ticket. Use 'claim <ticket-id>' first or specify a ticket ID explicitly.", "")
	}

	return s.TicketID, nil
}

// moveFromSession reads the session file and moves the ticket to the given status.
func moveFromSession(cmd *cobra.Command, targetStatus string, force bool) error {
	ticketID, err := resolveSessionTicket(cmd)
	if err != nil {
		return err
	}

	clientInstance := getClient(cmd)
	fmtter := getFormatter(cmd)

	fmt.Fprintf(cmd.OutOrStderr(), "\nMoving %s to %s...\n", ticketID, targetStatus)

	response, classErr := clientInstance.SafeMove(cmd.Context(), ticketID, targetStatus, force)
	if classErr != nil {
		return classErr
	}

	// Verify the move
	expectedColumn := map[string]string{
		"DONE":        "done",
		"CANCELLED":   "cancelled",
		"REVIEW":      "review",
		"TODO":        "todo",
		"BACKLOG":     "backlog",
		"IN_PROGRESS": "inprogress",
	}[targetStatus]

	if expectedColumn == "" {
		expectedColumn = targetStatus
	}

	_, classErr = clientInstance.VerifyMove(cmd.Context(), ticketID, expectedColumn)
	if classErr != nil {
		return classErr
	}

	fmt.Println("\n✓ Ticket Moved (Verified)")
	fmt.Println("--------------")
	fmtter.PrintTicket(types.Ticket(*response))
	fmt.Println()

	// Clear session on terminal/non-working states
	isTerminal := targetStatus == "DONE" || targetStatus == "CANCELLED"
	if isTerminal {
		session.Clear()
	} else if targetStatus == "TODO" || targetStatus == "BACKLOG" {
		session.Clear()
	}

	return nil
}

var doneCmd = &cobra.Command{
	Use:   "done",
	Short: "Move the active ticket to DONE",
	Long: `Moves the currently claimed (session) ticket to DONE status.
No arguments needed — uses the session file created by 'claim'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return moveFromSession(cmd, "DONE", force)
	},
}

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Move the active ticket to REVIEW",
	Long:  `Moves the currently claimed (session) ticket to REVIEW status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return moveFromSession(cmd, "REVIEW", false)
	},
}

var todoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Return the active ticket to TODO",
	Long:  `Returns the currently claimed (session) ticket back to TODO status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return moveFromSession(cmd, "TODO", false)
	},
}

var backlogCmd = &cobra.Command{
	Use:   "backlog",
	Short: "Return the active ticket to BACKLOG",
	Long:  `Returns the currently claimed (session) ticket back to BACKLOG status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return moveFromSession(cmd, "BACKLOG", false)
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel",
	Short: "Move the active ticket to CANCELLED",
	Long:  `Moves the currently claimed (session) ticket to CANCELLED status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		return moveFromSession(cmd, "CANCELLED", force)
	},
}

func init() {
	doneCmd.Flags().Bool("force", false, "Force move from terminal states")
	cancelCmd.Flags().Bool("force", false, "Force move from terminal states")

	rootCmd.AddCommand(doneCmd)
	rootCmd.AddCommand(reviewCmd)
	rootCmd.AddCommand(todoCmd)
	rootCmd.AddCommand(backlogCmd)
	rootCmd.AddCommand(cancelCmd)
}
