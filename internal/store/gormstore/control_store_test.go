package gormstore

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	controlAccountID    = "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"
	controlOtherID      = "0196f0ec-3e80-7a54-bd2b-56cfe90bf820"
	controlTenantID     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf801"
	controlOtherTenant  = "0196f0ec-3e80-7a54-bd2b-56cfe90bf802"
	controlCredentialID = "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
)

func newControlStore(test *testing.T) (*Store, *gorm.DB) {
	test.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		test.Fatalf("open: %v", err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		test.Fatalf("sql database: %v", err)
	}
	test.Cleanup(func() { _ = sqlDatabase.Close() })
	if err := database.Exec("PRAGMA foreign_keys=ON;").Error; err != nil {
		test.Fatalf("foreign keys: %v", err)
	}
	if err := database.AutoMigrate(
		&UserAccount{},
		&LedgerTenant{},
		&TenantCredential{},
		&IdempotencyRecord{},
		&ControlEvent{},
		&LedgerAccount{},
		&LedgerEntry{},
		&Reservation{},
	); err != nil {
		test.Fatalf("migrate: %v", err)
	}
	return New(database), database
}

func makeAccount(test *testing.T, idValue string, userID string, createdAt time.Time) useraccount.Account {
	test.Helper()
	id, err := useraccount.NewID(idValue)
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	identity, err := useraccount.NewExternalIdentity("tauth", "mprlab", userID)
	if err != nil {
		test.Fatalf("identity: %v", err)
	}
	account, err := useraccount.NewAccount(id, identity, createdAt)
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	return account
}

func makeTenant(test *testing.T, idValue string, owner useraccount.ID, nameValue string, createdAt time.Time) tenant.Tenant {
	test.Helper()
	id, _ := tenant.NewID(idValue)
	name, _ := tenant.NewName(nameValue)
	item, err := tenant.NewTenant(id, owner, name, createdAt)
	if err != nil {
		test.Fatalf("tenant: %v", err)
	}
	return item
}

