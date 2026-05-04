package client

import (
	"math"
	"net/http"
	"strings"
	"time"

	"goban-cli/internal/config"
)

// RetryPolicy wraps config settings into usable retry parameters.
type RetryPolicy struct {
	maxAttempts       int
	initialDelay      time.Duration
	backoffMultiplier float64
}

// NewRetryPolicy creates a RetryPolicy from config or defaults.
func NewRetryPolicy(cfg config.RetryConfig) RetryPolicy {
	return RetryPolicy{
		maxAttempts:       cfg.MaxAttempts,
		initialDelay:      time.Duration(cfg.InitialDelay) * time.Second,
		backoffMultiplier: float64(cfg.BackoffMultiplier),
	}
}

// IsRetryable returns true for transient failures that benefit from retrying.
func IsRetryable(statusCode int) bool {
	switch {
	case statusCode >= 500 && statusCode < 600: // Server errors
		return true
	case statusCode == http.StatusTooManyRequests: // Rate limiting (429)
		return true
	default:
		return false
	}
}

// IsNetworkError returns true if the error appears to be a transient network issue.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{"refused", "timeout", "dial", "no such host", "connection reset", "i/o timeout", "network is unreachable"} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// delayForAttempt calculates the exponential backoff delay for a given attempt.
func (p RetryPolicy) delayForAttempt(attempt int) time.Duration {
	multiplier := math.Pow(p.backoffMultiplier, float64(attempt))
	delay := p.initialDelay * time.Duration(multiplier)
	// Cap at 30 seconds maximum
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}
