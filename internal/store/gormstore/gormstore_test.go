package gormstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestStoreFlow(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant id: %v", err)
	}
	userID, err := ledger.NewUserID("user-123")
	if err != nil {
		test.Fatalf("user id: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger id: %v", err)
	}

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("get account: %v", err)
	}
	accountIDSecond, err := store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("get account second: %v", err)
	}
	if accountIDSecond.String() != accountID.String() {
		test.Fatalf("expected same account id, got %q vs %q", accountID.String(), accountIDSecond.String())
	}

	amount, err := ledger.NewPositiveAmountCents(1000)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("grant-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	createdUnixUTC := time.Now().UTC().Unix()
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryGrant,
		amount.ToEntryAmountCents(),
		nil,
		nil,
		idempotencyKey,
		0,
		metadata,
		createdUnixUTC,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := store.InsertEntry(ctx, entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if _, err := store.InsertEntry(ctx, entryInput); !errors.Is(err, ledger.ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency error, got %v", err)
	}

	total, err := store.SumTotal(ctx, accountID, time.Now().UTC().Unix())
	if err != nil {
		test.Fatalf("sum total: %v", err)
	}
	if total.Int64() != 1000 {
		test.Fatalf("expected total 1000, got %d", total.Int64())
	}
	holds, err := store.SumActiveHolds(ctx, accountID, time.Now().UTC().Unix())
	if err != nil {
		test.Fatalf("sum holds: %v", err)
	}
	if holds.Int64() != 0 {
		test.Fatalf("expected holds 0, got %d", holds.Int64())
	}

	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, amount, ledger.ReservationStatusActive)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}
	if err := store.CreateReservation(ctx, reservation); err != nil {
		test.Fatalf("create reservation: %v", err)
	}
	if err := store.CreateReservation(ctx, reservation); !errors.Is(err, ledger.ErrReservationExists) {
		test.Fatalf("expected reservation exists error, got %v", err)
	}
	holds, err = store.SumActiveHolds(ctx, accountID, time.Now().UTC().Unix())
	if err != nil {
		test.Fatalf("sum holds after reservation: %v", err)
	}
	if holds.Int64() != 1000 {
		test.Fatalf("expected holds 1000, got %d", holds.Int64())
	}
	gotReservation, err := store.GetReservation(ctx, accountID, reservationID)
	if err != nil {
		test.Fatalf("get reservation: %v", err)
	}
	if gotReservation.Status() != ledger.ReservationStatusActive {
		test.Fatalf("expected active reservation, got %v", gotReservation.Status())
	}
	if err := store.UpdateReservationStatus(ctx, accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured); err != nil {
		test.Fatalf("update reservation: %v", err)
	}
	if err := store.UpdateReservationStatus(ctx, accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured); !errors.Is(err, ledger.ErrReservationClosed) {
		test.Fatalf("expected reservation closed error, got %v", err)
	}

	entries, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(entries) == 0 {
		test.Fatalf("expected entries, got none")
	}
}

