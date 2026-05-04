package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// listBoardsCmd lists all available boards
var listBoardsCmd = &cobra.Command{
	Use:   "list-boards",
	Short: "List all available Kanban boards",
	Long:  `Lists all Kanban boards available on the Goban server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client := getClient(cmd)
		fmtter := getFormatter(cmd)

		ctx := cmd.Context()

		boards, classErr := client.SafeListBoards(ctx)
		if classErr != nil {
			return classErr
		}

		fmt.Println("\nAvailable Boards:")
		fmt.Println("-----------------")
		fmtter.PrintBoards(boards)
		fmt.Println()

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listBoardsCmd)
}
