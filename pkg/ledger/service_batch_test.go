package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestBatchReturnsNilWhenOperationsEmpty(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, nil, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if results != nil {
		test.Fatalf("expected nil results, got %v", results)
	}
}

func TestBatchBestEffortCommitsSuccessfulOperationsAndReturnsPerItemErrors(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
		newBatchSpendOperation(test, "spend-1", 200, "spend-1"),
		newBatchGrantOperation(test, "grant-2", 50, "grant-2"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != len(operations) {
		test.Fatalf("expected %d results, got %d", len(operations), len(results))
	}

	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result[0]: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[1].Error, ErrInsufficientFunds) || results[1].Entry != nil || results[1].Duplicate || results[1].RolledBack {
		test.Fatalf("unexpected result[1]: entry=%v err=%v dup=%v rolled_back=%v", results[1].Entry, results[1].Error, results[1].Duplicate, results[1].RolledBack)
	}
	if results[2].Entry == nil || results[2].Error != nil || results[2].Duplicate || results[2].RolledBack {
		test.Fatalf("unexpected result[2]: entry=%v err=%v dup=%v rolled_back=%v", results[2].Entry, results[2].Error, results[2].Duplicate, results[2].RolledBack)
	}

	if store.total != 150 {
		test.Fatalf("expected total 150, got %d", store.total)
	}
}

func TestBatchTreatsDuplicateIdempotencyKeyAsDuplicate(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "dup-1"),
		newBatchGrantOperation(test, "grant-2", 100, "dup-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		test.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate {
		test.Fatalf("unexpected first result: entry=%v err=%v dup=%v", results[0].Entry, results[0].Error, results[0].Duplicate)
	}
	if results[1].Entry != nil || results[1].Error != nil || !results[1].Duplicate {
		test.Fatalf("unexpected duplicate result: entry=%v err=%v dup=%v", results[1].Entry, results[1].Error, results[1].Duplicate)
	}
	if store.total != 100 {
		test.Fatalf("expected total 100, got %d", store.total)
	}
}

