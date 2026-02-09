package ledger

import "context"

// Spend debits the user's available balance immediately (no hold).
func (service *Service) Spend(requestContext context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	_, err := service.SpendEntry(requestContext, tenantID, userID, ledgerID, amount, idempotencyKey, metadata)
	return err
}

// SpendEntry debits the user's available balance immediately (no hold) and returns the persisted spend entry.
func (service *Service) SpendEntry(requestContext context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) (Entry, error) {
	var persistedEntry Entry
	operationError := service.store.WithTx(requestContext, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		nowUnixUTC := service.nowFn()
		total, err := transactionStore.SumTotal(ctx, accountID, nowUnixUTC)
		if err != nil {
			return err
		}
		holds, err := transactionStore.SumActiveHolds(ctx, accountID, nowUnixUTC)
		if err != nil {
			return err
		}
		available := calculateAvailable(total, holds)
		amountCents := amount.ToAmountCents()
		if available.Int64() < amountCents.Int64() {
			return ErrInsufficientFunds
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntrySpend,
			amount.ToEntryAmountCents().Negated(),
			nil,
			nil,
			idempotencyKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, entryInput)
		return err
	})
	service.logOperation(requestContext, OperationLog{
		Operation:      operationSpend,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
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

// ListEntries lists ledger entries for a user before a cutoff time.
func (service *Service) ListEntries(requestContext context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, beforeUnixUTC int64, limit int, filter ListEntriesFilter) ([]Entry, error) {
	accountID, err := service.store.GetOrCreateAccountID(requestContext, tenantID, userID, ledgerID)
	if err != nil {
		return nil, err
	}
	return service.store.ListEntries(requestContext, accountID, beforeUnixUTC, limit, filter)
}
