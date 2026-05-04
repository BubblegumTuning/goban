package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"goban-cli/internal/config"
	"goban-cli/internal/types"
)

// Formatter handles output formatting based on configuration
type Formatter struct {
	format   string
	colorize bool
}

// New creates a new formatter with the given config
func New(cfg *config.Config) *Formatter {
	return &Formatter{
		format:   cfg.Output.Format,
		colorize: cfg.Output.Colorize,
	}
}

// Format returns the current output format.
func (f *Formatter) Format() string {
	return f.format
}

// Colors for terminal output
type color int

const (
	colorReset   color = iota
	colorRed           // Used for errors and status like "Done"
	colorGreen         // Used for success messages and "In Progress" status
	colorYellow        // Used for warnings and "To Do" status
	colorBlue          // Used for IDs, usernames
	colorMagenta       // Used for titles
)

var colorCodes = map[color]string{
	colorReset:   "\033[0m",
	colorRed:     "\033[31m",
	colorGreen:   "\033[32m",
	colorYellow:  "\033[33m",
	colorBlue:    "\033[34m",
	colorMagenta: "\033[35m",
}

// color returns the ANSI code for a color, or empty string if colorize is disabled
func (f *Formatter) color(c color) string {
	if !f.colorize {
		return ""
	}
	return colorCodes[c]
}

// statusColor returns the appropriate color for a ticket status
func (f *Formatter) statusColor(status types.TicketStatus) string {
	switch status {
	case types.StatusDone:
		return f.color(colorRed)
	case types.StatusInProgress:
		return f.color(colorGreen)
	default:
		return f.color(colorYellow)
	}
}

