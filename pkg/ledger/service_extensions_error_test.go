package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestSpendEntryReturnsBootstrapError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 100))
	sentinel := errors.New("get account failed")
	store.getAccountError = sentinel

	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 10)
	idempotencyKey := mustIdempotencyKey(test, "spend-1")
	metadata := mustMetadata(test, "{}")

	_, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, metadata)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestSpendEntryPropagatesEntryInputValidationErrors(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 100))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 10)
	idempotencyKey := mustIdempotencyKey(test, "spend-1")

	_, err := service.SpendEntry(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, MetadataJSON{})
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}

func TestListEntriesReturnsBootstrapError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	sentinel := errors.New("get account failed")
	store.getAccountError = sentinel

	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	_, err := service.ListEntries(context.Background(), tenantID, userID, ledgerID, 1893456000, 10, ListEntriesFilter{})
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestGrantEntryReturnsBootstrapError(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	sentinel := errors.New("get account failed")
	store.getAccountError = sentinel

	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 10)
	idempotencyKey := mustIdempotencyKey(test, "grant-1")
	metadata := mustMetadata(test, "{}")

	_, err := service.GrantEntry(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, 0, metadata)
	if !errors.Is(err, sentinel) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestGrantEntryPropagatesEntryInputValidationErrors(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 10)
	idempotencyKey := mustIdempotencyKey(test, "grant-1")

	_, err := service.GrantEntry(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, 0, MetadataJSON{})
	if !errors.Is(err, ErrInvalidMetadataJSON) {
		test.Fatalf("expected ErrInvalidMetadataJSON, got %v", err)
	}
}
