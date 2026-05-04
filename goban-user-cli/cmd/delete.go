package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/output"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <user_id>",
	Short: "Delete a user account",
	Long: `Delete a user and all associated tokens.

Note: If the user has assigned tickets, those will be unassigned upon deletion.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := parseInt64(args[0])
		if userID < 0 {
			return fmt.Errorf("invalid user ID: %s", args[0])
		}

		c := getClient(cmd)
		err := c.DeleteUser(userID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting user: %v\n", err)
			os.Exit(1)
		}

		formatter := output.New("line", false)
		formatter.PrintSuccess("User deleted successfully (ID: %d)", userID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
