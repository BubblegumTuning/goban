package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link <parent_id> <child_id>",
	Short: "Link two tickets as parent-child dependency",
	Long:  "Create a parent-child dependency between tickets. The first ticket becomes the parent, the second becomes the child.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		parentID := args[0]
		childID := args[1]

		apiClient := getClient(cmd)

		ctx := cmd.Context()
		err := apiClient.LinkTickets(ctx, parentID, childID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}

		fmt.Printf("\x1b[32m✓ Linked: %s -> %s\x1b[0m\n", parentID, childID)
		return nil
	},
}

var unlinkCmd = &cobra.Command{
	Use:   "unlink <parent_id> <child_id>",
	Short: "Remove a parent-child dependency between tickets",
	Long:  "Remove an existing parent-child dependency between tickets.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		parentID := args[0]
		childID := args[1]

		apiClient := getClient(cmd)

		ctx := cmd.Context()
		err := apiClient.UnlinkTickets(ctx, parentID, childID)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}

		fmt.Printf("\x1b[32m✓ Unlinked: %s -> %s\x1b[0m\n", parentID, childID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(linkCmd)
	rootCmd.AddCommand(unlinkCmd)
}