func TestBatchAtomicRollsBackAllOperationsOnFailure(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
		newBatchSpendOperation(test, "spend-1", 50, "spend-1"),
		newBatchSpendOperation(test, "spend-2", 1000, "spend-2"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, true)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 3 {
		test.Fatalf("expected 3 results, got %d", len(results))
	}

	if results[0].Error != nil || results[0].Duplicate || !results[0].RolledBack || results[0].Entry != nil {
		test.Fatalf("unexpected result[0]: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if results[1].Error != nil || results[1].Duplicate || !results[1].RolledBack || results[1].Entry != nil {
		test.Fatalf("unexpected result[1]: entry=%v err=%v dup=%v rolled_back=%v", results[1].Entry, results[1].Error, results[1].Duplicate, results[1].RolledBack)
	}
	if !errors.Is(results[2].Error, ErrInsufficientFunds) || results[2].Duplicate || results[2].RolledBack || results[2].Entry != nil {
		test.Fatalf("unexpected result[2]: entry=%v err=%v dup=%v rolled_back=%v", results[2].Entry, results[2].Error, results[2].Duplicate, results[2].RolledBack)
	}

	if store.total != 0 {
		test.Fatalf("expected total 0 after rollback, got %d", store.total)
	}
	if len(store.entries) != 0 {
		test.Fatalf("expected no committed entries after rollback, got %d", len(store.entries))
	}
	if len(store.reservations) != 0 {
		test.Fatalf("expected no reservations after rollback, got %d", len(store.reservations))
	}
}

func TestBatchReserveCaptureAndReleaseSucceed(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchReserveOperation(test, "reserve-1", 60, "res-1", "reserve-1"),
		newBatchCaptureOperation(test, "capture-1", 60, "res-1", "capture-1"),
		newBatchReserveOperation(test, "reserve-2", 40, "res-2", "reserve-2"),
		newBatchReleaseOperation(test, "release-2", "res-2", "release-2"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != len(operations) {
		test.Fatalf("expected %d results, got %d", len(operations), len(results))
	}
	for resultIndex, result := range results {
		if result.Error != nil || result.Duplicate || result.RolledBack || result.Entry == nil {
			test.Fatalf("unexpected result[%d]: entry=%v err=%v dup=%v rolled_back=%v", resultIndex, result.Entry, result.Error, result.Duplicate, result.RolledBack)
		}
	}

	reservationOne := store.mustReservation(test, mustReservationID(test, "res-1"))
	if reservationOne.Status() != ReservationStatusCaptured {
		test.Fatalf("expected res-1 captured, got %s", reservationOne.Status())
	}
	reservationTwo := store.mustReservation(test, mustReservationID(test, "res-2"))
	if reservationTwo.Status() != ReservationStatusReleased {
		test.Fatalf("expected res-2 released, got %s", reservationTwo.Status())
	}
	if store.total != 140 {
		test.Fatalf("expected total 140 after capture, got %d", store.total)
	}
}

func TestBatchRefundByOriginalEntryIDSucceeds(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, spendEntry.EntryID(), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if results[0].Entry.Type() != EntryRefund {
		test.Fatalf("expected refund entry, got %s", results[0].Entry.Type())
	}
	refundOfEntryID, ok := results[0].Entry.RefundOfEntryID()
	if !ok || refundOfEntryID != spendEntry.EntryID() {
		test.Fatalf("expected refund_of_entry_id %s, got %v", spendEntry.EntryID().String(), refundOfEntryID.String())
	}
	if store.total != 850 {
		test.Fatalf("expected total 850, got %d", store.total)
	}
}

func TestBatchRefundByOriginalIdempotencyKeySucceeds(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByOriginalIdempotencyKeyOperation(test, "refund-1", 50, mustIdempotencyKey(test, "spend-1"), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	refundOfEntryID, ok := results[0].Entry.RefundOfEntryID()
	if !ok || refundOfEntryID != spendEntry.EntryID() {
		test.Fatalf("expected refund_of_entry_id %s, got %v", spendEntry.EntryID().String(), refundOfEntryID.String())
	}
	if store.total != 850 {
		test.Fatalf("expected total 850, got %d", store.total)
	}
}

func TestBatchRefundRejectsOverRefund(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 100), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 80, spendEntry.EntryID(), "refund-1"),
		newBatchRefundByEntryIDOperation(test, "refund-2", 30, spendEntry.EntryID(), "refund-2"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		test.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected first result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[1].Error, ErrRefundExceedsDebit) || results[1].Entry != nil || results[1].Duplicate || results[1].RolledBack {
		test.Fatalf("unexpected second result: entry=%v err=%v dup=%v rolled_back=%v", results[1].Entry, results[1].Error, results[1].Duplicate, results[1].RolledBack)
	}
	if store.total != 980 {
		test.Fatalf("expected total 980, got %d", store.total)
	}
}

func TestBatchRefundDuplicateIdempotencyIsMarkedDuplicate(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 100), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, spendEntry.EntryID(), "refund-1"),
		newBatchRefundByEntryIDOperation(test, "refund-2", 50, spendEntry.EntryID(), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		test.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected first result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if results[1].Entry != nil || results[1].Error != nil || !results[1].Duplicate || results[1].RolledBack {
		test.Fatalf("unexpected duplicate result: entry=%v err=%v dup=%v rolled_back=%v", results[1].Entry, results[1].Error, results[1].Duplicate, results[1].RolledBack)
	}
	if store.total != 950 {
		test.Fatalf("expected total 950, got %d", store.total)
	}
}

func TestBatchRefundReturnsErrorWhenIdempotencyKeyConflictsWithNonRefundEntry(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "collision-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, spendEntry.EntryID(), "collision-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[0].Error, ErrIdempotencyKeyConflict) {
		test.Fatalf("expected idempotency key conflict, got %v", results[0].Error)
	}
	if store.total != 800 {
		test.Fatalf("expected total 800, got %d", store.total)
	}
}

