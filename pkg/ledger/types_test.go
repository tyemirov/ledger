package ledger

import (
	"errors"
	"testing"
)

func TestNewUserID(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		input     string
		wantErr   error
		wantValue string
	}{
		{name: "valid", input: " user-123 ", wantValue: "user-123"},
		{name: "empty", input: "   ", wantErr: ErrInvalidUserID},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			result, err := NewUserID(testCase.input)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					test.Fatalf("expected error %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if result.String() != testCase.wantValue {
				test.Fatalf("expected %q, got %q", testCase.wantValue, result.String())
			}
		})
	}
}

func TestNewReservationID(test *testing.T) {
	test.Parallel()
	_, err := NewReservationID("")
	if !errors.Is(err, ErrInvalidReservationID) {
		test.Fatalf("expected ErrInvalidReservationID, got %v", err)
	}
}

func TestNewIdempotencyKey(test *testing.T) {
	test.Parallel()
	_, err := NewIdempotencyKey("   ")
	if !errors.Is(err, ErrInvalidIdempotencyKey) {
		test.Fatalf("expected ErrInvalidIdempotencyKey, got %v", err)
	}
}

func TestNewAccountID(test *testing.T) {
	test.Parallel()
	_, err := NewAccountID("")
	if !errors.Is(err, ErrInvalidAccountID) {
		test.Fatalf("expected ErrInvalidAccountID, got %v", err)
	}
	value, err := NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if value.String() != "acct-1" {
		test.Fatalf("expected acct-1, got %s", value.String())
	}
}

func TestNewEntryID(test *testing.T) {
	test.Parallel()
	_, err := NewEntryID("")
	if !errors.Is(err, ErrInvalidEntryID) {
		test.Fatalf("expected ErrInvalidEntryID, got %v", err)
	}
}

func TestNewAmountCents(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name    string
		input   int64
		wantErr error
		wantVal AmountCents
	}{
		{name: "negative", input: -1, wantErr: ErrInvalidAmountCents},
		{name: "zero", input: 0, wantVal: 0},
		{name: "positive", input: 100, wantVal: 100},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			value, err := NewAmountCents(testCase.input)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					test.Fatalf("expected error %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.wantVal {
				test.Fatalf("expected %d, got %d", testCase.wantVal, value)
			}
		})
	}
}

func TestNewPositiveAmountCents(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name    string
		input   int64
		wantErr error
		wantVal PositiveAmountCents
	}{
		{name: "negative", input: -1, wantErr: ErrInvalidAmountCents},
		{name: "zero", input: 0, wantErr: ErrInvalidAmountCents},
		{name: "positive", input: 50, wantVal: 50},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			value, err := NewPositiveAmountCents(testCase.input)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					test.Fatalf("expected error %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.wantVal {
				test.Fatalf("expected %d, got %d", testCase.wantVal, value)
			}
		})
	}
}

func TestNewEntryAmountCents(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name    string
		input   int64
		wantErr error
		wantVal EntryAmountCents
	}{
		{name: "zero", input: 0, wantErr: ErrInvalidEntryAmountCents},
		{name: "positive", input: 25, wantVal: 25},
		{name: "negative", input: -25, wantVal: -25},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			value, err := NewEntryAmountCents(testCase.input)
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					test.Fatalf("expected error %v, got %v", testCase.wantErr, err)
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.wantVal {
				test.Fatalf("expected %d, got %d", testCase.wantVal, value)
			}
		})
	}
}

