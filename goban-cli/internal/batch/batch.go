package batch

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"goban-cli/internal/client"
	gerr "goban-cli/internal/errors"
)

// BatchResult holds the aggregated results of a batch operation.
type BatchResult struct {
	Total     int
	Succeeded []string      // IDs that succeeded
	Failed    []Failure     // IDs that failed with reason
	Skipped   []SkippedItem // IDs skipped intentionally
}

// Failure records why a ticket failed during batch processing.
type Failure struct {
	TicketID string
	Reason   string
}

// SkippedItem records why a ticket was skipped during batch processing.
type SkippedItem struct {
	TicketID string
	Reason   string
}

// ProcessDone moves multiple tickets to DONE status.
// For each ID: auto-claim if needed, then move to done.
func ProcessDone(ctx context.Context, c *client.Client, boardID, user string,
	ticketIDs []string, force bool,
) BatchResult {
	result := BatchResult{Total: len(ticketIDs)}

	for _, ticketID := range ticketIDs {
		err := processSingleMove(ctx, c, ticketID, "DONE", boardID, user, force)
		if err == nil {
			result.Succeeded = append(result.Succeeded, ticketID)
		} else {
			result.Failed = append(result.Failed, Failure{TicketID: ticketID, Reason: err.Error()})
		}
	}

	return result
}

// ProcessCancel moves multiple tickets to CANCELLED status.
func ProcessCancel(ctx context.Context, c *client.Client, boardID, user string,
	ticketIDs []string, force bool,
) BatchResult {
	result := BatchResult{Total: len(ticketIDs)}

	for _, ticketID := range ticketIDs {
		err := processSingleMove(ctx, c, ticketID, "CANCELLED", boardID, user, force)
		if err == nil {
			result.Succeeded = append(result.Succeeded, ticketID)
		} else {
			result.Failed = append(result.Failed, Failure{TicketID: ticketID, Reason: err.Error()})
		}
	}

	return result
}

// processSingleMove handles claim+move for a single ticket.
func processSingleMove(ctx context.Context, c *client.Client, ticketID, targetStatus, boardID, user string, force bool) error {
	// Fetch current state
	currentTicket, classErr := c.SafeGet(ctx, ticketID)
	if classErr != nil {
		return fmt.Errorf("fetch failed: %s", classErr.Message)
	}

	// Auto-claim if unclaimed
	if currentTicket.Assignee == "" && user != "" {
		response, claimErr := c.SafeClaim(ctx, ticketID)
		if claimErr != nil {
			return fmt.Errorf("auto-claim failed: %s", claimErr.Message)
		}

		if response.Ticket == nil {
			return fmt.Errorf("server returned empty claim response")
		}

		expectedAssignee := response.Ticket.Assignee
		_, verifyErr := c.VerifyClaim(ctx, ticketID, expectedAssignee)
		if verifyErr != nil {
			return fmt.Errorf("claim verification failed: %s", verifyErr.Message)
		}
	} else if currentTicket.Assignee != "" && currentTicket.Assignee != user {
		if force {
			// Proceed with --force even if claimed by another user
		} else {
			return fmt.Errorf("claimed by @%s (use --force to override)", currentTicket.Assignee)
		}
	}

	// Move the ticket
	response, moveErr := c.SafeMove(ctx, ticketID, targetStatus, force)
	if moveErr != nil {
		return fmt.Errorf("move failed: %s", moveErr.Message)
	}

	_ = response // Response validated by verification step below

	// Verify the move
	expectedColumn := map[string]string{
		"DONE":      "done",
		"CANCELLED": "cancelled",
	}[targetStatus]

	if expectedColumn == "" {
		expectedColumn = targetStatus
	}

	_, verifyErr := c.VerifyMove(ctx, ticketID, expectedColumn)
	if verifyErr != nil {
		return fmt.Errorf("move verification failed: %s", verifyErr.Message)
	}

	return nil
}

// CollectTicketIDs checks args first, then file, then stdin (priority order).
func CollectTicketIDs(args []string, filePath string) []string {
	if len(args) > 0 {
		return FilterTicketIDs(args)
	}

	if filePath != "" {
		ids := ReadFromFile(filePath)
		if len(ids) > 0 {
			return ids
		}
	}

	// Read from stdin if it's a pipe/redirected input
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return ReadFromStdin()
	}

	return nil
}

// FilterTicketIDs filters args to only include ticket-like IDs.
func FilterTicketIDs(args []string) []string {
	var ids []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "ticket-") || len(arg) > 0 {
			ids = append(ids, arg)
		}
	}
	return ids
}

// ReadFromStdin reads ticket IDs from stdin (one per line).
func ReadFromStdin() []string {
	var ids []string
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			ids = append(ids, line)
		}
	}
	return ids
}

// ReadFromFile reads ticket IDs from a file (one per line).
func ReadFromFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open %s: %v\n", path, err)
		return nil
	}
	defer file.Close()

	var ids []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) > 0 && !strings.HasPrefix(line, "#") {
			ids = append(ids, line)
		}
	}
	return ids
}

// PrintBatchResult prints the batch operation results to stdout.
func PrintBatchResult(result BatchResult, targetStatus string) {
	fmt.Printf("\nProcessing %d tickets...\n", result.Total)

	for _, id := range result.Succeeded {
		fmt.Printf("✓ %s → %s\n", id, targetStatus)
	}
	for _, f := range result.Failed {
		fmt.Printf("✗ %s FAILED: %s\n", f.TicketID, f.Reason)
	}
	for _, s := range result.Skipped {
		fmt.Printf("- %s SKIPPED: %s\n", s.TicketID, s.Reason)
	}

	fmt.Printf("\nSummary: %d succeeded, %d skipped, %d failed\n",
		len(result.Succeeded), len(result.Skipped), len(result.Failed))
}

// NewUserError creates a user-facing error from classified errors.
func UserError(msg string) error {
	return gerr.NewUserError(msg, "")
}
