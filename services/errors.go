// Package services contains business logic for Goban operations.
package services

import "errors"

// ErrArchived is returned when an operation targets an archived ticket.
var ErrArchived = errors.New("ticket is archived")