func TestApplyBatchRefundReturnsDuplicateWhenInsertEntryDetectsExistingRefund(test *testing.T) {
	test.Parallel()
	service := &Service{nowFn: func() int64 { return 100 }}
	accountID := mustAccountID(test, "acct-1")

	originalEntryID := mustEntryID(test, "spend-1")
	originalEntry := mustEntry(
		test,
		originalEntryID,
		accountID,
		EntrySpend,
		mustEntryAmount(test, -200),
		mustIdempotencyKey(test, "spend-1"),
		mustMetadata(test, "{}"),
	)

	refundKey := mustIdempotencyKey(test, "refund-1")
	existingRefund := mustEntry(
		test,
		mustEntryID(test, "refund-existing"),
		accountID,
		EntryRefund,
		mustEntryAmount(test, 50),
		refundKey,
		mustMetadata(test, "{}"),
	)

	store := &duplicateInsertRefundStore{
		originalEntry: originalEntry,
		existingEntry: existingRefund,
	}

	operationOriginalEntryID := originalEntryID
	entry, err := service.applyBatchRefund(context.Background(), store, accountID, BatchRefundOperation{
		OriginalEntryID: &operationOriginalEntryID,
		Amount:          mustPositiveAmount(test, 50),
		IdempotencyKey:  refundKey,
		Metadata:        mustMetadata(test, "{}"),
	})
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency key, got %v", err)
	}
	if entry.Type() != EntryRefund {
		test.Fatalf("expected refund entry, got %s", entry.Type())
	}
}

func TestApplyBatchRefundReturnsConflictWhenInsertEntryDetectsExistingNonRefundEntry(test *testing.T) {
	test.Parallel()
	service := &Service{nowFn: func() int64 { return 100 }}
	accountID := mustAccountID(test, "acct-1")

	originalEntryID := mustEntryID(test, "spend-1")
	originalEntry := mustEntry(
		test,
		originalEntryID,
		accountID,
		EntrySpend,
		mustEntryAmount(test, -200),
		mustIdempotencyKey(test, "spend-1"),
		mustMetadata(test, "{}"),
	)

	refundKey := mustIdempotencyKey(test, "refund-1")
	existingGrant := mustEntry(
		test,
		mustEntryID(test, "grant-existing"),
		accountID,
		EntryGrant,
		mustEntryAmount(test, 50),
		refundKey,
		mustMetadata(test, "{}"),
	)

	store := &duplicateInsertRefundStore{
		originalEntry: originalEntry,
		existingEntry: existingGrant,
	}

	operationOriginalEntryID := originalEntryID
	_, err := service.applyBatchRefund(context.Background(), store, accountID, BatchRefundOperation{
		OriginalEntryID: &operationOriginalEntryID,
		Amount:          mustPositiveAmount(test, 50),
		IdempotencyKey:  refundKey,
		Metadata:        mustMetadata(test, "{}"),
	})
	if !errors.Is(err, ErrIdempotencyKeyConflict) {
		test.Fatalf("expected idempotency key conflict, got %v", err)
	}
}

func TestApplyBatchRefundReturnsErrorWhenInsertEntryDuplicateCannotBeResolved(test *testing.T) {
	test.Parallel()
	service := &Service{nowFn: func() int64 { return 100 }}
	accountID := mustAccountID(test, "acct-1")

	originalEntryID := mustEntryID(test, "spend-1")
	originalEntry := mustEntry(
		test,
		originalEntryID,
		accountID,
		EntrySpend,
		mustEntryAmount(test, -200),
		mustIdempotencyKey(test, "spend-1"),
		mustMetadata(test, "{}"),
	)

	lookupErr := errors.New("lookup failed")
	store := &duplicateInsertRefundStore{
		originalEntry: originalEntry,
		existingErr:   lookupErr,
	}

	operationOriginalEntryID := originalEntryID
	_, err := service.applyBatchRefund(context.Background(), store, accountID, BatchRefundOperation{
		OriginalEntryID: &operationOriginalEntryID,
		Amount:          mustPositiveAmount(test, 50),
		IdempotencyKey:  mustIdempotencyKey(test, "refund-1"),
		Metadata:        mustMetadata(test, "{}"),
	})
	if !errors.Is(err, lookupErr) {
		test.Fatalf("expected lookup error, got %v", err)
	}
}

