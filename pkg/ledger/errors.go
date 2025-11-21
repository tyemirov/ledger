package ledger

import "errors"

// Domain-level error values returned by the ledger service.
var (
	ErrInsufficientFunds       = errors.New("insufficient funds")
	ErrUnknownReservation      = errors.New("unknown reservation")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrReservationExists       = errors.New("reservation already exists")
	ErrReservationClosed       = errors.New("reservation closed")
	ErrInvalidUserID           = errors.New("invalid user id")
	ErrInvalidReservationID    = errors.New("invalid reservation id")
	ErrInvalidIdempotencyKey   = errors.New("invalid idempotency key")
	ErrInvalidAmountCents      = errors.New("invalid amount cents")
	ErrInvalidMetadataJSON     = errors.New("invalid metadata json")
	ErrInvalidServiceConfig    = errors.New("invalid service config")
)
