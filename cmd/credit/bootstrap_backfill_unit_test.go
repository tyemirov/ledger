package main

import (
	"context"
	"errors"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
)

type stubLedgerStore struct {
	withTxFn               func(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error
	getEntryByIdemFn       func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error)
	insertEntryFn          func(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error)
	getOrCreateAccountIDFn func(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error)
}

func (store *stubLedgerStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	if store.withTxFn != nil {
		return store.withTxFn(ctx, fn)
	}
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *stubLedgerStore) GetOrCreateAccountID(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error) {
	if store.getOrCreateAccountIDFn == nil {
		return ledger.AccountID{}, errors.New("not implemented")
	}
	return store.getOrCreateAccountIDFn(ctx, tenantID, userID, ledgerID)
}

func (store *stubLedgerStore) InsertEntry(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
	if store.insertEntryFn == nil {
		return ledger.Entry{}, errors.New("not implemented")
	}
	return store.insertEntryFn(ctx, entry)
}

func (store *stubLedgerStore) GetEntry(ctx context.Context, accountID ledger.AccountID, entryID ledger.EntryID) (ledger.Entry, error) {
	return ledger.Entry{}, errors.New("not implemented")
}

func (store *stubLedgerStore) GetEntryByIdempotencyKey(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
	if store.getEntryByIdemFn == nil {
		return ledger.Entry{}, errors.New("not implemented")
	}
	return store.getEntryByIdemFn(ctx, accountID, idempotencyKey)
}

func (store *stubLedgerStore) SumRefunds(ctx context.Context, accountID ledger.AccountID, originalEntryID ledger.EntryID) (ledger.AmountCents, error) {
	return 0, errors.New("not implemented")
}

func (store *stubLedgerStore) SumTotal(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.SignedAmountCents, error) {
	return 0, errors.New("not implemented")
}

func (store *stubLedgerStore) SumActiveHolds(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.AmountCents, error) {
	return 0, errors.New("not implemented")
}

func (store *stubLedgerStore) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	return errors.New("not implemented")
}

func (store *stubLedgerStore) GetReservation(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID) (ledger.Reservation, error) {
	return ledger.Reservation{}, errors.New("not implemented")
}

func (store *stubLedgerStore) UpdateReservationStatus(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID, from, to ledger.ReservationStatus) error {
	return errors.New("not implemented")
}

func (store *stubLedgerStore) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) ([]ledger.Entry, error) {
	return nil, errors.New("not implemented")
}

func mustBootstrapRule(test *testing.T) ledger.BootstrapGrantRule {
	test.Helper()
	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(1000)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKeyBase, err := ledger.NewIdempotencyKey("bootstrap")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	rule, err := ledger.NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		test.Fatalf("rule: %v", err)
	}
	return rule
}

func mustEntry(test *testing.T, accountID ledger.AccountID, entryType ledger.EntryType, idempotencyValue string) ledger.Entry {
	entryID, err := ledger.NewEntryID("entry-" + idempotencyValue)
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey(idempotencyValue)
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	amountCents, err := ledger.NewEntryAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	returnEntry, err := ledger.NewEntry(entryID, accountID, entryType, amountCents, nil, nil, idempotencyKey, 0, metadata, 1700000000)
	if err != nil {
		test.Fatalf("entry: %v", err)
	}
	return returnEntry
}

func TestApplyBootstrapGrantToAccountAppliesWhenMissing(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrUnknownEntry
		},
		insertEntryFn: func(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
			return mustEntry(test, accountID, ledger.EntryGrant, "bootstrap"), nil
		},
	}

	applied, err := applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if !applied {
		test.Fatalf("expected applied=true")
	}
}

func TestApplyBootstrapGrantToAccountSkipsWhenAlreadyPresent(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return mustEntry(test, accountID, ledger.EntryGrant, "bootstrap"), nil
		},
	}

	applied, err := applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if applied {
		test.Fatalf("expected applied=false")
	}
}

func TestApplyBootstrapGrantToAccountRejectsNonGrantIdempotencyCollisions(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return mustEntry(test, accountID, ledger.EntrySpend, "bootstrap"), nil
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if !errors.Is(err, ledger.ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency error, got %v", err)
	}
}

func TestApplyBootstrapGrantToAccountPropagatesLookupErrors(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)
	sentinel := errors.New("lookup failed")

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return ledger.Entry{}, sentinel
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestApplyBootstrapGrantToAccountPropagatesInsertErrors(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)
	sentinel := errors.New("insert failed")

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrUnknownEntry
		},
		insertEntryFn: func(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
			return ledger.Entry{}, sentinel
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestApplyBootstrapGrantToAccountTreatsDuplicateInsertAsNoopWhenExistingGrant(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	lookupCalls := 0
	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			lookupCalls++
			if lookupCalls == 1 {
				return ledger.Entry{}, ledger.ErrUnknownEntry
			}
			return mustEntry(test, accountID, ledger.EntryGrant, "bootstrap"), nil
		},
		insertEntryFn: func(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrDuplicateIdempotencyKey
		},
	}

	applied, err := applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if applied {
		test.Fatalf("expected applied=false")
	}
}

func TestApplyBootstrapGrantToAccountReturnsJoinedErrorWhenDuplicateInsertLookupFails(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrUnknownEntry
		},
		insertEntryFn: func(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrDuplicateIdempotencyKey
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if !errors.Is(err, ledger.ErrDuplicateIdempotencyKey) || !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected joined duplicate+unknown error, got %v", err)
	}
}

func TestApplyBootstrapGrantToAccountPropagatesEntryInputValidationErrors(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}

	store := &stubLedgerStore{
		getEntryByIdemFn: func(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
			return ledger.Entry{}, ledger.ErrUnknownEntry
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, ledger.BootstrapGrantRule{}, func() int64 { return 1700000000 })
	if !errors.Is(err, ledger.ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

func TestApplyBootstrapGrantToAccountPropagatesTransactionErrors(test *testing.T) {
	test.Parallel()
	accountID, err := ledger.NewAccountID("acct-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	rule := mustBootstrapRule(test)

	sentinel := errors.New("tx failed")
	store := &stubLedgerStore{
		withTxFn: func(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
			return sentinel
		},
	}

	_, err = applyBootstrapGrantToAccount(context.Background(), store, accountID, rule, func() int64 { return 1700000000 })
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}
