package ledger

import (
	"context"
	"errors"
	"testing"
)

func mustBootstrapPolicy(test *testing.T, amountCents int64) BootstrapGrantPolicy {
	test.Helper()
	tenantID := mustTenantID(test, defaultTenantIDValue)
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	amount := mustPositiveAmount(test, amountCents)
	idempotencyKeyBase := mustIdempotencyKey(test, "bootstrap")
	metadata := mustMetadata(test, `{"reason":"account_bootstrap"}`)
	rule, err := NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		test.Fatalf("rule: %v", err)
	}
	policy, err := NewBootstrapGrantPolicy([]BootstrapGrantRule{rule})
	if err != nil {
		test.Fatalf("policy: %v", err)
	}
	return policy
}

func mustNewServiceWithPolicy(test *testing.T, store Store, policy BootstrapGrantPolicy) *Service {
	test.Helper()
	service, err := NewService(store, func() int64 { return 100 }, WithBootstrapGrantPolicy(policy))
	if err != nil {
		test.Fatalf("new service: %v", err)
	}
	return service
}

func TestBootstrapGrantAppliesOnBalanceForNewAccount(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "bootstrap-user")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	balance, err := service.Balance(context.Background(), tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("balance: %v", err)
	}
	if balance.TotalCents != 1000 || balance.AvailableCents != 1000 {
		test.Fatalf("expected 1000/1000, got total=%d available=%d", balance.TotalCents, balance.AvailableCents)
	}
	if got := len(store.entries); got != 1 {
		test.Fatalf("expected 1 bootstrap entry, got %d", got)
	}
}

func TestBootstrapGrantIsNoopAfterFirstApply(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "bootstrap-user")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	if _, err := service.Balance(context.Background(), tenantID, userID, ledgerID); err != nil {
		test.Fatalf("balance: %v", err)
	}
	if _, err := service.Balance(context.Background(), tenantID, userID, ledgerID); err != nil {
		test.Fatalf("balance second: %v", err)
	}
	if got := len(store.entries); got != 1 {
		test.Fatalf("expected 1 bootstrap entry after retries, got %d", got)
	}
}

func TestBootstrapGrantSkipsWhenAccountHasEntries(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	existingKey := mustIdempotencyKey(test, "existing-grant")
	metadata := mustMetadata(test, "{}")
	entryInput, err := NewEntryInput(
		store.accountID,
		EntryGrant,
		mustEntryAmount(test, 500),
		nil,
		nil,
		existingKey,
		0,
		metadata,
		100,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := store.InsertEntry(context.Background(), entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}

	service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))
	userID := mustUserID(test, "bootstrap-user")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	balance, err := service.Balance(context.Background(), tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("balance: %v", err)
	}
	if balance.TotalCents != 500 || balance.AvailableCents != 500 {
		test.Fatalf("expected 500/500, got total=%d available=%d", balance.TotalCents, balance.AvailableCents)
	}
	if got := len(store.entries); got != 1 {
		test.Fatalf("expected no bootstrap entry for existing account, got %d entries", got)
	}
}

func TestBootstrapGrantRejectsIdempotencyConflicts(test *testing.T) {
	test.Parallel()
	store := newConcurrentBootstrapStore(test)
	idempotencyKeyBase := mustIdempotencyKey(test, "bootstrap")
	bootstrapKey, err := deriveIdempotencyKey(idempotencyKeyBase, bootstrapIdempotencySuffix)
	if err != nil {
		test.Fatalf("bootstrap idempotency: %v", err)
	}
	metadata := mustMetadata(test, "{}")
	entryInput, err := NewEntryInput(
		store.accountID,
		EntrySpend,
		mustEntryAmount(test, -10),
		nil,
		nil,
		bootstrapKey,
		0,
		metadata,
		100,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := store.InsertEntry(context.Background(), entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}

	policy := mustBootstrapPolicy(test, 1000)
	service := mustNewServiceWithPolicy(test, store, policy)
	userID := mustUserID(test, "bootstrap-user")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)

	_, err = service.Balance(context.Background(), tenantID, userID, ledgerID)
	if !errors.Is(err, ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency error, got %v", err)
	}
}

