package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

var regenerateTokenCmd = &cobra.Command{
	Use:   "regenerate-token",
	Short: "Regenerate an API/JWT token on the remote production server",
	Long: `Connects via SSH to the remote Goban server (kanban01) and executes
goban-user-cli regenerate for the specified user ID.

This is required for write operations (delete, claim, move, etc.) because
API tokens in config.yaml are read-only. The remote command uses the
production DB to issue a fresh token.

After running, copy the printed token into ~/.goban/goban-cli/config.yaml
under api.api_token (or use the --update-config flag once implemented).

Example:
  goban-cli regenerate-token --user-id 1
  goban-cli regenerate-token --user-id 2   # for a different human admin`,
	RunE: func(cmd *cobra.Command, args []string) error {
		userID, _ := cmd.Flags().GetInt("user-id")
		host := "kanban01" // production alias; override with --host if needed

		// Build the exact remote command (DB_TYPE etc. required for goban-user-cli)
		remoteCmd := fmt.Sprintf(
			"DB_TYPE=postgres DB_HOST=localhost DB_PORT=5432 DB_USER=goban DB_NAME=goban /opt/goban/bin/goban-user-cli regenerate %d",
			userID,
		)

		fullSSH := fmt.Sprintf("ssh %s \"%s\"", host, remoteCmd)

		fmt.Fprintf(cmd.OutOrStdout(), "\nRegenerating token for user ID %d on %s...\n", userID, host)
		fmt.Fprintf(cmd.OutOrStdout(), "Running: %s\n\n", fullSSH)

		// Execute via shell so the user can see the exact command and output
		sshCmd := exec.Command("sh", "-c", fullSSH)
		sshCmd.Stdout = cmd.OutOrStdout()
		sshCmd.Stderr = cmd.ErrOrStderr()

		err := sshCmd.Run()
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "\nSSH command failed: %v\n", err)
			fmt.Fprintf(cmd.ErrOrStderr(), "You can run the command manually:\n  %s\n", fullSSH)
			return err
		}

		fmt.Fprintln(cmd.OutOrStdout(), "\n✓ Token regeneration completed on remote.")
		fmt.Fprintln(cmd.OutOrStdout(), "Copy the new token value shown above into your config:")
		fmt.Fprintln(cmd.OutOrStdout(), "  ~/.goban/goban-cli/config.yaml → api.api_token")
		fmt.Fprintln(cmd.OutOrStdout(), "\nThen run 'goban-cli delete <id> ...' or other write commands.")

		return nil
	},
}

func init() {
	regenerateTokenCmd.Flags().Int("user-id", 1, "User ID to regenerate token for (1 = primary admin / nanami)")
	regenerateTokenCmd.Flags().String("host", "kanban01", "SSH host alias or IP for the production server")
	rootCmd.AddCommand(regenerateTokenCmd)
}