func TestControlStoreLifecycle(test *testing.T) {
	store, database := newControlStore(test)
	ctx := context.Background()
	createdAt := time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	account := makeAccount(test, controlAccountID, "user-one", createdAt)
	provisioned, created, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request-provision"})
	if err != nil || !created || provisioned != account {
		test.Fatalf("provision: created=%v err=%v", created, err)
	}
	replayCandidate := makeAccount(test, controlOtherID, "user-one", createdAt.Add(time.Hour))
	provisioned, created, err = store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: replayCandidate, RequestID: "request-replay"})
	if err != nil || created || provisioned != account {
		test.Fatalf("provision replay: created=%v account=%v err=%v", created, provisioned, err)
	}
	gotAccount, err := store.GetUserAccount(ctx, account.Identity())
	if err != nil || gotAccount != account {
		test.Fatalf("get account: %v", err)
	}
	missingIdentity, _ := useraccount.NewExternalIdentity("tauth", "mprlab", "missing")
	if _, err := store.GetUserAccount(ctx, missingIdentity); !errors.Is(err, useraccount.ErrNotFound) {
		test.Fatalf("expected missing account, got %v", err)
	}

	item := makeTenant(test, controlTenantID, account.ID(), "One", createdAt)
	key, _ := tenant.NewIdempotencyKey("tenant-key")
	input := tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request-tenant"}
	createdTenant, created, err := store.Create(ctx, input)
	if err != nil || !created || createdTenant != item {
		test.Fatalf("create tenant: created=%v err=%v", created, err)
	}
	replayTenant, created, err := store.Create(ctx, input)
	if err != nil || created || replayTenant != item {
		test.Fatalf("replay tenant: created=%v err=%v", created, err)
	}
	conflicting := input
	conflicting.Request = tenant.NewRequestDigest("Different")
	if _, _, err := store.Create(ctx, conflicting); !errors.Is(err, tenant.ErrIdempotencyConflict) {
		test.Fatalf("expected tenant idempotency conflict, got %v", err)
	}
	listed, err := store.List(ctx, account.ID(), nil, 10)
	if err != nil || len(listed) != 1 || listed[0] != item {
		test.Fatalf("list tenants: %v", err)
	}
	cursorID, _ := tenant.NewID(controlOtherTenant)
	cursor := &tenant.Cursor{CreatedAt: createdAt.Add(-time.Second), TenantID: cursorID}
	if listed, err := store.List(ctx, account.ID(), cursor, 10); err != nil || len(listed) != 1 {
		test.Fatalf("cursor list tenants: %v", err)
	}
	gotTenant, err := store.GetTenant(ctx, account.ID(), item.ID())
	if err != nil || gotTenant != item {
		test.Fatalf("get tenant: %v", err)
	}
	otherAccount := makeAccount(test, controlOtherID, "user-two", createdAt)
	if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: otherAccount, RequestID: "other"}); err != nil {
		test.Fatalf("provision other: %v", err)
	}
	if _, err := store.GetTenant(ctx, otherAccount.ID(), item.ID()); !errors.Is(err, tenant.ErrNotFound) {
		test.Fatalf("cross-owner tenant read: %v", err)
	}

	credentialID, _ := tenant.NewCredentialID(controlCredentialID)
	credential, _ := tenant.NewCredential(credentialID, item.ID(), createdAt, nil)
	credentialKey, _ := tenant.NewIdempotencyKey("credential-key")
	credentialInput := tenant.CreateCredentialInput{
		Credential: credential, SecretDigest: bytes.Repeat([]byte{7}, 32), Idempotency: credentialKey,
		Request: tenant.NewRequestDigest(item.ID().String()), RequestID: "request-credential",
	}
	createdCredential, created, err := store.CreateCredential(ctx, account.ID(), credentialInput)
	if err != nil || !created || createdCredential != credential {
		test.Fatalf("create credential: created=%v err=%v", created, err)
	}
	replayCredential, created, err := store.CreateCredential(ctx, account.ID(), credentialInput)
	if err != nil || created || replayCredential != credential {
		test.Fatalf("replay credential: created=%v err=%v", created, err)
	}
	credentialConflict := credentialInput
	credentialConflict.Request = tenant.NewRequestDigest(controlOtherTenant)
	if _, _, err := store.CreateCredential(ctx, account.ID(), credentialConflict); !errors.Is(err, tenant.ErrIdempotencyConflict) {
		test.Fatalf("expected credential idempotency conflict, got %v", err)
	}
	credentials, err := store.ListCredentials(ctx, account.ID(), item.ID())
	if err != nil || len(credentials) != 1 || credentials[0] != credential {
		test.Fatalf("list credentials: %v", err)
	}
	stored, err := store.FindCredential(ctx, credentialID)
	if err != nil || stored.Credential != credential || len(stored.Digest) != 32 {
		test.Fatalf("find credential: %v", err)
	}
	if _, err := store.FindCredential(ctx, tenant.CredentialID{}); !errors.Is(err, tenant.ErrCredentialInvalid) {
		test.Fatalf("missing credential must be opaque: %v", err)
	}
	revokedAt := createdAt.Add(time.Hour)
	revoked, err := store.RevokeCredential(ctx, account.ID(), item.ID(), credentialID, revokedAt, "request-revoke")
	if err != nil {
		test.Fatalf("revoke: %v", err)
	}
	gotRevokedAt, ok := revoked.RevokedAt()
	if !ok || !gotRevokedAt.Equal(revokedAt) {
		test.Fatalf("revocation time")
	}
	if _, err := store.RevokeCredential(ctx, account.ID(), item.ID(), credentialID, revokedAt.Add(time.Hour), "request-repeat"); err != nil {
		test.Fatalf("repeat revoke: %v", err)
	}
	if _, err := store.RevokeCredential(ctx, otherAccount.ID(), item.ID(), credentialID, revokedAt, "cross-owner"); !errors.Is(err, tenant.ErrNotFound) {
		test.Fatalf("cross-owner revoke: %v", err)
	}

	var events []ControlEvent
	if err := database.Order("created_at ASC").Find(&events).Error; err != nil || len(events) != 5 {
		test.Fatalf("events: count=%d err=%v", len(events), err)
	}
}