func TestBatchRefundReturnsErrorWhenOriginalReferenceMissing(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		{
			OperationID: "refund-1",
			Refund: &BatchRefundOperation{
				Amount:         mustPositiveAmount(test, 50),
				IdempotencyKey: mustIdempotencyKey(test, "refund-1"),
				Metadata:       mustMetadata(test, "{}"),
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
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if results[0].Error == nil || results[0].Error.Error() != "missing_refund_original" {
		test.Fatalf("expected missing_refund_original, got %v", results[0].Error)
	}
}

func TestBatchRefundRejectsNonDebitOriginal(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	grantEntry, err := service.GrantEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("grant entry: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, grantEntry.EntryID(), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[0].Error, ErrInvalidRefundOriginal) {
		test.Fatalf("expected invalid refund original, got %v", results[0].Error)
	}
	if store.total != 1000 {
		test.Fatalf("expected total 1000, got %d", store.total)
	}
}

func TestBatchRefundCopiesReservationIDWhenOriginalHasReservation(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	reservationID := mustReservationID(test, "res-1")
	if _, err := service.ReserveEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), reservationID, mustIdempotencyKey(test, "reserve-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("reserve entry: %v", err)
	}
	originalSpend, err := service.CaptureDebitEntry(context.Background(), tenantID, userID, ledgerID, reservationID, mustIdempotencyKey(test, "capture-1"), mustPositiveAmount(test, 200), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("capture debit entry: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, originalSpend.EntryID(), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	operationReservationID, ok := results[0].Entry.ReservationID()
	if !ok || operationReservationID != reservationID {
		test.Fatalf("expected reservation id %s, got %v", reservationID.String(), operationReservationID.String())
	}
	if store.total != 850 {
		test.Fatalf("expected total 850, got %d", store.total)
	}
}

func TestBatchRefundReturnsUnknownEntryWhenOriginalEntryDoesNotExist(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, mustEntryID(test, "missing-entry"), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[0].Error, ErrUnknownEntry) {
		test.Fatalf("expected unknown entry, got %v", results[0].Error)
	}
	if store.total != 1000 {
		test.Fatalf("expected total 1000, got %d", store.total)
	}
}

func TestBatchRefundReturnsErrorWhenInsertEntryFails(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	insertError := errors.New("insert failed")
	store.insertEntryError = insertError
	store.insertEntryErrorAtCall = store.insertEntryCallCount + 1

	operations := []BatchOperation{
		newBatchRefundByEntryIDOperation(test, "refund-1", 50, spendEntry.EntryID(), "refund-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[0].Error, insertError) {
		test.Fatalf("expected insert error, got %v", results[0].Error)
	}
	if store.total != 800 {
		test.Fatalf("expected total 800, got %d", store.total)
	}
}

func TestBatchCaptureReturnsErrorWhenUpdateReservationStatusFails(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	reservationID := mustReservationID(test, "res-1")
	if _, err := service.ReserveEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 10), reservationID, mustIdempotencyKey(test, "reserve-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("reserve entry: %v", err)
	}

	updateError := errors.New("update failed")
	store.updateReservationError = updateError

	operations := []BatchOperation{
		newBatchCaptureOperation(test, "capture-1", 10, reservationID.String(), "capture-1"),
	}
	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if !errors.Is(results[0].Error, updateError) {
		test.Fatalf("expected update error, got %v", results[0].Error)
	}
	reservation := store.mustReservation(test, reservationID)
	if reservation.Status() != ReservationStatusActive {
		test.Fatalf("expected reservation to remain active, got %s", reservation.Status())
	}
}

func TestBatchSavepointRollsBackFailedOperationButKeepsPriorSuccess(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	store.insertEntryError = errors.New("insert failed")
	store.insertEntryErrorAtCall = 3
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchReserveOperation(test, "reserve-1", 60, "res-1", "reserve-1"),
		newBatchCaptureOperation(test, "capture-1", 60, "res-1", "capture-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 2 {
		test.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Entry == nil || results[0].Error != nil || results[0].Duplicate || results[0].RolledBack {
		test.Fatalf("unexpected reserve result: entry=%v err=%v dup=%v rolled_back=%v", results[0].Entry, results[0].Error, results[0].Duplicate, results[0].RolledBack)
	}
	if results[1].Entry != nil || results[1].Duplicate || results[1].RolledBack || results[1].Error == nil {
		test.Fatalf("unexpected capture result: entry=%v err=%v dup=%v rolled_back=%v", results[1].Entry, results[1].Error, results[1].Duplicate, results[1].RolledBack)
	}

	reservation := store.mustReservation(test, mustReservationID(test, "res-1"))
	if reservation.Status() != ReservationStatusActive {
		test.Fatalf("expected reservation to remain active, got %s", reservation.Status())
	}
	if len(store.entries) != 1 {
		test.Fatalf("expected only the reserve entry to persist, got %d entries", len(store.entries))
	}
}

func TestBatchReturnsErrorsForUnknownOrClosedReservations(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	closedReservationID := mustReservationID(test, "closed-1")
	store.reservations[closedReservationID] = mustReservationRecord(test, store.accountID, closedReservationID, mustPositiveAmount(test, 10), ReservationStatusCaptured)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchCaptureOperation(test, "capture-missing", 10, "missing", "capture-missing"),
		newBatchReleaseOperation(test, "release-missing", "missing", "release-missing"),
		newBatchCaptureOperation(test, "capture-closed", 10, "closed-1", "capture-closed"),
		newBatchReleaseOperation(test, "release-closed", "closed-1", "release-closed"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != len(operations) {
		test.Fatalf("expected %d results, got %d", len(operations), len(results))
	}
	if !errors.Is(results[0].Error, ErrUnknownReservation) {
		test.Fatalf("expected unknown reservation, got %v", results[0].Error)
	}
	if !errors.Is(results[1].Error, ErrUnknownReservation) {
		test.Fatalf("expected unknown reservation, got %v", results[1].Error)
	}
	if !errors.Is(results[2].Error, ErrReservationClosed) {
		test.Fatalf("expected reservation closed, got %v", results[2].Error)
	}
	if !errors.Is(results[3].Error, ErrReservationClosed) {
		test.Fatalf("expected reservation closed, got %v", results[3].Error)
	}
}

func TestBatchCaptureReturnsInvalidAmountWhenAmountMismatch(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 200))
	reservationID := mustReservationID(test, "res-1")
	store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, mustPositiveAmount(test, 10), ReservationStatusActive)
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	operations := []BatchOperation{
		newBatchCaptureOperation(test, "capture-1", 5, "res-1", "capture-1"),
	}

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, operations, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidAmountCents) {
		test.Fatalf("expected invalid amount, got %v", results[0].Error)
	}
}

