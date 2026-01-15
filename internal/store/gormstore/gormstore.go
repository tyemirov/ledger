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
	defaultMetadataJSON             = "{}"
	pgUniqueViolationCode           = "23505"
	sqliteConstraintCode            = 19
	errorOperationStore             = "store"
	errorSubjectAccount             = "account"
	errorSubjectBalance             = "balance"
	errorSubjectEntry               = "entry"
	errorSubjectReservation         = "reservation"
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
func (store *Store) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	return store.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		return fn(ctx, &Store{db: transaction})
	})
}

func (store *Store) GetOrCreateAccountID(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error) {
	var account Account
	err := store.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "user_id"}, {Name: "ledger_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"tenant_id": clause.Expr{SQL: "excluded.tenant_id"},
				"user_id":   clause.Expr{SQL: "excluded.user_id"},
				"ledger_id": clause.Expr{SQL: "excluded.ledger_id"},
			}),
		}).
		FirstOrCreate(&account, Account{TenantID: tenantID.String(), UserID: userID.String(), LedgerID: ledgerID.String()}).Error
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeLookup, err)
	}
	accountID, err := ledger.NewAccountID(account.AccountID)
	if err != nil {
		return ledger.AccountID{}, wrapStoreError(errorSubjectAccount, errorCodeInvalid, err)
	}
	return accountID, nil
}

func (store *Store) InsertEntry(ctx context.Context, entryInput ledger.EntryInput) error {
	var expiresAt *time.Time
	if entryInput.ExpiresAtUnixUTC() != 0 {
		value := time.Unix(entryInput.ExpiresAtUnixUTC(), 0).UTC()
		expiresAt = &value
	}
	var reservationID *string
	reservationValue, hasReservation := entryInput.ReservationID()
	if hasReservation {
		value := reservationValue.String()
		reservationID = &value
	}
	entry := LedgerEntry{
		AccountID:      entryInput.AccountID().String(),
		Type:           entryInput.Type().String(),
		AmountCents:    entryInput.AmountCents().Int64(),
		ReservationID:  reservationID,
		IdempotencyKey: entryInput.IdempotencyKey().String(),
		ExpiresAt:      expiresAt,
		Metadata:       datatypesJSON(entryInput.MetadataJSON().String()),
		CreatedAt:      time.Unix(entryInput.CreatedUnixUTC(), 0).UTC(),
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	err := store.db.WithContext(ctx).Create(&entry).Error
	if isIdempotencyConflict(err) {
		return wrapStoreError(errorSubjectEntry, errorCodeDuplicate, ledger.ErrDuplicateIdempotencyKey)
	}
	if err != nil {
		return wrapStoreError(errorSubjectEntry, errorCodeInsert, err)
	}
	return nil
}

func (store *Store) SumTotal(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.AmountCents, error) {
	at := time.Unix(atUnixUTC, 0).UTC()
	var sum sqlSum
	err := store.db.WithContext(ctx).
		Model(&LedgerEntry{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ?", accountID.String()).
		Where("type not in ('hold','reverse_hold')").
		Where("(expires_at is null or expires_at > ?)", at).
		Scan(&sum).Error
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumTotal, err)
	}
	total, err := ledger.NewAmountCents(sum.Total)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeInvalid, err)
	}
	return total, nil
}

func (store *Store) SumActiveHolds(ctx context.Context, accountID ledger.AccountID, _ int64) (ledger.AmountCents, error) {
	var sum sqlSum
	err := store.db.WithContext(ctx).
		Model(&Reservation{}).
		Select("coalesce(sum(amount_cents),0) as total").
		Where("account_id = ? AND status = ?", accountID.String(), ledger.ReservationStatusActive.String()).
		Scan(&sum).Error
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeSumActiveHolds, err)
	}
	activeHolds, err := ledger.NewAmountCents(sum.Total)
	if err != nil {
		return 0, wrapStoreError(errorSubjectBalance, errorCodeInvalid, err)
	}
	return activeHolds, nil
}

func (store *Store) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	model := Reservation{
		AccountID:     reservation.AccountID().String(),
		ReservationID: reservation.ReservationID().String(),
		AmountCents:   reservation.AmountCents().Int64(),
		Status:        reservation.Status().String(),
	}
	err := store.db.WithContext(ctx).Create(&model).Error
	if isReservationConflict(err) {
		return wrapStoreError(errorSubjectReservation, errorCodeDuplicate, ledger.ErrReservationExists)
	}
	if err != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeCreate, err)
	}
	return nil
}

