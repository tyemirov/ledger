package ledger

import (
	"errors"
	"testing"
)

func TestNewAccountSummaryAcceptsValidInput(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test, "acct-1")
	tenantID := mustTenantID(test, defaultTenantIDValue)
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)

	account, err := NewAccountSummary(accountID, tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if account.AccountID() != accountID {
		test.Fatalf("expected account id %s, got %s", accountID.String(), account.AccountID().String())
	}
	if account.TenantID() != tenantID {
		test.Fatalf("expected tenant id %s, got %s", tenantID.String(), account.TenantID().String())
	}
	if account.UserID() != userID {
		test.Fatalf("expected user id %s, got %s", userID.String(), account.UserID().String())
	}
	if account.LedgerID() != ledgerID {
		test.Fatalf("expected ledger id %s, got %s", ledgerID.String(), account.LedgerID().String())
	}
}

func TestNewAccountSummaryRejectsInvalidInput(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name    string
		account AccountID
		tenant  TenantID
		user    UserID
		ledger  LedgerID
		wantErr error
	}{
		{
			name:    "invalid account id",
			account: AccountID{},
			tenant:  mustTenantID(test, defaultTenantIDValue),
			user:    mustUserID(test, "user-123"),
			ledger:  mustLedgerID(test, defaultLedgerIDValue),
			wantErr: ErrInvalidAccountID,
		},
		{
			name:    "invalid tenant id",
			account: mustAccountID(test, "acct-1"),
			tenant:  TenantID{},
			user:    mustUserID(test, "user-123"),
			ledger:  mustLedgerID(test, defaultLedgerIDValue),
			wantErr: ErrInvalidTenantID,
		},
		{
			name:    "invalid user id",
			account: mustAccountID(test, "acct-1"),
			tenant:  mustTenantID(test, defaultTenantIDValue),
			user:    UserID{},
			ledger:  mustLedgerID(test, defaultLedgerIDValue),
			wantErr: ErrInvalidUserID,
		},
		{
			name:    "invalid ledger id",
			account: mustAccountID(test, "acct-1"),
			tenant:  mustTenantID(test, defaultTenantIDValue),
			user:    mustUserID(test, "user-123"),
			ledger:  LedgerID{},
			wantErr: ErrInvalidLedgerID,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := NewAccountSummary(testCase.account, testCase.tenant, testCase.user, testCase.ledger)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf("expected %v, got %v", testCase.wantErr, err)
			}
		})
	}
}
