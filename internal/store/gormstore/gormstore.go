package gormstore

import (
	"context"
	"errors"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/jackc/pgx/v5/pgconn"
)

// Name of your unique index/constraint in db/migrations.sql
const (
	constraintAccountIdempotencyKey = "ledger_entries_account_id_idempotency_key_key"
	constraintReservationPrimary    = "reservations_pkey"
)

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store {
	// Optional: tighten logger/noisy settings here if you like
	_ = db // keep for future config
	return &Store{db: db}
}

// WithTx runs fn in a transaction, surfacing fn's return error.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context, txStore credit.Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &Store{db: tx})
	})
}

func (s *Store) GetOrCreateAccountID(ctx context.Context, userID string) (string, error) {
	var acc Account
	// INSERT ... ON CONFLICT (user_id) DO UPDATE user_id=excluded.user_id RETURNING account_id
	err := s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"user_id": gorm.Expr("excluded.user_id")}),
		}).
		FirstOrCreate(&acc, Account{UserID: userID}).Error
	if err != nil {
		return "", err
	}
	return acc.AccountID, nil
}

func (s *Store) InsertEntry(ctx context.Context, e credit.Entry) error {
	var exp *time.Time
	if e.ExpiresAtUnixUTC != 0 {
		t := time.Unix(e.ExpiresAtUnixUTC, 0).UTC()
		exp = &t
	}
	meta := []byte("{}")
	if e.MetadataJSON != "" {
		meta = []byte(e.MetadataJSON)
	}
	model := LedgerEntry{
		AccountID:      e.AccountID,
		Type:           string(e.Type),
		AmountCents:    int64(e.AmountCents),
		IdempotencyKey: e.IdempotencyKey,
		ReservationID:  nilIfEmpty(e.ReservationID),
		ExpiresAt:      exp,
		Metadata:       meta,
		CreatedAt:      time.Unix(e.CreatedUnixUTC, 0).UTC(),
	}
	if model.CreatedAt.IsZero() {
		model.CreatedAt = time.Now().UTC()
	}

	err := s.db.WithContext(ctx).Create(&model).Error
	if isIdempotencyConflict(err) {
		return credit.ErrDuplicateIdempotencyKey
	}
	return err
}

func (s *Store) SumTotal(ctx context.Context, accountID string, atUnixUTC int64) (credit.AmountCents, error) {
	at := time.Unix(atUnixUTC, 0).UTC()
	var sum sqlSum
	err := s.db.WithContext(ctx).
		Model(&LedgerEntry{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ?", accountID).
		Where("type not in ('hold','reverse_hold')").
		Where("(expires_at is null or expires_at > ?)", at).
		Scan(&sum).Error
	return credit.AmountCents(sum.Total), err
}

func (s *Store) SumActiveHolds(ctx context.Context, accountID string, _ int64) (credit.AmountCents, error) {
	var sum sqlSum
	err := s.db.WithContext(ctx).
		Model(&Reservation{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ? AND status = ?", accountID, string(credit.ReservationStatusActive)).
		Scan(&sum).Error
	return credit.AmountCents(sum.Total), err
}

func (s *Store) CreateReservation(ctx context.Context, reservation credit.Reservation) error {
	model := Reservation{
		AccountID:     reservation.AccountID,
		ReservationID: reservation.ReservationID,
		AmountCents:   int64(reservation.AmountCents),
		Status:        string(reservation.Status),
	}
	err := s.db.WithContext(ctx).Create(&model).Error
	if isReservationConflict(err) {
		return credit.ErrReservationExists
	}
	return err
}

func (s *Store) GetReservation(ctx context.Context, accountID string, reservationID string) (credit.Reservation, error) {
	var model Reservation
	err := s.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND reservation_id = ?", accountID, reservationID).
		Take(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return credit.Reservation{}, credit.ErrUnknownReservation
		}
		return credit.Reservation{}, err
	}
	return credit.Reservation{
		AccountID:     model.AccountID,
		ReservationID: model.ReservationID,
		AmountCents:   credit.AmountCents(model.AmountCents),
		Status:        credit.ReservationStatus(model.Status),
	}, nil
}

func (s *Store) UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to credit.ReservationStatus) error {
	res := s.db.WithContext(ctx).
		Model(&Reservation{}).
		Where("account_id = ? AND reservation_id = ? AND status = ?", accountID, reservationID, string(from)).
		Update("status", string(to))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return credit.ErrReservationClosed
	}
	return nil
}

func (s *Store) ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]credit.Entry, error) {
	before := time.Unix(beforeUnixUTC, 0).UTC()
	if beforeUnixUTC == 0 {
		before = time.Now().UTC().Add(1 * time.Second) // safe upper bound
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

	out := make([]credit.Entry, 0, len(rows))
	for _, r := range rows {
		out = append(out, credit.Entry{
			EntryID:          r.EntryID,
			AccountID:        r.AccountID,
			Type:             credit.EntryType(r.Type),
			AmountCents:      credit.AmountCents(r.AmountCents),
			ReservationID:    strOrEmpty(r.ReservationID),
			IdempotencyKey:   r.IdempotencyKey,
			ExpiresAtUnixUTC: timeOrZero(r.ExpiresAt),
			MetadataJSON:     string(r.Metadata),
			CreatedUnixUTC:   r.CreatedAt.Unix(),
		})
	}
	return out, nil
}

// ---- helpers ----

type sqlSum struct{ Total int64 }

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func timeOrZero(p *time.Time) int64 {
	if p == nil {
		return 0
	}
	return p.Unix()
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraintAccountIdempotencyKey
}

func isReservationConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraintReservationPrimary
}
