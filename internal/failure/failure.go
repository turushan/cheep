package failure

import (
	"errors"
	"fmt"
)

// Error carries a stable machine code and process exit status.
type Error struct {
	Code     string
	ExitCode int
	Message  string
	Cause    error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// New creates an error whose code is safe for scripts to inspect.
func New(code string, exitCode int, message string) *Error {
	return &Error{Code: code, ExitCode: exitCode, Message: message}
}

// Wrap adds a stable failure identity while preserving the cause for diagnostics.
func Wrap(code string, exitCode int, message string, cause error) *Error {
	return &Error{Code: code, ExitCode: exitCode, Message: message, Cause: cause}
}

// Details returns the stable fields used by the CLI error renderer.
func Details(err error) (code string, exitCode int, message string) {
	var target *Error
	if errors.As(err, &target) {
		return target.Code, target.ExitCode, target.Error()
	}
	return "unexpected_error", 1, fmt.Sprint(err)
}
