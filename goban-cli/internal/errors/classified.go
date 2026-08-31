package gerr

import "fmt"

// ClassifiedError attaches a category and structured message to an error,
// enabling differentiated exit codes and consistent user-facing formatting.
type ClassifiedError struct {
	Category Category // Error category (determines exit code + icon)
	Message  string   // Always shown to the user
	Details  string   // Shown only when --verbose flag is set
}

// Error implements the error interface.
func (e *ClassifiedError) Error() string {
	return e.Message
}

// UserMessage formats the error for display with icon prefix and optional details.
func (e *ClassifiedError) UserMessage(verbose bool) string {
	msg := fmt.Sprintf("[%s] %s", e.Category.Icon(), e.Message)
	if verbose && e.Details != "" {
		msg += "\n  Details: " + e.Details
	}
	return msg
}

// --- Factory constructors ---

func NewUserError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatUserError, Message: message, Details: details}
}

func NewAuthError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatAuth, Message: message, Details: details}
}

func NewNotFoundError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatNotFound, Message: message, Details: details}
}

func NewServerError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatServer, Message: message, Details: details}
}

func NewNetworkError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatNetwork, Message: message, Details: details}
}

func NewVerifyFailedError(message, details string) *ClassifiedError {
	return &ClassifiedError{Category: CatVerifyFailed, Message: message, Details: details}
}
