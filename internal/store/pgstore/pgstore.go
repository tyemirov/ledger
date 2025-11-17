package pgstore

import (
	"context"
	"errors"

	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// Matches Postgres' auto-generated name for: UNIQUE(account_id, idempotency_key)
	constraintAccountIdempotencyKey = "ledger_entries_account_id_idempotency_key_key"
	constraintReservationPrimary    = "reservations_pkey"

	sqlInsertOrGetAccount = `
		insert into accounts(user_id) values($1)
		on conflict (user_id) do update set user_id = excluded.user_id
		returning account_id
	`

	// expires_at: NULL when 0; metadata: '{}' when empty string.
	sqlInsertEntry = `
		insert into ledger_entries(
			entry_id, account_id, type, amount_cents, reservation_id, idempotency_key, expires_at, metadata, created_at
		)
		values(
			gen_random_uuid(), $1, $2, $3,
			nullif($4,''), $5,
			to_timestamp(nullif($6,0)),
			coalesce(nullif($7,''),'{}')::jsonb,
			to_timestamp($8)
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
		insert into reservations(account_id, reservation_id, amount_cents, status)
		values ($1, $2, $3, $4)
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

	sqlListEntriesBefore = `
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
		order by created_at desc
		limit $3
	`
)

// Store implements credit.Store using a pgx connection pool (autocommit).
type Store struct {
	pool *pgxpool.Pool
}

// TxStore implements credit.Store for an active transaction.
type TxStore struct {
	tx pgx.Tx
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// -------- pool-backed (autocommit) --------

func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context, txStore credit.Store) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	txStore := &TxStore{tx: tx}
	if err := fn(ctx, txStore); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) GetOrCreateAccountID(ctx context.Context, userID string) (string, error) {
	var accountID string
	err := s.pool.QueryRow(ctx, sqlInsertOrGetAccount, userID).Scan(&accountID)
	return accountID, err
}

func (s *Store) InsertEntry(ctx context.Context, e credit.Entry) error {
	_, err := s.pool.Exec(ctx, sqlInsertEntry,
		e.AccountID, string(e.Type), int64(e.AmountCents), e.ReservationID,
		e.IdempotencyKey, e.ExpiresAtUnixUTC, e.MetadataJSON, e.CreatedUnixUTC,
	)
	if isIdempotencyConflict(err) {
		return credit.ErrDuplicateIdempotencyKey
	}
	return err
}

func (s *Store) SumTotal(ctx context.Context, accountID string, atUnixUTC int64) (credit.AmountCents, error) {
	var sum int64
	err := s.pool.QueryRow(ctx, sqlSumTotal, accountID, atUnixUTC).Scan(&sum)
	return credit.AmountCents(sum), err
}

func (s *Store) SumActiveHolds(ctx context.Context, accountID string, _ int64) (credit.AmountCents, error) {
	var sum int64
	err := s.pool.QueryRow(ctx, sqlSumActiveHolds, accountID).Scan(&sum)
	return credit.AmountCents(sum), err
}

func (s *Store) CreateReservation(ctx context.Context, reservation credit.Reservation) error {
	_, err := s.pool.Exec(ctx, sqlInsertReservation,
		reservation.AccountID,
		reservation.ReservationID,
		int64(reservation.AmountCents),
		string(reservation.Status),
	)
	if isReservationConflict(err) {
		return credit.ErrReservationExists
	}
	return err
}

func (s *Store) GetReservation(ctx context.Context, accountID string, reservationID string) (credit.Reservation, error) {
	var (
		record credit.Reservation
		status string
		amount int64
	)
	err := s.pool.QueryRow(ctx, sqlSelectReservation, accountID, reservationID).Scan(
		&record.AccountID,
		&record.ReservationID,
		&amount,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credit.Reservation{}, credit.ErrUnknownReservation
		}
		return credit.Reservation{}, err
	}
	record.AmountCents = credit.AmountCents(amount)
	record.Status = credit.ReservationStatus(status)
	return record, nil
}

func (s *Store) UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to credit.ReservationStatus) error {
	tag, err := s.pool.Exec(ctx, sqlUpdateReservationStatus, accountID, reservationID, string(from), string(to))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credit.ErrReservationClosed
	}
	return nil
}

func (s *Store) ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]credit.Entry, error) {
	rows, err := s.pool.Query(ctx, sqlListEntriesBefore, accountID, beforeUnixUTC, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// -------- tx-backed --------

func (t *TxStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore credit.Store) error) error {
	// Already in a transaction
	return fn(ctx, t)
}

func (t *TxStore) GetOrCreateAccountID(ctx context.Context, userID string) (string, error) {
	var accountID string
	err := t.tx.QueryRow(ctx, sqlInsertOrGetAccount, userID).Scan(&accountID)
	return accountID, err
}

func (t *TxStore) InsertEntry(ctx context.Context, e credit.Entry) error {
	_, err := t.tx.Exec(ctx, sqlInsertEntry,
		e.AccountID, string(e.Type), int64(e.AmountCents), e.ReservationID,
		e.IdempotencyKey, e.ExpiresAtUnixUTC, e.MetadataJSON, e.CreatedUnixUTC,
	)
	if isIdempotencyConflict(err) {
		return credit.ErrDuplicateIdempotencyKey
	}
	return err
}

func (t *TxStore) SumTotal(ctx context.Context, accountID string, atUnixUTC int64) (credit.AmountCents, error) {
	var sum int64
	err := t.tx.QueryRow(ctx, sqlSumTotal, accountID, atUnixUTC).Scan(&sum)
	return credit.AmountCents(sum), err
}

func (t *TxStore) SumActiveHolds(ctx context.Context, accountID string, _ int64) (credit.AmountCents, error) {
	var sum int64
	err := t.tx.QueryRow(ctx, sqlSumActiveHolds, accountID).Scan(&sum)
	return credit.AmountCents(sum), err
}

func (t *TxStore) CreateReservation(ctx context.Context, reservation credit.Reservation) error {
	_, err := t.tx.Exec(ctx, sqlInsertReservation,
		reservation.AccountID,
		reservation.ReservationID,
		int64(reservation.AmountCents),
		string(reservation.Status),
	)
	if isReservationConflict(err) {
		return credit.ErrReservationExists
	}
	return err
}

func (t *TxStore) GetReservation(ctx context.Context, accountID string, reservationID string) (credit.Reservation, error) {
	var (
		record credit.Reservation
		status string
		amount int64
	)
	err := t.tx.QueryRow(ctx, sqlSelectReservation, accountID, reservationID).Scan(
		&record.AccountID,
		&record.ReservationID,
		&amount,
		&status,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return credit.Reservation{}, credit.ErrUnknownReservation
		}
		return credit.Reservation{}, err
	}
	record.AmountCents = credit.AmountCents(amount)
	record.Status = credit.ReservationStatus(status)
	return record, nil
}

func (t *TxStore) UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to credit.ReservationStatus) error {
	tag, err := t.tx.Exec(ctx, sqlUpdateReservationStatus, accountID, reservationID, string(from), string(to))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return credit.ErrReservationClosed
	}
	return nil
}

func (t *TxStore) ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]credit.Entry, error) {
	rows, err := t.tx.Query(ctx, sqlListEntriesBefore, accountID, beforeUnixUTC, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

// -------- helpers --------

func scanEntries(rows pgx.Rows) ([]credit.Entry, error) {
	out := make([]credit.Entry, 0, 32)
	for rows.Next() {
		var e credit.Entry
		if err := rows.Scan(
			&e.EntryID,
			&e.AccountID,
			(*string)(&e.Type),
			(*int64)(&e.AmountCents),
			&e.ReservationID,
			&e.IdempotencyKey,
			&e.ExpiresAtUnixUTC,
			&e.MetadataJSON,
			&e.CreatedUnixUTC,
		); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// unique_violation
		if pgErr.Code == "23505" && pgErr.ConstraintName == constraintAccountIdempotencyKey {
			return true
		}
	}
	return false
}

func isReservationConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintReservationPrimary
	}
	return false
}
