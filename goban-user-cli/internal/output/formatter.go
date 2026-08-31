package output

import (
	"encoding/json"
	"fmt"

	"goban-user-cli/internal/client"
)

// Formatter handles output formatting.
type Formatter struct {
	format string // "line" or "json"
	color  bool
}

// New creates a new formatter with the given settings.
func New(format string, color bool) *Formatter {
	return &Formatter{format: format, color: color}
}

// PrintUsers prints a list of users.
func (f *Formatter) PrintUsers(users []client.User) error {
	if f.format == "json" {
		data, err := json.MarshalIndent(users, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Line format - table-like output
	if len(users) == 0 {
		fmt.Println("No users found.")
		return nil
	}

	fmt.Printf("%-8s %-25s %-15s %s\n", "ID", "NAME", "ROLE", "CREATED")
	fmt.Println("-" + string(make([]byte, 67)))
	for _, u := range users {
		fmt.Printf("%-8d %-25s %-15s %s\n",
			u.ID, truncate(u.Name, 24), u.Role,
			u.CreatedAt.Format("2006-01-02"))
	}

	return nil
}

// PrintCreateUserResponse prints the response from creating a user.
func (f *Formatter) PrintCreateUserResponse(resp *client.CreateUserResponse) error {
	if f.format == "json" {
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Line format
	fmt.Println("✓ User created successfully!")
	fmt.Printf("\nUser:\n")
	fmt.Printf("  ID:        %d\n", resp.User.ID)
	fmt.Printf("  Name:      %s\n", resp.User.Name)
	fmt.Printf("  Role:      %s\n", resp.User.Role)

	fmt.Println("\n⚠️  IMPORTANT: Store this token securely. It will not be shown again!")
	fmt.Printf("\nToken:\n")
	fmt.Printf("  Token Name: %s\n", resp.Token.TokenName)
	fmt.Printf("  Token:      %s\n", resp.Token.Token)

	return nil
}

// PrintRegenerateTokenResponse prints the response from regenerating a token.
func (f *Formatter) PrintRegenerateTokenResponse(resp *client.RegenerateTokenResponse, username string) error {
	if f.format == "json" {
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✓ Token regenerated successfully for user '%s'!\n", username)
	fmt.Println("\n⚠️  IMPORTANT: Store this token securely. It will not be shown again!")
	fmt.Printf("\nToken:\n")
	fmt.Printf("  Token Name: %s\n", resp.Token.TokenName)
	fmt.Printf("  Token:      %s\n", resp.Token.Token)

	return nil
}

// PrintSuccess prints a success message.
func (f *Formatter) PrintSuccess(msg string, args ...interface{}) {
	if f.format == "json" {
		data := map[string]string{"status": "success", "message": fmt.Sprintf(msg, args...)}
		dataBytes, _ := json.MarshalIndent(data, "", "  ")
		fmt.Println(string(dataBytes))
	} else {
		fmt.Printf("✓ "+msg+"\n", args...)
	}
}

// truncate shortens a string to the given length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
