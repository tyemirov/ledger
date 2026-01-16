package ledger

import (
	"errors"
	"testing"
)

const (
	accountIDValue     = "acct-1"
	reservationIDValue = "res-1"
	entryIDValue       = "entry-1"
	idempotencyValue   = "idem-1"
	metadataValue      = "{\"source\":\"test\"}"
)

func TestReservationIDString(test *testing.T) {
	test.Parallel()
	reservationID := mustReservationID(test, reservationIDValue)
	if reservationID.String() != reservationIDValue {
		test.Fatalf("expected %q, got %q", reservationIDValue, reservationID.String())
	}
}

func TestEntryReservationIDAbsent(test *testing.T) {
	test.Parallel()
	entryID := mustEntryID(test, entryIDValue)
	accountID := mustAccountID(test, accountIDValue)
	idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
	metadata := mustMetadata(test, metadataValue)
	amount := mustEntryAmount(test, 10)

	entry, err := NewEntry(entryID, accountID, EntryGrant, amount, nil, idempotencyKey, 0, metadata, 100)
	if err != nil {
		test.Fatalf("entry: %v", err)
	}
	_, hasReservation := entry.ReservationID()
	if hasReservation {
		test.Fatalf("expected no reservation id")
	}
}

func TestNewReservationValidation(test *testing.T) {
	test.Parallel()
	validAccountID := mustAccountID(test, accountIDValue)
	validReservationID := mustReservationID(test, reservationIDValue)
	validAmount := mustPositiveAmount(test, 50)
	validStatus := ReservationStatusActive

	testCases := []struct {
		name          string
		accountID     AccountID
		reservationID ReservationID
		amount        PositiveAmountCents
		status        ReservationStatus
		wantErr       error
	}{
		{
			name:          "invalid account id",
			accountID:     AccountID{},
			reservationID: validReservationID,
			amount:        validAmount,
			status:        validStatus,
			wantErr:       ErrInvalidAccountID,
		},
		{
			name:          "invalid reservation id",
			accountID:     validAccountID,
			reservationID: ReservationID{},
			amount:        validAmount,
			status:        validStatus,
			wantErr:       ErrInvalidReservationID,
		},
		{
			name:          "invalid amount",
			accountID:     validAccountID,
			reservationID: validReservationID,
			amount:        PositiveAmountCents(0),
			status:        validStatus,
			wantErr:       ErrInvalidAmountCents,
		},
		{
			name:          "invalid status",
			accountID:     validAccountID,
			reservationID: validReservationID,
			amount:        validAmount,
			status:        ReservationStatus("invalid"),
			wantErr:       ErrInvalidReservationStatus,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := NewReservation(testCase.accountID, testCase.reservationID, testCase.amount, testCase.status)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestNewEntryInputValidation(test *testing.T) {
	test.Parallel()
	validAccountID := mustAccountID(test, accountIDValue)
	validReservationID := mustReservationID(test, reservationIDValue)
	validIdempotencyKey := mustIdempotencyKey(test, idempotencyValue)
	validMetadata := mustMetadata(test, metadataValue)
	validAmount := mustEntryAmount(test, 25)
	createdAt := int64(100)

	testCases := []struct {
		name          string
		accountID     AccountID
		entryType     EntryType
		amount        EntryAmountCents
		reservationID *ReservationID
		idempotency   IdempotencyKey
		metadata      MetadataJSON
		wantErr       error
	}{
		{
			name:          "invalid account id",
			accountID:     AccountID{},
			entryType:     EntryGrant,
			amount:        validAmount,
			reservationID: &validReservationID,
			idempotency:   validIdempotencyKey,
			metadata:      validMetadata,
			wantErr:       ErrInvalidAccountID,
		},
		{
			name:          "invalid idempotency key",
			accountID:     validAccountID,
			entryType:     EntryGrant,
			amount:        validAmount,
			reservationID: &validReservationID,
			idempotency:   IdempotencyKey{},
			metadata:      validMetadata,
			wantErr:       ErrInvalidIdempotencyKey,
		},
		{
			name:          "invalid metadata",
			accountID:     validAccountID,
			entryType:     EntryGrant,
			amount:        validAmount,
			reservationID: &validReservationID,
			idempotency:   validIdempotencyKey,
			metadata:      MetadataJSON{},
			wantErr:       ErrInvalidMetadataJSON,
		},
		{
			name:          "invalid reservation id",
			accountID:     validAccountID,
			entryType:     EntryGrant,
			amount:        validAmount,
			reservationID: &ReservationID{},
			idempotency:   validIdempotencyKey,
			metadata:      validMetadata,
			wantErr:       ErrInvalidReservationID,
		},
		{
			name:          "invalid entry type",
			accountID:     validAccountID,
			entryType:     EntryType("invalid"),
			amount:        validAmount,
			reservationID: &validReservationID,
			idempotency:   validIdempotencyKey,
			metadata:      validMetadata,
			wantErr:       ErrInvalidEntryType,
		},
		{
			name:          "invalid amount",
			accountID:     validAccountID,
			entryType:     EntryGrant,
			amount:        EntryAmountCents(0),
			reservationID: &validReservationID,
			idempotency:   validIdempotencyKey,
			metadata:      validMetadata,
			wantErr:       ErrInvalidEntryAmountCents,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := NewEntryInput(
				testCase.accountID,
				testCase.entryType,
				testCase.amount,
				testCase.reservationID,
				testCase.idempotency,
				0,
				testCase.metadata,
				createdAt,
			)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestNewEntryRejectsInvalidEntryID(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test, accountIDValue)
	idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
	metadata := mustMetadata(test, metadataValue)
	amount := mustEntryAmount(test, 15)

	_, err := NewEntry(EntryID{}, accountID, EntryGrant, amount, nil, idempotencyKey, 0, metadata, 100)
	if !errors.Is(err, ErrInvalidEntryID) {
		test.Fatalf(errorMismatchMessage, ErrInvalidEntryID, err)
	}
}