func (store *Store) GetReservation(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID) (ledger.Reservation, error) {
	var model Reservation
	err := store.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("account_id = ? AND reservation_id = ?", accountID.String(), reservationID.String()).
		Take(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, ledger.ErrUnknownReservation)
		}
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeGet, err)
	}
	parsedAccountID, err := ledger.NewAccountID(model.AccountID)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	parsedReservationID, err := ledger.NewReservationID(model.ReservationID)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	amountCents, err := ledger.NewPositiveAmountCents(model.AmountCents)
	if err != nil {
		return ledger.Reservation{}, wrapStoreError(errorSubjectReservation, errorCodeInvalid, err)
	}
	status, err := ledger.ParseReservationStatus(model.Status)
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
	result := store.db.WithContext(ctx).
		Model(&Reservation{}).
		Where("account_id = ? AND reservation_id = ? AND status = ?", accountID.String(), reservationID.String(), from.String()).
		Update("status", to.String())
	if result.Error != nil {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, result.Error)
	}
	if result.RowsAffected == 0 {
		return wrapStoreError(errorSubjectReservation, errorCodeUpdateStatus, ledger.ErrReservationClosed)
	}
	return nil
}

func (store *Store) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int) ([]ledger.Entry, error) {
	before := time.Unix(beforeUnixUTC, 0).UTC()
	if beforeUnixUTC == 0 {
		before = time.Now().UTC().Add(time.Second)
	}

	var rows []LedgerEntry
	err := store.db.WithContext(ctx).
		Where("account_id = ? AND created_at < ?", accountID.String(), before).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, wrapStoreError(errorSubjectEntry, errorCodeList, err)
	}

	entries := make([]ledger.Entry, 0, len(rows))
	for _, row := range rows {
		entry, err := mapLedgerEntry(row)
		if err != nil {
			return nil, wrapStoreError(errorSubjectEntry, errorCodeInvalid, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func wrapStoreError(subject string, code string, err error) error {
	return ledger.WrapError(errorOperationStore, subject, code, err)
}

type sqlSum struct {
	Total int64
}

func mapLedgerEntry(row LedgerEntry) (ledger.Entry, error) {
	entryID, err := ledger.NewEntryID(row.EntryID)
	if err != nil {
		return ledger.Entry{}, err
	}
	accountID, err := ledger.NewAccountID(row.AccountID)
	if err != nil {
		return ledger.Entry{}, err
	}
	entryType, err := ledger.ParseEntryType(row.Type)
	if err != nil {
		return ledger.Entry{}, err
	}
	amountCents, err := ledger.NewEntryAmountCents(row.AmountCents)
	if err != nil {
		return ledger.Entry{}, err
	}
	var reservationID *ledger.ReservationID
	if row.ReservationID != nil {
		parsedReservationID, err := ledger.NewReservationID(*row.ReservationID)
		if err != nil {
			return ledger.Entry{}, err
		}
		reservationID = &parsedReservationID
	}
	idempotencyKey, err := ledger.NewIdempotencyKey(row.IdempotencyKey)
	if err != nil {
		return ledger.Entry{}, err
	}
	metadata, err := ledger.NewMetadataJSON(string(row.Metadata))
	if err != nil {
		return ledger.Entry{}, err
	}
	entry, err := ledger.NewEntry(
		entryID,
		accountID,
		entryType,
		amountCents,
		reservationID,
		idempotencyKey,
		timeOrZero(row.ExpiresAt),
		metadata,
		row.CreatedAt.Unix(),
	)
	if err != nil {
		return ledger.Entry{}, err
	}
	return entry, nil
}

func timeOrZero(value *time.Time) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}

func datatypesJSON(raw string) datatypes.JSON {
	if raw == "" {
		return datatypes.JSON([]byte(defaultMetadataJSON))
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
		return pgErr.Code == pgUniqueViolationCode && pgErr.ConstraintName == constraintAccountIdempotencyKey
	}
	var sqliteErr *gosqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xFF == sqliteConstraintCode
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
		return pgErr.Code == pgUniqueViolationCode && pgErr.ConstraintName == constraintReservationPrimary
	}
	var sqliteErr *gosqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xFF == sqliteConstraintCode
	}
	return false
}
