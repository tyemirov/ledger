package gormstore

import (
	"time"

	"gorm.io/datatypes"
)

// Account represents the accounts table.
type Account struct {
	AccountID string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    string    `gorm:"uniqueIndex;not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

func (Account) TableName() string { return "accounts" }

// LedgerEntry mirrors the ledger_entries table.
type LedgerEntry struct {
	EntryID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	AccountID      string         `gorm:"type:uuid;not null;index:idx_ledger_account_created,priority:1"`
	Type           string         `gorm:"type:ledger_type;not null"`
	AmountCents    int64          `gorm:"not null"`
	ReservationID  *string        `gorm:"index:idx_ledger_account_reservation,priority:2"`
	IdempotencyKey string         `gorm:"not null;index:uniq_entry_idem,unique,priority:2"`
	ExpiresAt      *time.Time     `gorm:""`
	Metadata       datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'::jsonb"`
	CreatedAt      time.Time      `gorm:"not null;index:idx_ledger_account_created,priority:2;default:now()"`
}

func (LedgerEntry) TableName() string { return "ledger_entries" }

// Reservation mirrors the reservations table.
type Reservation struct {
	AccountID     string    `gorm:"type:uuid;primaryKey"`
	ReservationID string    `gorm:"primaryKey"`
	AmountCents   int64     `gorm:"not null"`
	Status        string    `gorm:"type:reservation_status;not null"`
	CreatedAt     time.Time `gorm:"not null;default:now()"`
	UpdatedAt     time.Time `gorm:"not null;default:now()"`
}

func (Reservation) TableName() string { return "reservations" }
