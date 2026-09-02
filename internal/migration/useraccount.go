package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/google/uuid"
	"go.yaml.in/yaml/v3"
	"gorm.io/gorm"
)

type File struct {
	LegacyTenantIDs []string        `yaml:"legacy_tenant_ids"`
	Tenants         []TenantMapping `yaml:"tenants"`
}

type TenantMapping struct {
	LegacyTenantID   string       `yaml:"legacy_tenant_id"`
	TenantID         string       `yaml:"tenant_id"`
	Name             string       `yaml:"name"`
	Owner            OwnerMapping `yaml:"owner"`
	CredentialID     string       `yaml:"credential_id"`
	CredentialSecret string       `yaml:"credential_secret"`
}

type OwnerMapping struct {
	UserAccountID string `yaml:"user_account_id"`
	AuthIssuer    string `yaml:"auth_issuer"`
	AuthTenantID  string `yaml:"auth_tenant_id"`
	AuthUserID    string `yaml:"auth_user_id"`
}

type preparedTenant struct {
	legacyID         string
	tenant           tenant.Tenant
	owner            useraccount.Account
	credential       tenant.Credential
	credentialDigest []byte
}

func Load(path string) (File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return File{}, fmt.Errorf("migration mapping stat: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return File{}, errors.New("migration mapping must be a regular mode-0600 file")
	}
	handle, err := os.Open(path)
	if err != nil {
		return File{}, fmt.Errorf("migration mapping open: %w", err)
	}
	defer func() { _ = handle.Close() }()
	decoder := yaml.NewDecoder(io.LimitReader(handle, 1<<20))
	decoder.KnownFields(true)
	var mapping File
	if err := decoder.Decode(&mapping); err != nil {
		return File{}, fmt.Errorf("migration mapping decode: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return File{}, errors.New("migration mapping must contain one YAML document")
	}
	return mapping, nil
}

func Apply(ctx context.Context, database *gorm.DB, mapping File) error {
	prepared, err := preflight(ctx, database, mapping)
	if err != nil {
		return err
	}
	return applyPrepared(ctx, database, prepared)
}

func applyPrepared(ctx context.Context, database *gorm.DB, prepared []preparedTenant) error {
	now := time.Now().UTC()
	return database.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.AutoMigrate(
			&gormstore.UserAccount{},
			&gormstore.LedgerTenant{},
			&gormstore.TenantCredential{},
			&gormstore.IdempotencyRecord{},
			&gormstore.ControlEvent{},
		); err != nil {
			return fmt.Errorf("migration create control schema: %w", err)
		}
		if err := transaction.Migrator().RenameTable("accounts", "ledger_accounts"); err != nil {
			return fmt.Errorf("migration rename accounts: %w", err)
		}

		owners := make(map[string]useraccount.Account)
		for _, item := range prepared {
			owners[item.owner.ID().String()] = item.owner
		}
		ownerIDs := make([]string, 0, len(owners))
		for ownerID := range owners {
			ownerIDs = append(ownerIDs, ownerID)
		}
		sort.Strings(ownerIDs)
		for _, ownerID := range ownerIDs {
			owner := owners[ownerID]
			identity := owner.Identity()
			if err := transaction.Create(&gormstore.UserAccount{
				UserAccountID: owner.ID().String(),
				AuthIssuer:    identity.Issuer(),
				AuthTenantID:  identity.TenantID(),
				AuthUserID:    identity.UserID(),
				CreatedAt:     now,
			}).Error; err != nil {
				return fmt.Errorf("migration create user account: %w", err)
			}
			if err := createEvent(transaction, owner.ID().String(), "user_account.provisioned", "user_account", owner.ID().String(), now); err != nil {
				return err
			}
		}

		for _, item := range prepared {
			if err := transaction.Create(&gormstore.LedgerTenant{
				TenantID:           item.tenant.ID().String(),
				OwnerUserAccountID: item.owner.ID().String(),
				Name:               item.tenant.Name().String(),
				CreatedAt:          now,
			}).Error; err != nil {
				return fmt.Errorf("migration create tenant: %w", err)
			}
			if err := transaction.Model(&gormstore.LedgerAccount{}).Where("tenant_id = ?", item.legacyID).Update("tenant_id", item.tenant.ID().String()).Error; err != nil {
				return fmt.Errorf("migration update ledger accounts: %w", err)
			}
			if err := transaction.Create(&gormstore.TenantCredential{
				CredentialID: item.credential.ID().String(),
				TenantID:     item.tenant.ID().String(),
				SecretDigest: append([]byte(nil), item.credentialDigest...),
				CreatedAt:    now,
			}).Error; err != nil {
				return fmt.Errorf("migration create credential: %w", err)
			}
			if err := createEvent(transaction, item.owner.ID().String(), "tenant.created", "tenant", item.tenant.ID().String(), now); err != nil {
				return err
			}
			if err := createEvent(transaction, item.owner.ID().String(), "tenant_credential.created", "tenant_credential", item.credential.ID().String(), now); err != nil {
				return err
			}
		}
		if err := transaction.Migrator().AlterColumn(&gormstore.LedgerAccount{}, "TenantID"); err != nil {
			return fmt.Errorf("migration alter ledger account tenant type: %w", err)
		}
		return transaction.Migrator().CreateConstraint(&gormstore.LedgerTenant{}, "Accounts")
	})
}

