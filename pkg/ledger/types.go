package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	defaultMetadataJSON        = "{}"
	errorEmptyValue            = "empty value"
	errorMustBeValidJSON       = "must be valid json"
	errorAmountZeroOrGreater   = "must be zero or greater"
	errorAmountGreaterThanZero = "must be greater than zero"
	errorAmountNonZero         = "must be non-zero"
	errorUnknownValue          = "unknown value"
)

// AmountCents is a non-negative currency value in cents.
type AmountCents int64

// SignedAmountCents is a currency value in cents that may be negative.
type SignedAmountCents int64

// PositiveAmountCents is a strictly positive currency value in cents.
type PositiveAmountCents int64

// EntryAmountCents is a non-zero ledger delta in cents.
type EntryAmountCents int64

// UserID identifies an account owner.
type UserID struct {
	value string
}

// TenantID identifies a tenant boundary.
type TenantID struct {
	value string
}

// LedgerID identifies a user ledger namespace.
type LedgerID struct {
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

// AccountID identifies a ledger account record.
type AccountID struct {
	value string
}

// EntryID identifies a ledger entry record.
type EntryID struct {
	value string
}

// ReservationStatus defines reservation lifecycle.
type ReservationStatus string

const (
	ReservationStatusActive   ReservationStatus = "active"
	ReservationStatusCaptured ReservationStatus = "captured"
	ReservationStatusReleased ReservationStatus = "released"
)

// EntryType enumerates ledger entry kinds.
type EntryType string

const (
	EntryGrant       EntryType = "grant"
	EntryHold        EntryType = "hold"
	EntryReverseHold EntryType = "reverse_hold"
	EntrySpend       EntryType = "spend"
)

// Reservation represents a stored reservation record.
type Reservation struct {
	accountID     AccountID
	reservationID ReservationID
	amountCents   PositiveAmountCents
	status        ReservationStatus
}

// EntryInput represents a new ledger entry to persist.
type EntryInput struct {
	accountID        AccountID
	entryType        EntryType
	amountCents      EntryAmountCents
	reservationID    *ReservationID
	idempotencyKey   IdempotencyKey
	expiresAtUnixUTC int64
	metadata         MetadataJSON
	createdUnixUTC   int64
}

// Entry represents a persisted ledger entry.
type Entry struct {
	entryID          EntryID
	accountID        AccountID
	entryType        EntryType
	amountCents      EntryAmountCents
	reservationID    *ReservationID
	idempotencyKey   IdempotencyKey
	expiresAtUnixUTC int64
	metadata         MetadataJSON
	createdUnixUTC   int64
}

// Balance is the current total and available funds for an account.
type Balance struct {
	TotalCents     SignedAmountCents
	AvailableCents SignedAmountCents
}

// NewUserID validates and normalizes a user id.
func NewUserID(raw string) (UserID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidUserID)
	if err != nil {
		return UserID{}, err
	}
	return UserID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id UserID) String() string {
	return id.value
}

// NewTenantID validates and normalizes a tenant id.
func NewTenantID(raw string) (TenantID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidTenantID)
	if err != nil {
		return TenantID{}, err
	}
	return TenantID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id TenantID) String() string {
	return id.value
}

// NewLedgerID validates and normalizes a ledger id.
func NewLedgerID(raw string) (LedgerID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidLedgerID)
	if err != nil {
		return LedgerID{}, err
	}
	return LedgerID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id LedgerID) String() string {
	return id.value
}

// NewReservationID validates and normalizes a reservation id.
func NewReservationID(raw string) (ReservationID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidReservationID)
	if err != nil {
		return ReservationID{}, err
	}
	return ReservationID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id ReservationID) String() string {
	return id.value
}

// NewIdempotencyKey validates and normalizes an idempotency key.
func NewIdempotencyKey(raw string) (IdempotencyKey, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidIdempotencyKey)
	if err != nil {
		return IdempotencyKey{}, err
	}
	return IdempotencyKey{value: normalized}, nil
}

// String returns the normalized key.
func (key IdempotencyKey) String() string {
	return key.value
}

