package pgstore

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestStoreWithTxCommits(test *testing.T) {
	test.Parallel()
	stubTransaction := &stubTx{}
	stubPool := &stubPool{
		beginTxFn: func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
			return stubTransaction, nil
		},
	}
	store := &Store{pool: stubPool}
	if err := store.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return nil
	}); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if !stubTransaction.committed {
		test.Fatalf("expected commit")
	}
	if stubTransaction.rolledBack {
		test.Fatalf("expected no rollback")
	}
}

func TestNewReturnsStore(test *testing.T) {
	test.Parallel()
	store := New(nil)
	if store == nil {
		test.Fatalf("expected store")
	}
}

func TestBuildListEntriesSQLIncludesFilters(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	idempotencyPrefix, err := ledger.NewIdempotencyKey("reserve")
	if err != nil {
		test.Fatalf("prefix: %v", err)
	}

	statement, args := buildListEntriesSQL(accountID, 1700000000, 50, ledger.ListEntriesFilter{
		Types:                []ledger.EntryType{ledger.EntryHold, ledger.EntryGrant},
		ReservationID:        &reservationID,
		IdempotencyKeyPrefix: &idempotencyPrefix,
	})
	if !strings.Contains(statement, "type in ($3,$4)") {
		test.Fatalf("expected type filter in sql, got %q", statement)
	}
	if !strings.Contains(statement, "reservation_id = $5") {
		test.Fatalf("expected reservation filter in sql, got %q", statement)
	}
	if !strings.Contains(statement, "idempotency_key like $6") {
		test.Fatalf("expected idempotency prefix filter in sql, got %q", statement)
	}
	if !strings.Contains(statement, "limit $7") {
		test.Fatalf("expected limit placeholder $7, got %q", statement)
	}

	if len(args) != 7 {
		test.Fatalf("expected 7 args, got %d", len(args))
	}
	if args[0] != accountID.String() {
		test.Fatalf("expected account arg %q, got %+v", accountID.String(), args[0])
	}
	if args[1] != int64(1700000000) {
		test.Fatalf("expected before arg 1700000000, got %+v", args[1])
	}
	if args[2] != ledger.EntryHold.String() || args[3] != ledger.EntryGrant.String() {
		test.Fatalf("unexpected type args: %+v", args[2:4])
	}
	if args[4] != reservationID.String() {
		test.Fatalf("expected reservation arg %q, got %+v", reservationID.String(), args[4])
	}
	if args[5] != idempotencyPrefix.String()+"%" {
		test.Fatalf("expected prefix arg %q, got %+v", idempotencyPrefix.String()+"%", args[5])
	}
	if args[6] != 50 {
		test.Fatalf("expected limit arg 50, got %+v", args[6])
	}
}

func TestBuildListReservationsSQLIncludesFilters(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	statement, args := buildListReservationsSQL(accountID, 1700000000, 50, ledger.ListReservationsFilter{
		Statuses: []ledger.ReservationStatus{ledger.ReservationStatusActive, ledger.ReservationStatusCaptured},
	})
	if !strings.Contains(statement, "status in ($3,$4)") {
		test.Fatalf("expected status filter in sql, got %q", statement)
	}
	if !strings.Contains(statement, "limit $5") {
		test.Fatalf("expected limit placeholder $5, got %q", statement)
	}

	if len(args) != 5 {
		test.Fatalf("expected 5 args, got %d", len(args))
	}
	if args[0] != accountID.String() {
		test.Fatalf("expected account arg %q, got %+v", accountID.String(), args[0])
	}
	if args[1] != int64(1700000000) {
		test.Fatalf("expected before arg 1700000000, got %+v", args[1])
	}
	if args[2] != ledger.ReservationStatusActive.String() || args[3] != ledger.ReservationStatusCaptured.String() {
		test.Fatalf("unexpected status args: %+v", args[2:4])
	}
	if args[4] != 50 {
		test.Fatalf("expected limit arg 50, got %+v", args[4])
	}
}

func TestStoreListReservationsScansRows(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)

	var capturedSQL string
	var capturedArgs []any
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			capturedSQL = sql
			capturedArgs = arguments
			return &stubRows{
				records: [][]any{
					{"order-2", int64(200), "active", int64(0), int64(1700000000), int64(1700000001)},
					{"order-1", int64(100), "captured", int64(1700000100), int64(1699999999), int64(1700000002)},
				},
			}, nil
		},
	}
	store := &Store{pool: stubPool}

	reservations, err := store.ListReservations(context.Background(), accountID, 1700001000, 10, ledger.ListReservationsFilter{
		Statuses: []ledger.ReservationStatus{ledger.ReservationStatusActive, ledger.ReservationStatusCaptured},
	})
	if err != nil {
		test.Fatalf("list reservations: %v", err)
	}
	if !strings.Contains(capturedSQL, "status in ($3,$4)") {
		test.Fatalf("expected status filter in sql, got %q", capturedSQL)
	}
	if len(capturedArgs) != 5 {
		test.Fatalf("expected 5 args, got %d", len(capturedArgs))
	}
	if capturedArgs[0] != accountID.String() || capturedArgs[1] != int64(1700001000) || capturedArgs[4] != 10 {
		test.Fatalf("unexpected args: %+v", capturedArgs)
	}
	if len(reservations) != 2 {
		test.Fatalf("expected 2 reservations, got %d", len(reservations))
	}

	if reservations[0].AccountID() != accountID || reservations[0].ReservationID().String() != "order-2" || reservations[0].AmountCents().Int64() != 200 {
		test.Fatalf("unexpected first reservation: %+v", reservations[0])
	}
	if reservations[0].Status() != ledger.ReservationStatusActive || reservations[0].ExpiresAtUnixUTC() != 0 || reservations[0].CreatedUnixUTC() != 1700000000 || reservations[0].UpdatedUnixUTC() != 1700000001 {
		test.Fatalf("unexpected first reservation values: %+v", reservations[0])
	}

	if reservations[1].AccountID() != accountID || reservations[1].ReservationID().String() != "order-1" || reservations[1].AmountCents().Int64() != 100 {
		test.Fatalf("unexpected second reservation: %+v", reservations[1])
	}
	if reservations[1].Status() != ledger.ReservationStatusCaptured || reservations[1].ExpiresAtUnixUTC() != 1700000100 || reservations[1].CreatedUnixUTC() != 1699999999 || reservations[1].UpdatedUnixUTC() != 1700000002 {
		test.Fatalf("unexpected second reservation values: %+v", reservations[1])
	}

	var capturedTxSQL string
	var capturedTxArgs []any
	stubTransaction := &stubTx{
		queryFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			capturedTxSQL = sql
			capturedTxArgs = arguments
			return &stubRows{
				records: [][]any{
					{"order-2", int64(200), "active", int64(0), int64(1700000000), int64(1700000001)},
				},
			}, nil
		},
	}
	txStore := &TxStore{tx: stubTransaction}
	txReservations, err := txStore.ListReservations(context.Background(), accountID, 1700001000, 10, ledger.ListReservationsFilter{
		Statuses: []ledger.ReservationStatus{ledger.ReservationStatusActive},
	})
	if err != nil {
		test.Fatalf("list reservations tx: %v", err)
	}
	if capturedTxSQL == "" || len(capturedTxArgs) == 0 {
		test.Fatalf("expected query to be executed")
	}
	if len(txReservations) != 1 || txReservations[0].ReservationID().String() != "order-2" {
		test.Fatalf("unexpected tx reservations: %+v", txReservations)
	}
}

