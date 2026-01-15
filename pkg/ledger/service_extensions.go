package ledger

import "context"

// Spend debits the user's available balance immediately (no hold).
func (service *Service) Spend(requestContext context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
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
		available, err := calculateAvailable(total, holds)
		if err != nil {
			return err
		}
		if available < amount.ToAmountCents() {
			return ErrInsufficientFunds
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntrySpend,
			amount.ToEntryAmountCents().Negated(),
			nil,
			idempotencyKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		return transactionStore.InsertEntry(ctx, entryInput)
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
	return operationError
}

// ListEntries lists ledger entries for a user before a cutoff time.
func (service *Service) ListEntries(requestContext context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, beforeUnixUTC int64, limit int) ([]Entry, error) {
	accountID, err := service.store.GetOrCreateAccountID(requestContext, tenantID, userID, ledgerID)
	if err != nil {
		return nil, err
	}
	return service.store.ListEntries(requestContext, accountID, beforeUnixUTC, limit)
}
