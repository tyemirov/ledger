package ledger

import (
	"context"
	"errors"
)

func (service *Service) applyBootstrapGrantIfEligible(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) error {
	rule, ok := service.bootstrapPolicy.ruleFor(tenantID, ledgerID)
	if !ok {
		return nil
	}
	return service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		beforeUnixUTC := service.nowFn() + 1
		entries, err := transactionStore.ListEntries(ctx, accountID, beforeUnixUTC, 1, ListEntriesFilter{})
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		bootstrapIdempotencyKey, err := deriveIdempotencyKey(rule.IdempotencyKeyBase(), bootstrapIdempotencySuffix)
		if err != nil {
			return err
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntryGrant,
			rule.Amount().ToEntryAmountCents(),
			nil,
			nil,
			bootstrapIdempotencyKey,
			0,
			rule.Metadata(),
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		_, insertErr := transactionStore.InsertEntry(ctx, entryInput)
		if insertErr == nil {
			return nil
		}
		if !errors.Is(insertErr, ErrDuplicateIdempotencyKey) {
			return insertErr
		}
		existingEntry, lookupErr := transactionStore.GetEntryByIdempotencyKey(ctx, accountID, bootstrapIdempotencyKey)
		if lookupErr != nil {
			return errors.Join(insertErr, lookupErr)
		}
		if existingEntry.Type() != EntryGrant {
			return ErrDuplicateIdempotencyKey
		}
		return nil
	})
}
