package gormstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	controlEventUserAccountProvisioned = "user_account.provisioned"
	controlEventTenantCreated          = "tenant.created"
	controlEventCredentialCreated      = "tenant_credential.created"
	controlEventCredentialRevoked      = "tenant_credential.revoked"
	controlOperationTenantCreate       = "tenant.create"
	controlOperationCredentialCreate   = "tenant_credential.create"
	controlResourceUserAccount         = "user_account"
	controlResourceTenant              = "tenant"
	controlResourceCredential          = "tenant_credential"
)

func (store *Store) ProvisionUserAccount(ctx context.Context, input useraccount.ProvisionInput) (useraccount.Account, bool, error) {
	identity := input.Account.Identity()
	model := UserAccount{
		UserAccountID: input.Account.ID().String(),
		AuthIssuer:    identity.Issuer(),
		AuthTenantID:  identity.TenantID(),
		AuthUserID:    identity.UserID(),
		CreatedAt:     input.Account.CreatedAt(),
	}
	created := false
	err := store.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		result := transaction.Clauses(clause.OnConflict{DoNothing: true}).Create(&model)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if created {
			return transaction.Create(&ControlEvent{
				ActorUserAccountID: model.UserAccountID,
				EventType:          controlEventUserAccountProvisioned,
				ResourceType:       controlResourceUserAccount,
				ResourceID:         model.UserAccountID,
				RequestID:          input.RequestID,
				CreatedAt:          model.CreatedAt,
			}).Error
		}
		model = UserAccount{}
		return transaction.Where(
			"auth_issuer = ? AND auth_tenant_id = ? AND auth_user_id = ?",
			identity.Issuer(), identity.TenantID(), identity.UserID(),
		).Take(&model).Error
	})
	if err != nil {
		return useraccount.Account{}, false, fmt.Errorf("user_account.provision: %w", err)
	}
	account, err := mapUserAccount(model)
	if err != nil {
		return useraccount.Account{}, false, fmt.Errorf("user_account.provision: %w", err)
	}
	return account, created, nil
}

func (store *Store) GetUserAccount(ctx context.Context, identity useraccount.ExternalIdentity) (useraccount.Account, error) {
	var model UserAccount
	err := store.db.WithContext(ctx).Where(
		"auth_issuer = ? AND auth_tenant_id = ? AND auth_user_id = ?",
		identity.Issuer(), identity.TenantID(), identity.UserID(),
	).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return useraccount.Account{}, useraccount.ErrNotFound
	}
	if err != nil {
		return useraccount.Account{}, fmt.Errorf("user_account.get: %w", err)
	}
	account, err := mapUserAccount(model)
	if err != nil {
		return useraccount.Account{}, fmt.Errorf("user_account.get: %w", err)
	}
	return account, nil
}

func mapUserAccount(model UserAccount) (useraccount.Account, error) {
	id, err := useraccount.NewID(model.UserAccountID)
	if err != nil {
		return useraccount.Account{}, err
	}
	identity, err := useraccount.NewExternalIdentity(model.AuthIssuer, model.AuthTenantID, model.AuthUserID)
	if err != nil {
		return useraccount.Account{}, err
	}
	return useraccount.NewAccount(id, identity, model.CreatedAt)
}

func (store *Store) Create(ctx context.Context, input tenant.CreateInput) (tenant.Tenant, bool, error) {
	created, err := store.createTenant(ctx, input)
	if err == nil {
		return created, true, nil
	}
	replayed, replayedOK, replayErr := store.replayTenant(ctx, input)
	if replayErr != nil {
		return tenant.Tenant{}, false, replayErr
	}
	if replayedOK {
		return replayed, false, nil
	}
	return tenant.Tenant{}, false, err
}

func (store *Store) createTenant(ctx context.Context, input tenant.CreateInput) (tenant.Tenant, error) {
	model := LedgerTenant{
		TenantID:           input.Tenant.ID().String(),
		OwnerUserAccountID: input.Tenant.OwnerID().String(),
		Name:               input.Tenant.Name().String(),
		CreatedAt:          input.Tenant.CreatedAt(),
	}
	err := store.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Create(&model).Error; err != nil {
			return err
		}
		if err := transaction.Create(&IdempotencyRecord{
			UserAccountID: input.Tenant.OwnerID().String(),
			Operation:     controlOperationTenantCreate,
			KeyDigest:     input.Idempotency.Digest(),
			RequestDigest: input.Request.Bytes(),
			ResourceID:    input.Tenant.ID().String(),
			CreatedAt:     input.Tenant.CreatedAt(),
		}).Error; err != nil {
			return err
		}
		return transaction.Create(&ControlEvent{
			ActorUserAccountID: input.Tenant.OwnerID().String(),
			EventType:          controlEventTenantCreated,
			ResourceType:       controlResourceTenant,
			ResourceID:         input.Tenant.ID().String(),
			RequestID:          input.RequestID,
			CreatedAt:          input.Tenant.CreatedAt(),
		}).Error
	})
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("tenant.create: %w", err)
	}
	return input.Tenant, nil
}

