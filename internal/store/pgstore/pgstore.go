package pgstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	constraintAccountIdempotencyKey = "ledger_entries_account_id_idempotency_key_key"
	constraintLedgerEntriesPrimary  = "ledger_entries_pkey"
	constraintReservationPrimary    = "reservations_pkey"
	pgUniqueViolationCode           = "23505"
	errorOperationStore             = "store"
	errorSubjectAccount             = "account"
	errorSubjectBalance             = "balance"
	errorSubjectEntry               = "entry"
	errorSubjectReservation         = "reservation"
	errorSubjectTransaction         = "transaction"
	errorCodeBegin                  = "begin"
	errorCodeCommit                 = "commit"
	errorCodeCreate                 = "create"
	errorCodeDuplicate              = "duplicate"
	errorCodeGet                    = "get"
	errorCodeInsert                 = "insert"
	errorCodeInvalid                = "invalid"
	errorCodeList                   = "list"
	errorCodeLookup                 = "lookup"
	errorCodeSumActiveHolds         = "sum_active_holds"
	errorCodeSumTotal               = "sum_total"
	errorCodeUpdateStatus           = "update_status"

	sqlInsertOrGetAccount = `
		insert into accounts(account_id, tenant_id, user_id, ledger_id, created_at) values($1, $2, $3, $4, now())
		on conflict (tenant_id, user_id, ledger_id) do update set tenant_id = excluded.tenant_id, user_id = excluded.user_id, ledger_id = excluded.ledger_id
		returning account_id
	`

	sqlInsertEntry = `
		insert into ledger_entries(
			entry_id, account_id, type, amount_cents, reservation_id, idempotency_key, expires_at, metadata, created_at
		)
		values(
			$1, $2, $3, $4,
			nullif($5,''), $6,
			to_timestamp(nullif($7,0)),
			coalesce(nullif($8,''),'{}')::jsonb,
			to_timestamp($9)
		)
	`

	sqlSumTotal = `
		select coalesce(sum(amount_cents),0) from ledger_entries
		where account_id = $1 and (expires_at is null or expires_at > to_timestamp($2))
		and type <> 'hold' and type <> 'reverse_hold'
	`

	sqlSumActiveHolds = `
		select coalesce(sum(amount_cents),0) from reservations
		where account_id = $1 and status = 'active'
	`

	sqlInsertReservation = `
		insert into reservations(account_id, reservation_id, amount_cents, status, created_at, updated_at)
		values ($1, $2, $3, $4, now(), now())
	`

	sqlSelectReservation = `
		select account_id::text, reservation_id, amount_cents, status::text
		from reservations
		where account_id = $1 and reservation_id = $2
		for update
	`

	sqlUpdateReservationStatus = `
		update reservations
		set status = $4, updated_at = now()
		where account_id = $1 and reservation_id = $2 and status = $3
	`
)

type queryRow interface {
	Scan(dest ...any) error
}

type queryRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

type queryExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) queryRow
	Query(ctx context.Context, sql string, arguments ...any) (queryRows, error)
}

type transaction interface {
	queryExecutor
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type connectionPool interface {
	queryExecutor
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (transaction, error)
}

type pgxPool interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
}

type pgxPoolAdapter struct {
	pool pgxPool
}

func (adapter pgxPoolAdapter) BeginTx(ctx context.Context, txOptions pgx.TxOptions) (transaction, error) {
	tx, err := adapter.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return nil, err
	}
	return pgxTxAdapter{tx: tx}, nil
}

func (adapter pgxPoolAdapter) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return adapter.pool.Exec(ctx, sql, arguments...)
}

func (adapter pgxPoolAdapter) QueryRow(ctx context.Context, sql string, arguments ...any) queryRow {
	return adapter.pool.QueryRow(ctx, sql, arguments...)
}

func (adapter pgxPoolAdapter) Query(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
	return adapter.pool.Query(ctx, sql, arguments...)
}

