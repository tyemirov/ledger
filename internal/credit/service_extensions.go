package credit

import "context"

// Spend debits the user's available balance immediately (no hold).
func (service *Service) Spend(requestContext context.Context, userID string, amount AmountCents, idempotencyKey string, metadataJSON string) error {
	return service.store.WithTx(requestContext, func(ctx context.Context, transactionStore Store) error {
		accountID, accountError := transactionStore.GetOrCreateAccountID(ctx, userID)
		if accountError != nil {
			return accountError
		}
		nowUnix := service.nowFn()
		total, totalError := transactionStore.SumTotal(ctx, accountID, nowUnix)
		if totalError != nil {
			return totalError
		}
		activeHolds, holdsError := transactionStore.SumActiveHolds(ctx, accountID, nowUnix)
		if holdsError != nil {
			return holdsError
		}
		available := total + activeHolds
		if available < amount {
			return ErrInsufficientFunds
		}
		return transactionStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntrySpend,
			AmountCents:    -amount,
			IdempotencyKey: idempotencyKey,
			MetadataJSON:   metadataJSON,
			CreatedUnixUTC: nowUnix,
		})
	})
}

// ListEntries lists ledger entries for a user before a cutoff time.
func (service *Service) ListEntries(requestContext context.Context, userID string, beforeUnixUTC int64, limit int) ([]Entry, error) {
	accountID, accountError := service.store.GetOrCreateAccountID(requestContext, userID)
	if accountError != nil {
		return nil, accountError
	}
	return service.store.ListEntries(requestContext, accountID, beforeUnixUTC, limit)
}
