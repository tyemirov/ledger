package ledger

import (
	"context"
	"errors"
)

// RefundByEntryID appends a refund credit for an original debit entry (spend/capture debit).
func (service *Service) RefundByEntryID(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, originalEntryID EntryID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	_, err := service.RefundByEntryIDEntry(ctx, tenantID, userID, ledgerID, originalEntryID, amount, idempotencyKey, metadata)
	return err
}

// RefundByEntryIDEntry appends a refund credit for an original debit entry (spend/capture debit) and returns the persisted refund entry.
func (service *Service) RefundByEntryIDEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, originalEntryID EntryID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) (Entry, error) {
	var reservationRef *ReservationID
	var persistedEntry Entry
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}

		existingEntry, err := transactionStore.GetEntryByIdempotencyKey(ctx, accountID, idempotencyKey)
		if err == nil {
			if existingEntry.Type() != EntryRefund {
				return ErrDuplicateIdempotencyKey
			}
			persistedEntry = existingEntry
			return nil
		}
		if !errors.Is(err, ErrUnknownEntry) {
			return err
		}

		originalEntry, err := transactionStore.GetEntry(ctx, accountID, originalEntryID)
		if err != nil {
			return err
		}
		reservationID, hasReservation := originalEntry.ReservationID()
		if hasReservation {
			reservationRef = &reservationID
		}

		if originalEntry.Type() != EntrySpend || originalEntry.AmountCents().Int64() >= 0 {
			return ErrInvalidRefundOriginal
		}

		refunded, err := transactionStore.SumRefunds(ctx, accountID, originalEntry.EntryID())
		if err != nil {
			return err
		}
		debitAmount, err := NewAmountCents(-originalEntry.AmountCents().Int64())
		if err != nil {
			return err
		}
		if refunded.Int64()+amount.ToAmountCents().Int64() > debitAmount.Int64() {
			return ErrRefundExceedsDebit
		}

		refundOfEntryID := originalEntry.EntryID()
		entryInput, err := NewEntryInput(
			accountID,
			EntryRefund,
			amount.ToEntryAmountCents(),
			reservationRef,
			&refundOfEntryID,
			idempotencyKey,
			0,
			metadata,
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, entryInput)
		if errors.Is(err, ErrDuplicateIdempotencyKey) {
			existingEntry, lookupErr := transactionStore.GetEntryByIdempotencyKey(ctx, accountID, idempotencyKey)
			if lookupErr != nil {
				return errors.Join(err, lookupErr)
			}
			if existingEntry.Type() != EntryRefund {
				return ErrDuplicateIdempotencyKey
			}
			persistedEntry = existingEntry
			return nil
		}
		return err
	})

	service.logOperation(ctx, OperationLog{
		Operation:      operationRefund,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		ReservationID:  reservationRef,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})

	if operationError != nil {
		return Entry{}, operationError
	}
	return persistedEntry, nil
}

// RefundByOriginalIdempotencyKey appends a refund credit for an original debit entry referenced by its idempotency key.
func (service *Service) RefundByOriginalIdempotencyKey(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, originalIdempotencyKey IdempotencyKey, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	_, err := service.RefundByOriginalIdempotencyKeyEntry(ctx, tenantID, userID, ledgerID, originalIdempotencyKey, amount, idempotencyKey, metadata)
	return err
}

// RefundByOriginalIdempotencyKeyEntry appends a refund credit for an original debit entry referenced by its idempotency key and returns the persisted refund entry.
func (service *Service) RefundByOriginalIdempotencyKeyEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, originalIdempotencyKey IdempotencyKey, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) (Entry, error) {
	var originalEntryID EntryID
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		entry, err := transactionStore.GetEntryByIdempotencyKey(ctx, accountID, originalIdempotencyKey)
		if err != nil {
			return err
		}
		originalEntryID = entry.EntryID()
		return nil
	})
	if operationError != nil {
		return Entry{}, operationError
	}
	return service.RefundByEntryIDEntry(ctx, tenantID, userID, ledgerID, originalEntryID, amount, idempotencyKey, metadata)
}
