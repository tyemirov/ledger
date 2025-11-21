package gormstore

import (
	"context"
	"errors"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	gosqlite "github.com/glebarez/go-sqlite"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	constraintAccountIdempotencyKey = "ledger_entries_account_id_idempotency_key_key"
	constraintReservationPrimary    = "reservations_pkey"
)

// Store implements ledger.Store using GORM.
type Store struct {
	db *gorm.DB
}

// New returns a Store backed by gorm.DB.
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// WithTx executes fn within a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &Store{db: tx})
	})
}

func (s *Store) GetOrCreateAccountID(ctx context.Context, userID string) (string, error) {
	var acc Account
	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"user_id": clause.Expr{SQL: "excluded.user_id"}}),
		}).
		FirstOrCreate(&acc, Account{UserID: userID}).Error
	if err != nil {
		return "", err
	}
	return acc.AccountID, nil
}

func (s *Store) InsertEntry(ctx context.Context, e ledger.Entry) error {
	var expiresAt *time.Time
	if e.ExpiresAtUnixUTC != 0 {
		t := time.Unix(e.ExpiresAtUnixUTC, 0).UTC()
		expiresAt = &t
	}
	entry := LedgerEntry{
		AccountID:      e.AccountID,
		Type:           string(e.Type),
		AmountCents:    int64(e.AmountCents),
		ReservationID:  nilIfEmpty(e.ReservationID),
		IdempotencyKey: e.IdempotencyKey,
		ExpiresAt:      expiresAt,
		Metadata:       datatypesJSON(e.MetadataJSON),
		CreatedAt:      time.Unix(e.CreatedUnixUTC, 0).UTC(),
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	err := s.db.WithContext(ctx).Create(&entry).Error
	if isIdempotencyConflict(err) {
		return ledger.ErrDuplicateIdempotencyKey
	}
	return err
}

func (s *Store) SumTotal(ctx context.Context, accountID string, atUnixUTC int64) (ledger.AmountCents, error) {
	at := time.Unix(atUnixUTC, 0).UTC()
	var sum sqlSum
	err := s.db.WithContext(ctx).
		Model(&LedgerEntry{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ?", accountID).
		Where("type not in ('hold','reverse_hold')").
		Where("(expires_at is null or expires_at > ?)", at).
		Scan(&sum).Error
	return ledger.AmountCents(sum.Total), err
}

func (s *Store) SumActiveHolds(ctx context.Context, accountID string, _ int64) (ledger.AmountCents, error) {
	var sum sqlSum
	err := s.db.WithContext(ctx).
		Model(&Reservation{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ? AND status = ?", accountID, string(ledger.ReservationStatusActive)).
		Scan(&sum).Error
	return ledger.AmountCents(sum.Total), err
}

func (s *Store) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	model := Reservation{
		AccountID:     reservation.AccountID,
		ReservationID: reservation.ReservationID,
		AmountCents:   int64(reservation.AmountCents),
		Status:        string(reservation.Status),
	}
	err := s.db.WithContext(ctx).Create(&model).Error
	if isReservationConflict(err) {
		return ledger.ErrReservationExists
	}
	return err
}

func (s *Store) GetReservation(ctx context.Context, accountID string, reservationID string) (ledger.Reservation, error) {
	var model Reservation
	err := s.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND reservation_id = ?", accountID, reservationID).
		Take(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ledger.Reservation{}, ledger.ErrUnknownReservation
		}
		return ledger.Reservation{}, err
	}
	return ledger.Reservation{
		AccountID:     model.AccountID,
		ReservationID: model.ReservationID,
		AmountCents:   ledger.AmountCents(model.AmountCents),
		Status:        ledger.ReservationStatus(model.Status),
	}, nil
}

func (s *Store) UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to ledger.ReservationStatus) error {
	res := s.db.WithContext(ctx).
		Model(&Reservation{}).
		Where("account_id = ? AND reservation_id = ? AND status = ?", accountID, reservationID, string(from)).
		Update("status", string(to))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ledger.ErrReservationClosed
	}
	return nil
}

func (s *Store) ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]ledger.Entry, error) {
	before := time.Unix(beforeUnixUTC, 0).UTC()
	if beforeUnixUTC == 0 {
		before = time.Now().UTC().Add(time.Second)
	}

	var rows []LedgerEntry
	err := s.db.WithContext(ctx).
		Where("account_id = ? AND created_at < ?", accountID, before).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]ledger.Entry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, ledger.Entry{
			EntryID:          r.EntryID,
			AccountID:        r.AccountID,
			Type:             ledger.EntryType(r.Type),
			AmountCents:      ledger.AmountCents(r.AmountCents),
			ReservationID:    strOrEmpty(r.ReservationID),
			IdempotencyKey:   r.IdempotencyKey,
			ExpiresAtUnixUTC: timeOrZero(r.ExpiresAt),
			MetadataJSON:     string(r.Metadata),
			CreatedUnixUTC:   r.CreatedAt.Unix(),
		})
	}
	return entries, nil
}

// ---- helpers ----

type sqlSum struct {
	Total int64
}

func nilIfEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func strOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeOrZero(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}

func datatypesJSON(raw string) datatypes.JSON {
	if raw == "" {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON([]byte(raw))
}

func isIdempotencyConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintAccountIdempotencyKey
	}
	var sqliteErr *gosqlite.Error
	if errors.As(err, &sqliteErr) {
		// SQLite extended constraint codes occupy the high byte; mask to handle UNIQUE vs generic constraint.
		return sqliteErr.Code()&0xFF == 19
	}
	return false
}

func isReservationConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraintReservationPrimary
	}
	var sqliteErr *gosqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xFF == 19
	}
	return false
}
