package ledger

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestRefundByEntryIDEntryCreditsBalanceAndReferencesOriginalDebit(test *testing.T) {
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

	refundEntry, err := service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	if refundEntry.Type() != EntryRefund {
		test.Fatalf("expected refund entry type, got %s", refundEntry.Type())
	}
	if refundEntry.AmountCents().Int64() != 50 {
		test.Fatalf("expected refund amount 50, got %d", refundEntry.AmountCents().Int64())
	}
	refundOfEntryID, ok := refundEntry.RefundOfEntryID()
	if !ok || refundOfEntryID != spendEntry.EntryID() {
		test.Fatalf("expected refund_of_entry_id=%s, got %v", spendEntry.EntryID().String(), refundOfEntryID.String())
	}
	if store.total.Int64() != 850 {
		test.Fatalf("expected total 850, got %d", store.total.Int64())
	}
}

func TestRefundByEntryIDWrapperCreditsBalance(test *testing.T) {
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

	if err := service.RefundByEntryID(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("refund: %v", err)
	}
	if store.total.Int64() != 850 {
		test.Fatalf("expected total 850, got %d", store.total.Int64())
	}
}

func TestRefundByEntryIDEntryReferencesReservationWhenOriginalHasReservation(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	metadata := mustMetadata(test, "{}")

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, metadata); err != nil {
		test.Fatalf("grant: %v", err)
	}
	reservationID := mustReservationID(test, "order-1")
	reservationAmount := mustPositiveAmount(test, 200)
	if err := service.Reserve(context.Background(), tenantID, userID, ledgerID, reservationAmount, reservationID, mustIdempotencyKey(test, "reserve-1"), 0, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	captureDebitEntry, err := service.CaptureDebitEntry(context.Background(), tenantID, userID, ledgerID, reservationID, mustIdempotencyKey(test, "capture-1"), reservationAmount, metadata)
	if err != nil {
		test.Fatalf("capture: %v", err)
	}

	refundEntry, err := service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, captureDebitEntry.EntryID(), reservationAmount, mustIdempotencyKey(test, "refund-1"), metadata)
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	linkedReservationID, ok := refundEntry.ReservationID()
	if !ok || linkedReservationID != reservationID {
		test.Fatalf("expected reservation id %s, got %v", reservationID.String(), linkedReservationID.String())
	}
	refundOfEntryID, ok := refundEntry.RefundOfEntryID()
	if !ok || refundOfEntryID != captureDebitEntry.EntryID() {
		test.Fatalf("expected refund_of_entry_id %s, got %v", captureDebitEntry.EntryID().String(), refundOfEntryID.String())
	}
	if store.total.Int64() != 1000 {
		test.Fatalf("expected total 1000, got %d", store.total.Int64())
	}
}

func TestRefundByOriginalIdempotencyKeyEntryResolvesOriginalAndCreditsBalance(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if _, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("spend: %v", err)
	}

	refundEntry, err := service.RefundByOriginalIdempotencyKeyEntry(context.Background(), tenantID, userID, ledgerID, mustIdempotencyKey(test, "spend-1"), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	if refundEntry.Type() != EntryRefund {
		test.Fatalf("expected refund entry type, got %s", refundEntry.Type())
	}
	if store.total.Int64() != 850 {
		test.Fatalf("expected total 850, got %d", store.total.Int64())
	}
}