func TestNewBootstrapGrantPolicyRejectsDuplicateScopes(test *testing.T) {
	test.Parallel()
	tenantID := mustTenantID(test, defaultTenantIDValue)
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	amount := mustPositiveAmount(test, 100)
	idempotencyKeyBase := mustIdempotencyKey(test, "bootstrap")
	metadata := mustMetadata(test, "{}")
	rule, err := NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		test.Fatalf("rule: %v", err)
	}
	_, err = NewBootstrapGrantPolicy([]BootstrapGrantRule{rule, rule})
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestNewBootstrapGrantRuleValidatesInputs(test *testing.T) {
	test.Parallel()

	validTenantID := mustTenantID(test, defaultTenantIDValue)
	validLedgerID := mustLedgerID(test, defaultLedgerIDValue)
	validAmount := mustPositiveAmount(test, 100)
	validKey := mustIdempotencyKey(test, "bootstrap")
	validMetadata := mustMetadata(test, "{}")

	testCases := []struct {
		name         string
		tenantID     TenantID
		ledgerID     LedgerID
		amount       PositiveAmountCents
		keyBase      IdempotencyKey
		metadataJSON MetadataJSON
		wantErr      error
	}{
		{
			name:         "invalid tenant id",
			tenantID:     TenantID{},
			ledgerID:     validLedgerID,
			amount:       validAmount,
			keyBase:      validKey,
			metadataJSON: validMetadata,
			wantErr:      ErrInvalidTenantID,
		},
		{
			name:         "invalid ledger id",
			tenantID:     validTenantID,
			ledgerID:     LedgerID{},
			amount:       validAmount,
			keyBase:      validKey,
			metadataJSON: validMetadata,
			wantErr:      ErrInvalidLedgerID,
		},
		{
			name:         "invalid amount",
			tenantID:     validTenantID,
			ledgerID:     validLedgerID,
			amount:       PositiveAmountCents(0),
			keyBase:      validKey,
			metadataJSON: validMetadata,
			wantErr:      ErrInvalidAmountCents,
		},
		{
			name:         "invalid idempotency key base",
			tenantID:     validTenantID,
			ledgerID:     validLedgerID,
			amount:       validAmount,
			keyBase:      IdempotencyKey{},
			metadataJSON: validMetadata,
			wantErr:      ErrInvalidIdempotencyKey,
		},
		{
			name:         "invalid metadata json",
			tenantID:     validTenantID,
			ledgerID:     validLedgerID,
			amount:       validAmount,
			keyBase:      validKey,
			metadataJSON: MetadataJSON{},
			wantErr:      ErrInvalidMetadataJSON,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := NewBootstrapGrantRule(testCase.tenantID, testCase.ledgerID, testCase.amount, testCase.keyBase, testCase.metadataJSON)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("expected %v, got %v", testCase.wantErr, err)
			}
		})
	}
}

func TestNewBootstrapGrantPolicyAllowsEmpty(test *testing.T) {
	test.Parallel()
	policy, err := NewBootstrapGrantPolicy(nil)
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if _, ok := policy.ruleFor(mustTenantID(test, defaultTenantIDValue), mustLedgerID(test, defaultLedgerIDValue)); ok {
		test.Fatalf("expected no rules")
	}
}

func TestBootstrapGrantPropagatesStoreErrors(test *testing.T) {
	test.Parallel()

	testCases := []struct {
		name      string
		setup     func(store *stubStore)
		wantError func(err error) bool
	}{
		{
			name: "get or create account id fails",
			setup: func(store *stubStore) {
				store.getAccountError = errors.New("get account failed")
			},
			wantError: func(err error) bool {
				return err != nil && err.Error() == "get account failed"
			},
		},
		{
			name: "list entries fails",
			setup: func(store *stubStore) {
				store.listErr = errors.New("list entries failed")
			},
			wantError: func(err error) bool {
				return err != nil && err.Error() == "list entries failed"
			},
		},
		{
			name: "insert entry fails (non-duplicate)",
			setup: func(store *stubStore) {
				store.insertEntryError = errors.New("insert failed")
			},
			wantError: func(err error) bool {
				return err != nil && err.Error() == "insert failed"
			},
		},
		{
			name: "duplicate idempotency with lookup failure returns joined error",
			setup: func(store *stubStore) {
				idempotencyKeyBase := mustIdempotencyKey(test, "bootstrap")
				bootstrapKey, err := deriveIdempotencyKey(idempotencyKeyBase, bootstrapIdempotencySuffix)
				if err != nil {
					test.Fatalf("derive: %v", err)
				}
				store.idempotency[bootstrapKey] = struct{}{}
			},
			wantError: func(err error) bool {
				return errors.Is(err, ErrDuplicateIdempotencyKey) && errors.Is(err, ErrUnknownEntry)
			},
		},
		{
			name: "duplicate idempotency with existing grant is treated as no-op",
			setup: func(store *stubStore) {
				store.listEntries = []Entry{} // Force insert attempt instead of skipping due to existing entries.

				idempotencyKeyBase := mustIdempotencyKey(test, "bootstrap")
				bootstrapKey, err := deriveIdempotencyKey(idempotencyKeyBase, bootstrapIdempotencySuffix)
				if err != nil {
					test.Fatalf("derive: %v", err)
				}
				store.idempotency[bootstrapKey] = struct{}{}

				entryInput, err := NewEntryInput(
					store.accountID,
					EntryGrant,
					mustEntryAmount(test, 123),
					nil,
					nil,
					bootstrapKey,
					0,
					mustMetadata(test, "{}"),
					100,
				)
				if err != nil {
					test.Fatalf("entry input: %v", err)
				}
				store.entries = append(store.entries, entryInput)
			},
			wantError: func(err error) bool {
				return err == nil
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()

			store := newStubStore(test, mustSignedAmount(test, 0))
			testCase.setup(store)
			service := mustNewServiceWithPolicy(test, store, mustBootstrapPolicy(test, 1000))

			userID := mustUserID(test, "bootstrap-user")
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			_, err := service.Balance(context.Background(), tenantID, userID, ledgerID)
			if !testCase.wantError(err) {
				test.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