func TestNewMetadataJSON(test *testing.T) {
	test.Parallel()
	metadata, err := NewMetadataJSON("")
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if metadata.String() != "{}" {
		test.Fatalf("expected default metadata to be '{}', got %q", metadata.String())
	}
	_, err = NewMetadataJSON("not-json")
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

func TestParseReservationStatus(test *testing.T) {
	test.Parallel()
	validStatuses := []ReservationStatus{ReservationStatusActive, ReservationStatusCaptured, ReservationStatusReleased}
	for _, status := range validStatuses {
		_, err := ParseReservationStatus(status.String())
		if err != nil {
			test.Fatalf("unexpected error for %s: %v", status, err)
		}
	}
	_, err := ParseReservationStatus("invalid")
	if !errors.Is(err, ErrInvalidReservationStatus) {
		test.Fatalf("expected ErrInvalidReservationStatus, got %v", err)
	}
}

func TestParseEntryType(test *testing.T) {
	test.Parallel()
	validTypes := []EntryType{EntryGrant, EntryHold, EntryReverseHold, EntrySpend}
	for _, entryType := range validTypes {
		_, err := ParseEntryType(entryType.String())
		if err != nil {
			test.Fatalf("unexpected error for %s: %v", entryType, err)
		}
	}
	_, err := ParseEntryType("invalid")
	if !errors.Is(err, ErrInvalidEntryType) {
		test.Fatalf("expected ErrInvalidEntryType, got %v", err)
	}
}

func TestOperationErrorAccessors(test *testing.T) {
	test.Parallel()
	baseError := errors.New("boom")
	wrappedError := WrapError("store", "entry", "invalid", baseError)
	if wrappedError == nil {
		test.Fatalf("expected wrapped error")
	}
	var operationError OperationError
	if !errors.As(wrappedError, &operationError) {
		test.Fatalf("expected OperationError")
	}
	if operationError.Operation() != "store" {
		test.Fatalf("expected operation store, got %s", operationError.Operation())
	}
	if operationError.Subject() != "entry" {
		test.Fatalf("expected subject entry, got %s", operationError.Subject())
	}
	if operationError.Code() != "invalid" {
		test.Fatalf("expected code invalid, got %s", operationError.Code())
	}
	if operationError.Unwrap() != baseError {
		test.Fatalf("expected unwrap to return base error")
	}
	if !errors.Is(wrappedError, baseError) {
		test.Fatalf("expected errors.Is to match base error")
	}
}

func TestEntryInputAndEntryGetters(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test, "acct-1")
	reservationID := mustReservationID(test, "res-1")
	entryID := mustEntryID(test, "entry-1")
	idempotencyKey := mustIdempotencyKey(test, "idem-1")
	metadata := mustMetadata(test, `{"source":"test"}`)
	amount := mustEntryAmount(test, 25)
	expiresAt := int64(50)
	createdAt := int64(100)

	entryInput, err := NewEntryInput(accountID, EntryGrant, amount, &reservationID, idempotencyKey, expiresAt, metadata, createdAt)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if entryInput.AccountID() != accountID {
		test.Fatalf("expected account id %s", accountID.String())
	}
	if entryInput.Type() != EntryGrant {
		test.Fatalf("expected entry type %s", EntryGrant)
	}
	if entryInput.AmountCents() != amount {
		test.Fatalf("expected amount %d", amount)
	}
	reservationValue, hasReservation := entryInput.ReservationID()
	if !hasReservation || reservationValue != reservationID {
		test.Fatalf("expected reservation id %s", reservationID.String())
	}
	if entryInput.IdempotencyKey() != idempotencyKey {
		test.Fatalf("expected idempotency key %s", idempotencyKey.String())
	}
	if entryInput.ExpiresAtUnixUTC() != expiresAt {
		test.Fatalf("expected expires at %d", expiresAt)
	}
	if entryInput.MetadataJSON().String() != metadata.String() {
		test.Fatalf("expected metadata %s", metadata.String())
	}
	if entryInput.CreatedUnixUTC() != createdAt {
		test.Fatalf("expected created at %d", createdAt)
	}

	entry, err := NewEntry(entryID, accountID, EntryGrant, amount, &reservationID, idempotencyKey, expiresAt, metadata, createdAt)
	if err != nil {
		test.Fatalf("entry: %v", err)
	}
	if entry.EntryID() != entryID {
		test.Fatalf("expected entry id %s", entryID.String())
	}
	if entry.AccountID() != accountID {
		test.Fatalf("expected account id %s", accountID.String())
	}
	if entry.Type() != EntryGrant {
		test.Fatalf("expected entry type %s", EntryGrant)
	}
	if entry.AmountCents() != amount {
		test.Fatalf("expected amount %d", amount)
	}
	reservationValue, hasReservation = entry.ReservationID()
	if !hasReservation || reservationValue != reservationID {
		test.Fatalf("expected reservation id %s", reservationID.String())
	}
	if entry.IdempotencyKey() != idempotencyKey {
		test.Fatalf("expected idempotency key %s", idempotencyKey.String())
	}
	if entry.ExpiresAtUnixUTC() != expiresAt {
		test.Fatalf("expected expires at %d", expiresAt)
	}
	if entry.MetadataJSON().String() != metadata.String() {
		test.Fatalf("expected metadata %s", metadata.String())
	}
	if entry.CreatedUnixUTC() != createdAt {
		test.Fatalf("expected created at %d", createdAt)
	}

	entryInputNoReservation, err := NewEntryInput(accountID, EntryGrant, amount, nil, idempotencyKey, 0, metadata, createdAt)
	if err != nil {
		test.Fatalf("entry input without reservation: %v", err)
	}
	_, hasReservation = entryInputNoReservation.ReservationID()
	if hasReservation {
		test.Fatalf("expected no reservation id")
	}
}