func TestControlStoreFailuresAndMapping(test *testing.T) {
	store, database := newControlStore(test)
	ctx := context.Background()
	createdAt := time.Now().UTC()
	missingOwner, _ := useraccount.NewID(controlAccountID)
	item := makeTenant(test, controlTenantID, missingOwner, "One", createdAt)
	key, _ := tenant.NewIdempotencyKey("key")
	input := tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}
	if _, _, err := store.Create(ctx, input); err == nil {
		test.Fatalf("tenant without owner was created")
	}
	credentialID, _ := tenant.NewCredentialID(controlCredentialID)
	credential, _ := tenant.NewCredential(credentialID, item.ID(), createdAt, nil)
	credentialInput := tenant.CreateCredentialInput{Credential: credential, SecretDigest: []byte{1}, Idempotency: key, Request: tenant.NewRequestDigest(item.ID().String()), RequestID: "request"}
	if _, _, err := store.CreateCredential(ctx, missingOwner, credentialInput); !errors.Is(err, tenant.ErrNotFound) {
		test.Fatalf("credential without tenant: %v", err)
	}
	if _, err := store.ListCredentials(ctx, missingOwner, item.ID()); !errors.Is(err, tenant.ErrNotFound) {
		test.Fatalf("credentials without tenant: %v", err)
	}

	if _, err := mapUserAccount(UserAccount{}); err == nil {
		test.Fatalf("invalid user account model mapped")
	}
	if _, err := mapUserAccount(UserAccount{UserAccountID: controlAccountID, AuthIssuer: "tauth", AuthTenantID: "mprlab", AuthUserID: "user"}); err == nil {
		test.Fatalf("zero user account time mapped")
	}
	if _, err := mapTenant(LedgerTenant{}); err == nil {
		test.Fatalf("invalid tenant id mapped")
	}
	if _, err := mapTenant(LedgerTenant{TenantID: controlTenantID}); err == nil {
		test.Fatalf("invalid tenant owner mapped")
	}
	if _, err := mapTenant(LedgerTenant{TenantID: controlTenantID, OwnerUserAccountID: controlAccountID}); err == nil {
		test.Fatalf("invalid tenant name mapped")
	}
	if _, err := mapTenant(LedgerTenant{TenantID: controlTenantID, OwnerUserAccountID: controlAccountID, Name: "One"}); err == nil {
		test.Fatalf("zero tenant time mapped")
	}
	if _, err := mapCredential(TenantCredential{}); err == nil {
		test.Fatalf("invalid credential id mapped")
	}
	if _, err := mapCredential(TenantCredential{CredentialID: controlCredentialID}); err == nil {
		test.Fatalf("invalid credential tenant mapped")
	}
	if _, err := mapCredential(TenantCredential{CredentialID: controlCredentialID, TenantID: controlTenantID}); err == nil {
		test.Fatalf("zero credential time mapped")
	}

	account := makeAccount(test, controlAccountID, "user", createdAt)
	if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request"}); err != nil {
		test.Fatalf("provision before close: %v", err)
	}
	sqlDatabase, _ := database.DB()
	if err := sqlDatabase.Close(); err != nil {
		test.Fatalf("close: %v", err)
	}
	if _, err := store.GetUserAccount(ctx, account.Identity()); err == nil {
		test.Fatalf("closed get account succeeded")
	}
	if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request"}); err == nil {
		test.Fatalf("closed provision succeeded")
	}
	if _, err := store.List(ctx, account.ID(), nil, 1); err == nil {
		test.Fatalf("closed list succeeded")
	}
	if _, err := store.GetTenant(ctx, account.ID(), item.ID()); err == nil {
		test.Fatalf("closed get tenant succeeded")
	}
	if _, err := store.FindCredential(ctx, credentialID); !errors.Is(err, tenant.ErrCredentialInvalid) {
		test.Fatalf("closed credential lookup leaked error")
	}
}

func provisionControlOwner(test *testing.T, store *Store, createdAt time.Time) useraccount.Account {
	test.Helper()
	account := makeAccount(test, controlAccountID, "owner", createdAt)
	if _, _, err := store.ProvisionUserAccount(context.Background(), useraccount.ProvisionInput{Account: account, RequestID: "seed-owner"}); err != nil {
		test.Fatalf("seed owner: %v", err)
	}
	return account
}

