package domain

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrLeaseLost    = errors.New("worker no longer owns the active attempt")
	ErrInvalidState = errors.New("operation not allowed in the current state")
)
