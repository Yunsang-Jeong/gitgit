package apperr

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestErrorMessagePrecedenceAndUnwrap(t *testing.T) {
	cause := errors.New("underlying failure")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "message", err: &Error{Code: "code", Message: "public message", Err: cause}, want: "public message"},
		{name: "cause", err: &Error{Code: "code", Err: cause}, want: "underlying failure"},
		{name: "code", err: &Error{Code: "fallback_code"}, want: "fallback_code"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Fatalf("Error() = %q, want %q", got, test.want)
			}
		})
	}
	if !errors.Is(&Error{Err: cause}, cause) {
		t.Fatal("Error.Unwrap did not expose the cause")
	}
}

func TestConstructorsExitCodeAndDetails(t *testing.T) {
	details := map[string]any{"field": "value", "count": 3}
	created := New("invalid_fixture", "fixture is invalid", ExitUsage, details)
	if created.Code != "invalid_fixture" || created.Message != "fixture is invalid" || created.ExitCode != ExitUsage {
		t.Fatalf("unexpected New result: %#v", created)
	}
	if !reflect.DeepEqual(created.Details, details) {
		t.Fatalf("details = %#v, want %#v", created.Details, details)
	}

	cause := errors.New("disk unavailable")
	wrapped := Wrap("write_failed", "could not write", ExitPrecondition, cause, details)
	if !errors.Is(wrapped, cause) {
		t.Fatal("Wrap did not retain the cause")
	}
	if got := ExitCode(fmt.Errorf("outer: %w", wrapped)); got != ExitPrecondition {
		t.Fatalf("ExitCode(wrapped) = %d, want %d", got, ExitPrecondition)
	}
	if got := ExitCode(&Error{Code: "zero"}); got != ExitFailure {
		t.Fatalf("zero ExitCode = %d, want %d", got, ExitFailure)
	}
	if got := ExitCode(errors.New("plain")); got != ExitFailure {
		t.Fatalf("plain ExitCode = %d, want %d", got, ExitFailure)
	}

	code, message, gotDetails := Details(fmt.Errorf("outer: %w", wrapped))
	if code != "write_failed" || message != "could not write" || !reflect.DeepEqual(gotDetails, details) {
		t.Fatalf("Details(app error) = %q, %q, %#v", code, message, gotDetails)
	}
	code, message, gotDetails = Details(cause)
	if code != "internal_error" || message != cause.Error() || gotDetails != nil {
		t.Fatalf("Details(plain error) = %q, %q, %#v", code, message, gotDetails)
	}
}