func TestStoreListReservationsDefaultsCursorWhenZero(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)

	var capturedArgs []any
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			capturedArgs = arguments
			return &stubRows{records: nil}, nil
		},
	}
	store := &Store{pool: stubPool}
	reservations, err := store.ListReservations(context.Background(), accountID, 0, 10, ledger.ListReservationsFilter{})
	if err != nil {
		test.Fatalf("list reservations: %v", err)
	}
	if len(reservations) != 0 {
		test.Fatalf("expected no reservations, got %d", len(reservations))
	}
	if len(capturedArgs) < 2 {
		test.Fatalf("expected query args to include before cursor")
	}
	if capturedArgs[1] == int64(0) {
		test.Fatalf("expected before cursor to be non-zero")
	}
}

func TestStoreListReservationsWrapsQueryErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	sentinelError := errors.New("query failed")
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			return nil, sentinelError
		},
	}
	store := &Store{pool: stubPool}
	_, err := store.ListReservations(context.Background(), accountID, 1700000000, 10, ledger.ListReservationsFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeList {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestStoreListReservationsWrapsScanErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			return &stubRows{
				records: [][]any{
					{"", int64(200), "active", int64(0), int64(1700000000), int64(1700000001)},
				},
			}, nil
		},
	}
	store := &Store{pool: stubPool}
	_, err := store.ListReservations(context.Background(), accountID, 1700000000, 10, ledger.ListReservationsFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
	if !errors.Is(err, ledger.ErrInvalidReservationID) {
		test.Fatalf("expected invalid reservation id error, got %v", err)
	}
}

func TestStoreListReservationsWrapsRowsErr(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	sentinelError := errors.New("rows error")
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			return &stubRows{records: nil, err: sentinelError}, nil
		},
	}
	store := &Store{pool: stubPool}
	_, err := store.ListReservations(context.Background(), accountID, 1700000000, 10, ledger.ListReservationsFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestStoreListReservationsWrapsInvalidStatus(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	stubPool := &stubPool{
		queryRowsFn: func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
			return &stubRows{
				records: [][]any{
					{"order-1", int64(200), "not-a-status", int64(0), int64(1700000000), int64(1700000001)},
				},
			}, nil
		},
	}
	store := &Store{pool: stubPool}
	_, err := store.ListReservations(context.Background(), accountID, 1700000000, 10, ledger.ListReservationsFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected operation error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
	if !errors.Is(err, ledger.ErrInvalidReservationStatus) {
		test.Fatalf("expected invalid reservation status error, got %v", err)
	}
}

func TestTxStoreWithTxDelegates(test *testing.T) {
	test.Parallel()
	executedStatements := []string{}
	stubTransaction := &stubTx{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			executedStatements = append(executedStatements, sql)
			return pgconn.CommandTag{}, nil
		},
	}
	txStore := &TxStore{tx: stubTransaction}
	sentinelError := errors.New("callback error")
	err := txStore.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	if len(executedStatements) != 3 {
		test.Fatalf("expected 3 exec calls, got %d", len(executedStatements))
	}
	if !strings.HasPrefix(executedStatements[0], "savepoint sp_") {
		test.Fatalf("expected savepoint, got %q", executedStatements[0])
	}
	savepointName := strings.TrimPrefix(executedStatements[0], "savepoint ")
	if executedStatements[1] != "rollback to savepoint "+savepointName {
		test.Fatalf("expected rollback to savepoint, got %q", executedStatements[1])
	}
	if executedStatements[2] != "release savepoint "+savepointName {
		test.Fatalf("expected release savepoint, got %q", executedStatements[2])
	}
}

func TestTxStoreWithTxCreatesSavepointAndReleasesOnSuccess(test *testing.T) {
	test.Parallel()
	executedStatements := []string{}
	stubTransaction := &stubTx{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			executedStatements = append(executedStatements, sql)
			return pgconn.CommandTag{}, nil
		},
	}
	txStore := &TxStore{tx: stubTransaction}
	if err := txStore.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return nil
	}); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if len(executedStatements) != 2 {
		test.Fatalf("expected 2 exec calls, got %d", len(executedStatements))
	}
	if !strings.HasPrefix(executedStatements[0], "savepoint sp_") {
		test.Fatalf("expected savepoint, got %q", executedStatements[0])
	}
	savepointName := strings.TrimPrefix(executedStatements[0], "savepoint ")
	if executedStatements[1] != "release savepoint "+savepointName {
		test.Fatalf("expected release savepoint, got %q", executedStatements[1])
	}
}

func TestTxStoreWithTxReturnsNilWhenCallbackIsNil(test *testing.T) {
	test.Parallel()
	txStore := &TxStore{tx: &stubTx{}}
	if err := txStore.WithTx(context.Background(), nil); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
}

func TestTxStoreWithTxJoinsRollbackToSavepointErrors(test *testing.T) {
	test.Parallel()
	sentinelError := errors.New("callback error")
	rollbackError := errors.New("rollback error")
	savepointName := ""
	stubTransaction := &stubTx{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			if strings.HasPrefix(sql, "savepoint ") {
				savepointName = strings.TrimPrefix(sql, "savepoint ")
				return pgconn.CommandTag{}, nil
			}
			if sql == "rollback to savepoint "+savepointName {
				return pgconn.CommandTag{}, rollbackError
			}
			return pgconn.CommandTag{}, nil
		},
	}
	txStore := &TxStore{tx: stubTransaction}
	err := txStore.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Operation() != errorOperationStore || operationError.Subject() != errorSubjectTransaction || operationError.Code() != errorCodeRollbackToSavepoint {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreWithTxJoinsReleaseSavepointErrors(test *testing.T) {
	test.Parallel()
	sentinelError := errors.New("callback error")
	releaseError := errors.New("release error")
	savepointName := ""
	stubTransaction := &stubTx{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			if strings.HasPrefix(sql, "savepoint ") {
				savepointName = strings.TrimPrefix(sql, "savepoint ")
				return pgconn.CommandTag{}, nil
			}
			if sql == "release savepoint "+savepointName {
				return pgconn.CommandTag{}, releaseError
			}
			return pgconn.CommandTag{}, nil
		},
	}
	txStore := &TxStore{tx: stubTransaction}
	err := txStore.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Operation() != errorOperationStore || operationError.Subject() != errorSubjectTransaction || operationError.Code() != errorCodeReleaseSavepoint {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreWithTxRollsBackOnCallbackError(test *testing.T) {
	test.Parallel()
	stubTransaction := &stubTx{}
	stubPool := &stubPool{
		beginTxFn: func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
			return stubTransaction, nil
		},
	}
	store := &Store{pool: stubPool}
	sentinelError := errors.New("callback failed")
	err := store.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		return sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	if !stubTransaction.rolledBack {
		test.Fatalf("expected rollback")
	}
	if stubTransaction.committed {
		test.Fatalf("expected no commit")
	}
}

func TestStoreWithTxWrapsBeginAndCommitErrors(test *testing.T) {
	test.Parallel()
	beginError := errors.New("begin failed")
	store := &Store{pool: &stubPool{
		beginTxFn: func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
			return nil, beginError
		},
	}}
	err := store.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error { return nil })
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectTransaction || operationError.Code() != errorCodeBegin {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	commitError := errors.New("commit failed")
	commitTransaction := &stubTx{commitErr: commitError}
	store = &Store{pool: &stubPool{
		beginTxFn: func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
			return commitTransaction, nil
		},
	}}
	err = store.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error { return nil })
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectTransaction || operationError.Code() != errorCodeCommit {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreGetOrCreateAccountID(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlInsertOrGetAccount {
			test.Fatalf("unexpected sql: %q", sql)
		}
		if len(arguments) != 4 {
			test.Fatalf("expected 4 args, got %d", len(arguments))
		}
		candidateAccountID, ok := arguments[0].(string)
		if !ok {
			test.Fatalf("expected candidate account id string")
		}
		if _, err := uuid.Parse(candidateAccountID); err != nil {
			test.Fatalf("expected uuid candidate account id, got %q", candidateAccountID)
		}
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			*destAccountID = "account-1"
			return nil
		}}
	}
	store := &Store{pool: stubPool}
	tenantID := mustTenantID(test)
	userID := mustUserID(test)
	ledgerID := mustLedgerID(test)
	accountID, err := store.GetOrCreateAccountID(context.Background(), tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if accountID.String() != "account-1" {
		test.Fatalf("expected account-1, got %q", accountID.String())
	}
}

