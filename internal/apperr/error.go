package apperr

import (
	"errors"
	"fmt"
)

const (
	ExitFailure      = 1
	ExitUsage        = 2
	ExitPrecondition = 3
	ExitConfirmation = 4
	ExitPaused       = 5
)

type Error struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
	ExitCode int            `json:"-"`
	Err      error          `json:"-"`
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func New(code, message string, exitCode int, details map[string]any) *Error {
	return &Error{Code: code, Message: message, ExitCode: exitCode, Details: details}
}

func Wrap(code, message string, exitCode int, err error, details map[string]any) *Error {
	return &Error{Code: code, Message: message, ExitCode: exitCode, Err: err, Details: details}
}

func ExitCode(err error) int {
	var appErr *Error
	if errors.As(err, &appErr) && appErr.ExitCode != 0 {
		return appErr.ExitCode
	}
	return ExitFailure
}

func Details(err error) (string, string, map[string]any) {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code, appErr.Error(), appErr.Details
	}
	return "internal_error", fmt.Sprint(err), nil
}