func (store *Store) replayTenant(ctx context.Context, input tenant.CreateInput) (tenant.Tenant, bool, error) {
	var record IdempotencyRecord
	err := store.db.WithContext(ctx).Where(
		"user_account_id = ? AND operation = ? AND key_digest = ?",
		input.Tenant.OwnerID().String(), controlOperationTenantCreate, input.Idempotency.Digest(),
	).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tenant.Tenant{}, false, nil
	}
	if err != nil {
		return tenant.Tenant{}, false, fmt.Errorf("tenant.replay: %w", err)
	}
	if !bytes.Equal(record.RequestDigest, input.Request.Bytes()) {
		return tenant.Tenant{}, false, tenant.ErrIdempotencyConflict
	}
	tenantID, err := tenant.NewID(record.ResourceID)
	if err != nil {
		return tenant.Tenant{}, false, fmt.Errorf("tenant.replay: %w", err)
	}
	replayed, err := store.GetTenant(ctx, input.Tenant.OwnerID(), tenantID)
	if err != nil {
		return tenant.Tenant{}, false, err
	}
	return replayed, true, nil
}

func (store *Store) List(ctx context.Context, ownerID useraccount.ID, cursor *tenant.Cursor, limit int) ([]tenant.Tenant, error) {
	query := store.db.WithContext(ctx).Where("owner_user_account_id = ?", ownerID.String())
	if cursor != nil {
		query = query.Where(
			"created_at > ? OR (created_at = ? AND tenant_id > ?)",
			cursor.CreatedAt, cursor.CreatedAt, cursor.TenantID.String(),
		)
	}
	var models []LedgerTenant
	if err := query.Order("created_at ASC").Order("tenant_id ASC").Limit(limit).Find(&models).Error; err != nil {
		return nil, fmt.Errorf("tenant.list: %w", err)
	}
	items := make([]tenant.Tenant, 0, len(models))
	for _, model := range models {
		item, err := mapTenant(model)
		if err != nil {
			return nil, fmt.Errorf("tenant.list: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (store *Store) GetTenant(ctx context.Context, ownerID useraccount.ID, tenantID tenant.ID) (tenant.Tenant, error) {
	var model LedgerTenant
	err := store.db.WithContext(ctx).Where(
		"owner_user_account_id = ? AND tenant_id = ?", ownerID.String(), tenantID.String(),
	).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tenant.Tenant{}, tenant.ErrNotFound
	}
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("tenant.get: %w", err)
	}
	item, err := mapTenant(model)
	if err != nil {
		return tenant.Tenant{}, fmt.Errorf("tenant.get: %w", err)
	}
	return item, nil
}

func mapTenant(model LedgerTenant) (tenant.Tenant, error) {
	tenantID, err := tenant.NewID(model.TenantID)
	if err != nil {
		return tenant.Tenant{}, err
	}
	ownerID, err := useraccount.NewID(model.OwnerUserAccountID)
	if err != nil {
		return tenant.Tenant{}, err
	}
	name, err := tenant.NewName(model.Name)
	if err != nil {
		return tenant.Tenant{}, err
	}
	return tenant.NewTenant(tenantID, ownerID, name, model.CreatedAt)
}

func (store *Store) CreateCredential(ctx context.Context, ownerID useraccount.ID, input tenant.CreateCredentialInput) (tenant.Credential, bool, error) {
	created, err := store.createCredential(ctx, ownerID, input)
	if err == nil {
		return created, true, nil
	}
	replayed, replayedOK, replayErr := store.replayCredential(ctx, ownerID, input)
	if replayErr != nil {
		return tenant.Credential{}, false, replayErr
	}
	if replayedOK {
		return replayed, false, nil
	}
	return tenant.Credential{}, false, err
}

func (store *Store) createCredential(ctx context.Context, ownerID useraccount.ID, input tenant.CreateCredentialInput) (tenant.Credential, error) {
	if _, err := store.GetTenant(ctx, ownerID, input.Credential.TenantID()); err != nil {
		return tenant.Credential{}, err
	}
	model := TenantCredential{
		CredentialID: input.Credential.ID().String(),
		TenantID:     input.Credential.TenantID().String(),
		SecretDigest: append([]byte(nil), input.SecretDigest...),
		CreatedAt:    input.Credential.CreatedAt(),
	}
	err := store.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		if err := transaction.Create(&model).Error; err != nil {
			return err
		}
		if err := transaction.Create(&IdempotencyRecord{
			UserAccountID: ownerID.String(),
			Operation:     controlOperationCredentialCreate,
			KeyDigest:     input.Idempotency.Digest(),
			RequestDigest: input.Request.Bytes(),
			ResourceID:    input.Credential.ID().String(),
			CreatedAt:     input.Credential.CreatedAt(),
		}).Error; err != nil {
			return err
		}
		return transaction.Create(&ControlEvent{
			ActorUserAccountID: ownerID.String(),
			EventType:          controlEventCredentialCreated,
			ResourceType:       controlResourceCredential,
			ResourceID:         input.Credential.ID().String(),
			RequestID:          input.RequestID,
			CreatedAt:          input.Credential.CreatedAt(),
		}).Error
	})
	if err != nil {
		return tenant.Credential{}, fmt.Errorf("tenant_credential.create: %w", err)
	}
	return input.Credential, nil
}