func TestStoreGetOrCreateAccountIDWrapsErrors(test *testing.T) {
	test.Parallel()
	scanError := errors.New("scan failed")
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	store := &Store{pool: stubPool}
	_, err := store.GetOrCreateAccountID(context.Background(), mustTenantID(test), mustUserID(test), mustLedgerID(test))
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeLookup {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			*destAccountID = ""
			return nil
		}}
	}
	_, err = store.GetOrCreateAccountID(context.Background(), mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreInsertEntryHandlesConflicts(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		if sql != sqlInsertEntry {
			test.Fatalf("unexpected sql: %q", sql)
		}
		if len(arguments) != 10 {
			test.Fatalf("expected 10 args, got %d", len(arguments))
		}
		candidateEntryID, ok := arguments[0].(string)
		if !ok {
			test.Fatalf("expected entry id string")
		}
		if _, err := uuid.Parse(candidateEntryID); err != nil {
			test.Fatalf("expected uuid entry id, got %q", candidateEntryID)
		}
		return pgconn.CommandTag{}, &pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintAccountIdempotencyKey}
	}
	store := &Store{pool: stubPool}
	accountID := mustAccountID(test)
	entryInput := mustGrantEntryInput(test, accountID, "grant-1")
	_, err := store.InsertEntry(context.Background(), entryInput)
	if !errors.Is(err, ledger.ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency key, got %v", err)
	}
}

func TestStoreInsertEntryWrapsInsertErrorsAndSuccess(test *testing.T) {
	test.Parallel()
	insertError := errors.New("insert failed")
	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, insertError
	}
	store := &Store{pool: stubPool}
	accountID := mustAccountID(test)
	entryInput := mustGrantEntryInput(test, accountID, "grant-1")
	_, err := store.InsertEntry(context.Background(), entryInput)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeInsert {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	if _, err := store.InsertEntry(context.Background(), entryInput); err != nil {
		test.Fatalf("expected success, got %v", err)
	}
}

func TestStoreInsertEntryPassesReservationID(test *testing.T) {
	test.Parallel()
	capturedReservationID := ""
	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		reservationIDArg := arguments[4].(string)
		capturedReservationID = reservationIDArg
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	store := &Store{pool: stubPool}

	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("reserve-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryHold,
		amount.ToEntryAmountCents().Negated(),
		&reservationID,
		nil,
		idempotencyKey,
		0,
		metadata,
		1700000000,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := store.InsertEntry(context.Background(), entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if capturedReservationID != "order-1" {
		test.Fatalf("expected reservation id order-1, got %q", capturedReservationID)
	}
}

func TestStoreInsertEntryUsesNowWhenCreatedUnixUTCIsZero(test *testing.T) {
	test.Parallel()
	capturedCreatedUnixUTC := int64(0)
	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		createdArg, ok := arguments[9].(int64)
		if !ok {
			test.Fatalf("expected created unix utc int64 arg")
		}
		capturedCreatedUnixUTC = createdArg
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	store := &Store{pool: stubPool}

	accountID := mustAccountID(test)
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("grant-now")
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
		0,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	entry, err := store.InsertEntry(context.Background(), entryInput)
	if err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if capturedCreatedUnixUTC == 0 {
		test.Fatalf("expected created unix utc to be set")
	}
	if entry.CreatedUnixUTC() != capturedCreatedUnixUTC {
		test.Fatalf("expected created unix utc %d, got %d", capturedCreatedUnixUTC, entry.CreatedUnixUTC())
	}
}

func TestTxStoreInsertEntryUsesNowWhenCreatedUnixUTCIsZero(test *testing.T) {
	test.Parallel()
	capturedCreatedUnixUTC := int64(0)
	tx := &stubTx{}
	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		createdArg, ok := arguments[9].(int64)
		if !ok {
			test.Fatalf("expected created unix utc int64 arg")
		}
		capturedCreatedUnixUTC = createdArg
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	txStore := &TxStore{tx: tx}

	accountID := mustAccountID(test)
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("grant-now-tx")
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
		0,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	entry, err := txStore.InsertEntry(context.Background(), entryInput)
	if err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if capturedCreatedUnixUTC == 0 {
		test.Fatalf("expected created unix utc to be set")
	}
	if entry.CreatedUnixUTC() != capturedCreatedUnixUTC {
		test.Fatalf("expected created unix utc %d, got %d", capturedCreatedUnixUTC, entry.CreatedUnixUTC())
	}
}

func TestStoreSumActiveHoldsRejectsNegative(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = -1
			return nil
		}}
	}
	store := &Store{pool: stubPool}
	accountID := mustAccountID(test)
	_, err := store.SumActiveHolds(context.Background(), accountID, 0)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreSumTotalWrapsScanError(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("sum failed") }}
	}
	store := &Store{pool: stubPool}
	_, err := store.SumTotal(context.Background(), mustAccountID(test), 0)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumTotal {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreSumActiveHoldsWrapsScanError(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("holds failed") }}
	}
	store := &Store{pool: stubPool}
	_, err := store.SumActiveHolds(context.Background(), mustAccountID(test), 0)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumActiveHolds {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestScanEntriesHandlesRows(test *testing.T) {
	test.Parallel()
	rows := &stubRows{
		records: [][]any{
			{"entry-1", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)},
			{"entry-2", "account-1", "spend", int64(-50), "order-1", "", "spend-1", int64(0), "{}", int64(1700000001)},
		},
	}
	entries, err := scanEntries(rows)
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		test.Fatalf("expected 2 entries, got %d", len(entries))
	}
	firstReservationID, hasReservation := entries[0].ReservationID()
	if hasReservation {
		test.Fatalf("expected no reservation id, got %q", firstReservationID.String())
	}
	secondReservationID, hasReservation := entries[1].ReservationID()
	if !hasReservation || secondReservationID.String() != "order-1" {
		test.Fatalf("expected reservation order-1, got %v", secondReservationID)
	}
}

