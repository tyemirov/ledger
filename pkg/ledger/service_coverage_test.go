package ledger

import (
	"context"
	"errors"
	"testing"
)

// errDeriveKey is a sentinel used to simulate deriveKeyFn failures.
var errDeriveKey = errors.New("derive key failed")

// failingDeriveKeyFn always returns errDeriveKey.
func failingDeriveKeyFn(_ IdempotencyKey, _ string) (IdempotencyKey, error) {
	return IdempotencyKey{}, errDeriveKey
}

// deriveKeyFailOnSuffix returns a DeriveKeyFunc that fails only for a specific suffix.
func deriveKeyFailOnSuffix(failSuffix string) DeriveKeyFunc {
	return func(baseKey IdempotencyKey, suffix string) (IdempotencyKey, error) {
		if suffix == failSuffix {
			return IdempotencyKey{}, errDeriveKey
		}
		return deriveIdempotencyKey(baseKey, suffix)
	}
}

// deriveKeyInvalidOnSuffix returns a DeriveKeyFunc that returns a zero-value
// IdempotencyKey (empty value) for a specific suffix without reporting an error,
// causing downstream NewEntryInput validation to fail.
func deriveKeyInvalidOnSuffix(invalidSuffix string) DeriveKeyFunc {
	return func(baseKey IdempotencyKey, suffix string) (IdempotencyKey, error) {
		if suffix == invalidSuffix {
			return IdempotencyKey{}, nil // empty key, no error
		}
		return deriveIdempotencyKey(baseKey, suffix)
	}
}

func mustNewServiceWithOptions(test *testing.T, store Store) *Service {
	test.Helper()
	service, err := NewService(store, func() int64 { return 100 })
	if err != nil {
		test.Fatalf("new service: %v", err)
	}
	return service
}

func mustNewServiceWithDeriveKeyFunc(test *testing.T, store Store, fn DeriveKeyFunc) *Service {
	test.Helper()
	service := mustNewServiceWithOptions(test, store)
	service.deriveKeyFn = fn
	return service
}

// ---------------------------------------------------------------------------
// service.go: ReserveEntry NewReservation error (lines 129-132)
// ---------------------------------------------------------------------------

func TestReserveEntryNewReservationError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 10)

	// Zero-value ReservationID has empty .value, causing NewReservation to fail.
	_, err := service.ReserveEntry(
		context.Background(), tenantID, userID, ledgerID,
		amount, ReservationID{}, mustIdempotencyKey(test, "idem-1"), 0, metadata,
	)
	if !errors.Is(err, ErrInvalidReservationID) {
		test.Fatalf("expected ErrInvalidReservationID, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service.go: ReserveEntry NewEntryInput error (lines 147-149)
// ---------------------------------------------------------------------------

func TestReserveEntryNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 10)
	reservationID := mustReservationID(test, "res-1")

	// MetadataJSON{} has empty .value, causing NewEntryInput validation to fail.
	_, err := service.ReserveEntry(
		context.Background(), tenantID, userID, ledgerID,
		amount, reservationID, mustIdempotencyKey(test, "idem-1"), 0, MetadataJSON{},
	)
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service.go: CaptureDebitEntry deriveIdempotencyKey errors (lines 203-205, 224-226)
// ---------------------------------------------------------------------------

func TestCaptureDebitEntryDeriveReverseKeyError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewServiceWithDeriveKeyFunc(test, store, failingDeriveKeyFn)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	metadata := mustMetadata(test, "{}")

	_, err := service.CaptureDebitEntry(
		context.Background(), tenantID, userID, ledgerID,
		reservationID, mustIdempotencyKey(test, "cap-1"), amount, metadata,
	)
	if !errors.Is(err, errDeriveKey) {
		test.Fatalf("expected errDeriveKey, got %v", err)
	}
}

