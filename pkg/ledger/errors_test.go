package ledger

import (
	"errors"
	"testing"
)

const (
	operationName    = "ledger"
	subjectName      = "entry"
	codeName         = "invalid"
	baseErrorMessage = "base error"
)

func TestOperationErrorFormatting(test *testing.T) {
	test.Parallel()
	baseError := errors.New(baseErrorMessage)
	wrappedError := WrapError(operationName, subjectName, codeName, baseError)
	if wrappedError == nil {
		test.Fatalf("expected wrapped error")
	}
	expected := operationName + "." + subjectName + "." + codeName + ": " + baseErrorMessage
	if wrappedError.Error() != expected {
		test.Fatalf("expected %q, got %q", expected, wrappedError.Error())
	}
}

func TestWrapErrorNil(test *testing.T) {
	test.Parallel()
	if WrapError(operationName, subjectName, codeName, nil) != nil {
		test.Fatalf("expected nil wrapped error")
	}
}