func TestScanEntriesRejectsInvalidRows(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name string
		rows *stubRows
	}{
		{
			name: "scan failure",
			rows: &stubRows{records: [][]any{{"only-one-column"}}},
		},
		{
			name: "invalid entry id",
			rows: &stubRows{records: [][]any{{"", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)}}},
		},
		{
			name: "invalid account id",
			rows: &stubRows{records: [][]any{{"entry-1", "", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)}}},
		},
		{
			name: "invalid entry type",
			rows: &stubRows{records: [][]any{{"entry-1", "account-1", "invalid", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)}}},
		},
		{
			name: "invalid amount",
			rows: &stubRows{records: [][]any{{"entry-1", "account-1", "grant", int64(0), "", "", "grant-1", int64(0), "{}", int64(1700000000)}}},
		},
		{
			name: "invalid reservation id",
			rows: &stubRows{records: [][]any{{"entry-1", "account-1", "spend", int64(-50), "", "", "spend-1", int64(0), "{}", int64(1700000000)}, {"entry-2", "account-1", "spend", int64(-50), " ", "", "spend-2", int64(0), "{}", int64(1700000001)}}},
		},
		{
			name: "invalid idempotency key",
			rows: &stubRows{records: [][]any{{"entry-1", "account-1", "grant", int64(100), "", "", " ", int64(0), "{}", int64(1700000000)}}},
		},
		{
			name: "invalid metadata",
			rows: &stubRows{records: [][]any{{"entry-1", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{", int64(1700000000)}}},
		},
		{
			name: "rows err",
			rows: &stubRows{records: nil, err: errors.New("rows error")},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := scanEntries(testCase.rows)
			if err == nil {
				test.Fatalf("expected error")
			}
		})
	}
}

func TestStoreGetEntryParsesRefundReference(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSelectEntry {
			test.Fatalf("unexpected sql: %q", sql)
		}
		if len(arguments) != 2 {
			test.Fatalf("expected 2 args, got %d", len(arguments))
		}
		if arguments[0] != accountID.String() || arguments[1] != entryID.String() {
			test.Fatalf("unexpected args: %+v", arguments)
		}
		return stubRow{scanFn: func(dest ...any) error {
			if len(dest) != 10 {
				return errors.New("dest size mismatch")
			}
			*(dest[0].(*string)) = "entry-1"
			*(dest[1].(*string)) = accountID.String()
			*(dest[2].(*string)) = ledger.EntryRefund.String()
			*(dest[3].(*int64)) = 30
			*(dest[4].(*string)) = "order-1"
			*(dest[5].(*string)) = "entry-0"
			*(dest[6].(*string)) = "refund-1"
			*(dest[7].(*int64)) = 0
			*(dest[8].(*string)) = "{}"
			*(dest[9].(*int64)) = 1700000000
			return nil
		}}
	}
	store := &Store{pool: stubPool}

	entry, err := store.GetEntry(context.Background(), accountID, entryID)
	if err != nil {
		test.Fatalf("get entry: %v", err)
	}
	if entry.Type() != ledger.EntryRefund {
		test.Fatalf("expected refund entry, got %s", entry.Type().String())
	}
	refundOfEntryID, ok := entry.RefundOfEntryID()
	if !ok || refundOfEntryID.String() != "entry-0" {
		test.Fatalf("expected refund_of_entry_id entry-0, got %v", refundOfEntryID.String())
	}
	reservationID, ok := entry.ReservationID()
	if !ok || reservationID.String() != "order-1" {
		test.Fatalf("expected reservation order-1, got %v", reservationID.String())
	}
}

func TestStoreGetEntryReturnsUnknownEntryWhenNoRows(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	store := &Store{pool: stubPool}
	_, err = store.GetEntry(context.Background(), accountID, entryID)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestStoreGetEntryWrapsScanErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	scanError := errors.New("scan failed")
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	store := &Store{pool: stubPool}

	_, err = store.GetEntry(context.Background(), accountID, entryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreGetEntryByIdempotencyKeyParsesRefundReference(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSelectEntryByIdempotencyKey {
			test.Fatalf("unexpected sql: %q", sql)
		}
		return stubRow{scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = "entry-1"
			*(dest[1].(*string)) = accountID.String()
			*(dest[2].(*string)) = ledger.EntryRefund.String()
			*(dest[3].(*int64)) = 30
			*(dest[4].(*string)) = ""
			*(dest[5].(*string)) = "entry-0"
			*(dest[6].(*string)) = idempotencyKey.String()
			*(dest[7].(*int64)) = 0
			*(dest[8].(*string)) = "{}"
			*(dest[9].(*int64)) = 1700000000
			return nil
		}}
	}
	store := &Store{pool: stubPool}

	entry, err := store.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	if err != nil {
		test.Fatalf("get by idempotency: %v", err)
	}
	if entry.Type() != ledger.EntryRefund {
		test.Fatalf("expected refund entry, got %s", entry.Type().String())
	}
}

func TestStoreGetEntryByIdempotencyKeyReturnsUnknownEntryWhenNoRows(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	store := &Store{pool: stubPool}

	_, err = store.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestStoreGetEntryByIdempotencyKeyWrapsScanErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	scanError := errors.New("scan failed")
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	store := &Store{pool: stubPool}

	_, err = store.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreSumRefundsReturnsAmount(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSumRefunds {
			test.Fatalf("unexpected sql: %q", sql)
		}
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = 30
			return nil
		}}
	}
	store := &Store{pool: stubPool}

	amount, err := store.SumRefunds(context.Background(), accountID, originalEntryID)
	if err != nil {
		test.Fatalf("sum refunds: %v", err)
	}
	if amount.Int64() != 30 {
		test.Fatalf("expected sum 30, got %d", amount.Int64())
	}
}

