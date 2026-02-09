package ledger

import (
	"context"
	"errors"
	"fmt"
)

// BatchGrantOperation describes a grant mutation within a batch request.
type BatchGrantOperation struct {
	Amount           PositiveAmountCents
	IdempotencyKey   IdempotencyKey
	ExpiresAtUnixUTC int64
	Metadata         MetadataJSON
}

// BatchReserveOperation describes a reserve mutation within a batch request.
type BatchReserveOperation struct {
	Amount         PositiveAmountCents
	ReservationID  ReservationID
	IdempotencyKey IdempotencyKey
	Metadata       MetadataJSON
}

// BatchCaptureOperation describes a capture mutation within a batch request.
type BatchCaptureOperation struct {
	ReservationID  ReservationID
	IdempotencyKey IdempotencyKey
	Amount         PositiveAmountCents
	Metadata       MetadataJSON
}

// BatchReleaseOperation describes a release mutation within a batch request.
type BatchReleaseOperation struct {
	ReservationID  ReservationID
	IdempotencyKey IdempotencyKey
	Metadata       MetadataJSON
}

// BatchSpendOperation describes a spend mutation within a batch request.
type BatchSpendOperation struct {
	Amount         PositiveAmountCents
	IdempotencyKey IdempotencyKey
	Metadata       MetadataJSON
}

// BatchRefundOperation describes a refund mutation within a batch request.
type BatchRefundOperation struct {
	OriginalEntryID        *EntryID
	OriginalIdempotencyKey *IdempotencyKey
	Amount                 PositiveAmountCents
	IdempotencyKey         IdempotencyKey
	Metadata               MetadataJSON
}

// BatchOperation is a single credit mutation within a batch request.
type BatchOperation struct {
	OperationID string
	Grant       *BatchGrantOperation
	Reserve     *BatchReserveOperation
	Capture     *BatchCaptureOperation
	Release     *BatchReleaseOperation
	Spend       *BatchSpendOperation
	Refund      *BatchRefundOperation
}

// BatchOperationResult captures the outcome of a single batch operation.
type BatchOperationResult struct {
	OperationID string
	Entry       *Entry
	Duplicate   bool
	RolledBack  bool
	Error       error
}

var errBatchAtomicRollback = errors.New("batch_atomic_rollback")

// Batch executes the supplied operations in a single database transaction and returns per-operation outcomes.
func (service *Service) Batch(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, operations []BatchOperation, atomic bool) ([]BatchOperationResult, error) {
	if len(operations) == 0 {
		return nil, nil
	}

	results := make([]BatchOperationResult, len(operations))
	batchRolledBack := false
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}

		hasFailure := false
		for index, operation := range operations {
			operation := operation
			result := BatchOperationResult{OperationID: operation.OperationID}
			entry, err := service.applyBatchOperation(ctx, transactionStore, accountID, operation)
			if err != nil {
				if errors.Is(err, ErrDuplicateIdempotencyKey) {
					result.Duplicate = true
				} else {
					result.Error = err
					hasFailure = true
				}
				results[index] = result
				continue
			}
			result.Entry = &entry
			results[index] = result
		}

		if atomic && hasFailure {
			batchRolledBack = true
			return errBatchAtomicRollback
		}
		return nil
	})
	if operationError != nil && !errors.Is(operationError, errBatchAtomicRollback) {
		return nil, operationError
	}

	if batchRolledBack {
		for index := range results {
			result := results[index]
			if result.Error == nil && !result.Duplicate && result.Entry != nil {
				result.RolledBack = true
				result.Entry = nil
				results[index] = result
			}
		}
	}

	return results, nil
}

func (service *Service) applyBatchOperation(ctx context.Context, transactionStore Store, accountID AccountID, operation BatchOperation) (Entry, error) {
	var persistedEntry Entry
	err := transactionStore.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		entry, err := service.applyBatchOperationWithinTx(ctx, txStore, accountID, operation)
		if err != nil {
			return err
		}
		persistedEntry = entry
		return nil
	})
	if err != nil {
		return Entry{}, err
	}
	return persistedEntry, nil
}