// NewMetadataJSON validates metadata string (defaulting to "{}" for empty inputs).
func NewMetadataJSON(raw string) (MetadataJSON, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		normalized = defaultMetadataJSON
	}
	if !json.Valid([]byte(normalized)) {
		return MetadataJSON{}, fmt.Errorf("%w: %s", ErrInvalidMetadataJSON, errorMustBeValidJSON)
	}
	return MetadataJSON{value: normalized}, nil
}

// String returns the normalized JSON blob.
func (metadata MetadataJSON) String() string {
	return metadata.value
}

// NewAccountID validates a generated account id.
func NewAccountID(raw string) (AccountID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidAccountID)
	if err != nil {
		return AccountID{}, err
	}
	return AccountID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id AccountID) String() string {
	return id.value
}

// NewEntryID validates a generated entry id.
func NewEntryID(raw string) (EntryID, error) {
	normalized, err := normalizeIdentifier(raw, ErrInvalidEntryID)
	if err != nil {
		return EntryID{}, err
	}
	return EntryID{value: normalized}, nil
}

// String returns the normalized identifier.
func (id EntryID) String() string {
	return id.value
}

// NewAmountCents validates a non-negative amount.
func NewAmountCents(raw int64) (AmountCents, error) {
	if raw < 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidAmountCents, errorAmountZeroOrGreater)
	}
	return AmountCents(raw), nil
}

// Int64 returns the amount as a primitive value.
func (amount AmountCents) Int64() int64 {
	return int64(amount)
}

// NewSignedAmountCents returns a signed amount value.
func NewSignedAmountCents(raw int64) (SignedAmountCents, error) {
	return SignedAmountCents(raw), nil
}

// Int64 returns the amount as a primitive value.
func (amount SignedAmountCents) Int64() int64 {
	return int64(amount)
}

// NewPositiveAmountCents validates an amount and ensures it is strictly positive.
func NewPositiveAmountCents(raw int64) (PositiveAmountCents, error) {
	if raw <= 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidAmountCents, errorAmountGreaterThanZero)
	}
	return PositiveAmountCents(raw), nil
}

// Int64 returns the amount as a primitive value.
func (amount PositiveAmountCents) Int64() int64 {
	return int64(amount)
}

// ToAmountCents converts a positive amount into a non-negative amount.
func (amount PositiveAmountCents) ToAmountCents() AmountCents {
	return AmountCents(amount)
}

// ToEntryAmountCents converts a positive amount into an entry delta.
func (amount PositiveAmountCents) ToEntryAmountCents() EntryAmountCents {
	return EntryAmountCents(amount)
}

// NewEntryAmountCents validates a non-zero entry delta.
func NewEntryAmountCents(raw int64) (EntryAmountCents, error) {
	if raw == 0 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidEntryAmountCents, errorAmountNonZero)
	}
	return EntryAmountCents(raw), nil
}

// Int64 returns the amount as a primitive value.
func (amount EntryAmountCents) Int64() int64 {
	return int64(amount)
}

// Negated flips the sign of an entry delta.
func (amount EntryAmountCents) Negated() EntryAmountCents {
	return EntryAmountCents(-amount)
}

// ParseReservationStatus validates reservation status values.
func ParseReservationStatus(raw string) (ReservationStatus, error) {
	status := ReservationStatus(strings.TrimSpace(raw))
	if !status.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidReservationStatus, errorUnknownValue)
	}
	return status, nil
}

// String returns the status as a primitive value.
func (status ReservationStatus) String() string {
	return string(status)
}

// IsValid reports whether the status is recognized.
func (status ReservationStatus) IsValid() bool {
	switch status {
	case ReservationStatusActive, ReservationStatusCaptured, ReservationStatusReleased:
		return true
	default:
		return false
	}
}

// ParseEntryType validates ledger entry type values.
func ParseEntryType(raw string) (EntryType, error) {
	entryType := EntryType(strings.TrimSpace(raw))
	if !entryType.IsValid() {
		return "", fmt.Errorf("%w: %s", ErrInvalidEntryType, errorUnknownValue)
	}
	return entryType, nil
}