func TestStoreListEntriesAppliesFilters(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	createdUnixUTC := time.Now().UTC().Unix()

	grantIdempotencyKey, err := ledger.NewIdempotencyKey("grant-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	grantInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryGrant,
		amount.ToEntryAmountCents(),
		nil,
		nil,
		grantIdempotencyKey,
		0,
		metadata,
		createdUnixUTC,
	)
	if err != nil {
		test.Fatalf("grant input: %v", err)
	}
	grantEntry, err := store.InsertEntry(ctx, grantInput)
	if err != nil {
		test.Fatalf("insert grant: %v", err)
	}

	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	holdIdempotencyKey, err := ledger.NewIdempotencyKey("reserve-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	holdInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryHold,
		amount.ToEntryAmountCents().Negated(),
		&reservationID,
		nil,
		holdIdempotencyKey,
		0,
		metadata,
		createdUnixUTC,
	)
	if err != nil {
		test.Fatalf("hold input: %v", err)
	}
	holdEntry, err := store.InsertEntry(ctx, holdInput)
	if err != nil {
		test.Fatalf("insert hold: %v", err)
	}

	spendIdempotencyKey, err := ledger.NewIdempotencyKey("spend-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	spendInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntrySpend,
		amount.ToEntryAmountCents().Negated(),
		nil,
		nil,
		spendIdempotencyKey,
		0,
		metadata,
		createdUnixUTC,
	)
	if err != nil {
		test.Fatalf("spend input: %v", err)
	}
	spendEntry, err := store.InsertEntry(ctx, spendInput)
	if err != nil {
		test.Fatalf("insert spend: %v", err)
	}

	grantOnly, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{Types: []ledger.EntryType{ledger.EntryGrant}})
	if err != nil {
		test.Fatalf("list entries by type: %v", err)
	}
	if len(grantOnly) != 1 || grantOnly[0].EntryID() != grantEntry.EntryID() {
		test.Fatalf("expected grant entry %s, got %+v", grantEntry.EntryID().String(), grantOnly)
	}

	byReservation, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{ReservationID: &reservationID})
	if err != nil {
		test.Fatalf("list entries by reservation: %v", err)
	}
	if len(byReservation) != 1 || byReservation[0].EntryID() != holdEntry.EntryID() {
		test.Fatalf("expected hold entry %s, got %+v", holdEntry.EntryID().String(), byReservation)
	}

	prefix, err := ledger.NewIdempotencyKey("spend")
	if err != nil {
		test.Fatalf("prefix: %v", err)
	}
	byPrefix, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{IdempotencyKeyPrefix: &prefix})
	if err != nil {
		test.Fatalf("list entries by prefix: %v", err)
	}
	if len(byPrefix) != 1 || byPrefix[0].EntryID() != spendEntry.EntryID() {
		test.Fatalf("expected spend entry %s, got %+v", spendEntry.EntryID().String(), byPrefix)
	}

	combined, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{
		Types:                []ledger.EntryType{ledger.EntryHold},
		ReservationID:        &reservationID,
		IdempotencyKeyPrefix: &prefix,
	})
	if err != nil {
		test.Fatalf("list entries combined: %v", err)
	}
	if len(combined) != 0 {
		test.Fatalf("expected no entries, got %+v", combined)
	}
}

func TestStoreListAccountSummariesPaginatesByAccountID(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	tenantID := mustTenantID(test)
	ledgerID := mustLedgerID(test)
	otherLedgerID, err := ledger.NewLedgerID("other")
	if err != nil {
		test.Fatalf("other ledger: %v", err)
	}

	for i := 0; i < 5; i++ {
		userID, err := ledger.NewUserID("user-list-" + string(rune('a'+i)))
		if err != nil {
			test.Fatalf("user id: %v", err)
		}
		if _, err := store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID); err != nil {
			test.Fatalf("account: %v", err)
		}
	}

	otherUserID, err := ledger.NewUserID("user-other-ledger")
	if err != nil {
		test.Fatalf("other user: %v", err)
	}
	if _, err := store.GetOrCreateAccountID(ctx, tenantID, otherUserID, otherLedgerID); err != nil {
		test.Fatalf("other ledger account: %v", err)
	}

	page1, err := store.ListAccountSummaries(ctx, tenantID, ledgerID, nil, 3)
	if err != nil {
		test.Fatalf("list accounts page1: %v", err)
	}
	if len(page1) != 3 {
		test.Fatalf("expected 3 accounts, got %d", len(page1))
	}
	for index, account := range page1 {
		if account.TenantID() != tenantID || account.LedgerID() != ledgerID {
			test.Fatalf("unexpected account scope: %+v", account)
		}
		if index > 0 && account.AccountID().String() <= page1[index-1].AccountID().String() {
			test.Fatalf("expected accounts sorted by account_id")
		}
	}

	cursor := page1[len(page1)-1].AccountID()
	page2, err := store.ListAccountSummaries(ctx, tenantID, ledgerID, &cursor, 10)
	if err != nil {
		test.Fatalf("list accounts page2: %v", err)
	}
	if len(page2) != 2 {
		test.Fatalf("expected 2 accounts, got %d", len(page2))
	}
	for _, account := range page2 {
		if account.AccountID().String() <= cursor.String() {
			test.Fatalf("expected account_id > cursor")
		}
	}

	seen := make(map[string]struct{}, 5)
	for _, account := range append(page1, page2...) {
		seen[account.AccountID().String()] = struct{}{}
	}
	if len(seen) != 5 {
		test.Fatalf("expected 5 unique accounts, got %d", len(seen))
	}
}