func TestControlStoreRejectsInvalidPersistedModels(test *testing.T) {
	store, database := newControlStore(test)
	ctx := context.Background()
	createdAt := time.Now().UTC()
	owner := provisionControlOwner(test, store, createdAt)

	invalidIdentity := UserAccount{UserAccountID: controlOtherID, AuthIssuer: "", AuthTenantID: "mprlab", AuthUserID: "invalid-identity", CreatedAt: createdAt}
	if err := database.Create(&invalidIdentity).Error; err != nil {
		test.Fatalf("insert invalid identity: %v", err)
	}
	identity, _ := useraccount.NewExternalIdentity("tauth", "mprlab", "lookup")
	if err := database.Model(&invalidIdentity).Updates(map[string]any{"auth_issuer": identity.Issuer(), "auth_user_id": identity.UserID(), "user_account_id": "bad-id"}).Error; err != nil {
		test.Fatalf("prepare invalid account: %v", err)
	}
	if _, err := store.GetUserAccount(ctx, identity); err == nil {
		test.Fatalf("invalid user account id mapped")
	}
	if _, err := mapUserAccount(UserAccount{UserAccountID: controlOtherID, AuthIssuer: "", AuthTenantID: "mprlab", AuthUserID: "user", CreatedAt: createdAt}); err == nil {
		test.Fatalf("invalid user identity mapped")
	}

	if err := database.Create(&LedgerTenant{TenantID: "bad-id", OwnerUserAccountID: owner.ID().String(), Name: "Bad", CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("insert invalid tenant: %v", err)
	}
	if _, err := store.List(ctx, owner.ID(), nil, 10); err == nil {
		test.Fatalf("invalid listed tenant mapped")
	}
	if err := database.Where("tenant_id = ?", "bad-id").Delete(&LedgerTenant{}).Error; err != nil {
		test.Fatalf("delete invalid tenant: %v", err)
	}
	if err := database.Create(&LedgerTenant{TenantID: controlTenantID, OwnerUserAccountID: owner.ID().String(), Name: "", CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("insert invalid named tenant: %v", err)
	}
	tenantID, _ := tenant.NewID(controlTenantID)
	if _, err := store.GetTenant(ctx, owner.ID(), tenantID); err == nil {
		test.Fatalf("invalid tenant mapped")
	}
	if err := database.Model(&LedgerTenant{}).Where("tenant_id = ?", controlTenantID).Update("name", "Valid").Error; err != nil {
		test.Fatalf("repair tenant: %v", err)
	}
	if err := database.Create(&TenantCredential{CredentialID: "bad-id", TenantID: controlTenantID, SecretDigest: []byte{1}, CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("insert invalid credential: %v", err)
	}
	if _, err := store.ListCredentials(ctx, owner.ID(), tenantID); err == nil {
		test.Fatalf("invalid listed credential mapped")
	}
	if _, err := store.FindCredential(ctx, tenant.CredentialID{}); !errors.Is(err, tenant.ErrCredentialInvalid) {
		test.Fatalf("empty credential lookup: %v", err)
	}
}

func TestControlStoreTransactionFailures(test *testing.T) {
	createdAt := time.Now().UTC()
	ctx := context.Background()

	test.Run("provision event", func(test *testing.T) {
		store, database := newControlStore(test)
		if err := database.Migrator().DropTable(&ControlEvent{}); err != nil {
			test.Fatalf("drop events: %v", err)
		}
		account := makeAccount(test, controlAccountID, "owner", createdAt)
		if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request"}); err == nil {
			test.Fatalf("provision without event table succeeded")
		}
	})

	test.Run("tenant idempotency", func(test *testing.T) {
		store, database := newControlStore(test)
		owner := provisionControlOwner(test, store, createdAt)
		item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
		key, _ := tenant.NewIdempotencyKey("key")
		input := tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}
		if err := database.Migrator().DropTable(&IdempotencyRecord{}); err != nil {
			test.Fatalf("drop idempotency: %v", err)
		}
		if _, _, err := store.Create(ctx, input); err == nil {
			test.Fatalf("tenant without idempotency table succeeded")
		}
	})

	test.Run("tenant event", func(test *testing.T) {
		store, database := newControlStore(test)
		owner := provisionControlOwner(test, store, createdAt)
		item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
		key, _ := tenant.NewIdempotencyKey("key")
		if err := database.Migrator().DropTable(&ControlEvent{}); err != nil {
			test.Fatalf("drop events: %v", err)
		}
		if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}); err == nil {
			test.Fatalf("tenant without event table succeeded")
		}
	})

	test.Run("credential idempotency and event", func(test *testing.T) {
		for _, table := range []any{&IdempotencyRecord{}, &ControlEvent{}} {
			store, database := newControlStore(test)
			owner := provisionControlOwner(test, store, createdAt)
			item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
			key, _ := tenant.NewIdempotencyKey("tenant-key")
			if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}); err != nil {
				test.Fatalf("create tenant: %v", err)
			}
			credentialID, _ := tenant.NewCredentialID(controlCredentialID)
			credential, _ := tenant.NewCredential(credentialID, item.ID(), createdAt, nil)
			credentialKey, _ := tenant.NewIdempotencyKey("credential-key")
			if err := database.Migrator().DropTable(table); err != nil {
				test.Fatalf("drop table: %v", err)
			}
			if _, _, err := store.CreateCredential(ctx, owner.ID(), tenant.CreateCredentialInput{Credential: credential, SecretDigest: []byte{1}, Idempotency: credentialKey, Request: tenant.NewRequestDigest(item.ID().String()), RequestID: "request"}); err == nil {
				test.Fatalf("credential transaction succeeded after table drop")
			}
		}
	})
}

