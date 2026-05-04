package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/output"
)

var updateCmd = &cobra.Command{
	Use:   "update <user_id>",
	Short: "Update a user's role",
	Long: `Update the role of an existing user.

Role options:
  HUMAN_ADMIN - Full control over all users and tickets
  OVERSEER_AI - Can manage any ticket, read-only on users  
  NORMAL_AI   - Can only claim/manage own tickets`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := parseInt64(args[0])
		if userID < 0 {
			return fmt.Errorf("invalid user ID: %s", args[0])
		}

		role, _ := cmd.Flags().GetString("role")

		c := getClient(cmd)
		err := c.UpdateUserRole(userID, role)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error updating user: %v\n", err)
			os.Exit(1)
		}

		formatter := output.New("line", false)
		formatter.PrintSuccess("User role updated successfully (ID: %d)", userID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringP("role", "r", "NORMAL_AI", "New role: HUMAN_ADMIN, OVERSEER_AI, or NORMAL_AI")
}
