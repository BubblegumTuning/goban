package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"goban-user-cli/internal/output"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	Long:  `Display all users with their IDs, names, roles, and creation dates. Does not expose tokens.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c := getClient(cmd)
		users, err := c.ListUsers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing users: %v\n", err)
			os.Exit(1)
		}

		formatter := output.New("line", false)
		return formatter.PrintUsers(users)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