func TestRefundByOriginalIdempotencyKeyEntryReturnsUnknownEntryWhenOriginalIsMissing(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	_, err := service.RefundByOriginalIdempotencyKeyEntry(context.Background(), tenantID, userID, ledgerID, mustIdempotencyKey(test, "missing-spend"), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestRefundByOriginalIdempotencyKeyEntryReturnsErrorWhenAccountLookupFails(test *testing.T) {
	test.Parallel()
	sentinelError := errors.New("account lookup failed")
	store := newStubStore(test, mustSignedAmount(test, 0))
	store.getAccountError = sentinelError
	service := mustNewService(test, store)

	_, err := service.RefundByOriginalIdempotencyKeyEntry(context.Background(), mustTenantID(test, defaultTenantIDValue), mustUserID(test, "user-1"), mustLedgerID(test, defaultLedgerIDValue), mustIdempotencyKey(test, "spend-1"), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestRefundByOriginalIdempotencyKeyWrapperCreditsBalance(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if _, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 200), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("spend: %v", err)
	}

	if err := service.RefundByOriginalIdempotencyKey(context.Background(), tenantID, userID, ledgerID, mustIdempotencyKey(test, "spend-1"), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("refund: %v", err)
	}
	if store.total.Int64() != 850 {
		test.Fatalf("expected total 850, got %d", store.total.Int64())
	}
}

func TestRefundDuplicateIdempotencyKeyReturnsExistingEntry(test *testing.T) {
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

	refundKey := mustIdempotencyKey(test, "refund-1")
	firstRefund, err := service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 50), refundKey, mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	beforeTotal := store.total.Int64()
	beforeEntries := len(store.entries)

	secondRefund, err := service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 50), refundKey, mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("refund duplicate: %v", err)
	}
	if secondRefund.EntryID() != firstRefund.EntryID() {
		test.Fatalf("expected same entry id %s, got %s", firstRefund.EntryID().String(), secondRefund.EntryID().String())
	}
	if store.total.Int64() != beforeTotal {
		test.Fatalf("expected total unchanged %d, got %d", beforeTotal, store.total.Int64())
	}
	if len(store.entries) != beforeEntries {
		test.Fatalf("expected entries unchanged %d, got %d", beforeEntries, len(store.entries))
	}
}

func TestRefundReturnsDuplicateIdempotencyKeyWhenExistingEntryIsNotRefund(test *testing.T) {
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

	if _, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("spend occupying refund idempotency: %v", err)
	}

	_, err = service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected ErrDuplicateIdempotencyKey, got %v", err)
	}
}

func TestRefundRejectsNonDebitOriginal(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	grantEntry, err := service.GrantEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("grant: %v", err)
	}
	_, err = service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, grantEntry.EntryID(), mustPositiveAmount(test, 50), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, ErrInvalidRefundOriginal) {
		test.Fatalf("expected ErrInvalidRefundOriginal, got %v", err)
	}
}

func TestRefundReturnsErrorWhenIdempotencyLookupFailsWithNonUnknownError(test *testing.T) {
	test.Parallel()
	sentinelError := errors.New("idempotency lookup failed")
	store := newFailingStore(test, sentinelError)
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	err := service.RefundByEntryID(context.Background(), tenantID, userID, ledgerID, mustEntryID(test, "entry-1"), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestRefundRejectsOverRefund(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 100), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}
	if _, err := service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 80), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}")); err != nil {
		test.Fatalf("refund: %v", err)
	}
	beforeTotal := store.total.Int64()
	beforeEntries := len(store.entries)

	_, err = service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 30), mustIdempotencyKey(test, "refund-2"), mustMetadata(test, "{}"))
	if !errors.Is(err, ErrRefundExceedsDebit) {
		test.Fatalf("expected ErrRefundExceedsDebit, got %v", err)
	}
	if store.total.Int64() != beforeTotal {
		test.Fatalf("expected total unchanged %d, got %d", beforeTotal, store.total.Int64())
	}
	if len(store.entries) != beforeEntries {
		test.Fatalf("expected entries unchanged %d, got %d", beforeEntries, len(store.entries))
	}
}