// String returns the entry type as a primitive value.
func (entryType EntryType) String() string {
	return string(entryType)
}

// IsValid reports whether the entry type is recognized.
func (entryType EntryType) IsValid() bool {
	switch entryType {
	case EntryGrant, EntryHold, EntryReverseHold, EntrySpend:
		return true
	default:
		return false
	}
}

// NewReservation constructs a reservation record.
func NewReservation(accountID AccountID, reservationID ReservationID, amountCents PositiveAmountCents, status ReservationStatus) (Reservation, error) {
	if err := validateIdentifierValue(accountID.value, ErrInvalidAccountID); err != nil {
		return Reservation{}, err
	}
	if err := validateIdentifierValue(reservationID.value, ErrInvalidReservationID); err != nil {
		return Reservation{}, err
	}
	if err := validatePositiveAmount(amountCents); err != nil {
		return Reservation{}, err
	}
	if !status.IsValid() {
		return Reservation{}, fmt.Errorf("%w: %s", ErrInvalidReservationStatus, errorUnknownValue)
	}
	return Reservation{
		accountID:     accountID,
		reservationID: reservationID,
		amountCents:   amountCents,
		status:        status,
	}, nil
}

// AccountID returns the associated account.
func (reservation Reservation) AccountID() AccountID {
	return reservation.accountID
}

// ReservationID returns the reservation identifier.
func (reservation Reservation) ReservationID() ReservationID {
	return reservation.reservationID
}

// AmountCents returns the reserved amount.
func (reservation Reservation) AmountCents() PositiveAmountCents {
	return reservation.amountCents
}

// Status returns the reservation status.
func (reservation Reservation) Status() ReservationStatus {
	return reservation.status
}

// NewEntryInput constructs a new ledger entry payload.
func NewEntryInput(accountID AccountID, entryType EntryType, amountCents EntryAmountCents, reservationID *ReservationID, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON, createdUnixUTC int64) (EntryInput, error) {
	if err := validateIdentifierValue(accountID.value, ErrInvalidAccountID); err != nil {
		return EntryInput{}, err
	}
	if err := validateIdentifierValue(idempotencyKey.value, ErrInvalidIdempotencyKey); err != nil {
		return EntryInput{}, err
	}
	if err := validateIdentifierValue(metadata.value, ErrInvalidMetadataJSON); err != nil {
		return EntryInput{}, err
	}
	if reservationID != nil {
		if err := validateIdentifierValue(reservationID.value, ErrInvalidReservationID); err != nil {
			return EntryInput{}, err
		}
	}
	if !entryType.IsValid() {
		return EntryInput{}, fmt.Errorf("%w: %s", ErrInvalidEntryType, errorUnknownValue)
	}
	if err := validateEntryAmount(amountCents); err != nil {
		return EntryInput{}, err
	}
	return EntryInput{
		accountID:        accountID,
		entryType:        entryType,
		amountCents:      amountCents,
		reservationID:    reservationID,
		idempotencyKey:   idempotencyKey,
		expiresAtUnixUTC: expiresAtUnixUTC,
		metadata:         metadata,
		createdUnixUTC:   createdUnixUTC,
	}, nil
}

// AccountID returns the associated account.
func (entry EntryInput) AccountID() AccountID {
	return entry.accountID
}

// Type returns the entry type.
func (entry EntryInput) Type() EntryType {
	return entry.entryType
}

// AmountCents returns the entry amount delta.
func (entry EntryInput) AmountCents() EntryAmountCents {
	return entry.amountCents
}

// ReservationID returns the reservation identifier, if present.
func (entry EntryInput) ReservationID() (ReservationID, bool) {
	if entry.reservationID == nil {
		return ReservationID{}, false
	}
	return *entry.reservationID, true
}

// IdempotencyKey returns the idempotency key.
func (entry EntryInput) IdempotencyKey() IdempotencyKey {
	return entry.idempotencyKey
}