func (service *Service) applyBatchOperationWithinTx(ctx context.Context, txStore Store, accountID AccountID, operation BatchOperation) (Entry, error) {
	if operation.Grant != nil {
		return service.applyBatchGrant(ctx, txStore, accountID, *operation.Grant)
	}
	if operation.Spend != nil {
		return service.applyBatchSpend(ctx, txStore, accountID, *operation.Spend)
	}
	if operation.Reserve != nil {
		return service.applyBatchReserve(ctx, txStore, accountID, *operation.Reserve)
	}
	if operation.Capture != nil {
		return service.applyBatchCapture(ctx, txStore, accountID, *operation.Capture)
	}
	if operation.Release != nil {
		return service.applyBatchRelease(ctx, txStore, accountID, *operation.Release)
	}
	if operation.Refund != nil {
		return service.applyBatchRefund(ctx, txStore, accountID, *operation.Refund)
	}
	return Entry{}, errors.New("unknown_batch_operation")
}

func (service *Service) applyBatchGrant(ctx context.Context, txStore Store, accountID AccountID, operation BatchGrantOperation) (Entry, error) {
	entryInput, err := NewEntryInput(
		accountID,
		EntryGrant,
		operation.Amount.ToEntryAmountCents(),
		nil,
		nil,
		operation.IdempotencyKey,
		operation.ExpiresAtUnixUTC,
		operation.Metadata,
		service.nowFn(),
	)
	if err != nil {
		return Entry{}, err
	}
	return txStore.InsertEntry(ctx, entryInput)
}

func (service *Service) applyBatchSpend(ctx context.Context, txStore Store, accountID AccountID, operation BatchSpendOperation) (Entry, error) {
	nowUnixUTC := service.nowFn()
	total, err := txStore.SumTotal(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Entry{}, err
	}
	holds, err := txStore.SumActiveHolds(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Entry{}, err
	}
	available := calculateAvailable(total, holds)
	amountCents := operation.Amount.ToAmountCents()
	if available.Int64() < amountCents.Int64() {
		return Entry{}, ErrInsufficientFunds
	}
	entryInput, err := NewEntryInput(
		accountID,
		EntrySpend,
		operation.Amount.ToEntryAmountCents().Negated(),
		nil,
		nil,
		operation.IdempotencyKey,
		0,
		operation.Metadata,
		nowUnixUTC,
	)
	if err != nil {
		return Entry{}, err
	}
	return txStore.InsertEntry(ctx, entryInput)
}

func (service *Service) applyBatchReserve(ctx context.Context, txStore Store, accountID AccountID, operation BatchReserveOperation) (Entry, error) {
	nowUnixUTC := service.nowFn()
	total, err := txStore.SumTotal(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Entry{}, err
	}
	holds, err := txStore.SumActiveHolds(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Entry{}, err
	}
	available := calculateAvailable(total, holds)
	amountCents := operation.Amount.ToAmountCents()
	if available.Int64() < amountCents.Int64() {
		return Entry{}, ErrInsufficientFunds
	}
	reservation, err := NewReservation(accountID, operation.ReservationID, operation.Amount, ReservationStatusActive)
	if err != nil {
		return Entry{}, err
	}
	if err := txStore.CreateReservation(ctx, reservation); err != nil {
		return Entry{}, err
	}
	entryInput, err := NewEntryInput(
		accountID,
		EntryHold,
		operation.Amount.ToEntryAmountCents().Negated(),
		&operation.ReservationID,
		nil,
		operation.IdempotencyKey,
		0,
		operation.Metadata,
		nowUnixUTC,
	)
	if err != nil {
		return Entry{}, err
	}
	return txStore.InsertEntry(ctx, entryInput)
}

func (service *Service) applyBatchCapture(ctx context.Context, txStore Store, accountID AccountID, operation BatchCaptureOperation) (Entry, error) {
	reservation, err := txStore.GetReservation(ctx, accountID, operation.ReservationID)
	if err != nil {
		return Entry{}, err
	}
	if reservation.Status() != ReservationStatusActive {
		return Entry{}, ErrReservationClosed
	}
	if reservation.AmountCents() != operation.Amount {
		return Entry{}, fmt.Errorf("%w: capture amount mismatch", ErrInvalidAmountCents)
	}
	if err := txStore.UpdateReservationStatus(ctx, accountID, operation.ReservationID, ReservationStatusActive, ReservationStatusCaptured); err != nil {
		return Entry{}, err
	}
	nowUnixUTC := service.nowFn()
	reverseKey, err := deriveIdempotencyKey(operation.IdempotencyKey, idempotencySuffixReverse)
	if err != nil {
		return Entry{}, err
	}
	reverseEntry, err := NewEntryInput(
		accountID,
		EntryReverseHold,
		reservation.AmountCents().ToEntryAmountCents(),
		&operation.ReservationID,
		nil,
		reverseKey,
		0,
		operation.Metadata,
		nowUnixUTC,
	)
	if err != nil {
		return Entry{}, err
	}
	if _, err := txStore.InsertEntry(ctx, reverseEntry); err != nil {
		return Entry{}, err
	}
	spendKey, err := deriveIdempotencyKey(operation.IdempotencyKey, idempotencySuffixSpend)
	if err != nil {
		return Entry{}, err
	}
	spendEntry, err := NewEntryInput(
		accountID,
		EntrySpend,
		operation.Amount.ToEntryAmountCents().Negated(),
		&operation.ReservationID,
		nil,
		spendKey,
		0,
		operation.Metadata,
		nowUnixUTC,
	)
	if err != nil {
		return Entry{}, err
	}
	return txStore.InsertEntry(ctx, spendEntry)
}

