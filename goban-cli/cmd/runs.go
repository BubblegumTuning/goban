package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"goban-cli/internal/client"
)

var runsCmd = &cobra.Command{
	Use:   "runs <ticket-id>",
	Short: "View run history for a ticket",
	Long:  "Display all run attempts and the current active run for a ticket.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("ticket-id is required")
		}

		ticketID := args[0]
		apiClient := getClient(cmd)

		ctx := cmd.Context()

		runsWithError, err := apiClient.GetRuns(ctx, ticketID)
		if err != nil {
			return fmt.Errorf("failed to get runs: %w", err)
		}

		fmt.Printf("\nRun History for %s\n", ticketID)
		fmt.Println(strings.Repeat("-", 50))

		if len(runsWithError) == 0 {
			fmt.Println("No run history found.")
			return nil
		}

		for i, run := range runsWithError {
			outcome := run.Outcome
			activeMarker := ""
			if outcome == "active" {
				activeMarker = " [ACTIVE]"
			}

			startedAt := truncateTime(run.StartedAt)
			endedStr := ""
			if run.EndedAt != nil && *run.EndedAt != "" {
				endedStr = fmt.Sprintf(" | ended: %s", truncateTime(*run.EndedAt))
			}

			fmt.Printf("\n  #%d (ID: %d) - %s%s\n", i+1, run.ID, outcome, activeMarker)
			fmt.Printf("    started: %s%s\n", startedAt, endedStr)
			if run.Actor != "" {
				fmt.Printf("    actor:   %s\n", run.Actor)
			}
			if run.Summary != "" {
				fmt.Printf("    summary: %s\n", run.Summary)
			}
		}

		return nil
	},
}

var startRunCmd = &cobra.Command{
	Use:   "start <ticket-id>",
	Short: "Start a new run for a ticket",
	Long:  "Create a new active run record tracking the current attempt on a ticket.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("ticket-id is required")
		}

		ticketID := args[0]
		apiClient := getClient(cmd)

		ctx := cmd.Context()

		req := client.RunRequest{Summary: summaryFlag}
		run, err := apiClient.CreateRun(ctx, ticketID, req)
		if err != nil {
			return fmt.Errorf("failed to create run: %w", err)
		}

		fmt.Printf("\nRun Started\n")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Printf("  Run ID:    %d\n", run.ID)
		fmt.Printf("  Ticket:    %s\n", run.TicketID)
		fmt.Printf("  Outcome:   %s\n", run.Outcome)
		fmt.Printf("  Started:   %s\n", truncateTime(run.StartedAt))
		fmt.Printf("  Actor:     %s\n", run.Actor)

		return nil
	},
}

var finishRunCmd = &cobra.Command{
	Use:   "finish <ticket-id> [--run-id <id>]",
	Short: "Finish the current active run for a ticket",
	Long:  "Mark the active run as completed (or with specified outcome).",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("ticket-id is required")
		}

		ticketID := args[0]
		apiClient := getClient(cmd)

		ctx := cmd.Context()

		if runIDFlag == 0 {
			activeRun, err := apiClient.GetActiveRun(ctx, ticketID)
			if err != nil {
				return fmt.Errorf("no active run found: %w", err)
			}
			runIDFlag = activeRun.ID
		}

		req := client.RunRequest{Summary: outcomeFlag}
		err := apiClient.UpdateRun(ctx, ticketID, runIDFlag, req)
		if err != nil {
			return fmt.Errorf("failed to update run: %w", err)
		}

		fmt.Printf("\nRun Finished\n")
		fmt.Println(strings.Repeat("-", 30))
		fmt.Printf("  Run ID:   %d\n", runIDFlag)
		if outcomeFlag != "" {
			fmt.Printf("  Outcome:  %s\n", outcomeFlag)
		} else {
			fmt.Printf("  Outcome:  completed\n")
		}

		return nil
	},
}

func truncateTime(ts string) string {
	if ts == "" {
		return "N/A"
	}
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts[:min(len(ts), 19)]
	}
	return parsed.Format("2006-01-02 15:04")
}

var summaryFlag   string
var outcomeFlag   string
var runIDFlag     int64

func init() {
	rootCmd.AddCommand(runsCmd)
	rootCmd.AddCommand(startRunCmd)
	rootCmd.AddCommand(finishRunCmd)

	startRunCmd.Flags().StringVar(&summaryFlag, "summary", "", "Summary for the new run")
	finishRunCmd.Flags().StringVar(&outcomeFlag, "outcome", "completed", "Outcome: completed, released, blocked")
	finishRunCmd.Flags().Int64VarP(&runIDFlag, "run-id", "r", 0, "Specific run ID (defaults to active)")
}
