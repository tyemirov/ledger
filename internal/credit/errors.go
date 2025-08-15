package credit

import "errors"

// Domain-level error values returned by the credit service.
var (
	ErrInsufficientFunds       = errors.New("insufficient funds")
	ErrUnknownReservation      = errors.New("unknown reservation")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
)