type pgxTx interface {
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, arguments ...any) pgx.Row
	Query(ctx context.Context, sql string, arguments ...any) (pgx.Rows, error)
}

type pgxTxAdapter struct {
	tx pgxTx
}

func (adapter pgxTxAdapter) Commit(ctx context.Context) error {
	return adapter.tx.Commit(ctx)
}

func (adapter pgxTxAdapter) Rollback(ctx context.Context) error {
	return adapter.tx.Rollback(ctx)
}

func (adapter pgxTxAdapter) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return adapter.tx.Exec(ctx, sql, arguments...)
}

func (adapter pgxTxAdapter) QueryRow(ctx context.Context, sql string, arguments ...any) queryRow {
	return adapter.tx.QueryRow(ctx, sql, arguments...)
}

func (adapter pgxTxAdapter) Query(ctx context.Context, sql string, arguments ...any) (queryRows, error) {
	return adapter.tx.Query(ctx, sql, arguments...)
}

// Store implements ledger.Store using a pgx connection pool (autocommit).
type Store struct {
	pool connectionPool
}

// TxStore implements ledger.Store for an active transaction.
type TxStore struct {
	tx transaction
}

// New returns a Store backed by a pgx pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pgxPoolAdapter{pool: pool}}
}

func (store *Store) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return wrapStoreError(errorSubjectTransaction, errorCodeBegin, err)
	}
	transactionStore := &TxStore{tx: tx}
	if err := fn(ctx, transactionStore); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return wrapStoreError(errorSubjectTransaction, errorCodeCommit, err)
	}
	return nil
}

func (store *Store) GetOrCreateAccountID(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error) {
	var accountIDValue string
	candidateAccountID := uuid.NewString()
	err := store.pool.QueryRow(ctx, sqlInsertOrGetAccount, candidateAccountID, tenantID.String(), userID.String(), ledgerID.String()).Scan(&accountIDValue)
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeLookup, err)
	}
	accountID, err := ledger.NewAccountID(accountIDValue)
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeInvalid, err)
	}
	return accountID, nil
}

func (store *Store) InsertEntry(ctx context.Context, entryInput ledger.EntryInput) (ledger.Entry, error) {
	reservationValue, hasReservation := entryInput.ReservationID()
	reservationID := ""
	if hasReservation {
		reservationID = reservationValue.String()
	}
	candidateEntryID := uuid.NewString()
	createdUnixUTC := entryInput.CreatedUnixUTC()
	if createdUnixUTC == 0 {
		createdUnixUTC = time.Now().UTC().Unix()
	}
	_, err := store.pool.Exec(ctx, sqlInsertEntry,
		candidateEntryID,
		entryInput.AccountID().String(),
		entryInput.Type().String(),
		entryInput.AmountCents().Int64(),
		reservationID,
		entryInput.IdempotencyKey().String(),
		entryInput.ExpiresAtUnixUTC(),
		entryInput.MetadataJSON().String(),
		createdUnixUTC,
	)
	if isIdempotencyConflict(err) {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeDuplicate, ledger.ErrDuplicateIdempotencyKey)
	}
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInsert, err)
	}
	entryID, err := ledger.NewEntryID(candidateEntryID)
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
	}
	var reservation *ledger.ReservationID
	if hasReservation {
		reservation = &reservationValue
	}
	entry, err := ledger.NewEntry(
		entryID,
		entryInput.AccountID(),
		entryInput.Type(),
		entryInput.AmountCents(),
		reservation,
		entryInput.IdempotencyKey(),
		entryInput.ExpiresAtUnixUTC(),
		entryInput.MetadataJSON(),
		createdUnixUTC,
	)
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
	}
	return entry, nil
}

func (store *Store) SumTotal(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.SignedAmountCents, error) {
	var sum int64
	err := store.pool.QueryRow(ctx, sqlSumTotal, accountID.String(), atUnixUTC).Scan(&sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumTotal, err)
	}
	return ledger.SignedAmountCents(sum), nil
}