func TestBatchReserveReturnsInsufficientFundsWhenTotalTooLow(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
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
	if !errors.Is(results[0].Error, ErrInsufficientFunds) {
		test.Fatalf("expected insufficient funds, got %v", results[0].Error)
	}
}

func TestBatchReturnsErrorWhenOperationVariantMissing(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{{OperationID: "op-1"}}, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil || results[0].Error.Error() != "unknown_batch_operation" {
		test.Fatalf("expected unknown_batch_operation error, got %v", results[0].Error)
	}
}

func TestBatchGrantReturnsErrorWhenAccountIDInvalid(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	store.accountID = AccountID{}
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
	}, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, ErrInvalidAccountID) {
		test.Fatalf("expected invalid account id, got %v", results[0].Error)
	}
}

func TestBatchGrantReturnsErrorWhenInsertEntryFails(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	insertError := errors.New("insert failed")
	store.insertEntryError = insertError
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{
		newBatchGrantOperation(test, "grant-1", 100, "grant-1"),
	}, false)
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !errors.Is(results[0].Error, insertError) {
		test.Fatalf("expected insert error, got %v", results[0].Error)
	}
}

func TestBatchSpendReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	sumTotalError := errors.New("sum total failed")
	sumHoldsError := errors.New("sum holds failed")
	insertError := errors.New("insert failed")

	testCases := []struct {
		name      string
		configure func(store *stubStore)
		wantErr   error
	}{
		{
			name: "sum total error",
			configure: func(store *stubStore) {
				store.sumTotalError = sumTotalError
			},
			wantErr: sumTotalError,
		},
		{
			name: "sum holds error",
			configure: func(store *stubStore) {
				store.sumActiveHoldsError = sumHoldsError
			},
			wantErr: sumHoldsError,
		},
		{
			name: "insert entry error",
			configure: func(store *stubStore) {
				store.insertEntryError = insertError
			},
			wantErr: insertError,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 200))
			testCase.configure(store)
			service := mustNewService(test, store)
			userID := mustUserID(test, "user-123")
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{
				newBatchSpendOperation(test, "spend-1", 10, "spend-1"),
			}, false)
			if err != nil {
				test.Fatalf("batch: %v", err)
			}
			if len(results) != 1 {
				test.Fatalf("expected 1 result, got %d", len(results))
			}
			if !errors.Is(results[0].Error, testCase.wantErr) {
				test.Fatalf("expected %v, got %v", testCase.wantErr, results[0].Error)
			}
		})
	}
}

func TestBatchReserveReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	sumTotalError := errors.New("sum total failed")
	sumHoldsError := errors.New("sum holds failed")
	createError := errors.New("create reservation failed")
	insertError := errors.New("insert failed")

	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore)
		wantErr   error
	}{
		{
			name: "sum total error",
			configure: func(test *testing.T, store *stubStore) {
				store.sumTotalError = sumTotalError
			},
			wantErr: sumTotalError,
		},
		{
			name: "sum holds error",
			configure: func(test *testing.T, store *stubStore) {
				store.sumActiveHoldsError = sumHoldsError
			},
			wantErr: sumHoldsError,
		},
		{
			name: "create reservation error",
			configure: func(test *testing.T, store *stubStore) {
				store.createReservationError = createError
			},
			wantErr: createError,
		},
		{
			name: "reservation exists",
			configure: func(test *testing.T, store *stubStore) {
				reservationID := mustReservationID(test, "res-1")
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, mustPositiveAmount(test, 10), ReservationStatusActive)
			},
			wantErr: ErrReservationExists,
		},
		{
			name: "insert entry error",
			configure: func(test *testing.T, store *stubStore) {
				store.insertEntryError = insertError
			},
			wantErr: insertError,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 200))
			testCase.configure(test, store)
			service := mustNewService(test, store)
			userID := mustUserID(test, "user-123")
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{
				newBatchReserveOperation(test, "reserve-1", 10, "res-1", "reserve-1"),
			}, false)
			if err != nil {
				test.Fatalf("batch: %v", err)
			}
			if len(results) != 1 {
				test.Fatalf("expected 1 result, got %d", len(results))
			}
			if !errors.Is(results[0].Error, testCase.wantErr) {
				test.Fatalf("expected %v, got %v", testCase.wantErr, results[0].Error)
			}
		})
	}
}

