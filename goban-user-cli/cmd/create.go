package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/output"
)

var createCmd = &cobra.Command{
	Use:   "create --username=<name> --role=<HUMAN_ADMIN|OVERSEER_AI|NORMAL_AI>",
	Short: "Create a new user with specified role",
	Long: `Create a new user account with the specified role and generate an API token.

The full token will be displayed ONCE at creation time. Store it securely as
it cannot be retrieved later (only regenerated).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		username, _ := cmd.Flags().GetString("username")
		role, _ := cmd.Flags().GetString("role")

		if username == "" {
			return fmt.Errorf("--username is required")
		}

		validRoles := map[string]bool{
			"HUMAN_ADMIN": true,
			"OVERSEER_AI": true,
			"NORMAL_AI":   true,
		}
		if !validRoles[role] {
			return fmt.Errorf("--role must be one of: HUMAN_ADMIN, OVERSEER_AI, NORMAL_AI")
		}

		c := getClient(cmd)
		resp, err := c.CreateUser(username, role)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating user: %v\n", err)
			os.Exit(1)
		}

		formatter := output.New("line", false)
		return formatter.PrintCreateUserResponse(resp)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().String("username", "", "Username for the new user")
	createCmd.Flags().String("role", "NORMAL_AI", "Role: HUMAN_ADMIN, OVERSEER_AI, or NORMAL_AI (default: NORMAL_AI)")
}
