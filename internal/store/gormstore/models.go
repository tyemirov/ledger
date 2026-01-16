package gormstore

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Account represents the accounts table.
type Account struct {
	AccountID string    `gorm:"type:uuid;primaryKey"`
	TenantID  string    `gorm:"not null;index:idx_accounts_tenant_user_ledger,unique,priority:1"`
	UserID    string    `gorm:"not null;index:idx_accounts_tenant_user_ledger,unique,priority:2"`
	LedgerID  string    `gorm:"not null;index:idx_accounts_tenant_user_ledger,unique,priority:3"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Account) TableName() string { return "accounts" }

func (account *Account) BeforeCreate(tx *gorm.DB) error {
	if account.AccountID == "" {
		account.AccountID = uuid.NewString()
	}
	return nil
}

// LedgerEntry mirrors the ledger_entries table.
type LedgerEntry struct {
	EntryID        string         `gorm:"type:uuid;primaryKey"`
	AccountID      string         `gorm:"type:uuid;not null;index:idx_ledger_account_created,priority:1"`
	Type           string         `gorm:"type:ledger_type;not null"`
	AmountCents    int64          `gorm:"not null"`
	ReservationID  *string        `gorm:"index:idx_ledger_account_reservation,priority:2"`
	IdempotencyKey string         `gorm:"not null;index:uniq_entry_idem,unique,priority:2"`
	ExpiresAt      *time.Time     `gorm:""`
	Metadata       datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt      time.Time      `gorm:"not null;index:idx_ledger_account_created,priority:2"`
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

func (entry *LedgerEntry) BeforeCreate(tx *gorm.DB) error {
	if entry.EntryID == "" {
		entry.EntryID = uuid.NewString()
	}
	return nil
}

// Reservation mirrors the reservations table.
type Reservation struct {
	AccountID     string    `gorm:"type:uuid;primaryKey"`
	ReservationID string    `gorm:"primaryKey"`
	AmountCents   int64     `gorm:"not null"`
	Status        string    `gorm:"type:reservation_status;not null"`
	CreatedAt     time.Time `gorm:"not null"`
	UpdatedAt     time.Time `gorm:"not null"`
}

func (Reservation) TableName() string { return "reservations" }