func (store *Store) SumActiveHolds(ctx context.Context, accountID ledger.AccountID, _ int64) (ledger.AmountCents, error) {
	var sum int64
	err := store.pool.QueryRow(ctx, sqlSumActiveHolds, accountID.String()).Scan(&sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumActiveHolds, err)
	}
	activeHolds, err := ledger.NewAmountCents(sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeInvalid, err)
	}
	return activeHolds, nil
}

func (store *Store) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	_, err := store.pool.Exec(ctx, sqlInsertReservation,
		reservation.AccountID().String(),
		reservation.ReservationID().String(),
		reservation.AmountCents().Int64(),
		reservation.Status().String(),
	)
	if isReservationConflict(err) {
		return wrapStoreError(errorSubjectReservation, errorCodeDuplicate, ledger.ErrReservationExists)
	}
	if err != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeCreate, err)
	}
	return nil
}

func (store *Store) GetReservation(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID) (ledger.Reservation, error) {
	var (
		accountValue   string
		reservationVal string
		statusValue    string
		amountValue    int64
	)
	err := store.pool.QueryRow(ctx, sqlSelectReservation, accountID.String(), reservationID.String()).Scan(
		&accountValue,
		&reservationVal,
		&amountValue,
		&statusValue,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, ledger.ErrUnknownReservation)
		}
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, err)
	}
	parsedAccountID, err := ledger.NewAccountID(accountValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	parsedReservationID, err := ledger.NewReservationID(reservationVal)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	amountCents, err := ledger.NewPositiveAmountCents(amountValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	status, err := ledger.ParseReservationStatus(statusValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	reservation, err := ledger.NewReservation(parsedAccountID, parsedReservationID, amountCents, status)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	return reservation, nil
}

func (store *Store) UpdateReservationStatus(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID, from, to ledger.ReservationStatus) error {
	tag, err := store.pool.Exec(ctx, sqlUpdateReservationStatus, accountID.String(), reservationID.String(), from.String(), to.String())
	if err != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, err)
	}
	if tag.RowsAffected() == 0 {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, ledger.ErrReservationClosed)
	}
	return nil
}

func (store *Store) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) ([]ledger.Entry, error) {
	return listEntries(ctx, store.pool, accountID, beforeUnixUTC, limit, filter)
}

func (store *TxStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	return fn(ctx, store)
}

func (store *TxStore) GetOrCreateAccountID(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error) {
	var accountIDValue string
	candidateAccountID := uuid.NewString()
	err := store.tx.QueryRow(ctx, sqlInsertOrGetAccount, candidateAccountID, tenantID.String(), userID.String(), ledgerID.String()).Scan(&accountIDValue)
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeLookup, err)
	}
	accountID, err := ledger.NewAccountID(accountIDValue)
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeInvalid, err)
	}
	return accountID, nil
}

func (store *TxStore) InsertEntry(ctx context.Context, entryInput ledger.EntryInput) (ledger.Entry, error) {
	reservationValue, hasReservation := entryInput.ReservationID()
	reservationID := ""
	if hasReservation {
		reservationID = reservationValue.String()
	}
	candidateEntryID := uuid.NewString()
	createdUnixUTC := entryInput.CreatedUnixUTC()
	if createdUnixUTC == 0 {
		createdUnixUTC = time.Now().UTC().Unix()
	}
	_, err := store.tx.Exec(ctx, sqlInsertEntry,
		candidateEntryID,
		entryInput.AccountID().String(),
		entryInput.Type().String(),
		entryInput.AmountCents().Int64(),
		reservationID,
		entryInput.IdempotencyKey().String(),
		entryInput.ExpiresAtUnixUTC(),
		entryInput.MetadataJSON().String(),
		createdUnixUTC,
	)
	if isIdempotencyConflict(err) {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeDuplicate, ledger.ErrDuplicateIdempotencyKey)
	}
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInsert, err)
	}
	entryID, err := ledger.NewEntryID(candidateEntryID)
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
	}
	var reservation *ledger.ReservationID
	if hasReservation {
		reservation = &reservationValue
	}
	entry, err := ledger.NewEntry(
		entryID,
		entryInput.AccountID(),
		entryInput.Type(),
		entryInput.AmountCents(),
		reservation,
		entryInput.IdempotencyKey(),
		entryInput.ExpiresAtUnixUTC(),
		entryInput.MetadataJSON(),
		createdUnixUTC,
	)
	if err != nil {
		return ledger.Entry{}, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
	}
	return entry, nil
}

