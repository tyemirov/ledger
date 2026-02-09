package ledger

// AccountSummary is a minimally useful account representation for admin workflows.
// It is intentionally small to keep bulk operations (like backfills) efficient.
type AccountSummary struct {
	accountID AccountID
	tenantID  TenantID
	userID    UserID
	ledgerID  LedgerID
}

func NewAccountSummary(accountID AccountID, tenantID TenantID, userID UserID, ledgerID LedgerID) (AccountSummary, error) {
	if err := validateIdentifierValue(accountID.value, ErrInvalidAccountID); err != nil {
		return AccountSummary{}, err
	}
	if err := validateIdentifierValue(tenantID.value, ErrInvalidTenantID); err != nil {
		return AccountSummary{}, err
	}
	if err := validateIdentifierValue(userID.value, ErrInvalidUserID); err != nil {
		return AccountSummary{}, err
	}
	if err := validateIdentifierValue(ledgerID.value, ErrInvalidLedgerID); err != nil {
		return AccountSummary{}, err
	}
	return AccountSummary{
		accountID: accountID,
		tenantID:  tenantID,
		userID:    userID,
		ledgerID:  ledgerID,
	}, nil
}

func (account AccountSummary) AccountID() AccountID {
	return account.accountID
}

func (account AccountSummary) TenantID() TenantID {
	return account.tenantID
}

func (account AccountSummary) UserID() UserID {
	return account.userID
}

func (account AccountSummary) LedgerID() LedgerID {
	return account.ledgerID
}