func TestStoreSumRefundsRejectsNegativeSum(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = -1
			return nil
		}}
	}
	store := &Store{pool: stubPool}

	_, err = store.SumRefunds(context.Background(), accountID, originalEntryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreSumRefundsWrapsScanError(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	scanError := errors.New("sum scan failed")
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	store := &Store{pool: stubPool}

	_, err = store.SumRefunds(context.Background(), accountID, originalEntryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumRefunds {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreGetEntryParsesRefundReference(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSelectEntry {
			test.Fatalf("unexpected sql: %q", sql)
		}
		return stubRow{scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = "entry-1"
			*(dest[1].(*string)) = accountID.String()
			*(dest[2].(*string)) = ledger.EntryRefund.String()
			*(dest[3].(*int64)) = 30
			*(dest[4].(*string)) = ""
			*(dest[5].(*string)) = "entry-0"
			*(dest[6].(*string)) = "refund-1"
			*(dest[7].(*int64)) = 0
			*(dest[8].(*string)) = "{}"
			*(dest[9].(*int64)) = 1700000000
			return nil
		}}
	}
	txStore := &TxStore{tx: tx}

	entry, err := txStore.GetEntry(context.Background(), accountID, entryID)
	if err != nil {
		test.Fatalf("get entry: %v", err)
	}
	if entry.Type() != ledger.EntryRefund {
		test.Fatalf("expected refund entry, got %s", entry.Type().String())
	}
}

func TestTxStoreGetEntryReturnsUnknownEntryWhenNoRows(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.GetEntry(context.Background(), accountID, entryID)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestTxStoreGetEntryWrapsScanErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	entryID, err := ledger.NewEntryID("entry-1")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	scanError := errors.New("scan failed")
	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.GetEntry(context.Background(), accountID, entryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreGetEntryByIdempotencyKeyParsesRefundReference(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSelectEntryByIdempotencyKey {
			test.Fatalf("unexpected sql: %q", sql)
		}
		return stubRow{scanFn: func(dest ...any) error {
			*(dest[0].(*string)) = "entry-1"
			*(dest[1].(*string)) = accountID.String()
			*(dest[2].(*string)) = ledger.EntryRefund.String()
			*(dest[3].(*int64)) = 30
			*(dest[4].(*string)) = ""
			*(dest[5].(*string)) = "entry-0"
			*(dest[6].(*string)) = idempotencyKey.String()
			*(dest[7].(*int64)) = 0
			*(dest[8].(*string)) = "{}"
			*(dest[9].(*int64)) = 1700000000
			return nil
		}}
	}
	txStore := &TxStore{tx: tx}

	entry, err := txStore.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	if err != nil {
		test.Fatalf("get by idempotency: %v", err)
	}
	if entry.Type() != ledger.EntryRefund {
		test.Fatalf("expected refund entry, got %s", entry.Type().String())
	}
}

func TestTxStoreGetEntryByIdempotencyKeyReturnsUnknownEntryWhenNoRows(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	if !errors.Is(err, ledger.ErrUnknownEntry) {
		test.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
}

func TestTxStoreGetEntryByIdempotencyKeyWrapsScanErrors(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	idempotencyKey, err := ledger.NewIdempotencyKey("refund-1")
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}

	scanError := errors.New("scan failed")
	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.GetEntryByIdempotencyKey(context.Background(), accountID, idempotencyKey)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreSumRefundsReturnsAmount(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		if sql != sqlSumRefunds {
			test.Fatalf("unexpected sql: %q", sql)
		}
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = 30
			return nil
		}}
	}
	txStore := &TxStore{tx: tx}

	amount, err := txStore.SumRefunds(context.Background(), accountID, originalEntryID)
	if err != nil {
		test.Fatalf("sum refunds: %v", err)
	}
	if amount.Int64() != 30 {
		test.Fatalf("expected sum 30, got %d", amount.Int64())
	}
}

func TestTxStoreSumRefundsRejectsNegativeSum(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = -1
			return nil
		}}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.SumRefunds(context.Background(), accountID, originalEntryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreSumRefundsWrapsScanError(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	originalEntryID, err := ledger.NewEntryID("entry-0")
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}

	scanError := errors.New("sum scan failed")
	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return scanError }}
	}
	txStore := &TxStore{tx: tx}

	_, err = txStore.SumRefunds(context.Background(), accountID, originalEntryID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumRefunds {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreTxStoreMethods(test *testing.T) {
	test.Parallel()
	callCounts := map[string]int{}
	tx := &stubTx{}
	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		callCounts[sql]++
		switch sql {
		case sqlInsertEntry:
			return pgconn.NewCommandTag("INSERT 1"), nil
		case sqlInsertReservation:
			return pgconn.NewCommandTag("INSERT 1"), nil
		case sqlUpdateReservationStatus:
			if callCounts[sql] == 1 {
				return pgconn.NewCommandTag("UPDATE 1"), nil
			}
			return pgconn.NewCommandTag("UPDATE 0"), nil
		default:
			return pgconn.CommandTag{}, errors.New("unexpected exec sql")
		}
	}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		callCounts[sql]++
		switch sql {
		case sqlInsertOrGetAccount:
			return stubRow{scanFn: func(dest ...any) error {
				destAccountID := dest[0].(*string)
				*destAccountID = "account-1"
				return nil
			}}
		case sqlSumTotal:
			return stubRow{scanFn: func(dest ...any) error {
				destSum := dest[0].(*int64)
				*destSum = 1000
				return nil
			}}
		case sqlSumActiveHolds:
			return stubRow{scanFn: func(dest ...any) error {
				destSum := dest[0].(*int64)
				*destSum = 0
				return nil
			}}
		case sqlSelectReservation:
			return stubRow{scanFn: func(dest ...any) error {
				destAccountID := dest[0].(*string)
				destReservationID := dest[1].(*string)
				destAmount := dest[2].(*int64)
				destStatus := dest[3].(*string)
				*destAccountID = "account-1"
				*destReservationID = "order-1"
				*destAmount = 100
				*destStatus = "active"
				return nil
			}}
		default:
			return stubRow{scanFn: func(dest ...any) error {
				return errors.New("unexpected query row sql")
			}}
		}
	}
	tx.queryFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		callCounts[sql]++
		return &stubRows{records: [][]any{
			{"entry-1", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)},
		}}, nil
	}

	store := &Store{pool: &stubPool{
		beginTxFn: func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
			return tx, nil
		},
	}}
	tenantID := mustTenantID(test)
	userID := mustUserID(test)
	ledgerID := mustLedgerID(test)
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservationAmount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, reservationAmount, ledger.ReservationStatusActive, 0)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}

	err = store.WithTx(context.Background(), func(ctx context.Context, txStore ledger.Store) error {
		gotAccountID, err := txStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		if gotAccountID.String() != "account-1" {
			return errors.New("unexpected account id")
		}
		if _, err := txStore.InsertEntry(ctx, mustGrantEntryInput(test, gotAccountID, "grant-1")); err != nil {
			return err
		}
		if _, err := txStore.SumTotal(ctx, gotAccountID, 0); err != nil {
			return err
		}
		if _, err := txStore.SumActiveHolds(ctx, gotAccountID, 0); err != nil {
			return err
		}
		if err := txStore.CreateReservation(ctx, reservation); err != nil {
			return err
		}
		if _, err := txStore.GetReservation(ctx, gotAccountID, reservationID); err != nil {
			return err
		}
		if err := txStore.UpdateReservationStatus(ctx, gotAccountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured); err != nil {
			return err
		}
		if err := txStore.UpdateReservationStatus(ctx, gotAccountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured); !errors.Is(err, ledger.ErrReservationClosed) {
			return errors.New("expected reservation closed")
		}
		entries, err := txStore.ListEntries(ctx, gotAccountID, 0, 10, ledger.ListEntriesFilter{})
		if err != nil {
			return err
		}
		if len(entries) != 1 {
			return errors.New("expected one entry")
		}
		return nil
	})
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if !tx.committed {
		test.Fatalf("expected commit")
	}
	if tx.rolledBack {
		test.Fatalf("expected no rollback")
	}
	if callCounts[sqlInsertOrGetAccount] == 0 {
		test.Fatalf("expected account lookup query")
	}
}