func preflight(ctx context.Context, database *gorm.DB, mapping File) ([]preparedTenant, error) {
	if database == nil || database.Config == nil || database.Dialector == nil {
		return nil, errors.New("migration database handle is invalid")
	}
	if !database.Migrator().HasTable("accounts") || database.Migrator().HasTable("ledger_accounts") {
		return nil, errors.New("migration requires accounts and forbids ledger_accounts")
	}
	expected := make(map[string]struct{}, len(mapping.LegacyTenantIDs))
	for _, raw := range mapping.LegacyTenantIDs {
		legacyID := strings.TrimSpace(raw)
		if legacyID == "" {
			return nil, errors.New("migration legacy tenant id is required")
		}
		if _, exists := expected[legacyID]; exists {
			return nil, fmt.Errorf("migration duplicate legacy tenant %q", legacyID)
		}
		expected[legacyID] = struct{}{}
	}
	if len(expected) == 0 || len(mapping.Tenants) != len(expected) {
		return nil, errors.New("migration requires one mapping for each legacy tenant")
	}

	var persisted []string
	if err := database.WithContext(ctx).Table("accounts").Distinct("tenant_id").Pluck("tenant_id", &persisted).Error; err != nil {
		return nil, fmt.Errorf("migration read legacy tenants: %w", err)
	}
	for _, legacyID := range persisted {
		if _, exists := expected[legacyID]; !exists {
			return nil, fmt.Errorf("migration owner mapping missing for legacy tenant %q", legacyID)
		}
	}

	seenLegacy := make(map[string]struct{}, len(mapping.Tenants))
	seenTenantIDs := make(map[string]struct{}, len(mapping.Tenants))
	seenCredentialIDs := make(map[string]struct{}, len(mapping.Tenants))
	ownerIdentities := make(map[string]string)
	ownerIDs := make(map[string]string)
	prepared := make([]preparedTenant, 0, len(mapping.Tenants))
	for _, raw := range mapping.Tenants {
		legacyID := strings.TrimSpace(raw.LegacyTenantID)
		if _, exists := expected[legacyID]; !exists {
			return nil, fmt.Errorf("migration unknown legacy tenant %q", legacyID)
		}
		if _, exists := seenLegacy[legacyID]; exists {
			return nil, fmt.Errorf("migration duplicate mapping for legacy tenant %q", legacyID)
		}
		seenLegacy[legacyID] = struct{}{}

		ownerID, err := useraccount.NewID(raw.Owner.UserAccountID)
		if err != nil {
			return nil, fmt.Errorf("migration owner id for %q: %w", legacyID, err)
		}
		identity, err := useraccount.NewExternalIdentity(raw.Owner.AuthIssuer, raw.Owner.AuthTenantID, raw.Owner.AuthUserID)
		if err != nil {
			return nil, fmt.Errorf("migration owner identity for %q: %w", legacyID, err)
		}
		identityKey := identity.Issuer() + "\x00" + identity.TenantID() + "\x00" + identity.UserID()
		if existingID, exists := ownerIdentities[identityKey]; exists && existingID != ownerID.String() {
			return nil, errors.New("migration owner identity has multiple UserAccount ids")
		}
		if existingIdentity, exists := ownerIDs[ownerID.String()]; exists && existingIdentity != identityKey {
			return nil, errors.New("migration UserAccount id has multiple owner identities")
		}
		ownerIdentities[identityKey] = ownerID.String()
		ownerIDs[ownerID.String()] = identityKey
		owner, _ := useraccount.NewAccount(ownerID, identity, time.Unix(1, 0))

		tenantID, err := tenant.NewID(raw.TenantID)
		if err != nil {
			return nil, fmt.Errorf("migration tenant id for %q: %w", legacyID, err)
		}
		if _, exists := seenTenantIDs[tenantID.String()]; exists {
			return nil, errors.New("migration duplicate canonical tenant id")
		}
		seenTenantIDs[tenantID.String()] = struct{}{}
		name, err := tenant.NewName(raw.Name)
		if err != nil {
			return nil, fmt.Errorf("migration tenant name for %q: %w", legacyID, err)
		}
		item, _ := tenant.NewTenant(tenantID, ownerID, name, time.Unix(1, 0))

		credentialID, digest, err := tenant.ParseCredentialSecret(raw.CredentialSecret)
		if err != nil {
			return nil, fmt.Errorf("migration credential for %q: %w", legacyID, err)
		}
		if strings.TrimSpace(raw.CredentialID) != credentialID.String() {
			return nil, errors.New("migration credential id does not match its secret")
		}
		if _, exists := seenCredentialIDs[credentialID.String()]; exists {
			return nil, errors.New("migration duplicate credential id")
		}
		seenCredentialIDs[credentialID.String()] = struct{}{}
		credential, _ := tenant.NewCredential(credentialID, tenantID, time.Unix(1, 0), nil)
		prepared = append(prepared, preparedTenant{legacyID: legacyID, tenant: item, owner: owner, credential: credential, credentialDigest: digest})
	}
	return prepared, nil
}

func createEvent(database *gorm.DB, actorID string, eventType string, resourceType string, resourceID string, createdAt time.Time) error {
	if err := database.Create(&gormstore.ControlEvent{
		EventID:            uuid.NewString(),
		ActorUserAccountID: actorID,
		EventType:          eventType,
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		RequestID:          "user-account-migration",
		CreatedAt:          createdAt,
	}).Error; err != nil {
		return fmt.Errorf("migration create control event: %w", err)
	}
	return nil
}