func (store *TxStore) SumTotal(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.SignedAmountCents, error) {
	var sum int64
	err := store.tx.QueryRow(ctx, sqlSumTotal, accountID.String(), atUnixUTC).Scan(&sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumTotal, err)
	}
	return ledger.SignedAmountCents(sum), nil
}

func (store *TxStore) SumActiveHolds(ctx context.Context, accountID ledger.AccountID, _ int64) (ledger.AmountCents, error) {
	var sum int64
	err := store.tx.QueryRow(ctx, sqlSumActiveHolds, accountID.String()).Scan(&sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumActiveHolds, err)
	}
	activeHolds, err := ledger.NewAmountCents(sum)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeInvalid, err)
	}
	return activeHolds, nil
}

func (store *TxStore) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	_, err := store.tx.Exec(ctx, sqlInsertReservation,
		reservation.AccountID().String(),
		reservation.ReservationID().String(),
		reservation.AmountCents().Int64(),
		reservation.Status().String(),
	)
	if isReservationConflict(err) {
		return wrapStoreError(errorSubjectReservation, errorCodeDuplicate, ledger.ErrReservationExists)
	}
	if err != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeCreate, err)
	}
	return nil
}

func (store *TxStore) GetReservation(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID) (ledger.Reservation, error) {
	var (
		accountValue   string
		reservationVal string
		statusValue    string
		amountValue    int64
	)
	err := store.tx.QueryRow(ctx, sqlSelectReservation, accountID.String(), reservationID.String()).Scan(
		&accountValue,
		&reservationVal,
		&amountValue,
		&statusValue,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, ledger.ErrUnknownReservation)
		}
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, err)
	}
	parsedAccountID, err := ledger.NewAccountID(accountValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	parsedReservationID, err := ledger.NewReservationID(reservationVal)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	amountCents, err := ledger.NewPositiveAmountCents(amountValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	status, err := ledger.ParseReservationStatus(statusValue)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	reservation, err := ledger.NewReservation(parsedAccountID, parsedReservationID, amountCents, status)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	return reservation, nil
}

func (store *TxStore) UpdateReservationStatus(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID, from, to ledger.ReservationStatus) error {
	tag, err := store.tx.Exec(ctx, sqlUpdateReservationStatus, accountID.String(), reservationID.String(), from.String(), to.String())
	if err != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, err)
	}
	if tag.RowsAffected() == 0 {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, ledger.ErrReservationClosed)
	}
	return nil
}

func (store *TxStore) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) ([]ledger.Entry, error) {
	return listEntries(ctx, store.tx, accountID, beforeUnixUTC, limit, filter)
}

func listEntries(ctx context.Context, executor queryExecutor, accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) ([]ledger.Entry, error) {
	effectiveBeforeUnixUTC := beforeUnixUTC
	if effectiveBeforeUnixUTC == 0 {
		effectiveBeforeUnixUTC = time.Now().UTC().Add(time.Second).Unix()
	}
	statement, args := buildListEntriesSQL(accountID, effectiveBeforeUnixUTC, limit, filter)
	rows, err := executor.Query(ctx, statement, args...)
	if err != nil {
		return nil, wrapStoreError(errorSubjectEntry, errorCodeList, err)
	}
	defer rows.Close()
	entries, err := scanEntries(rows)
	if err != nil {
		return nil, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
	}
	return entries, nil
}