func TestStoreAutocommitMethods(test *testing.T) {
	test.Parallel()
	callCounts := map[string]int{}
	listEntriesCalled := false
	pool := &stubPool{}
	pool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		callCounts[sql]++
		switch sql {
		case sqlInsertEntry:
			return pgconn.NewCommandTag("INSERT 1"), nil
		case sqlInsertReservation:
			return pgconn.NewCommandTag("INSERT 1"), nil
		case sqlUpdateReservationStatus:
			return pgconn.NewCommandTag("UPDATE 1"), nil
		default:
			return pgconn.CommandTag{}, errors.New("unexpected exec sql")
		}
	}
	pool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		callCounts[sql]++
		switch sql {
		case sqlInsertOrGetAccount:
			return stubRow{scanFn: func(dest ...any) error {
				destAccountID := dest[0].(*string)
				*destAccountID = "account-1"
				return nil
			}}
		case sqlSumTotal:
			return stubRow{scanFn: func(dest ...any) error {
				destSum := dest[0].(*int64)
				*destSum = 1000
				return nil
			}}
		case sqlSumActiveHolds:
			return stubRow{scanFn: func(dest ...any) error {
				destSum := dest[0].(*int64)
				*destSum = 0
				return nil
			}}
		case sqlSelectReservation:
			return stubRow{scanFn: func(dest ...any) error {
				destAccountID := dest[0].(*string)
				destReservationID := dest[1].(*string)
				destAmount := dest[2].(*int64)
				destStatus := dest[3].(*string)
				*destAccountID = "account-1"
				*destReservationID = "order-1"
				*destAmount = 100
				*destStatus = "active"
				return nil
			}}
		default:
			return stubRow{scanFn: func(dest ...any) error { return errors.New("unexpected query row sql") }}
		}
	}
	pool.queryRowsFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		callCounts[sql]++
		listEntriesCalled = true
		return &stubRows{records: [][]any{
			{"entry-1", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)},
		}}, nil
	}

	store := &Store{pool: pool}
	ctx := context.Background()
	tenantID := mustTenantID(test)
	userID := mustUserID(test)
	ledgerID := mustLedgerID(test)
	accountID, err := store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	if _, err := store.InsertEntry(ctx, mustGrantEntryInput(test, accountID, "grant-1")); err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if _, err := store.SumTotal(ctx, accountID, 0); err != nil {
		test.Fatalf("sum total: %v", err)
	}
	if _, err := store.SumActiveHolds(ctx, accountID, 0); err != nil {
		test.Fatalf("sum holds: %v", err)
	}
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservationAmount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, reservationAmount, ledger.ReservationStatusActive, 0)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}
	if err := store.CreateReservation(ctx, reservation); err != nil {
		test.Fatalf("create reservation: %v", err)
	}
	if _, err := store.GetReservation(ctx, accountID, reservationID); err != nil {
		test.Fatalf("get reservation: %v", err)
	}
	if err := store.UpdateReservationStatus(ctx, accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured); err != nil {
		test.Fatalf("update reservation: %v", err)
	}
	entries, err := store.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		test.Fatalf("expected one entry, got %d", len(entries))
	}
	if !listEntriesCalled {
		test.Fatalf("expected list entries query")
	}
}

func TestTxStoreMethodsWrapErrors(test *testing.T) {
	test.Parallel()
	tx := &stubTx{}
	txStore := &TxStore{tx: tx}
	ctx := context.Background()

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query row failed") }}
	}
	_, err := txStore.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeLookup {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			*destAccountID = ""
			return nil
		}}
	}
	_, err = txStore.GetOrCreateAccountID(ctx, mustTenantID(test), mustUserID(test), mustLedgerID(test))
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectAccount || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintAccountIdempotencyKey}
	}
	accountID := mustAccountID(test)
	entryInput := mustGrantEntryInput(test, accountID, "grant-1")
	_, err = txStore.InsertEntry(ctx, entryInput)
	if !errors.Is(err, ledger.ErrDuplicateIdempotencyKey) {
		test.Fatalf("expected duplicate idempotency, got %v", err)
	}

	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("insert failed")
	}
	_, err = txStore.InsertEntry(ctx, entryInput)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeInsert {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("sum failed") }}
	}
	_, err = txStore.SumTotal(ctx, accountID, 0)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumTotal {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	_, err = txStore.SumActiveHolds(ctx, accountID, 0)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeSumActiveHolds {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintReservationPrimary}
	}
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservationAmount, err := ledger.NewPositiveAmountCents(10)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, reservationAmount, ledger.ReservationStatusActive, 0)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}
	err = txStore.CreateReservation(ctx, reservation)
	if !errors.Is(err, ledger.ErrReservationExists) {
		test.Fatalf("expected reservation exists, got %v", err)
	}

	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("create failed")
	}
	err = txStore.CreateReservation(ctx, reservation)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeCreate {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	_, err = txStore.GetReservation(ctx, accountID, reservationID)
	if !errors.Is(err, ledger.ErrUnknownReservation) {
		test.Fatalf("expected unknown reservation, got %v", err)
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = ""
			*destReservationID = "order-1"
			*destAmount = 10
			*destStatus = "active"
			return nil
		}}
	}
	_, err = txStore.GetReservation(ctx, accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("update failed")
	}
	err = txStore.UpdateReservationStatus(ctx, accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeUpdateStatus {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		return nil, errors.New("query failed")
	}
	_, err = txStore.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeList {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		return &stubRows{records: [][]any{{"", "account-1", "grant", int64(100), "", "grant-1", int64(0), "{}", int64(1700000000)}}}, nil
	}
	_, err = txStore.ListEntries(ctx, accountID, 0, 10, ledger.ListEntriesFilter{})
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreGetReservationWrapsErrors(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return pgx.ErrNoRows }}
	}
	store := &Store{pool: stubPool}
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	if !errors.Is(err, ledger.ErrUnknownReservation) {
		test.Fatalf("expected unknown reservation, got %v", err)
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query failed") }}
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = ""
			*destReservationID = "order-1"
			*destAmount = 100
			*destStatus = "active"
			return nil
		}}
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = ""
			*destAmount = 100
			*destStatus = "active"
			return nil
		}}
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = "order-1"
			*destAmount = 0
			*destStatus = "active"
			return nil
		}}
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = "order-1"
			*destAmount = 100
			*destStatus = "invalid"
			return nil
		}}
	}
	_, err = store.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestStoreUpdateReservationStatusWrapsErrors(test *testing.T) {
	test.Parallel()
	updateError := errors.New("update failed")
	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, updateError
	}
	store := &Store{pool: stubPool}
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	err = store.UpdateReservationStatus(context.Background(), accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeUpdateStatus {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}
	err = store.UpdateReservationStatus(context.Background(), accountID, reservationID, ledger.ReservationStatusActive, ledger.ReservationStatusCaptured)
	if !errors.Is(err, ledger.ErrReservationClosed) {
		test.Fatalf("expected reservation closed, got %v", err)
	}
}