func TestStoreWithTxCommitsAndRollsBack(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}

	idempotencyKey, err := ledger.NewIdempotencyKey("grant-tx-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryGrant,
		amount.ToEntryAmountCents(),
		nil,
		nil,
		idempotencyKey,
		0,
		metadata,
		time.Now().UTC().Unix(),
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}

	if err := store.WithTx(ctx, func(ctx context.Context, txStore ledger.Store) error {
		_, err := txStore.InsertEntry(ctx, entryInput)
		return err
	}); err != nil {
		test.Fatalf("with tx: %v", err)
	}
	entries, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		test.Fatalf("expected 1 entry, got %d", len(entries))
	}

	rollbackKey, err := ledger.NewIdempotencyKey("grant-tx-2")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	rollbackEntry, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryGrant,
		amount.ToEntryAmountCents(),
		nil,
		nil,
		rollbackKey,
		0,
		metadata,
		time.Now().UTC().Unix(),
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	sentinelError := errors.New("rollback requested")
	err = store.WithTx(ctx, func(ctx context.Context, txStore ledger.Store) error {
		if _, err := txStore.InsertEntry(ctx, rollbackEntry); err != nil {
			return err
		}
		return sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	entries, err = store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if err != nil {
		test.Fatalf("list entries after rollback: %v", err)
	}
	if len(entries) != 1 {
		test.Fatalf("expected 1 entry after rollback, got %d", len(entries))
	}
}

func TestStoreGetReservationUnknownReservation(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	reservationID, err := ledger.NewReservationID("missing")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	_, err = store.GetReservation(ctx, accountID, reservationID)
	if !errors.Is(err, ledger.ErrUnknownReservation) {
		test.Fatalf("expected unknown reservation, got %v", err)
	}
}

func TestStoreGetReservationRejectsInvalidAmountCents(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("get account: %v", err)
	}
	reservationID, err := ledger.NewReservationID("order-invalid")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	model := Reservation{
		AccountID:     accountID.String(),
		ReservationID: reservationID.String(),
		AmountCents:   -1,
		Status:        ledger.ReservationStatusActive.String(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		test.Fatalf("create reservation row: %v", err)
	}

	_, err = store.GetReservation(ctx, accountID, reservationID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Code() != errorCodeInvalid {
		test.Fatalf("expected code %q, got %q", errorCodeInvalid, operationError.Code())
	}
}

func TestStoreSumActiveHoldsRejectsNegativeSum(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	model := Reservation{
		AccountID:     accountID.String(),
		ReservationID: "neg",
		AmountCents:   -1,
		Status:        ledger.ReservationStatusActive.String(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		test.Fatalf("insert negative reservation: %v", err)
	}
	_, err = store.SumActiveHolds(ctx, accountID, time.Now().UTC().Unix())
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreWrapsDatabaseErrors(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		test.Fatalf("close db: %v", err)
	}
	store := New(db)
	ctx := context.Background()

	_, err = store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeLookup {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	accountID, err := ledger.NewAccountID("account-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("grant-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryGrant,
		amount.ToEntryAmountCents(),
		nil,
		nil,
		idempotencyKey,
		0,
		metadata,
		time.Now().UTC().Unix(),
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	_, err = store.InsertEntry(ctx, entryInput)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeInsert {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = store.SumTotal(ctx, accountID, time.Now().UTC().Unix())
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumTotal {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = store.SumActiveHolds(ctx, accountID, time.Now().UTC().Unix())
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumActiveHolds {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservationAmount, err := ledger.NewPositiveAmountCents(10)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, reservationAmount, ledger.ReservationStatusActive)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}
	err = store.CreateReservation(ctx, reservation)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeCreate {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	err = store.UpdateReservationStatus(ctx, accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeUpdateStatus {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = store.GetReservation(ctx, accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeList {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}
	_, err = store.GetEntry(ctx, accountID, entryID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = store.GetEntryByIdempotencyKey(ctx, accountID, idempotencyKey)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreGetOrCreateAccountIDRejectsInvalidAccountID(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	account := Account{
		AccountID: " ",
		TenantID:  mustTenantID(test).String(),
		UserID:    mustUserID(test).String(),
		LedgerID:  mustLedgerID(test).String(),
		CreatedAt: time.Now().UTC(),
	}
	if err := db.WithContext(context.Background()).Create(&account).Error; err != nil {
		test.Fatalf("create account: %v", err)
	}

	_, err := store.GetOrCreateAccountID(context.Background(), mustTenantID(test), mustUserID(test), mustLedgerID(test))
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreInsertEntryStoresExpiresAtAndReservationAndUsesNowWhenCreatedUnixUTCIsZero(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("reserve-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	expiresAtUnixUTC := time.Now().UTC().Add(time.Hour).Unix()
	beforeInsertUnixUTC := time.Now().UTC().Unix()
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryHold,
		amount.ToEntryAmountCents().Negated(),
		&reservationID,
		nil,
		idempotencyKey,
		expiresAtUnixUTC,
		metadata,
		0,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := store.InsertEntry(ctx, entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	entries, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		test.Fatalf("expected 1 entry, got %d", len(entries))
	}
	reservationValue, hasReservation := entries[0].ReservationID()
	if !hasReservation || reservationValue.String() != reservationID.String() {
		test.Fatalf("expected reservation %q, got %v", reservationID.String(), reservationValue)
	}
	if entries[0].ExpiresAtUnixUTC() != expiresAtUnixUTC {
		test.Fatalf("expected expires at %d, got %d", expiresAtUnixUTC, entries[0].ExpiresAtUnixUTC())
	}
	if entries[0].CreatedUnixUTC() < beforeInsertUnixUTC {
		test.Fatalf("expected created unix utc >= %d, got %d", beforeInsertUnixUTC, entries[0].CreatedUnixUTC())
	}
}

func TestMapLedgerEntry(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name    string
		row     LedgerEntry
		wantErr bool
	}{
		{
			name: "success",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "grant",
				AmountCents:    100,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
		},
		{
			name: "success with reservation id",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "hold",
				AmountCents:    -100,
				ReservationID:  ptr("order-1"),
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
		},
		{
			name: "invalid entry id",
			row: LedgerEntry{
				EntryID:        " ",
				AccountID:      "account-1",
				Type:           "grant",
				AmountCents:    100,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid account id",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      " ",
				Type:           "grant",
				AmountCents:    100,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "unknown",
				AmountCents:    100,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid amount",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "grant",
				AmountCents:    0,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid reservation id",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "hold",
				AmountCents:    -100,
				ReservationID:  ptr(" "),
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid idempotency key",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "grant",
				AmountCents:    100,
				IdempotencyKey: " ",
				Metadata:       datatypesJSON("{}"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
		{
			name: "invalid metadata",
			row: LedgerEntry{
				EntryID:        "entry-1",
				AccountID:      "account-1",
				Type:           "grant",
				AmountCents:    100,
				IdempotencyKey: "key-1",
				Metadata:       datatypesJSON("{"),
				CreatedAt:      time.Now().UTC(),
			},
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := mapLedgerEntry(testCase.row)
			if testCase.wantErr {
				if err == nil {
					test.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTimeOrZero(test *testing.T) {
	test.Parallel()
	if timeOrZero(nil) != 0 {
		test.Fatalf("expected zero")
	}
	value := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if timeOrZero(&value) != value.Unix() {
		test.Fatalf("expected unix")
	}
}

func TestDatatypesJSON(test *testing.T) {
	test.Parallel()
	empty := datatypesJSON("")
	if string(empty) != "{}" {
		test.Fatalf("expected default metadata json, got %q", string(empty))
	}
	value := datatypesJSON("{\"ok\":true}")
	if string(value) != "{\"ok\":true}" {
		test.Fatalf("expected raw json, got %q", string(value))
	}
}

func TestConflictDetectionHelpers(test *testing.T) {
	test.Parallel()
	if isIdempotencyConflict(nil) {
		test.Fatalf("expected no conflict")
	}
	if !isIdempotencyConflict(gorm.ErrDuplicatedKey) {
		test.Fatalf("expected gorm duplicated key")
	}
	if isIdempotencyConflict(&pgconn.PgError{Code: "00000", ConstraintName: constraintAccountIdempotencyKey}) {
		test.Fatalf("expected non-unique pg error not to conflict")
	}
	if isIdempotencyConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintLedgerEntriesPrimary}) {
		test.Fatalf("expected primary key violation not to be treated as idempotency conflict")
	}
	if !isIdempotencyConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintAccountIdempotencyKey}) {
		test.Fatalf("expected idempotency conflict")
	}

	if isReservationConflict(nil) {
		test.Fatalf("expected no conflict")
	}
	if !isReservationConflict(gorm.ErrDuplicatedKey) {
		test.Fatalf("expected gorm duplicated key")
	}
	if isReservationConflict(&pgconn.PgError{Code: "00000", ConstraintName: constraintReservationPrimary}) {
		test.Fatalf("expected non-unique pg error not to conflict")
	}
	if !isReservationConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintReservationPrimary}) {
		test.Fatalf("expected reservation conflict")
	}
	if !isReservationConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: "other"}) {
		test.Fatalf("expected other unique violations to be treated as conflict")
	}
}

func TestStoreListEntriesRejectsInvalidRows(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("get account: %v", err)
	}
	model := LedgerEntry{
		AccountID:      accountID.String(),
		Type:           "not-a-type",
		AmountCents:    100,
		IdempotencyKey: "invalid-entry",
		CreatedAt:      time.Now().UTC(),
		Metadata:       datatypesJSON("{}"),
	}
	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		test.Fatalf("create row: %v", err)
	}
	_, err = store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Code() != errorCodeInvalid {
		test.Fatalf("expected code %q, got %q", errorCodeInvalid, operationError.Code())
	}
}

func TestStoreGetReservationRejectsInvalidRows(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)

	ctx := context.Background()
	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("get account: %v", err)
	}
	reservationID, err := ledger.NewReservationID("order-invalid")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	model := Reservation{
		AccountID:     accountID.String(),
		ReservationID: reservationID.String(),
		AmountCents:   100,
		Status:        "not-a-status",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&model).Error; err != nil {
		test.Fatalf("create reservation row: %v", err)
	}
	_, err = store.GetReservation(ctx, accountID, reservationID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Code() != errorCodeInvalid {
		test.Fatalf("expected code %q, got %q", errorCodeInvalid, operationError.Code())
	}
}

func TestStoreRefundReferenceAndSumRefunds(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)
	ctx := context.Background()

	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	nowUnixUTC := time.Now().UTC().Unix()

	originalIdempotencyKey, err := ledger.NewIdempotencyKey("spend-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	originalInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntrySpend,
		ledger.EntryAmountCents(-100),
		nil,
		nil,
		originalIdempotencyKey,
		0,
		metadata,
		nowUnixUTC,
	)
	if err != nil {
		test.Fatalf("original input: %v", err)
	}
	originalEntry, err := store.InsertEntry(ctx, originalInput)
	if err != nil {
		test.Fatalf("insert original: %v", err)
	}

	refundIdempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	originalEntryID := originalEntry.EntryID()
	refundInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryRefund,
		ledger.EntryAmountCents(30),
		nil,
		&originalEntryID,
		refundIdempotencyKey,
		0,
		metadata,
		nowUnixUTC,
	)
	if err != nil {
		test.Fatalf("refund input: %v", err)
	}
	refundEntry, err := store.InsertEntry(ctx, refundInput)
	if err != nil {
		test.Fatalf("insert refund: %v", err)
	}

	sumRefunds, err := store.SumRefunds(ctx, accountID, originalEntry.EntryID())
	if err != nil {
		test.Fatalf("sum refunds: %v", err)
	}
	if sumRefunds.Int64() != 30 {
		test.Fatalf("expected refunds sum 30, got %d", sumRefunds.Int64())
	}

	gotByID, err := store.GetEntry(ctx, accountID, refundEntry.EntryID())
	if err != nil {
		test.Fatalf("get entry: %v", err)
	}
	refundOfEntryID, ok := gotByID.RefundOfEntryID()
	if !ok || refundOfEntryID != originalEntry.EntryID() {
		test.Fatalf("expected refund_of_entry_id %s, got %v", originalEntry.EntryID().String(), refundOfEntryID.String())
	}

	gotByIdempotency, err := store.GetEntryByIdempotencyKey(ctx, accountID, refundIdempotencyKey)
	if err != nil {
		test.Fatalf("get by idempotency: %v", err)
	}
	if gotByIdempotency.EntryID() != refundEntry.EntryID() {
		test.Fatalf("expected entry id %s, got %s", refundEntry.EntryID().String(), gotByIdempotency.EntryID().String())
	}

	missingEntryID, err := ledger.NewEntryID("missing-entry")
	if err != nil {
		test.Fatalf("missing entry id: %v", err)
	}
	_, err = store.GetEntry(ctx, accountID, missingEntryID)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestStoreGetEntryByIdempotencyKeyReturnsUnknownEntry(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)
	ctx := context.Background()

	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("missing-idem")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}

	_, err = store.GetEntryByIdempotencyKey(ctx, accountID, idempotencyKey)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestStoreSumRefundsRejectsNegativeTotals(test *testing.T) {
	test.Parallel()
	db := newSQLiteDB(test)
	store := New(db)
	ctx := context.Background()

	accountID, err := store.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	nowUnixUTC := time.Now().UTC().Unix()

	originalIdempotencyKey, err := ledger.NewIdempotencyKey("spend-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	originalInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntrySpend,
		ledger.EntryAmountCents(-100),
		nil,
		nil,
		originalIdempotencyKey,
		0,
		metadata,
		nowUnixUTC,
	)
	if err != nil {
		test.Fatalf("original input: %v", err)
	}
	originalEntry, err := store.InsertEntry(ctx, originalInput)
	if err != nil {
		test.Fatalf("insert original: %v", err)
	}

	refundIdempotencyKey, err := ledger.NewIdempotencyKey("refund-neg")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	originalEntryID := originalEntry.EntryID()
	refundInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryRefund,
		ledger.EntryAmountCents(-30),
		nil,
		&originalEntryID,
		refundIdempotencyKey,
		0,
		metadata,
		nowUnixUTC,
	)
	if err != nil {
		test.Fatalf("refund input: %v", err)
	}
	if _, err := store.InsertEntry(ctx, refundInput); err != nil {
		test.Fatalf("insert refund: %v", err)
	}

	_, err = store.SumRefunds(ctx, accountID, originalEntry.EntryID())
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func newSQLiteDB(test *testing.T) *gorm.DB {
	test.Helper()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Account{}, &LedgerEntry{}, &Reservation{}); err != nil {
		test.Fatalf("auto migrate: %v", err)
	}
	return db
}

func mustTenantID(test *testing.T) ledger.TenantID {
	test.Helper()
	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant id: %v", err)
	}
	return tenantID
}

func mustUserID(test *testing.T) ledger.UserID {
	test.Helper()
	userID, err := ledger.NewUserID("user-123")
	if err != nil {
		test.Fatalf("user id: %v", err)
	}
	return userID
}

func mustLedgerID(test *testing.T) ledger.LedgerID {
	test.Helper()
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger id: %v", err)
	}
	return ledgerID
}

func ptr(value string) *string {
	return &value
}