func buildListEntriesSQL(accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) (string, []any) {
	var builder strings.Builder
	builder.WriteString(`
		select
			entry_id::text,
			account_id::text,
			type::text,
			amount_cents,
			coalesce(reservation_id,''),
			idempotency_key,
			coalesce(extract(epoch from expires_at)::bigint,0),
			coalesce(metadata::text,'{}'),
			extract(epoch from created_at)::bigint
		from ledger_entries
		where account_id = $1 and created_at < to_timestamp($2)
	`)
	args := []any{accountID.String(), beforeUnixUTC}
	nextParam := 3
	if len(filter.Types) > 0 {
		builder.WriteString(" and type in (")
		for index, entryType := range filter.Types {
			if index > 0 {
				builder.WriteString(",")
			}
			builder.WriteString(fmt.Sprintf("$%d", nextParam))
			args = append(args, entryType.String())
			nextParam++
		}
		builder.WriteString(")")
	}
	if filter.ReservationID != nil {
		builder.WriteString(fmt.Sprintf(" and reservation_id = $%d", nextParam))
		args = append(args, filter.ReservationID.String())
		nextParam++
	}
	if filter.IdempotencyKeyPrefix != nil {
		builder.WriteString(fmt.Sprintf(" and idempotency_key like $%d", nextParam))
		args = append(args, filter.IdempotencyKeyPrefix.String()+"%")
		nextParam++
	}
	builder.WriteString(fmt.Sprintf(" order by created_at desc limit $%d", nextParam))
	args = append(args, limit)
	return builder.String(), args
}

func scanEntries(rows queryRows) ([]ledger.Entry, error) {
	entries := make([]ledger.Entry, 0, 32)
	for rows.Next() {
		var (
			entryIDValue     string
			accountIDValue   string
			entryTypeValue   string
			amountValue      int64
			reservationValue string
			idempotencyValue string
			expiresAtUnixUTC int64
			metadataValue    string
			createdAtUnixUTC int64
		)
		if err := rows.Scan(
			&entryIDValue,
			&accountIDValue,
			&entryTypeValue,
			&amountValue,
			&reservationValue,
			&idempotencyValue,
			&expiresAtUnixUTC,
			&metadataValue,
			&createdAtUnixUTC,
		); err != nil {
			return nil, err
		}
		entryID, err := ledger.NewEntryID(entryIDValue)
		if err != nil {
			return nil, err
		}
		accountID, err := ledger.NewAccountID(accountIDValue)
		if err != nil {
			return nil, err
		}
		entryType, err := ledger.ParseEntryType(entryTypeValue)
		if err != nil {
			return nil, err
		}
		amountCents, err := ledger.NewEntryAmountCents(amountValue)
		if err != nil {
			return nil, err
		}
		var reservationID *ledger.ReservationID
		if reservationValue != "" {
			parsedReservationID, err := ledger.NewReservationID(reservationValue)
			if err != nil {
				return nil, err
			}
			reservationID = &parsedReservationID
		}
		idempotencyKey, err := ledger.NewIdempotencyKey(idempotencyValue)
		if err != nil {
			return nil, err
		}
		metadata, err := ledger.NewMetadataJSON(metadataValue)
		if err != nil {
			return nil, err
		}
		entry, err := ledger.NewEntry(
			entryID,
			accountID,
			entryType,
			amountCents,
			reservationID,
			idempotencyKey,
			expiresAtUnixUTC,
			metadata,
			createdAtUnixUTC,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func wrapStoreError(subject string, code string, err error) error {
	return ledger.WrapError(errorOperationStore, subject, code, err)
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code != pgUniqueViolationCode {
			return false
		}
		if pgErr.ConstraintName == constraintLedgerEntriesPrimary {
			return false
		}
		if pgErr.ConstraintName == constraintAccountIdempotencyKey {
			return true
		}
		return true
	}
	return false
}

func isReservationConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode && pgErr.ConstraintName == constraintReservationPrimary
	}
	return false
}