func TestStoreListEntriesWrapsErrors(test *testing.T) {
	test.Parallel()
	stubPool := &stubPool{}
	stubPool.queryRowsFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		return nil, errors.New("query failed")
	}
	store := &Store{pool: stubPool}
	_, err := store.ListEntries(context.Background(), mustAccountID(test), 0, 10, ledger.ListEntriesFilter{})
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeList {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	stubPool.queryRowsFn = func(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
		return &stubRows{records: [][]any{{"", "account-1", "grant", int64(100), "", "grant-1", int64(0), "{}", int64(1700000000)}}}, nil
	}
	_, err = store.ListEntries(context.Background(), mustAccountID(test), 0, 10, ledger.ListEntriesFilter{})
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectEntry || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestConflictDetectionFunctions(test *testing.T) {
	test.Parallel()
	if isIdempotencyConflict(nil) {
		test.Fatalf("expected no conflict")
	}
	if isIdempotencyConflict(&pgconn.PgError{Code: "00000", ConstraintName: constraintAccountIdempotencyKey}) {
		test.Fatalf("expected non-unique pg error not to conflict")
	}
	if isIdempotencyConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintLedgerEntriesPrimary}) {
		test.Fatalf("expected primary key violation not to conflict")
	}
	if !isIdempotencyConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintAccountIdempotencyKey}) {
		test.Fatalf("expected idempotency conflict")
	}
	if !isIdempotencyConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: "other"}) {
		test.Fatalf("expected other unique violations to conflict")
	}

	if isReservationConflict(nil) {
		test.Fatalf("expected no conflict")
	}
	if !isReservationConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintReservationPrimary}) {
		test.Fatalf("expected reservation conflict")
	}
	if isReservationConflict(&pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: "other"}) {
		test.Fatalf("expected other unique violations not to conflict")
	}
}

func TestStoreCreateReservationWrapsErrorsAndConflicts(test *testing.T) {
	test.Parallel()
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	reservationAmount, err := ledger.NewPositiveAmountCents(10)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	reservation, err := ledger.NewReservation(accountID, reservationID, reservationAmount, ledger.ReservationStatusActive, 0)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}

	stubPool := &stubPool{}
	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, &pgconn.PgError{Code: pgUniqueViolationCode, ConstraintName: constraintReservationPrimary}
	}
	store := &Store{pool: stubPool}
	err = store.CreateReservation(context.Background(), reservation)
	if !errors.Is(err, ledger.ErrReservationExists) {
		test.Fatalf("expected reservation exists, got %v", err)
	}

	stubPool.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("create failed")
	}
	err = store.CreateReservation(context.Background(), reservation)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeCreate {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestPGXAdaptersDelegate(test *testing.T) {
	test.Parallel()
	queryRows := &stubRows{records: [][]any{
		{"entry-1", "account-1", "grant", int64(100), "", "", "grant-1", int64(0), "{}", int64(1700000000)},
	}}
	queryRow := stubRow{scanFn: func(dest ...any) error {
		destAccountID := dest[0].(*string)
		*destAccountID = "ok"
		return nil
	}}

	stubPGXTx := &stubPGXTx{
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 1"), nil
		},
		queryRowFn: func(ctx context.Context, sql string, arguments ...any) pgx.Row {
			return queryRow
		},
		queryFn: func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
			return queryRows, nil
		},
	}
	stubPGXPool := &stubPGXPool{
		tx: stubPGXTx,
		execFn: func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 1"), nil
		},
		queryRowFn: func(ctx context.Context, sql string, arguments ...any) pgx.Row {
			return queryRow
		},
		queryFn: func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
			return queryRows, nil
		},
	}
	poolAdapter := pgxPoolAdapter{pool: stubPGXPool}

	if _, err := poolAdapter.Exec(context.Background(), "insert", 1); err != nil {
		test.Fatalf("exec: %v", err)
	}
	if err := poolAdapter.QueryRow(context.Background(), "select", 1).Scan(new(string)); err != nil {
		test.Fatalf("query row: %v", err)
	}
	rows, err := poolAdapter.Query(context.Background(), "list", 1)
	if err != nil {
		test.Fatalf("query: %v", err)
	}
	rows.Close()

	tx, err := poolAdapter.BeginTx(context.Background(), pgx.TxOptions{})
	if err != nil {
		test.Fatalf("begin tx: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		test.Fatalf("commit: %v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		test.Fatalf("rollback: %v", err)
	}
	if _, err := tx.Exec(context.Background(), "insert", 1); err != nil {
		test.Fatalf("tx exec: %v", err)
	}
	if err := tx.QueryRow(context.Background(), "select", 1).Scan(new(string)); err != nil {
		test.Fatalf("tx query row: %v", err)
	}
	txRows, err := tx.Query(context.Background(), "list", 1)
	if err != nil {
		test.Fatalf("tx query: %v", err)
	}
	txRows.Close()

	if !stubPGXTx.committed {
		test.Fatalf("expected commit")
	}
	if !stubPGXTx.rolledBack {
		test.Fatalf("expected rollback")
	}
}

func TestPGXPoolAdapterBeginTxHandlesErrors(test *testing.T) {
	test.Parallel()
	stubPGXPool := &stubPGXPool{beginErr: errors.New("begin failed")}
	poolAdapter := pgxPoolAdapter{pool: stubPGXPool}
	_, err := poolAdapter.BeginTx(context.Background(), pgx.TxOptions{})
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestTxStoreInsertEntryPassesReservationID(test *testing.T) {
	test.Parallel()
	capturedReservationID := ""
	tx := &stubTx{}
	tx.execFn = func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
		capturedReservationID = arguments[4].(string)
		return pgconn.NewCommandTag("INSERT 1"), nil
	}
	txStore := &TxStore{tx: tx}

	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("reserve-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	entryInput, err := ledger.NewEntryInput(
		accountID,
		ledger.EntryHold,
		amount.ToEntryAmountCents().Negated(),
		&reservationID,
		nil,
		idempotencyKey,
		0,
		metadata,
		1700000000,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	if _, err := txStore.InsertEntry(context.Background(), entryInput); err != nil {
		test.Fatalf("insert entry: %v", err)
	}
	if capturedReservationID != "order-1" {
		test.Fatalf("expected reservation id order-1, got %q", capturedReservationID)
	}
}

func TestTxStoreSumActiveHoldsRejectsNegative(test *testing.T) {
	test.Parallel()
	tx := &stubTx{}
	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destSum := dest[0].(*int64)
			*destSum = -1
			return nil
		}}
	}
	txStore := &TxStore{tx: tx}
	_, err := txStore.SumActiveHolds(context.Background(), mustAccountID(test), 0)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectBalance || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