// ExpiresAtUnixUTC returns the expiration timestamp.
func (entry EntryInput) ExpiresAtUnixUTC() int64 {
	return entry.expiresAtUnixUTC
}

// MetadataJSON returns the entry metadata.
func (entry EntryInput) MetadataJSON() MetadataJSON {
	return entry.metadata
}

// CreatedUnixUTC returns the creation timestamp.
func (entry EntryInput) CreatedUnixUTC() int64 {
	return entry.createdUnixUTC
}

// NewEntry constructs a persisted ledger entry.
func NewEntry(entryID EntryID, accountID AccountID, entryType EntryType, amountCents EntryAmountCents, reservationID *ReservationID, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON, createdUnixUTC int64) (Entry, error) {
	if err := validateIdentifierValue(entryID.value, ErrInvalidEntryID); err != nil {
		return Entry{}, err
	}
	entryInput, err := NewEntryInput(accountID, entryType, amountCents, reservationID, idempotencyKey, expiresAtUnixUTC, metadata, createdUnixUTC)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		entryID:          entryID,
		accountID:        entryInput.accountID,
		entryType:        entryInput.entryType,
		amountCents:      entryInput.amountCents,
		reservationID:    entryInput.reservationID,
		idempotencyKey:   entryInput.idempotencyKey,
		expiresAtUnixUTC: entryInput.expiresAtUnixUTC,
		metadata:         entryInput.metadata,
		createdUnixUTC:   entryInput.createdUnixUTC,
	}, nil
}

// EntryID returns the entry identifier.
func (entry Entry) EntryID() EntryID {
	return entry.entryID
}

// AccountID returns the associated account.
func (entry Entry) AccountID() AccountID {
	return entry.accountID
}

// Type returns the entry type.
func (entry Entry) Type() EntryType {
	return entry.entryType
}

// AmountCents returns the entry amount delta.
func (entry Entry) AmountCents() EntryAmountCents {
	return entry.amountCents
}

// ReservationID returns the reservation identifier, if present.
func (entry Entry) ReservationID() (ReservationID, bool) {
	if entry.reservationID == nil {
		return ReservationID{}, false
	}
	return *entry.reservationID, true
}

// IdempotencyKey returns the idempotency key.
func (entry Entry) IdempotencyKey() IdempotencyKey {
	return entry.idempotencyKey
}

// ExpiresAtUnixUTC returns the expiration timestamp.
func (entry Entry) ExpiresAtUnixUTC() int64 {
	return entry.expiresAtUnixUTC
}

// MetadataJSON returns the entry metadata.
func (entry Entry) MetadataJSON() MetadataJSON {
	return entry.metadata
}

// CreatedUnixUTC returns the creation timestamp.
func (entry Entry) CreatedUnixUTC() int64 {
	return entry.createdUnixUTC
}

// Store is the persistence contract used by Service.
type Store interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error
	GetOrCreateAccountID(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) (AccountID, error)
	InsertEntry(ctx context.Context, entry EntryInput) error
	SumTotal(ctx context.Context, accountID AccountID, atUnixUTC int64) (SignedAmountCents, error)
	SumActiveHolds(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error)
	CreateReservation(ctx context.Context, reservation Reservation) error
	GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error)
	UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from, to ReservationStatus) error
	ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int) ([]Entry, error)
}

func normalizeIdentifier(raw string, invalidError error) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s", invalidError, errorEmptyValue)
	}
	return trimmed, nil
}

func validateIdentifierValue(value string, invalidError error) error {
	if value == "" {
		return fmt.Errorf("%w: %s", invalidError, errorEmptyValue)
	}
	return nil
}

func validatePositiveAmount(value PositiveAmountCents) error {
	if value <= 0 {
		return fmt.Errorf("%w: %s", ErrInvalidAmountCents, errorAmountGreaterThanZero)
	}
	return nil
}

func validateEntryAmount(value EntryAmountCents) error {
	if value == 0 {
		return fmt.Errorf("%w: %s", ErrInvalidEntryAmountCents, errorAmountNonZero)
	}
	return nil
}
