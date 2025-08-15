package gormstore

import (
	"time"

	"gorm.io/datatypes"
)

// Table: accounts
type Account struct {
	AccountID string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    string    `gorm:"uniqueIndex;not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

func (Account) TableName() string { return "accounts" }

// Table: ledger_entries
// NOTE: we reuse your Postgres enum `ledger_type`; GORM won't try to recreate it.
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

// Unique (account_id, idempotency_key)
func (LedgerEntry) TableName() string { return "ledger_entries" }
