package ledger

import (
	"errors"
	"fmt"
)

// Domain-level error values returned by the ledger service.
var (
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrUnknownReservation       = errors.New("unknown reservation")
	ErrDuplicateIdempotencyKey  = errors.New("duplicate idempotency key")
	ErrReservationExists        = errors.New("reservation already exists")
	ErrReservationClosed        = errors.New("reservation closed")
	ErrInvalidAccountID         = errors.New("invalid account id")
	ErrInvalidEntryID           = errors.New("invalid entry id")
	ErrInvalidUserID            = errors.New("invalid user id")
	ErrInvalidTenantID          = errors.New("invalid tenant id")
	ErrInvalidLedgerID          = errors.New("invalid ledger id")
	ErrInvalidReservationID     = errors.New("invalid reservation id")
	ErrInvalidIdempotencyKey    = errors.New("invalid idempotency key")
	ErrInvalidAmountCents       = errors.New("invalid amount cents")
	ErrInvalidEntryAmountCents  = errors.New("invalid entry amount cents")
	ErrInvalidEntryType         = errors.New("invalid entry type")
	ErrInvalidReservationStatus = errors.New("invalid reservation status")
	ErrInvalidMetadataJSON      = errors.New("invalid metadata json")
	ErrInvalidServiceConfig     = errors.New("invalid service config")
	ErrInvalidBalance           = errors.New("invalid balance")
)

// OperationError wraps a failure with a stable operation code.
type OperationError struct {
	operation string
	subject   string
	code      string
	err       error
}

// Error returns the formatted error message.
func (operationError OperationError) Error() string {
	return fmt.Sprintf("%s.%s.%s: %v", operationError.operation, operationError.subject, operationError.code, operationError.err)
}

// Unwrap returns the underlying error.
func (operationError OperationError) Unwrap() error {
	return operationError.err
}

// Operation returns the operation segment.
func (operationError OperationError) Operation() string {
	return operationError.operation
}

// Subject returns the subject segment.
func (operationError OperationError) Subject() string {
	return operationError.subject
}

// Code returns the stable error code segment.
func (operationError OperationError) Code() string {
	return operationError.code
}

// WrapError wraps an error with operation, subject, and code metadata.
func WrapError(operation string, subject string, code string, err error) error {
	if err == nil {
		return nil
	}
	return OperationError{
		operation: operation,
		subject:   subject,
		code:      code,
		err:       err,
	}
}
