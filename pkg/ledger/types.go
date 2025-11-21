package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// AmountCents is an integer currency in cents.
type AmountCents int64

// UserID identifies an account owner.
type UserID struct {
	value string
}

// ReservationID identifies a reservation.
type ReservationID struct {
	value string
}

// IdempotencyKey scopes duplicate detection.
type IdempotencyKey struct {
	value string
}

// MetadataJSON stores arbitrary request metadata.
type MetadataJSON struct {
	value string
}

// ReservationStatus defines reservation lifecycle.
type ReservationStatus string

const (
	ReservationStatusActive   ReservationStatus = "active"
	ReservationStatusCaptured ReservationStatus = "captured"
	ReservationStatusReleased ReservationStatus = "released"
)

// Reservation represents a stored reservation record.
type Reservation struct {
	AccountID     string
	ReservationID string
	AmountCents   AmountCents
	Status        ReservationStatus
}

// NewUserID validates and normalizes a user id.
func NewUserID(raw string) (UserID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return UserID{}, fmt.Errorf("%w: empty value", ErrInvalidUserID)
	}
	return UserID{value: trimmed}, nil
}

// String returns the normalized identifier.
func (id UserID) String() string {
	return id.value
}

// NewReservationID validates and normalizes a reservation id.
func NewReservationID(raw string) (ReservationID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ReservationID{}, fmt.Errorf("%w: empty value", ErrInvalidReservationID)
	}
	return ReservationID{value: trimmed}, nil
}

// String returns the normalized identifier.
func (id ReservationID) String() string {
	return id.value
}

// NewIdempotencyKey validates and normalizes an idempotency key.
func NewIdempotencyKey(raw string) (IdempotencyKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return IdempotencyKey{}, fmt.Errorf("%w: empty value", ErrInvalidIdempotencyKey)
	}
	return IdempotencyKey{value: trimmed}, nil
}

// String returns the normalized key.
func (key IdempotencyKey) String() string {
	return key.value
}

// NewMetadataJSON validates metadata string (defaulting to "{}" for empty inputs).
func NewMetadataJSON(raw string) (MetadataJSON, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		normalized = "{}"
	}
	if !json.Valid([]byte(normalized)) {
		return MetadataJSON{}, fmt.Errorf("%w: must be valid json", ErrInvalidMetadataJSON)
	}
	return MetadataJSON{value: normalized}, nil
}

// String returns the normalized JSON blob.
func (metadata MetadataJSON) String() string {
	return metadata.value
}

// NewAmountCents validates an amount and ensures it is strictly positive.
func NewAmountCents(raw int64) (AmountCents, error) {
	if raw <= 0 {
		return 0, fmt.Errorf("%w: must be greater than zero", ErrInvalidAmountCents)
	}
	return AmountCents(raw), nil
}

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
	CreateReservation(ctx context.Context, reservation Reservation) error
	GetReservation(ctx context.Context, accountID string, reservationID string) (Reservation, error)
	UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to ReservationStatus) error
	ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]Entry, error)
}
