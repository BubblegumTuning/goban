package session

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Session holds the state of the currently active ticket.
type Session struct {
	TicketID string `json:"ticket_id"`
	BoardID  string `json:"board_id"`
	User     string `json:"user"`
}

const sessionFile = "session.json"

// sessionDir returns ~/.goban-cli/ (creates it if missing).
func sessionDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".goban", "goban-cli")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// sessionPath returns the full path to the session file.
func sessionPath() (string, error) {
	dir, err := sessionDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionFile), nil
}

// Read loads the session from disk. Returns nil if no session exists.
func Read() (*Session, error) {
	path, err := sessionPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No session — not an error
		}
		return nil, err
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}

	if s.TicketID == "" {
		return nil, nil // Empty session treated as no session
	}

	return &s, nil
}

// Write saves the session to disk.
func Write(ticketID, boardID, user string) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}

	s := Session{
		TicketID: ticketID,
		BoardID:  boardID,
		User:     user,
	}

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// Clear removes the session file.
func Clear() error {
	path, err := sessionPath()
	if err != nil {
		return err
	}
	return os.Remove(path)
}
