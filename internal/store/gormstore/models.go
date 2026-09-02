package gormstore

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// LedgerAccount represents the ledger_accounts table.
type LedgerAccount struct {
	AccountID    string        `gorm:"type:uuid;primaryKey"`
	TenantID     string        `gorm:"type:uuid;not null;index:idx_ledger_accounts_tenant_user_ledger,unique,priority:1"`
	UserID       string        `gorm:"not null;index:idx_ledger_accounts_tenant_user_ledger,unique,priority:2"`
	LedgerID     string        `gorm:"not null;index:idx_ledger_accounts_tenant_user_ledger,unique,priority:3"`
	CreatedAt    time.Time     `gorm:"not null"`
	Entries      []LedgerEntry `gorm:"foreignKey:AccountID;references:AccountID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Reservations []Reservation `gorm:"foreignKey:AccountID;references:AccountID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
}

func (LedgerAccount) TableName() string { return "ledger_accounts" }

func (account *LedgerAccount) BeforeCreate(tx *gorm.DB) error {
	if account.AccountID == "" {
		account.AccountID = uuid.NewString()
	}
	return nil
}

// UserAccount represents one durable TAuth identity.
type UserAccount struct {
	UserAccountID string              `gorm:"type:uuid;primaryKey"`
	AuthIssuer    string              `gorm:"not null;index:idx_user_accounts_external_identity,unique,priority:1"`
	AuthTenantID  string              `gorm:"not null;index:idx_user_accounts_external_identity,unique,priority:2"`
	AuthUserID    string              `gorm:"not null;index:idx_user_accounts_external_identity,unique,priority:3"`
	CreatedAt     time.Time           `gorm:"not null"`
	Tenants       []LedgerTenant      `gorm:"foreignKey:OwnerUserAccountID;references:UserAccountID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Idempotency   []IdempotencyRecord `gorm:"foreignKey:UserAccountID;references:UserAccountID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Events        []ControlEvent      `gorm:"foreignKey:ActorUserAccountID;references:UserAccountID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
}

func (UserAccount) TableName() string { return "user_accounts" }

// LedgerTenant represents one UserAccount-owned tenant.
type LedgerTenant struct {
	TenantID           string             `gorm:"type:uuid;primaryKey"`
	OwnerUserAccountID string             `gorm:"type:uuid;not null;index:idx_tenants_owner_created,priority:1"`
	Name               string             `gorm:"not null"`
	CreatedAt          time.Time          `gorm:"not null;index:idx_tenants_owner_created,priority:2"`
	Credentials        []TenantCredential `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
	Accounts           []LedgerAccount    `gorm:"foreignKey:TenantID;references:TenantID;constraint:OnUpdate:RESTRICT,OnDelete:RESTRICT"`
}

func (LedgerTenant) TableName() string { return "tenants" }

// TenantCredential stores one revocable tenant credential digest.
type TenantCredential struct {
	CredentialID string     `gorm:"type:uuid;primaryKey"`
	TenantID     string     `gorm:"type:uuid;not null;index:idx_tenant_credentials_tenant_created,priority:1"`
	SecretDigest []byte     `gorm:"not null"`
	CreatedAt    time.Time  `gorm:"not null;index:idx_tenant_credentials_tenant_created,priority:2"`
	RevokedAt    *time.Time `gorm:"index"`
}

func (TenantCredential) TableName() string { return "tenant_credentials" }

// IdempotencyRecord stores one retry-sensitive control plane decision.
type IdempotencyRecord struct {
	UserAccountID string    `gorm:"type:uuid;primaryKey"`
	Operation     string    `gorm:"primaryKey"`
	KeyDigest     []byte    `gorm:"primaryKey"`
	RequestDigest []byte    `gorm:"not null"`
	ResourceID    string    `gorm:"type:uuid;not null"`
	CreatedAt     time.Time `gorm:"not null"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_records" }

// ControlEvent stores one append-only control plane event.
type ControlEvent struct {
	EventID            string    `gorm:"type:uuid;primaryKey"`
	ActorUserAccountID string    `gorm:"type:uuid;not null;index"`
	EventType          string    `gorm:"not null"`
	ResourceType       string    `gorm:"not null"`
	ResourceID         string    `gorm:"type:uuid;not null"`
	RequestID          string    `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
}

func (ControlEvent) TableName() string { return "control_events" }

func (event *ControlEvent) BeforeCreate(_ *gorm.DB) error {
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	return nil
}

// LedgerEntry mirrors the ledger_entries table.
type LedgerEntry struct {
	EntryID         string         `gorm:"type:uuid;primaryKey"`
	AccountID       string         `gorm:"type:uuid;not null;index:idx_ledger_account_created,priority:1;index:idx_ledger_account_reservation,priority:1;index:idx_ledger_account_refund_of,priority:1;index:uniq_entry_idem,unique,priority:1"`
	Type            string         `gorm:"not null"`
	AmountCents     int64          `gorm:"not null"`
	ReservationID   *string        `gorm:"index:idx_ledger_account_reservation,priority:2"`
	RefundOfEntryID *string        `gorm:"type:uuid;index:idx_ledger_account_refund_of,priority:2"`
	IdempotencyKey  string         `gorm:"not null;index:uniq_entry_idem,unique,priority:2"`
	ExpiresAt       *time.Time     `gorm:""`
	Metadata        datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt       time.Time      `gorm:"not null;index:idx_ledger_account_created,priority:2"`
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
	AccountID     string     `gorm:"type:uuid;primaryKey"`
	ReservationID string     `gorm:"primaryKey"`
	AmountCents   int64      `gorm:"not null"`
	Status        string     `gorm:"not null"`
	ExpiresAt     *time.Time `gorm:""`
	CreatedAt     time.Time  `gorm:"not null"`
	UpdatedAt     time.Time  `gorm:"not null"`
}

func (Reservation) TableName() string { return "reservations" }