func (service *Service) applyBatchRelease(ctx context.Context, txStore Store, accountID AccountID, operation BatchReleaseOperation) (Entry, error) {
	reservation, err := txStore.GetReservation(ctx, accountID, operation.ReservationID)
	if err != nil {
		return Entry{}, err
	}
	if reservation.Status() != ReservationStatusActive {
		return Entry{}, ErrReservationClosed
	}
	if err := txStore.UpdateReservationStatus(ctx, accountID, operation.ReservationID, ReservationStatusActive, ReservationStatusReleased); err != nil {
		return Entry{}, err
	}
	entryInput, err := NewEntryInput(
		accountID,
		EntryReverseHold,
		reservation.AmountCents().ToEntryAmountCents(),
		&operation.ReservationID,
		nil,
		operation.IdempotencyKey,
		0,
		operation.Metadata,
		service.nowFn(),
	)
	if err != nil {
		return Entry{}, err
	}
	return txStore.InsertEntry(ctx, entryInput)
}

func (service *Service) applyBatchRefund(ctx context.Context, txStore Store, accountID AccountID, operation BatchRefundOperation) (Entry, error) {
	existingEntry, err := txStore.GetEntryByIdempotencyKey(ctx, accountID, operation.IdempotencyKey)
	if err == nil {
		if existingEntry.Type() != EntryRefund {
			return Entry{}, fmt.Errorf("%w: existing entry is %s", ErrIdempotencyKeyConflict, existingEntry.Type())
		}
		return existingEntry, ErrDuplicateIdempotencyKey
	}
	if !errors.Is(err, ErrUnknownEntry) {
		return Entry{}, err
	}

	var originalEntry Entry
	if operation.OriginalEntryID != nil {
		originalEntry, err = txStore.GetEntry(ctx, accountID, *operation.OriginalEntryID)
	} else if operation.OriginalIdempotencyKey != nil {
		originalEntry, err = txStore.GetEntryByIdempotencyKey(ctx, accountID, *operation.OriginalIdempotencyKey)
	} else {
		return Entry{}, errors.New("missing_refund_original")
	}
	if err != nil {
		return Entry{}, err
	}

	reservationID, hasReservation := originalEntry.ReservationID()
	var reservationRef *ReservationID
	if hasReservation {
		reservationRef = &reservationID
	}

	if originalEntry.Type() != EntrySpend || originalEntry.AmountCents().Int64() >= 0 {
		return Entry{}, ErrInvalidRefundOriginal
	}

	refunded, err := txStore.SumRefunds(ctx, accountID, originalEntry.EntryID())
	if err != nil {
		return Entry{}, err
	}
	debitAmount, err := NewAmountCents(-originalEntry.AmountCents().Int64())
	if err != nil {
		return Entry{}, err
	}
	if refunded.Int64()+operation.Amount.ToAmountCents().Int64() > debitAmount.Int64() {
		return Entry{}, ErrRefundExceedsDebit
	}

	refundOfEntryID := originalEntry.EntryID()
	entryInput, err := NewEntryInput(
		accountID,
		EntryRefund,
		operation.Amount.ToEntryAmountCents(),
		reservationRef,
		&refundOfEntryID,
		operation.IdempotencyKey,
		0,
		operation.Metadata,
		service.nowFn(),
	)
	if err != nil {
		return Entry{}, err
	}

	persistedEntry, err := txStore.InsertEntry(ctx, entryInput)
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		return persistedEntry, err
	}

	existingEntry, lookupErr := txStore.GetEntryByIdempotencyKey(ctx, accountID, operation.IdempotencyKey)
	if lookupErr != nil {
		return Entry{}, lookupErr
	}
	if existingEntry.Type() != EntryRefund {
		return Entry{}, fmt.Errorf("%w: existing entry is %s", ErrIdempotencyKeyConflict, existingEntry.Type())
	}
	return existingEntry, ErrDuplicateIdempotencyKey
}
