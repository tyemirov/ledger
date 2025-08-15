package credit

import "context"

// AmountCents is an integer currency in cents.
type AmountCents int64

// EntryType enumerates ledger entry kinds.
type EntryType string

const (
	EntryGrant       EntryType = "grant"
	EntryHold        EntryType = "hold"
	EntryReverseHold EntryType = "reverse_hold"
	EntrySpend       EntryType = "spend"
)

// A single immutable line in the ledger.
type Entry struct {
	EntryID          string
	AccountID        string
	Type             EntryType
	AmountCents      AmountCents
	ReservationID    string
	IdempotencyKey   string
	ExpiresAtUnixUTC int64
	MetadataJSON     string
	CreatedUnixUTC   int64
}

// Balance view for an account.
type Balance struct {
	TotalCents     AmountCents
	AvailableCents AmountCents
}

// Store is the persistence contract used by Service.
// (pgstore implements this already.)
type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error
	GetOrCreateAccountID(ctx context.Context, userID string) (string, error)
	InsertEntry(ctx context.Context, entry Entry) error
	SumTotal(ctx context.Context, accountID string, atUnixUTC int64) (AmountCents, error)
	SumActiveHolds(ctx context.Context, accountID string, atUnixUTC int64) (AmountCents, error)
	ReservationExists(ctx context.Context, accountID string, reservationID string) (bool, error)
	ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]Entry, error)
}
