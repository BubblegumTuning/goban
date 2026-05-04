package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"goban-cli/internal/batch"
	gerr "goban-cli/internal/errors"
	"goban-cli/internal/validate"
)

var (
	batchFilePath string
	batchForce    bool
)

// batchDoneCmd moves multiple tickets to DONE status.
var batchDoneCmd = &cobra.Command{
	Use:   "batch-done [ticket-id...]",
	Short: "Move multiple tickets to DONE",
	Long: `Moves multiple tickets to DONE status. Accepts IDs from:
  - Command-line arguments: goban-cli batch-done ticket-a ticket-b
  - File:                   goban-cli batch-done --file ids.txt
  - Stdin pipe:             echo -e "a\nb" | goban-cli batch-done

Unclaimed tickets are auto-claimed before moving. Use --force to override
tickets claimed by other users or move from terminal states.`,
	Args: cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		clientInstance := getClient(cmd)
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		userName := getUser(cmd)
		if userName == "" {
			return gerr.NewUserError("username is required", "Use --user flag, GOBAN_USER env var, or 'user' in config.yaml")
		}

		ticketIDs := batch.CollectTicketIDs(args, batchFilePath)
		if len(ticketIDs) == 0 {
			return fmt.Errorf("no ticket IDs provided; use arguments, --file, or pipe via stdin")
		}

		result := batch.ProcessDone(cmd.Context(), clientInstance, boardID, userName, ticketIDs, batchForce)
		batch.PrintBatchResult(result, "DONE")

		if len(result.Failed) > 0 {
			return fmt.Errorf("%d failure(s)", len(result.Failed))
		}
		return nil
	},
}

// batchCancelCmd moves multiple tickets to CANCELLED status.
var batchCancelCmd = &cobra.Command{
	Use:   "batch-cancel [ticket-id...]",
	Short: "Move multiple tickets to CANCELLED",
	Long: `Moves multiple tickets to CANCELLED status. Accepts IDs from:
  - Command-line arguments: goban-cli batch-cancel ticket-a ticket-b
  - File:                   goban-cli batch-cancel --file ids.txt
  - Stdin pipe:             echo -e "a\nb" | goban-cli batch-cancel

Unclaimed tickets are auto-claimed before cancelling. Use --force to override
tickets claimed by other users or move from terminal states.`,
	Args: cobra.MinimumNArgs(0),
	RunE: func(cmd *cobra.Command, args []string) error {
		clientInstance := getClient(cmd)
		boardID := getBoardID(cmd)
		if err := validate.ValidateBoardID(boardID); err != nil {
			return gerr.NewUserError(err.Error(), "Use --board flag, GOBAN_BOARD env var, or 'board_id' in config.yaml")
		}

		userName := getUser(cmd)
		if userName == "" {
			return gerr.NewUserError("username is required", "Use --user flag, GOBAN_USER env var, or 'user' in config.yaml")
		}

		ticketIDs := batch.CollectTicketIDs(args, batchFilePath)
		if len(ticketIDs) == 0 {
			return fmt.Errorf("no ticket IDs provided; use arguments, --file, or pipe via stdin")
		}

		result := batch.ProcessCancel(cmd.Context(), clientInstance, boardID, userName, ticketIDs, batchForce)
		batch.PrintBatchResult(result, "CANCELLED")

		if len(result.Failed) > 0 {
			return fmt.Errorf("%d failure(s)", len(result.Failed))
		}
		return nil
	},
}

func init() {
	batchDoneCmd.Flags().StringVar(&batchFilePath, "file", "", "File containing ticket IDs (one per line)")
	batchDoneCmd.Flags().BoolVar(&batchForce, "force", false, "Override tickets claimed by other users or from terminal states")

	batchCancelCmd.Flags().StringVar(&batchFilePath, "file", "", "File containing ticket IDs (one per line)")
	batchCancelCmd.Flags().BoolVar(&batchForce, "force", false, "Override tickets claimed by other users or from terminal states")

	rootCmd.AddCommand(batchDoneCmd)
	rootCmd.AddCommand(batchCancelCmd)
}