func TestBatchCaptureAndReleaseReturnStoreErrors(test *testing.T) {
	test.Parallel()
	updateError := errors.New("update failed")
	insertError := errors.New("insert failed")

	testCases := []struct {
		name      string
		operation BatchOperation
		configure func(store *stubStore)
		wantErr   error
	}{
		{
			name:      "capture update status error",
			operation: newBatchCaptureOperation(test, "capture-1", 10, "res-1", "capture-1"),
			configure: func(store *stubStore) {
				store.updateReservationError = updateError
			},
			wantErr: updateError,
		},
		{
			name:      "capture reverse entry insert error",
			operation: newBatchCaptureOperation(test, "capture-1", 10, "res-1", "capture-1"),
			configure: func(store *stubStore) {
				store.insertEntryError = insertError
				store.insertEntryErrorAtCall = 1
			},
			wantErr: insertError,
		},
		{
			name:      "release update status error",
			operation: newBatchReleaseOperation(test, "release-1", "res-1", "release-1"),
			configure: func(store *stubStore) {
				store.updateReservationError = updateError
			},
			wantErr: updateError,
		},
		{
			name:      "release insert entry error",
			operation: newBatchReleaseOperation(test, "release-1", "res-1", "release-1"),
			configure: func(store *stubStore) {
				store.insertEntryError = insertError
			},
			wantErr: insertError,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 200))
			reservationID := mustReservationID(test, "res-1")
			store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, mustPositiveAmount(test, 10), ReservationStatusActive)
			testCase.configure(store)
			service := mustNewService(test, store)
			userID := mustUserID(test, "user-123")
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			results, err := service.Batch(context.Background(), tenantID, userID, ledgerID, []BatchOperation{testCase.operation}, false)
			if err != nil {
				test.Fatalf("batch: %v", err)
			}
			if len(results) != 1 {
				test.Fatalf("expected 1 result, got %d", len(results))
			}
			if !errors.Is(results[0].Error, testCase.wantErr) {
				test.Fatalf("expected %v, got %v", testCase.wantErr, results[0].Error)
			}
		})
	}
}

func newBatchGrantOperation(test *testing.T, operationID string, amountCents int64, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	return BatchOperation{
		OperationID: operationID,
		Grant: &BatchGrantOperation{
			Amount:           mustPositiveAmount(test, amountCents),
			IdempotencyKey:   mustIdempotencyKey(test, idempotencyKeyValue),
			ExpiresAtUnixUTC: 0,
			Metadata:         mustMetadata(test, "{}"),
		},
	}
}

func newBatchSpendOperation(test *testing.T, operationID string, amountCents int64, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	return BatchOperation{
		OperationID: operationID,
		Spend: &BatchSpendOperation{
			Amount:         mustPositiveAmount(test, amountCents),
			IdempotencyKey: mustIdempotencyKey(test, idempotencyKeyValue),
			Metadata:       mustMetadata(test, "{}"),
		},
	}
}

func newBatchReserveOperation(test *testing.T, operationID string, amountCents int64, reservationIDValue string, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	return BatchOperation{
		OperationID: operationID,
		Reserve: &BatchReserveOperation{
			Amount:         mustPositiveAmount(test, amountCents),
			ReservationID:  mustReservationID(test, reservationIDValue),
			IdempotencyKey: mustIdempotencyKey(test, idempotencyKeyValue),
			Metadata:       mustMetadata(test, "{}"),
		},
	}
}

func newBatchCaptureOperation(test *testing.T, operationID string, amountCents int64, reservationIDValue string, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	return BatchOperation{
		OperationID: operationID,
		Capture: &BatchCaptureOperation{
			ReservationID:  mustReservationID(test, reservationIDValue),
			IdempotencyKey: mustIdempotencyKey(test, idempotencyKeyValue),
			Amount:         mustPositiveAmount(test, amountCents),
			Metadata:       mustMetadata(test, "{}"),
		},
	}
}

