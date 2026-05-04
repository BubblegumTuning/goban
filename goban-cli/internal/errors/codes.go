package gerr

// Exit codes for goban-cli. These allow scripts, CI pipelines, and AI agents
// to distinguish error categories programmatically from the shell perspective.
const (
	ExitSuccess      = 0 // Operation completed successfully
	ExitUserError    = 1 // User input error (invalid args, missing flags)
	ExitAuth         = 2 // Authentication failure (bad token, unauthorized)
	ExitNotFound     = 3 // Resource not found (ticket/board doesn't exist)
	ExitServerError  = 4 // Server returned 5xx error
	ExitNetwork      = 5 // Network failure (connection refused, timeout)
	ExitVerifyFailed = 6 // Post-mutation verification failed (state mismatch)
	ExitConfig       = 7 // Configuration error
)

// Category represents an error category with a corresponding OS exit code.
type Category int

const (
	CatSuccess Category = iota
	CatUserError
	CatAuth
	CatNotFound
	CatServer
	CatNetwork
	CatVerifyFailed
	CatConfig
)

// ExitCode returns the OS exit code for this category.
func (c Category) ExitCode() int {
	switch c {
	case CatSuccess:
		return ExitSuccess
	case CatUserError:
		return ExitUserError
	case CatAuth:
		return ExitAuth
	case CatNotFound:
		return ExitNotFound
	case CatServer:
		return ExitServerError
	case CatNetwork:
		return ExitNetwork
	case CatVerifyFailed:
		return ExitVerifyFailed
	case CatConfig:
		return ExitConfig
	default:
		return ExitUserError
	}
}

// Icon returns the visual icon used to prefix this error category in output.
func (c Category) Icon() string {
	switch c {
	case CatUserError:
		return "!"
	case CatAuth:
		return "*"
	case CatNotFound:
		return "?"
	case CatServer:
		return "#"
	case CatNetwork:
		return "~"
	case CatVerifyFailed:
		return "^"
	case CatConfig:
		return "%"
	default:
		return "!"
	}
}