func TestControlStoreReplayAndRevocationFailures(test *testing.T) {
	store, database := newControlStore(test)
	ctx := context.Background()
	createdAt := time.Now().UTC()
	owner := provisionControlOwner(test, store, createdAt)
	item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
	key, _ := tenant.NewIdempotencyKey("tenant-key")
	requestDigest := tenant.NewRequestDigest("One")
	if err := database.Create(&IdempotencyRecord{UserAccountID: owner.ID().String(), Operation: controlOperationTenantCreate, KeyDigest: key.Digest(), RequestDigest: requestDigest.Bytes(), ResourceID: "bad-id", CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("seed invalid tenant replay: %v", err)
	}
	if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: requestDigest, RequestID: "request"}); err == nil {
		test.Fatalf("invalid tenant replay resource succeeded")
	}
	if err := database.Where("operation = ?", controlOperationTenantCreate).Delete(&IdempotencyRecord{}).Error; err != nil {
		test.Fatalf("delete tenant replay: %v", err)
	}
	missingTenantID, _ := tenant.NewID(controlOtherTenant)
	if err := database.Create(&IdempotencyRecord{UserAccountID: owner.ID().String(), Operation: controlOperationTenantCreate, KeyDigest: key.Digest(), RequestDigest: requestDigest.Bytes(), ResourceID: missingTenantID.String(), CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("seed missing tenant replay: %v", err)
	}
	if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: requestDigest, RequestID: "request"}); !errors.Is(err, tenant.ErrNotFound) {
		test.Fatalf("missing replay tenant: %v", err)
	}
	if err := database.Where("operation = ?", controlOperationTenantCreate).Delete(&IdempotencyRecord{}).Error; err != nil {
		test.Fatalf("delete replay: %v", err)
	}
	if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: requestDigest, RequestID: "request"}); err != nil {
		test.Fatalf("create tenant: %v", err)
	}

	credentialID, _ := tenant.NewCredentialID(controlCredentialID)
	credential, _ := tenant.NewCredential(credentialID, item.ID(), createdAt, nil)
	credentialKey, _ := tenant.NewIdempotencyKey("credential-key")
	credentialRequest := tenant.NewRequestDigest(item.ID().String())
	if err := database.Create(&IdempotencyRecord{UserAccountID: owner.ID().String(), Operation: controlOperationCredentialCreate, KeyDigest: credentialKey.Digest(), RequestDigest: credentialRequest.Bytes(), ResourceID: "bad-id", CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("seed invalid credential replay: %v", err)
	}
	credentialInput := tenant.CreateCredentialInput{Credential: credential, SecretDigest: []byte{1}, Idempotency: credentialKey, Request: credentialRequest, RequestID: "request"}
	if _, _, err := store.CreateCredential(ctx, owner.ID(), credentialInput); err == nil {
		test.Fatalf("invalid credential replay resource succeeded")
	}
	if err := database.Where("operation = ?", controlOperationCredentialCreate).Delete(&IdempotencyRecord{}).Error; err != nil {
		test.Fatalf("delete credential replay: %v", err)
	}
	missingCredentialID, _ := tenant.NewCredentialID("0196f0ec-3e80-7a54-bd2b-56cfe90bf812")
	if err := database.Create(&IdempotencyRecord{UserAccountID: owner.ID().String(), Operation: controlOperationCredentialCreate, KeyDigest: credentialKey.Digest(), RequestDigest: credentialRequest.Bytes(), ResourceID: missingCredentialID.String(), CreatedAt: createdAt}).Error; err != nil {
		test.Fatalf("seed missing credential replay: %v", err)
	}
	if _, _, err := store.CreateCredential(ctx, owner.ID(), credentialInput); err == nil {
		test.Fatalf("missing credential replay succeeded")
	}
	if err := database.Where("operation = ?", controlOperationCredentialCreate).Delete(&IdempotencyRecord{}).Error; err != nil {
		test.Fatalf("delete credential replay: %v", err)
	}
	if _, _, err := store.CreateCredential(ctx, owner.ID(), credentialInput); err != nil {
		test.Fatalf("create credential: %v", err)
	}
	if err := database.Migrator().DropTable(&ControlEvent{}); err != nil {
		test.Fatalf("drop events: %v", err)
	}
	if _, err := store.RevokeCredential(ctx, owner.ID(), item.ID(), credentialID, createdAt.Add(time.Hour), "request"); err == nil {
		test.Fatalf("revoke without event table succeeded")
	}
}