func (store *Store) replayCredential(ctx context.Context, ownerID useraccount.ID, input tenant.CreateCredentialInput) (tenant.Credential, bool, error) {
	var record IdempotencyRecord
	err := store.db.WithContext(ctx).Where(
		"user_account_id = ? AND operation = ? AND key_digest = ?",
		ownerID.String(), controlOperationCredentialCreate, input.Idempotency.Digest(),
	).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tenant.Credential{}, false, nil
	}
	if err != nil {
		return tenant.Credential{}, false, fmt.Errorf("tenant_credential.replay: %w", err)
	}
	if !bytes.Equal(record.RequestDigest, input.Request.Bytes()) {
		return tenant.Credential{}, false, tenant.ErrIdempotencyConflict
	}
	credentialID, err := tenant.NewCredentialID(record.ResourceID)
	if err != nil {
		return tenant.Credential{}, false, fmt.Errorf("tenant_credential.replay: %w", err)
	}
	var model TenantCredential
	if err := store.db.WithContext(ctx).Where(
		"credential_id = ? AND tenant_id = ?", credentialID.String(), input.Credential.TenantID().String(),
	).Take(&model).Error; err != nil {
		return tenant.Credential{}, false, fmt.Errorf("tenant_credential.replay: %w", err)
	}
	credential, err := mapCredential(model)
	if err != nil {
		return tenant.Credential{}, false, fmt.Errorf("tenant_credential.replay: %w", err)
	}
	return credential, true, nil
}

func (store *Store) ListCredentials(ctx context.Context, ownerID useraccount.ID, tenantID tenant.ID) ([]tenant.Credential, error) {
	if _, err := store.GetTenant(ctx, ownerID, tenantID); err != nil {
		return nil, err
	}
	var models []TenantCredential
	if err := store.db.WithContext(ctx).Where("tenant_id = ?", tenantID.String()).Order("created_at ASC").Order("credential_id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("tenant_credential.list: %w", err)
	}
	credentials := make([]tenant.Credential, 0, len(models))
	for _, model := range models {
		credential, err := mapCredential(model)
		if err != nil {
			return nil, fmt.Errorf("tenant_credential.list: %w", err)
		}
		credentials = append(credentials, credential)
	}
	return credentials, nil
}

func (store *Store) RevokeCredential(ctx context.Context, ownerID useraccount.ID, tenantID tenant.ID, credentialID tenant.CredentialID, revokedAt time.Time, requestID string) (tenant.Credential, error) {
	var model TenantCredential
	err := store.db.WithContext(ctx).Transaction(func(transaction *gorm.DB) error {
		var owned LedgerTenant
		if err := transaction.Where("tenant_id = ? AND owner_user_account_id = ?", tenantID.String(), ownerID.String()).Take(&owned).Error; err != nil {
			return err
		}
		if err := transaction.Where("credential_id = ? AND tenant_id = ?", credentialID.String(), tenantID.String()).Take(&model).Error; err != nil {
			return err
		}
		if model.RevokedAt != nil {
			return nil
		}
		value := revokedAt.UTC()
		if err := transaction.Model(&model).Update("revoked_at", value).Error; err != nil {
			return err
		}
		model.RevokedAt = &value
		return transaction.Create(&ControlEvent{
			ActorUserAccountID: ownerID.String(),
			EventType:          controlEventCredentialRevoked,
			ResourceType:       controlResourceCredential,
			ResourceID:         credentialID.String(),
			RequestID:          requestID,
			CreatedAt:          value,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tenant.Credential{}, tenant.ErrNotFound
	}
	if err != nil {
		return tenant.Credential{}, fmt.Errorf("tenant_credential.revoke: %w", err)
	}
	credential, err := mapCredential(model)
	if err != nil {
		return tenant.Credential{}, fmt.Errorf("tenant_credential.revoke: %w", err)
	}
	return credential, nil
}

func (store *Store) FindCredential(ctx context.Context, credentialID tenant.CredentialID) (tenant.StoredCredential, error) {
	var model TenantCredential
	if err := store.db.WithContext(ctx).Where("credential_id = ?", credentialID.String()).Take(&model).Error; err != nil {
		return tenant.StoredCredential{}, tenant.ErrCredentialInvalid
	}
	credential, err := mapCredential(model)
	if err != nil {
		return tenant.StoredCredential{}, fmt.Errorf("tenant_credential.find: %w", err)
	}
	return tenant.StoredCredential{Credential: credential, Digest: append([]byte(nil), model.SecretDigest...)}, nil
}

func mapCredential(model TenantCredential) (tenant.Credential, error) {
	credentialID, err := tenant.NewCredentialID(model.CredentialID)
	if err != nil {
		return tenant.Credential{}, err
	}
	tenantID, err := tenant.NewID(model.TenantID)
	if err != nil {
		return tenant.Credential{}, err
	}
	return tenant.NewCredential(credentialID, tenantID, model.CreatedAt, model.RevokedAt)
}
