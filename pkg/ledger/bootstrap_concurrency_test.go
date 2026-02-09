package ledger

import (
	"context"
	"sync"
	"testing"
)

type concurrentBootstrapStore struct {
	mutex                sync.Mutex
	accountID            AccountID
	entriesByID          map[EntryID]Entry
	entriesByIdempotency map[IdempotencyKey]Entry
	total                SignedAmountCents
}

func newConcurrentBootstrapStore(test *testing.T) *concurrentBootstrapStore {
	test.Helper()
	return &concurrentBootstrapStore{
		accountID:            mustAccountID(test, "acct-1"),
		entriesByID:          make(map[EntryID]Entry),
		entriesByIdempotency: make(map[IdempotencyKey]Entry),
	}
}

func (store *concurrentBootstrapStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *concurrentBootstrapStore) GetOrCreateAccountID(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) (AccountID, error) {
	return store.accountID, nil
}

func (store *concurrentBootstrapStore) InsertEntry(ctx context.Context, entryInput EntryInput) (Entry, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, exists := store.entriesByIdempotency[entryInput.IdempotencyKey()]; exists {
		return Entry{}, ErrDuplicateIdempotencyKey
	}
	entryID, err := NewEntryID(entryInput.IdempotencyKey().String())
	if err != nil {
		return Entry{}, err
	}
	var reservationID *ReservationID
	if value, ok := entryInput.ReservationID(); ok {
		reservationID = &value
	}
	var refundOfEntryID *EntryID
	if value, ok := entryInput.RefundOfEntryID(); ok {
		refundOfEntryID = &value
	}
	entry, err := NewEntry(
		entryID,
		entryInput.AccountID(),
		entryInput.Type(),
		entryInput.AmountCents(),
		reservationID,
		refundOfEntryID,
		entryInput.IdempotencyKey(),
		entryInput.ExpiresAtUnixUTC(),
		entryInput.MetadataJSON(),
		entryInput.CreatedUnixUTC(),
	)
	if err != nil {
		return Entry{}, err
	}
	store.entriesByIdempotency[entryInput.IdempotencyKey()] = entry
	store.entriesByID[entry.EntryID()] = entry
	switch entryInput.Type() {
	case EntryGrant, EntrySpend, EntryRefund:
		store.total = applyEntryDelta(store.total, entryInput.AmountCents())
	}
	return entry, nil
}

func (store *concurrentBootstrapStore) GetEntry(ctx context.Context, accountID AccountID, entryID EntryID) (Entry, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	entry, ok := store.entriesByID[entryID]
	if !ok {
		return Entry{}, ErrUnknownEntry
	}
	return entry, nil
}

func (store *concurrentBootstrapStore) GetEntryByIdempotencyKey(ctx context.Context, accountID AccountID, idempotencyKey IdempotencyKey) (Entry, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	entry, ok := store.entriesByIdempotency[idempotencyKey]
	if !ok {
		return Entry{}, ErrUnknownEntry
	}
	return entry, nil
}

func (store *concurrentBootstrapStore) SumRefunds(ctx context.Context, accountID AccountID, originalEntryID EntryID) (AmountCents, error) {
	return NewAmountCents(0)
}

func (store *concurrentBootstrapStore) SumTotal(ctx context.Context, accountID AccountID, atUnixUTC int64) (SignedAmountCents, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.total, nil
}

func (store *concurrentBootstrapStore) SumActiveHolds(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error) {
	return NewAmountCents(0)
}

func (store *concurrentBootstrapStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	return nil
}

func (store *concurrentBootstrapStore) GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error) {
	return Reservation{}, ErrUnknownReservation
}

func (store *concurrentBootstrapStore) UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from, to ReservationStatus) error {
	return ErrReservationClosed
}

func (store *concurrentBootstrapStore) ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int, filter ListEntriesFilter) ([]Entry, error) {
	return nil, nil
}

func TestBootstrapGrantIsIdempotentUnderConcurrentBalanceCalls(test *testing.T) {
	test.Parallel()
	store := newConcurrentBootstrapStore(test)
	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "bootstrap-user")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	var waitGroup sync.WaitGroup
	errorsByCall := make([]error, 2)
	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			balance, err := service.Balance(context.Background(), tenantID, userID, ledgerID)
			if err != nil {
				errorsByCall[index] = err
				return
			}
			if balance.TotalCents != 1000 || balance.AvailableCents != 1000 {
				errorsByCall[index] = ErrInvalidBalance
				return
			}
		}(index)
	}
	waitGroup.Wait()
	for _, err := range errorsByCall {
		if err != nil {
			test.Fatalf("unexpected error: %v", err)
		}
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if got := len(store.entriesByIdempotency); got != 1 {
		test.Fatalf("expected 1 bootstrap entry, got %d", got)
	}
}