func TestRefundInsertDuplicateIdempotencyFetchesExistingRefundEntry(test *testing.T) {
	test.Parallel()
	store := newInsertDuplicateRefundStore(test, insertDuplicateRefundStoreConfig{
		lookupEntryAfterDuplicate: refundEntryWithType(test, EntryRefund),
		lookupErrorAfterDuplicate: nil,
	})
	service := mustNewService(test, store)

	err := service.RefundByEntryID(context.Background(), mustTenantID(test, defaultTenantIDValue), mustUserID(test, "user-1"), mustLedgerID(test, defaultLedgerIDValue), store.originalEntry.EntryID(), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
}

func TestRefundInsertDuplicateIdempotencyJoinsLookupError(test *testing.T) {
	test.Parallel()
	lookupError := errors.New("lookup failed")
	store := newInsertDuplicateRefundStore(test, insertDuplicateRefundStoreConfig{
		lookupEntryAfterDuplicate: Entry{},
		lookupErrorAfterDuplicate: lookupError,
	})
	service := mustNewService(test, store)

	err := service.RefundByEntryID(context.Background(), mustTenantID(test, defaultTenantIDValue), mustUserID(test, "user-1"), mustLedgerID(test, defaultLedgerIDValue), store.originalEntry.EntryID(), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if err == nil {
		test.Fatalf("expected error")
	}
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected ErrDuplicateIdempotencyKey to be joined, got %v", err)
	}
	if !errors.Is(err, lookupError) {
		test.Fatalf("expected lookup error to be joined, got %v", err)
	}
}

func TestRefundInsertDuplicateIdempotencyRejectsWhenExistingEntryIsNotRefund(test *testing.T) {
	test.Parallel()
	store := newInsertDuplicateRefundStore(test, insertDuplicateRefundStoreConfig{
		lookupEntryAfterDuplicate: refundEntryWithType(test, EntryGrant),
		lookupErrorAfterDuplicate: nil,
	})
	service := mustNewService(test, store)

	err := service.RefundByEntryID(context.Background(), mustTenantID(test, defaultTenantIDValue), mustUserID(test, "user-1"), mustLedgerID(test, defaultLedgerIDValue), store.originalEntry.EntryID(), mustPositiveAmount(test, 1), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected ErrDuplicateIdempotencyKey, got %v", err)
	}
}

func TestRefundConcurrentRequestsCannotOverRefund(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	storeLock := &sync.Mutex{}

	wrappedStore := &lockingStore{Store: store, lock: storeLock}
	service := mustNewService(test, wrappedStore)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-1")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	if err := service.Grant(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 1000), mustIdempotencyKey(test, "grant-1"), 0, mustMetadata(test, "{}")); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendEntry, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, mustPositiveAmount(test, 100), mustIdempotencyKey(test, "spend-1"), mustMetadata(test, "{}"))
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	refundErrors := make([]error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		_, refundErrors[0] = service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 100), mustIdempotencyKey(test, "refund-1"), mustMetadata(test, "{}"))
	}()
	go func() {
		defer waitGroup.Done()
		_, refundErrors[1] = service.RefundByEntryIDEntry(context.Background(), tenantID, userID, ledgerID, spendEntry.EntryID(), mustPositiveAmount(test, 100), mustIdempotencyKey(test, "refund-2"), mustMetadata(test, "{}"))
	}()
	waitGroup.Wait()

	successCount := 0
	for _, err := range refundErrors {
		if err == nil {
			successCount++
			continue
		}
		if !errors.Is(err, ErrRefundExceedsDebit) {
			test.Fatalf("unexpected error: %v", err)
		}
	}
	if successCount != 1 {
		test.Fatalf("expected exactly one refund to succeed, got %d", successCount)
	}
}

type lockingStore struct {
	Store
	lock *sync.Mutex
}

func (store *lockingStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	store.lock.Lock()
	defer store.lock.Unlock()
	return store.Store.WithTx(ctx, fn)
}

type insertDuplicateRefundStoreConfig struct {
	lookupEntryAfterDuplicate Entry
	lookupErrorAfterDuplicate error
}

type insertDuplicateRefundStore struct {
	accountID     AccountID
	originalEntry Entry
	config        insertDuplicateRefundStoreConfig
	lookupCalls   int
}

