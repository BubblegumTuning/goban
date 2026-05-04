package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password <user_id>",
	Short: "Reset a user's password",
	Long: `Reset the password for an existing user.

The new password will be bcrypt-hashed and stored securely in the database.
This command requires direct database access (no authentication needed).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := parseInt64(args[0])
		if userID < 0 {
			return fmt.Errorf("invalid user ID: %s", args[0])
		}

		newPassword, _ := cmd.Flags().GetString("password")
		if newPassword == "" {
			return fmt.Errorf("--password is required")
		}

		c := getClient(cmd)

		// First get the username for display
		users, err := c.ListUsers()
		var username string
		for _, u := range users {
			if u.ID == userID {
				username = u.Name
				break
			}
		}
		if username == "" {
			fmt.Fprintf(os.Stderr, "Error: user not found (ID: %d)\n", userID)
			os.Exit(1)
		}

		err = c.UpdateUserPassword(userID, newPassword)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resetting password: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Password reset successfully for user '%s' (ID: %d)\n", username, userID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resetPasswordCmd)
	resetPasswordCmd.Flags().StringP("password", "p", "", "New password for the user")
}
