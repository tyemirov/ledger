package credit

import "errors"

// Domain-level error values returned by the credit service.
var (
	ErrInsufficientFunds       = errors.New("insufficient funds")
	ErrUnknownReservation      = errors.New("unknown reservation")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrInvalidUserID           = errors.New("invalid user id")
	ErrInvalidReservationID    = errors.New("invalid reservation id")
	ErrInvalidIdempotencyKey   = errors.New("invalid idempotency key")
	ErrInvalidAmountCents      = errors.New("invalid amount cents")
	ErrInvalidMetadataJSON     = errors.New("invalid metadata json")
	ErrInvalidServiceConfig    = errors.New("invalid service config")
)