// formatLine formats a single line with optional colors
func (f *Formatter) formatLine(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// PrintBoard lists a board in line format or JSON
func (f *Formatter) PrintBoard(board types.Board) {
	if f.format == "json" {
		f.printJSON(board)
		return
	}

	// Line format
	id := fmt.Sprintf("%s[%s]%s", f.color(colorBlue), board.ID, f.color(colorReset))
	name := fmt.Sprintf("%s%s%s", f.color(colorMagenta), board.Name, f.color(colorReset))
	f.formatLine("Board: %s | ID: %s", name, id)
}

// PrintBoards lists multiple boards
func (f *Formatter) PrintBoards(boards []types.Board) {
	if f.format == "json" {
		f.printJSON(boards)
		return
	}

	if len(boards) == 0 {
		f.formatLine("No boards found")
		return
	}

	for _, board := range boards {
		id := fmt.Sprintf("%s[%s]%s", f.color(colorBlue), board.ID, f.color(colorReset))
		name := fmt.Sprintf("%s%s%s", f.color(colorMagenta), board.Name, f.color(colorReset))
		f.formatLine(" %s | %s", id, name)
	}
}

// PrintTicket displays a single ticket
func (f *Formatter) PrintTicket(ticket types.Ticket) {
	if f.format == "json" {
		f.printJSON(ticket)
		return
	}
	if f.format == "compact" {
		f.PrintTicketCompact(ticket)
		return
	}

	statusColor := f.statusColor(ticket.GetStatus())
	status := fmt.Sprintf("%s[%s]%s", statusColor, ticket.Column, f.color(colorReset))
	id := fmt.Sprintf("%s%s%s", f.color(colorBlue), ticket.ID, f.color(colorReset))
	title := fmt.Sprintf("%s%s%s", f.color(colorMagenta), ticket.Title, f.color(colorReset))

	f.formatLine("Ticket: %s", title)
	f.formatLine(" ID: %s", id)
	f.formatLine(" Column: %s", status)
	if ticket.Description != "" {
		f.formatLine(" Description:")
		for _, line := range splitLines(ticket.Description) {
			f.formatLine(" %s", line)
		}
	}
	if ticket.Assignee != "" {
		user := fmt.Sprintf("%s%s%s", f.color(colorBlue), ticket.Assignee, f.color(colorReset))
		f.formatLine(" Assigned to: %s", user)
	}
}

// PrintTickets lists multiple tickets in a compact format
func (f *Formatter) PrintTickets(tickets []types.Ticket) {
	if f.format == "json" {
		f.printJSON(tickets)
		return
	}
	if f.format == "compact" {
		f.PrintTicketsCompact(tickets)
		return
	}

	if len(tickets) == 0 {
		f.formatLine("No tickets found")
		return
	}

	for _, ticket := range tickets {
		statusColor := f.statusColor(ticket.GetStatus())
		status := fmt.Sprintf("%s[%s]%s", statusColor, ticket.Column, f.color(colorReset))
		id := fmt.Sprintf("%s%s%s", f.color(colorBlue), ticket.ID, f.color(colorReset))
		title := fmt.Sprintf("%s%s%s", f.color(colorMagenta), ticket.Title, f.color(colorReset))

		var claimed string
		if ticket.Assignee != "" {
			user := fmt.Sprintf("@%s%s%s", f.color(colorBlue), ticket.Assignee, f.color(colorReset))
			claimed = " | " + user
		}

		f.formatLine(" %s | %s%s: %s", status, id, claimed, title)
	}
}

// PrintComment displays a single comment
func (f *Formatter) PrintComment(comment types.Comment) {
	if f.format == "json" {
		f.printJSON(comment)
		return
	}
	if f.format == "compact" {
		f.PrintCommentCompact(comment)
		return
	}

	id := fmt.Sprintf("%s%s%s", f.color(colorBlue), comment.ID, f.color(colorReset))
	author := fmt.Sprintf("%s%s%s", f.color(colorBlue), comment.Who, f.color(colorReset))

	f.formatLine("Comment %s by %s:", id, author)
	for _, line := range splitLines(comment.Text) {
		f.formatLine(" %s", line)
	}
}

// PrintComments lists multiple comments
func (f *Formatter) PrintComments(comments []types.Comment) {
	if f.format == "json" {
		f.printJSON(comments)
		return
	}
	if f.format == "compact" {
		f.PrintCommentsCompact(comments)
		return
	}

	if len(comments) == 0 {
		f.formatLine("No comments found")
		return
	}

	for _, comment := range comments {
		id := fmt.Sprintf("%s%s%s", f.color(colorBlue), comment.ID, f.color(colorReset))
		author := fmt.Sprintf("%s%s%s", f.color(colorBlue), comment.Who, f.color(colorReset))
		f.formatLine("\n %s by %s:", id, author)

		for _, line := range splitLines(comment.Text) {
			f.formatLine(" %s", line)
		}
	}
}

// PrintSuccess prints a success message
func (f *Formatter) PrintSuccess(message string) {
	if f.colorize {
		fmt.Fprintf(os.Stdout, "%s✓ %s%s\n", f.color(colorGreen), message, f.color(colorReset))
		return
	}
	f.formatLine("✓ " + message)
}

// PrintError prints an error message
func (f *Formatter) PrintError(message string) {
	if f.colorize {
		fmt.Fprintf(os.Stderr, "%s✗ %s%s\n", f.color(colorRed), message, f.color(colorReset))
		return
	}
	f.formatLine("✗ " + message)
}

// PrintWarning prints a warning message
func (f *Formatter) PrintWarning(message string) {
	if f.colorize {
		fmt.Fprintf(os.Stdout, "%s⚠ %s%s\n", f.color(colorYellow), message, f.color(colorReset))
		return
	}
	f.formatLine("⚠ " + message)
}

// stripColumnSuffix removes the "-0" suffix from column IDs (e.g., "todo-0" -> "todo")
func stripColumnSuffix(col string) string {
	if strings.HasSuffix(col, "-0") {
		return col[:len(col)-2]
	}
	return col
}

// truncate truncates text to maxLen characters, appending "..." if truncated.
func truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// PrintTicketCompact outputs a ticket in pipe-delimited single-line format.
// Format: TICKET_ID|COLUMN_BASE|ASSIGNEE_OR_DASH|TITLE
func (f *Formatter) PrintTicketCompact(ticket types.Ticket) {
	col := stripColumnSuffix(ticket.Column)
	assignee := ticket.Assignee
	if assignee == "" {
		assignee = "-"
	}
	fmt.Fprintf(os.Stdout, "%s|%s|%s|%s\n", ticket.ID, col, assignee, ticket.Title)
}

// PrintTicketsCompact outputs multiple tickets in compact format.
func (f *Formatter) PrintTicketsCompact(tickets []types.Ticket) {
	if len(tickets) == 0 {
		fmt.Fprintf(os.Stdout, "No tickets found\n")
		return
	}
	for _, ticket := range tickets {
		f.PrintTicketCompact(ticket)
	}
}

// PrintViewCompact outputs a single ticket view in compact format.
// Line 1: TICKET_ID|COLUMN_BASE|ASSIGNEE_OR_DASH|TITLE
// Line 2: desc:[description truncated to 200 chars] (if present)
// Line 3: comments:N subtasks:X/Y created_at:TIMESTAMP
func (f *Formatter) PrintViewCompact(ticket types.Ticket) {
	f.PrintTicketCompact(ticket)

	if ticket.Description != "" {
		desc := truncate(strings.TrimSpace(ticket.Description), 200)
		fmt.Fprintf(os.Stdout, "desc:[%s]\n", desc)
	}

	completed := 0
	for _, st := range ticket.Subtasks {
		if st.Completed {
			completed++
		}
	}
	meta := fmt.Sprintf("comments:%d subtasks:%d/%d created_at:%s",
		len(ticket.Comments),
		completed,
		len(ticket.Subtasks),
		ticket.CreatedAt)
	fmt.Fprintf(os.Stdout, "%s\n", meta)
}

// PrintCommentCompact outputs a comment in compact format.
func (f *Formatter) PrintCommentCompact(comment types.Comment) {
	text := truncate(comment.Text, 100)
	fmt.Fprintf(os.Stdout, "comment:%s|%s|%s\n", comment.ID, comment.Who, text)
}

// PrintCommentsCompact outputs multiple comments in compact format.
func (f *Formatter) PrintCommentsCompact(comments []types.Comment) {
	for _, comment := range comments {
		f.PrintCommentCompact(comment)
	}
}

// printJSON outputs data as JSON
func (f *Formatter) printJSON(data interface{}) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
	}
}

// splitLines splits a string into lines for pretty printing
func splitLines(text string) []string {
	var lines []string
	currentLine := ""

	for _, ch := range text {
		if ch == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
		} else {
			currentLine += string(ch)
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}