func TestControlStoreRemainingStorageFailures(test *testing.T) {
	ctx := context.Background()
	createdAt := time.Now().UTC()

	test.Run("provision create and map", func(test *testing.T) {
		store, database := newControlStore(test)
		account := makeAccount(test, controlAccountID, "owner", createdAt)
		if err := database.Migrator().DropTable(&UserAccount{}); err != nil {
			test.Fatalf("drop user accounts: %v", err)
		}
		if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request"}); err == nil {
			test.Fatalf("provision without user table succeeded")
		}

		store, database = newControlStore(test)
		identity := account.Identity()
		if err := database.Create(&UserAccount{UserAccountID: "bad-id", AuthIssuer: identity.Issuer(), AuthTenantID: identity.TenantID(), AuthUserID: identity.UserID(), CreatedAt: createdAt}).Error; err != nil {
			test.Fatalf("seed invalid account: %v", err)
		}
		if _, _, err := store.ProvisionUserAccount(ctx, useraccount.ProvisionInput{Account: account, RequestID: "request"}); err == nil {
			test.Fatalf("invalid replayed account mapped")
		}
	})

	test.Run("credential replay map", func(test *testing.T) {
		store, database := newControlStore(test)
		owner := provisionControlOwner(test, store, createdAt)
		item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
		key, _ := tenant.NewIdempotencyKey("tenant-key")
		if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}); err != nil {
			test.Fatalf("create tenant: %v", err)
		}
		credentialID, _ := tenant.NewCredentialID(controlCredentialID)
		credential, _ := tenant.NewCredential(credentialID, item.ID(), createdAt, nil)
		credentialKey, _ := tenant.NewIdempotencyKey("credential-key")
		credentialRequest := tenant.NewRequestDigest(item.ID().String())
		if err := database.Create(&TenantCredential{CredentialID: credentialID.String(), TenantID: item.ID().String(), SecretDigest: []byte{1}, CreatedAt: time.Time{}}).Error; err != nil {
			test.Fatalf("seed invalid credential: %v", err)
		}
		if err := database.Model(&TenantCredential{}).Where("credential_id = ?", credentialID.String()).Update("created_at", time.Time{}).Error; err != nil {
			test.Fatalf("invalidate credential time: %v", err)
		}
		if err := database.Create(&IdempotencyRecord{UserAccountID: owner.ID().String(), Operation: controlOperationCredentialCreate, KeyDigest: credentialKey.Digest(), RequestDigest: credentialRequest.Bytes(), ResourceID: credentialID.String(), CreatedAt: createdAt}).Error; err != nil {
			test.Fatalf("seed replay: %v", err)
		}
		input := tenant.CreateCredentialInput{Credential: credential, SecretDigest: []byte{1}, Idempotency: credentialKey, Request: credentialRequest, RequestID: "request"}
		if _, _, err := store.CreateCredential(ctx, owner.ID(), input); err == nil {
			test.Fatalf("invalid replayed credential mapped")
		}
	})

	test.Run("credential list query", func(test *testing.T) {
		store, database := newControlStore(test)
		owner := provisionControlOwner(test, store, createdAt)
		item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
		key, _ := tenant.NewIdempotencyKey("tenant-key")
		if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}); err != nil {
			test.Fatalf("create tenant: %v", err)
		}
		if err := database.Migrator().DropTable(&TenantCredential{}); err != nil {
			test.Fatalf("drop credentials: %v", err)
		}
		if _, err := store.ListCredentials(ctx, owner.ID(), item.ID()); err == nil {
			test.Fatalf("credential list without table succeeded")
		}
	})

	test.Run("revoke missing update and map", func(test *testing.T) {
		makeCredential := func(test *testing.T, zeroTime bool) (*Store, *gorm.DB, useraccount.Account, tenant.Tenant, tenant.CredentialID) {
			store, database := newControlStore(test)
			owner := provisionControlOwner(test, store, createdAt)
			item := makeTenant(test, controlTenantID, owner.ID(), "One", createdAt)
			key, _ := tenant.NewIdempotencyKey("tenant-key")
			if _, _, err := store.Create(ctx, tenant.CreateInput{Tenant: item, Idempotency: key, Request: tenant.NewRequestDigest("One"), RequestID: "request"}); err != nil {
				test.Fatalf("create tenant: %v", err)
			}
			credentialID, _ := tenant.NewCredentialID(controlCredentialID)
			credentialTime := createdAt
			if zeroTime {
				credentialTime = time.Time{}
			}
			if err := database.Create(&TenantCredential{CredentialID: credentialID.String(), TenantID: item.ID().String(), SecretDigest: []byte{1}, CreatedAt: credentialTime}).Error; err != nil {
				test.Fatalf("seed credential: %v", err)
			}
			if zeroTime {
				if err := database.Model(&TenantCredential{}).Where("credential_id = ?", credentialID.String()).Update("created_at", time.Time{}).Error; err != nil {
					test.Fatalf("invalidate credential time: %v", err)
				}
			}
			return store, database, owner, item, credentialID
		}

		store, _, owner, item, _ := makeCredential(test, false)
		missingID, _ := tenant.NewCredentialID("0196f0ec-3e80-7a54-bd2b-56cfe90bf812")
		if _, err := store.RevokeCredential(ctx, owner.ID(), item.ID(), missingID, createdAt, "request"); !errors.Is(err, tenant.ErrNotFound) {
			test.Fatalf("missing credential revoke: %v", err)
		}

		store, database, owner, item, credentialID := makeCredential(test, false)
		if err := database.Callback().Update().Before("gorm:update").Register("force_update_error", func(transaction *gorm.DB) {
			transaction.AddError(errors.New("forced update error"))
		}); err != nil {
			test.Fatalf("register update callback: %v", err)
		}
		if _, err := store.RevokeCredential(ctx, owner.ID(), item.ID(), credentialID, createdAt, "request"); err == nil {
			test.Fatalf("forced update error did not fail revoke")
		}

		store, _, owner, item, credentialID = makeCredential(test, true)
		if _, err := store.RevokeCredential(ctx, owner.ID(), item.ID(), credentialID, createdAt, "request"); err == nil {
			test.Fatalf("invalid revoked credential mapped")
		}
		if _, err := store.FindCredential(ctx, credentialID); err == nil {
			test.Fatalf("invalid stored credential mapped")
		}
	})
}