func newInsertDuplicateRefundStore(test *testing.T, config insertDuplicateRefundStoreConfig) *insertDuplicateRefundStore {
	test.Helper()
	accountID := mustAccountID(test, "acct-1")
	originalIdempotencyKey := mustIdempotencyKey(test, "spend-1")
	metadata := mustMetadata(test, "{}")
	originalEntry, err := NewEntry(
		mustEntryID(test, "entry-1"),
		accountID,
		EntrySpend,
		mustPositiveAmount(test, 10).ToEntryAmountCents().Negated(),
		nil,
		nil,
		originalIdempotencyKey,
		0,
		metadata,
		100,
	)
	if err != nil {
		test.Fatalf("original entry: %v", err)
	}
	return &insertDuplicateRefundStore{
		accountID:     accountID,
		originalEntry: originalEntry,
		config:        config,
	}
}

func (store *insertDuplicateRefundStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *insertDuplicateRefundStore) GetOrCreateAccountID(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) (AccountID, error) {
	return store.accountID, nil
}

func (store *insertDuplicateRefundStore) InsertEntry(ctx context.Context, entry EntryInput) (Entry, error) {
	return Entry{}, ErrDuplicateIdempotencyKey
}

func (store *insertDuplicateRefundStore) GetEntry(ctx context.Context, accountID AccountID, entryID EntryID) (Entry, error) {
	if entryID == store.originalEntry.EntryID() {
		return store.originalEntry, nil
	}
	return Entry{}, ErrUnknownEntry
}

func (store *insertDuplicateRefundStore) GetEntryByIdempotencyKey(ctx context.Context, accountID AccountID, idempotencyKey IdempotencyKey) (Entry, error) {
	if idempotencyKey.String() != "refund-1" {
		return Entry{}, ErrUnknownEntry
	}
	store.lookupCalls++
	if store.lookupCalls == 1 {
		return Entry{}, ErrUnknownEntry
	}
	if store.config.lookupErrorAfterDuplicate == nil {
		return store.config.lookupEntryAfterDuplicate, nil
	}
	return Entry{}, store.config.lookupErrorAfterDuplicate
}

func (store *insertDuplicateRefundStore) SumRefunds(ctx context.Context, accountID AccountID, originalEntryID EntryID) (AmountCents, error) {
	return AmountCents(0), nil
}

func (store *insertDuplicateRefundStore) SumTotal(ctx context.Context, accountID AccountID, atUnixUTC int64) (SignedAmountCents, error) {
	return SignedAmountCents(0), nil
}

func (store *insertDuplicateRefundStore) SumActiveHolds(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error) {
	return AmountCents(0), nil
}

func (store *insertDuplicateRefundStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	return nil
}

func (store *insertDuplicateRefundStore) GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error) {
	return Reservation{}, ErrUnknownReservation
}

func (store *insertDuplicateRefundStore) UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from, to ReservationStatus) error {
	return ErrUnknownReservation
}

func (store *insertDuplicateRefundStore) ListReservations(ctx context.Context, accountID AccountID, beforeCreatedUnixUTC int64, limit int, filter ListReservationsFilter) ([]Reservation, error) {
	return nil, nil
}

func (store *insertDuplicateRefundStore) ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int, filter ListEntriesFilter) ([]Entry, error) {
	return nil, nil
}

func refundEntryWithType(test *testing.T, entryType EntryType) Entry {
	test.Helper()
	accountID := mustAccountID(test, "acct-1")
	metadata := mustMetadata(test, "{}")
	entry, err := NewEntry(
		mustEntryID(test, "refund-entry"),
		accountID,
		entryType,
		mustPositiveAmount(test, 1).ToEntryAmountCents(),
		nil,
		nil,
		mustIdempotencyKey(test, "refund-1"),
		0,
		metadata,
		100,
	)
	if err != nil {
		test.Fatalf("refund entry: %v", err)
	}
	return entry
}