func newBatchReleaseOperation(test *testing.T, operationID string, reservationIDValue string, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	return BatchOperation{
		OperationID: operationID,
		Release: &BatchReleaseOperation{
			ReservationID:  mustReservationID(test, reservationIDValue),
			IdempotencyKey: mustIdempotencyKey(test, idempotencyKeyValue),
			Metadata:       mustMetadata(test, "{}"),
		},
	}
}

func newBatchRefundByEntryIDOperation(test *testing.T, operationID string, amountCents int64, originalEntryID EntryID, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	originalEntryIDValue := originalEntryID
	return BatchOperation{
		OperationID: operationID,
		Refund: &BatchRefundOperation{
			OriginalEntryID: &originalEntryIDValue,
			Amount:          mustPositiveAmount(test, amountCents),
			IdempotencyKey:  mustIdempotencyKey(test, idempotencyKeyValue),
			Metadata:        mustMetadata(test, "{}"),
		},
	}
}

func newBatchRefundByOriginalIdempotencyKeyOperation(test *testing.T, operationID string, amountCents int64, originalIdempotencyKey IdempotencyKey, idempotencyKeyValue string) BatchOperation {
	test.Helper()
	originalIdempotencyKeyValue := originalIdempotencyKey
	return BatchOperation{
		OperationID: operationID,
		Refund: &BatchRefundOperation{
			OriginalIdempotencyKey: &originalIdempotencyKeyValue,
			Amount:                 mustPositiveAmount(test, amountCents),
			IdempotencyKey:         mustIdempotencyKey(test, idempotencyKeyValue),
			Metadata:               mustMetadata(test, "{}"),
		},
	}
}

type duplicateInsertRefundStore struct {
	originalEntry Entry
	existingEntry Entry
	existingErr   error
	idemCalls     int
}

func (store *duplicateInsertRefundStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *duplicateInsertRefundStore) GetOrCreateAccountID(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) (AccountID, error) {
	panic("GetOrCreateAccountID not used")
}

func (store *duplicateInsertRefundStore) InsertEntry(ctx context.Context, entry EntryInput) (Entry, error) {
	return Entry{}, ErrDuplicateIdempotencyKey
}

func (store *duplicateInsertRefundStore) GetEntry(ctx context.Context, accountID AccountID, entryID EntryID) (Entry, error) {
	if store.originalEntry.EntryID() != entryID {
		return Entry{}, ErrUnknownEntry
	}
	return store.originalEntry, nil
}

func (store *duplicateInsertRefundStore) GetEntryByIdempotencyKey(ctx context.Context, accountID AccountID, idempotencyKey IdempotencyKey) (Entry, error) {
	store.idemCalls++
	if store.idemCalls == 1 {
		return Entry{}, ErrUnknownEntry
	}
	if store.existingErr != nil {
		return Entry{}, store.existingErr
	}
	return store.existingEntry, nil
}

func (store *duplicateInsertRefundStore) SumRefunds(ctx context.Context, accountID AccountID, originalEntryID EntryID) (AmountCents, error) {
	return NewAmountCents(0)
}

func (store *duplicateInsertRefundStore) SumTotal(ctx context.Context, accountID AccountID, atUnixUTC int64) (SignedAmountCents, error) {
	panic("SumTotal not used")
}

func (store *duplicateInsertRefundStore) SumActiveHolds(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error) {
	panic("SumActiveHolds not used")
}

func (store *duplicateInsertRefundStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	panic("CreateReservation not used")
}

func (store *duplicateInsertRefundStore) GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error) {
	panic("GetReservation not used")
}

func (store *duplicateInsertRefundStore) UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from ReservationStatus, to ReservationStatus) error {
	panic("UpdateReservationStatus not used")
}

func (store *duplicateInsertRefundStore) ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int, filter ListEntriesFilter) ([]Entry, error) {
	panic("ListEntries not used")
}