func TestTxStoreGetReservationWrapsErrors(test *testing.T) {
	test.Parallel()
	tx := &stubTx{}
	txStore := &TxStore{tx: tx}
	accountID := mustAccountID(test)
	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query failed") }}
	}
	_, err = txStore.GetReservation(context.Background(), accountID, reservationID)
	var operationError ledger.OperationError
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeGet {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = "order-1"
			*destAmount = 10
			*destStatus = "invalid"
			return nil
		}}
	}
	_, err = txStore.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = ""
			*destAmount = 10
			*destStatus = "active"
			return nil
		}}
	}
	_, err = txStore.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}

	tx.queryRowFn = func(ctx context.Context, sql string, arguments ...any) queryRow {
		return stubRow{scanFn: func(dest ...any) error {
			destAccountID := dest[0].(*string)
			destReservationID := dest[1].(*string)
			destAmount := dest[2].(*int64)
			destStatus := dest[3].(*string)
			*destAccountID = "account-1"
			*destReservationID = "order-1"
			*destAmount = 0
			*destStatus = "active"
			return nil
		}}
	}
	_, err = txStore.GetReservation(context.Background(), accountID, reservationID)
	if !errors.As(err, &operationError) {
		test.Fatalf("expected operation error, got %v", err)
	}
	if operationError.Subject() != errorSubjectReservation || operationError.Code() != errorCodeInvalid {
		test.Fatalf("unexpected op error: %s.%s.%s", operationError.Operation(), operationError.Subject(), operationError.Code())
	}
}

type stubPool struct {
	beginTxFn   func(ctx context.Context, txOptions pgx.TxOptions) (transaction, error)
	execFn      func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFn  func(ctx context.Context, sql string, arguments ...any) queryRow
	queryRowsFn func(ctx context.Context, sql string, arguments ...any) (queryRows, error)
}

func (pool *stubPool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
	if pool.beginTxFn == nil {
		return nil, errors.New("begin tx not configured")
	}
	return pool.beginTxFn(ctx, txOptions)
}

func (pool *stubPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if pool.execFn == nil {
		return pgconn.CommandTag{}, errors.New("exec not configured")
	}
	return pool.execFn(ctx, sql, arguments...)
}

func (pool *stubPool) QueryRow(ctx context.Context, sql string, arguments ...any) queryRow {
	if pool.queryRowFn == nil {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query row not configured") }}
	}
	return pool.queryRowFn(ctx, sql, arguments...)
}

func (pool *stubPool) Query(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
	if pool.queryRowsFn == nil {
		return nil, errors.New("query not configured")
	}
	return pool.queryRowsFn(ctx, sql, arguments...)
}

type stubTx struct {
	committed  bool
	rolledBack bool
	commitErr  error
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, arguments ...any) queryRow
	queryFn    func(ctx context.Context, sql string, arguments ...any) (queryRows, error)
}

func (tx *stubTx) Commit(ctx context.Context) error {
	tx.committed = true
	return tx.commitErr
}

func (tx *stubTx) Rollback(ctx context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *stubTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if tx.execFn == nil {
		return pgconn.CommandTag{}, nil
	}
	return tx.execFn(ctx, sql, arguments...)
}

func (tx *stubTx) QueryRow(ctx context.Context, sql string, arguments ...any) queryRow {
	if tx.queryRowFn == nil {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query row not configured") }}
	}
	return tx.queryRowFn(ctx, sql, arguments...)
}

func (tx *stubTx) Query(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
	if tx.queryFn == nil {
		return nil, errors.New("query not configured")
	}
	return tx.queryFn(ctx, sql, arguments...)
}

type stubRow struct {
	scanFn func(dest ...any) error
}

func (row stubRow) Scan(dest ...any) error {
	return row.scanFn(dest...)
}

type stubPGXPool struct {
	tx         pgx.Tx
	beginErr   error
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, arguments ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
}

func (pool *stubPGXPool) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error) {
	if pool.beginErr != nil {
		return nil, pool.beginErr
	}
	return pool.tx, nil
}

func (pool *stubPGXPool) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if pool.execFn == nil {
		return pgconn.CommandTag{}, nil
	}
	return pool.execFn(ctx, sql, arguments...)
}

func (pool *stubPGXPool) QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row {
	if pool.queryRowFn == nil {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query row not configured") }}
	}
	return pool.queryRowFn(ctx, sql, arguments...)
}

func (pool *stubPGXPool) Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error) {
	if pool.queryFn == nil {
		return nil, errors.New("query not configured")
	}
	return pool.queryFn(ctx, sql, arguments...)
}

type stubPGXTx struct {
	committed  bool
	rolledBack bool
	execFn     func(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	queryRowFn func(ctx context.Context, sql string, arguments ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
}

func (tx *stubPGXTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return tx, nil
}

func (tx *stubPGXTx) Commit(ctx context.Context) error {
	tx.committed = true
	return nil
}

func (tx *stubPGXTx) Rollback(ctx context.Context) error {
	tx.rolledBack = true
	return nil
}

func (tx *stubPGXTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (tx *stubPGXTx) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return nil
}

func (tx *stubPGXTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (tx *stubPGXTx) Prepare(ctx context.Context, name string, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (tx *stubPGXTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if tx.execFn == nil {
		return pgconn.CommandTag{}, nil
	}
	return tx.execFn(ctx, sql, arguments...)
}

func (tx *stubPGXTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if tx.queryFn == nil {
		return nil, errors.New("query not configured")
	}
	return tx.queryFn(ctx, sql, args...)
}

func (tx *stubPGXTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if tx.queryRowFn == nil {
		return stubRow{scanFn: func(dest ...any) error { return errors.New("query row not configured") }}
	}
	return tx.queryRowFn(ctx, sql, args...)
}

func (tx *stubPGXTx) Conn() *pgx.Conn {
	return nil
}

type stubRows struct {
	records [][]any
	index   int
	closed  bool
	err     error
}

func (rows *stubRows) Next() bool {
	return rows.index < len(rows.records)
}

func (rows *stubRows) Scan(dest ...any) error {
	record := rows.records[rows.index]
	rows.index++
	if len(dest) != len(record) {
		return errors.New("dest size mismatch")
	}
	for index, value := range record {
		switch ptr := dest[index].(type) {
		case *string:
			*ptr = value.(string)
		case *int64:
			*ptr = value.(int64)
		default:
			return errors.New("unsupported dest type")
		}
	}
	return nil
}

func (rows *stubRows) Err() error {
	return rows.err
}

func (rows *stubRows) Close() {
	rows.closed = true
}

func (rows *stubRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func (rows *stubRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}

func (rows *stubRows) Values() ([]any, error) {
	return nil, nil
}

func (rows *stubRows) RawValues() [][]byte {
	return nil
}

func (rows *stubRows) Conn() *pgx.Conn {
	return nil
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

func mustGrantEntryInput(test *testing.T, accountID ledger.AccountID, idempotencyValue string) ledger.EntryInput {
	test.Helper()
	amount, err := ledger.NewPositiveAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey(idempotencyValue)
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
		1700000000,
	)
	if err != nil {
		test.Fatalf("entry input: %v", err)
	}
	return entryInput
}

func mustAccountID(test *testing.T) ledger.AccountID {
	test.Helper()
	accountID, err := ledger.NewAccountID("account-1")
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	return accountID
}
