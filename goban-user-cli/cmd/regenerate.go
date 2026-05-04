package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/output"
)

var regenerateCmd = &cobra.Command{
	Use:   "regenerate <user_id>",
	Short: "Regenerate a user's API token",
	Long: `Regenerate the API token for an existing user.

The old token will be invalidated and a new one created. The full token
will be displayed ONCE at regeneration time. Store it securely as it
cannot be retrieved later.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		userID := parseInt64(args[0])
		if userID < 0 {
			return fmt.Errorf("invalid user ID: %s", args[0])
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

		resp, err := c.RegenerateToken(userID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error regenerating token: %v\n", err)
			os.Exit(1)
		}

		formatter := output.New("line", false)
		return formatter.PrintRegenerateTokenResponse(resp, username)
	},
}

func init() {
	rootCmd.AddCommand(regenerateCmd)
}