func TestCaptureDebitEntryDeriveSpendKeyError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	// Only fail on the spend suffix; the reverse suffix should succeed.
	service := mustNewServiceWithDeriveKeyFunc(test, store, deriveKeyFailOnSuffix(idempotencySuffixSpend))
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	metadata := mustMetadata(test, "{}")

	_, err := service.CaptureDebitEntry(
		context.Background(), tenantID, userID, ledgerID,
		reservationID, mustIdempotencyKey(test, "cap-1"), amount, metadata,
	)
	if !errors.Is(err, errDeriveKey) {
		test.Fatalf("expected errDeriveKey, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service.go: CaptureDebitEntry NewEntryInput errors (lines 217-219, 238-240)
// ---------------------------------------------------------------------------

func TestCaptureDebitEntryNewEntryInputReverseHoldError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	// MetadataJSON{} causes NewEntryInput to fail for the reverse hold entry.
	_, err := service.CaptureDebitEntry(
		context.Background(), tenantID, userID, ledgerID,
		reservationID, mustIdempotencyKey(test, "cap-1"), amount, MetadataJSON{},
	)
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

func TestCaptureDebitEntryNewEntryInputSpendError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	// deriveKeyInvalidOnSuffix returns an empty IdempotencyKey for "spend" suffix.
	// This causes NewEntryInput to fail with ErrInvalidIdempotencyKey for the spend entry,
	// while the reverse hold entry succeeds because its key is derived normally.
	service := mustNewServiceWithDeriveKeyFunc(test, store, deriveKeyInvalidOnSuffix(idempotencySuffixSpend))
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	metadata := mustMetadata(test, "{}")

	_, err := service.CaptureDebitEntry(
		context.Background(), tenantID, userID, ledgerID,
		reservationID, mustIdempotencyKey(test, "cap-1"), amount, metadata,
	)
	if !errors.Is(err, ErrInvalidIdempotencyKey) {
		test.Fatalf("expected ErrInvalidIdempotencyKey, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service.go: ReleaseEntry NewEntryInput error (lines 299-301)
// ---------------------------------------------------------------------------

func TestReleaseEntryNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	// MetadataJSON{} causes NewEntryInput to fail for the reverse hold entry.
	_, err := service.ReleaseEntry(
		context.Background(), tenantID, userID, ledgerID,
		reservationID, mustIdempotencyKey(test, "rel-1"), MetadataJSON{},
	)
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: Batch GetOrCreateAccountID error in tx (lines 89-91)
// ---------------------------------------------------------------------------

func TestBatchGetOrCreateAccountIDError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	store.getAccountError = errStoreFailure
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
	}

	// When GetOrCreateAccountID fails, Batch should return a top-level error (non-rollback).
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if !errors.Is(err, errStoreFailure) {
		test.Fatalf("expected errStoreFailure, got %v (results=%v)", err, results)
	}
	if results != nil {
		test.Fatalf("expected nil results, got %v", results)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: non-rollback error from batch tx (lines 118-120)
// This is the same path as above: GetOrCreateAccountID returns a non-rollback error.
// ---------------------------------------------------------------------------

func TestBatchNonRollbackErrorPropagated(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	sentinel := errors.New("tx infrastructure error")
	store.getAccountError = sentinel
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
	}

	_, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, true)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchSpend NewEntryInput error
// ---------------------------------------------------------------------------

func TestBatchSpendNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "spend-1",
			Spend: &BatchSpendOperation{
				Amount:         mustPositiveAmount(test, 10),
				IdempotencyKey: mustIdempotencyKey(test, "spend-1"),
				Metadata:       MetadataJSON{}, // empty metadata triggers NewEntryInput error
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchReserve CreateReservation error
// ---------------------------------------------------------------------------

func TestBatchReserveCreateReservationError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	createError := errors.New("create reservation failed")
	store.createReservationError = createError
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchReserveOperation(test, "reserve-1", 10, "res-1", "reserve-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, createError) {
		test.Fatalf("expected create reservation error, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchReserve NewReservation error (line 240)
// ---------------------------------------------------------------------------

func TestBatchReserveNewReservationError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "reserve-1",
			Reserve: &BatchReserveOperation{
				Amount:           mustPositiveAmount(test, 10),
				ReservationID:    ReservationID{}, // empty ReservationID triggers NewReservation error
				IdempotencyKey:   mustIdempotencyKey(test, "reserve-1"),
				ExpiresAtUnixUTC: 0,
				Metadata:         mustMetadata(test, "{}"),
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidReservationID) {
		test.Fatalf("expected ErrInvalidReservationID, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchReserve NewEntryInput error
// ---------------------------------------------------------------------------

func TestBatchReserveNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "reserve-1",
			Reserve: &BatchReserveOperation{
				Amount:           mustPositiveAmount(test, 10),
				ReservationID:    mustReservationID(test, "res-1"),
				IdempotencyKey:   mustIdempotencyKey(test, "reserve-1"),
				ExpiresAtUnixUTC: 0,
				Metadata:         MetadataJSON{}, // empty metadata triggers NewEntryInput error
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchCapture deriveIdempotencyKey errors
// ---------------------------------------------------------------------------

func TestBatchCaptureDeriveReverseKeyError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewServiceWithDeriveKeyFunc(test, store, failingDeriveKeyFn)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchCaptureOperation(test, "capture-1", amount.Int64(), reservationID.String(), "capture-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, errDeriveKey) {
		test.Fatalf("expected errDeriveKey, got %v", results[0].Error)
	}
}

func TestBatchCaptureDeriveSpendKeyError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewServiceWithDeriveKeyFunc(test, store, deriveKeyFailOnSuffix(idempotencySuffixSpend))
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchCaptureOperation(test, "capture-1", amount.Int64(), reservationID.String(), "capture-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, errDeriveKey) {
		test.Fatalf("expected errDeriveKey, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchCapture NewEntryInput errors
// ---------------------------------------------------------------------------

func TestBatchCaptureNewEntryInputReverseHoldError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "capture-1",
			Capture: &BatchCaptureOperation{
				ReservationID:  reservationID,
				IdempotencyKey: mustIdempotencyKey(test, "capture-1"),
				Amount:         amount,
				Metadata:       MetadataJSON{}, // empty metadata triggers NewEntryInput error
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", results[0].Error)
	}
}

func TestBatchCaptureNewEntryInputSpendError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	// deriveKeyInvalidOnSuffix returns an empty IdempotencyKey for "spend" suffix,
	// causing NewEntryInput to fail for the spend entry while the reverse hold succeeds.
	service := mustNewServiceWithDeriveKeyFunc(test, store, deriveKeyInvalidOnSuffix(idempotencySuffixSpend))
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	metadata := mustMetadata(test, "{}")

	operations := []BatchOperation{
		{
			OperationID: "capture-1",
			Capture: &BatchCaptureOperation{
				ReservationID:  reservationID,
				IdempotencyKey: mustIdempotencyKey(test, "capture-1"),
				Amount:         amount,
				Metadata:       metadata,
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidIdempotencyKey) {
		test.Fatalf("expected ErrInvalidIdempotencyKey, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchRelease NewEntryInput error
// ---------------------------------------------------------------------------

func TestBatchReleaseNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	amount := mustPositiveAmount(test, 50)
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "release-1",
			Release: &BatchReleaseOperation{
				ReservationID:  reservationID,
				IdempotencyKey: mustIdempotencyKey(test, "release-1"),
				Metadata:       MetadataJSON{}, // empty metadata triggers NewEntryInput error
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchRelease InsertEntry error path
// This is already tested in TestBatchCaptureAndReleaseReturnStoreErrors
// via "release insert entry error". Verified covered.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchRefund GetEntry/GetEntryByIdempotencyKey errors
// ---------------------------------------------------------------------------

func TestBatchRefundGetEntryError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	sentinel := errors.New("get entry boom")
	// Use a wrapper store that fails GetEntry with a non-ErrUnknownEntry error.
	wrappedStore := &getEntryFailStore{
		stubStore: store,
		err:       sentinel,
	}
	service := mustNewServiceWithOptions(test, wrappedStore)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	entryID := mustEntryID(test, "entry-1")
	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, entryID, "refund-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, sentinel) {
		test.Fatalf("expected sentinel error, got %v", results[0].Error)
	}
}

func TestBatchRefundGetEntryByIdempotencyKeyError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	sentinel := errors.New("idempotency lookup boom")
	// Use a wrapper store that fails GetEntryByIdempotencyKey with a non-ErrUnknownEntry error.
	wrappedStore := &getEntryByIdemKeyFailStore{
		stubStore: store,
		err:       sentinel,
	}
	service := mustNewServiceWithOptions(test, wrappedStore)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	originalKey := mustIdempotencyKey(test, "spend-1")
	operations := []BatchOperation{
		newBatchRefundByOriginalIdempotencyKeyOperation(test, "refund-1", 50, originalKey, "refund-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, sentinel) {
		test.Fatalf("expected sentinel error, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchRefund NewEntryInput error
// ---------------------------------------------------------------------------

func TestBatchRefundNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	// First set up a spend entry.
	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	originalEntryID := spendEntry.EntryID()
	operations := []BatchOperation{
		{
			OperationID: "refund-1",
			Refund: &BatchRefundOperation{
				OriginalEntryID: &originalEntryID,
				Amount:          mustPositiveAmount(test, 50),
				IdempotencyKey:  mustIdempotencyKey(test, "refund-1"),
				Metadata:        MetadataJSON{}, // empty metadata triggers NewEntryInput error
			},
		},
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_batch.go: applyBatchRefund SumRefunds error
// ---------------------------------------------------------------------------

func TestBatchRefundSumRefundsError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	sentinel := errors.New("sum refunds boom")
	sumRefundsFailStore := &sumRefundsFailingStore{
		stubStore: store,
		err:       sentinel,
	}
	service2 := mustNewServiceWithOptions(test, sumRefundsFailStore)

	originalEntryID := spendEntry.EntryID()
	operations := []BatchOperation{
		{
			OperationID: "refund-1",
			Refund: &BatchRefundOperation{
				OriginalEntryID: &originalEntryID,
				Amount:          mustPositiveAmount(test, 50),
				IdempotencyKey:  mustIdempotencyKey(test, "refund-1"),
				Metadata:        mustMetadata(test, "{}"),
			},
		},
	}

	results, err := service2.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, sentinel) {
		test.Fatalf("expected sum refunds sentinel error, got %v", results[0].Error)
	}
}

// ---------------------------------------------------------------------------
// service_refund.go: RefundByEntryIDEntry errors
// ---------------------------------------------------------------------------

func TestRefundByEntryIDEntryGetOrCreateAccountIDError(test *testing.T) {
	test.Parallel()
	sentinel := errors.New("account lookup failed")
	store := newStubStore(test, mustSignedAmount(test, 0))
	store.getAccountError = sentinel
	service := mustNewService(test, store)

	_, err := service.RefundByEntryIDEntry(
		context.Background(),
		mustTenantID(test, defaultTenantIDValue),
		mustUserID(test, "user-1"),
		mustLedgerID(test, defaultLedgerIDValue),
		mustEntryID(test, "entry-1"),
		mustPositiveAmount(test, 1),
		mustIdempotencyKey(test, "refund-1"),
		mustMetadata(test, "{}"),
	)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestRefundByEntryIDEntryGetEntryError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)

	// No entries in store, so GetEntry will return ErrUnknownEntry.
	_, err := service.RefundByEntryIDEntry(
		context.Background(),
		mustTenantID(test, defaultTenantIDValue),
		mustUserID(test, "user-1"),
		mustLedgerID(test, defaultLedgerIDValue),
		mustEntryID(test, "missing-entry"),
		mustPositiveAmount(test, 1),
		mustIdempotencyKey(test, "refund-1"),
		mustMetadata(test, "{}"),
	)
	if !errors.Is(err, ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestRefundByEntryIDEntrySumRefundsError(test *testing.T) {
	test.Parallel()
	sentinel := errors.New("sum refunds failed")
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	// Create a spend entry so GetEntry succeeds.
	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	// Wrap with a store that fails SumRefunds.
	wrappedStore := &sumRefundsFailingStore{
		stubStore: store,
		err:       sentinel,
	}
	service2 := mustNewServiceWithOptions(test, wrappedStore)

	_, err = service2.RefundByEntryIDEntry(
		context.Background(), tenantID, userID, ledgerID,
		spendEntry.EntryID(), mustPositiveAmount(test, 50),
		mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"),
	)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sum refunds sentinel, got %v", err)
	}
}

func TestRefundByEntryIDEntryNewEntryInputError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	// MetadataJSON{} causes NewEntryInput to fail.
	_, err = service.RefundByEntryIDEntry(
		context.Background(), tenantID, userID, ledgerID,
		spendEntry.EntryID(), mustPositiveAmount(test, 50),
		mustIdempotencyKey(test, "refund-1"), MetadataJSON{},
	)
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// service_reservations.go: ListReservationStates GetOrCreateAccountID error
// This is already covered in TestReservationStateMethodsPropagateStoreErrors.
// Adding an explicit test for clarity.
// ---------------------------------------------------------------------------

func TestListReservationStatesGetOrCreateAccountIDError(test *testing.T) {
	test.Parallel()
	sentinel := errors.New("account error")
	store := newStubStore(test, mustSignedAmount(test, 0))
	store.getAccountError = sentinel
	service := mustNewService(test, store)

	_, err := service.ListReservationStates(
		context.Background(),
		mustTenantID(test, defaultTenantIDValue),
		mustUserID(test, "user-1"),
		mustLedgerID(test, defaultLedgerIDValue),
		0, 10, ListReservationsFilter{},
	)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Wrapper stores for testing specific error paths
// ---------------------------------------------------------------------------

// getEntryFailStore wraps stubStore and fails GetEntry with a specific error.
type getEntryFailStore struct {
	*stubStore
	err error
}

func (store *getEntryFailStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *getEntryFailStore) GetEntry(ctx context.Context, accountID AccountID, entryID EntryID) (Entry, error) {
	return Entry{}, store.err
}

// getEntryByIdemKeyFailStore wraps stubStore and fails GetEntryByIdempotencyKey
// with a non-ErrUnknownEntry error (to exercise the "!errors.Is(err, ErrUnknownEntry)" branch).
type getEntryByIdemKeyFailStore struct {
	*stubStore
	err error
}

func (store *getEntryByIdemKeyFailStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *getEntryByIdemKeyFailStore) GetEntryByIdempotencyKey(ctx context.Context, accountID AccountID, idempotencyKey IdempotencyKey) (Entry, error) {
	return Entry{}, store.err
}

// sumRefundsFailingStore wraps stubStore and fails SumRefunds with a specific error.
type sumRefundsFailingStore struct {
	*stubStore
	err error
}

func (store *sumRefundsFailingStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *sumRefundsFailingStore) SumRefunds(ctx context.Context, accountID AccountID, originalEntryID EntryID) (AmountCents, error) {
	return 0, store.err
}
