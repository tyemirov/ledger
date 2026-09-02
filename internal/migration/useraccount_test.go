package migration

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	migrationAccountID     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"
	migrationOtherOwnerID  = "0196f0ec-3e80-7a54-bd2b-56cfe90bf820"
	migrationTenantOne     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf801"
	migrationTenantTwo     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf802"
	migrationCredentialOne = "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
	migrationCredentialTwo = "0196f0ec-3e80-7a54-bd2b-56cfe90bf812"
)

func openLegacyDatabase(test *testing.T, tenants ...string) *gorm.DB {
	test.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(test.Name(), "/", "_")+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		test.Fatalf("open database: %v", err)
	}
	sqlDatabase, _ := database.DB()
	test.Cleanup(func() { _ = sqlDatabase.Close() })
	statements := []string{
		`CREATE TABLE accounts (account_id text PRIMARY KEY, tenant_id text NOT NULL, user_id text NOT NULL, ledger_id text NOT NULL, created_at datetime NOT NULL)`,
		`CREATE UNIQUE INDEX idx_accounts_tenant_user_ledger ON accounts(tenant_id, user_id, ledger_id)`,
		`CREATE TABLE ledger_entries (entry_id text PRIMARY KEY, account_id text NOT NULL, type text NOT NULL, amount_cents integer NOT NULL, idempotency_key text NOT NULL, metadata blob NOT NULL, created_at datetime NOT NULL)`,
	}
	for _, statement := range statements {
		if err := database.Exec(statement).Error; err != nil {
			test.Fatalf("legacy schema: %v", err)
		}
	}
	for index, legacyTenantID := range tenants {
		accountID := "0196f0ec-3e80-7a54-bd2b-56cfe90bf9" + string(rune('0'+index)) + "0"
		if err := database.Exec(`INSERT INTO accounts(account_id, tenant_id, user_id, ledger_id, created_at) VALUES (?, ?, ?, ?, ?)`, accountID, legacyTenantID, "user", "default", time.Now().UTC()).Error; err != nil {
			test.Fatalf("legacy account: %v", err)
		}
		if err := database.Exec(`INSERT INTO ledger_entries(entry_id, account_id, type, amount_cents, idempotency_key, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, "entry-"+legacyTenantID, accountID, "grant", 100, "grant-1", []byte("{}"), time.Now().UTC()).Error; err != nil {
			test.Fatalf("legacy entry: %v", err)
		}
	}
	return database
}

func migrationSecret(credentialID string, fill byte) string {
	part := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
	return "ledger_" + credentialID + "_" + part
}

func validMapping() File {
	owner := OwnerMapping{UserAccountID: migrationAccountID, AuthIssuer: "tauth", AuthTenantID: "mprlab", AuthUserID: "owner"}
	return File{
		LegacyTenantIDs: []string{"legacy-one", "legacy-two"},
		Tenants: []TenantMapping{
			{LegacyTenantID: "legacy-one", TenantID: migrationTenantOne, Name: "One", Owner: owner, CredentialID: migrationCredentialOne, CredentialSecret: migrationSecret(migrationCredentialOne, 255)},
			{LegacyTenantID: "legacy-two", TenantID: migrationTenantTwo, Name: "Two", Owner: owner, CredentialID: migrationCredentialTwo, CredentialSecret: migrationSecret(migrationCredentialTwo, 2)},
		},
	}
}

func TestMigrationLoadAndApply(test *testing.T) {
	directory := test.TempDir()
	path := filepath.Join(directory, "mapping.yml")
	content := `legacy_tenant_ids: [legacy-one, legacy-two]
tenants:
  - legacy_tenant_id: legacy-one
    tenant_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf801
    name: One
    owner:
      user_account_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf810
      auth_issuer: tauth
      auth_tenant_id: mprlab
      auth_user_id: owner
    credential_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf811
    credential_secret: ` + migrationSecret(migrationCredentialOne, 255) + `
  - legacy_tenant_id: legacy-two
    tenant_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf802
    name: Two
    owner:
      user_account_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf810
      auth_issuer: tauth
      auth_tenant_id: mprlab
      auth_user_id: owner
    credential_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf812
    credential_secret: ` + migrationSecret(migrationCredentialTwo, 2) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		test.Fatalf("write mapping: %v", err)
	}
	mapping, err := Load(path)
	if err != nil || len(mapping.Tenants) != 2 {
		test.Fatalf("load mapping: %v", err)
	}

	database := openLegacyDatabase(test, "legacy-one")
	if err := Apply(context.Background(), database, mapping); err != nil {
		test.Fatalf("apply: %v", err)
	}
	if database.Migrator().HasTable("accounts") || !database.Migrator().HasTable("ledger_accounts") {
		test.Fatalf("account table was not renamed")
	}
	columnTypes, err := database.Migrator().ColumnTypes(&gormstore.LedgerAccount{})
	if err != nil {
		test.Fatalf("ledger account columns: %v", err)
	}
	foundTenantUUID := false
	for _, columnType := range columnTypes {
		if columnType.Name() == "tenant_id" && strings.EqualFold(columnType.DatabaseTypeName(), "uuid") {
			foundTenantUUID = true
		}
	}
	if !foundTenantUUID {
		test.Fatalf("migrated tenant_id column is not UUID")
	}
	if !database.Migrator().HasConstraint(&gormstore.LedgerTenant{}, "Accounts") {
		test.Fatalf("ledger account tenant constraint is missing")
	}
	var accounts []gormstore.LedgerAccount
	if err := database.Find(&accounts).Error; err != nil || len(accounts) != 1 || accounts[0].TenantID != migrationTenantOne {
		test.Fatalf("migrated accounts: %v %+v", err, accounts)
	}
	var entryCount int64
	if err := database.Table("ledger_entries").Where("entry_id = ? AND amount_cents = ?", "entry-legacy-one", 100).Count(&entryCount).Error; err != nil || entryCount != 1 {
		test.Fatalf("accounting history changed: count=%d err=%v", entryCount, err)
	}
	for model, count := range map[any]int64{
		&gormstore.UserAccount{}:      1,
		&gormstore.LedgerTenant{}:     2,
		&gormstore.TenantCredential{}: 2,
		&gormstore.ControlEvent{}:     5,
	} {
		var got int64
		if err := database.Model(model).Count(&got).Error; err != nil || got != count {
			test.Fatalf("model %T count=%d want=%d err=%v", model, got, count, err)
		}
	}
	store := gormstore.New(database)
	service, _ := tenant.NewDefaultService(store)
	addressed, _ := tenant.NewID(migrationTenantOne)
	if authenticatedID, err := service.Authenticate(context.Background(), migrationSecret(migrationCredentialOne, 255)); err != nil || authenticatedID != addressed {
		test.Fatalf("migrated credential: %v", err)
	}
	if err := Apply(context.Background(), database, mapping); err == nil {
		test.Fatalf("migration rerun succeeded")
	}
}

func TestMigrationLoadRejectsInvalidFiles(test *testing.T) {
	if _, err := Load(filepath.Join(test.TempDir(), "missing")); err == nil {
		test.Fatalf("missing mapping loaded")
	}
	directory := test.TempDir()
	for name, content := range map[string]string{
		"unknown.yml":  "unknown: true\n",
		"multiple.yml": "legacy_tenant_ids: []\n---\nlegacy_tenant_ids: []\n",
	} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			test.Fatalf("write: %v", err)
		}
		if _, err := Load(path); err == nil {
			test.Fatalf("invalid mapping %s loaded", name)
		}
	}
	insecurePath := filepath.Join(directory, "insecure.yml")
	if err := os.WriteFile(insecurePath, []byte("legacy_tenant_ids: []\n"), 0o644); err != nil {
		test.Fatalf("write insecure: %v", err)
	}
	if _, err := Load(insecurePath); err == nil {
		test.Fatalf("insecure mapping loaded")
	}
	directoryPath := filepath.Join(directory, "mapping-dir")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		test.Fatalf("mkdir: %v", err)
	}
	if _, err := Load(directoryPath); err == nil {
		test.Fatalf("directory mapping loaded")
	}
	unreadablePath := filepath.Join(directory, "unreadable.yml")
	if err := os.WriteFile(unreadablePath, []byte("legacy_tenant_ids: []\n"), 0o600); err != nil {
		test.Fatalf("write unreadable: %v", err)
	}
	if err := os.Chmod(unreadablePath, 0o000); err != nil {
		test.Fatalf("chmod unreadable: %v", err)
	}
	if _, err := Load(unreadablePath); err == nil {
		test.Fatalf("unreadable mapping loaded")
	}
}

func TestMigrationPreflightRejectsIncompleteMappings(test *testing.T) {
	if _, err := preflight(context.Background(), nil, File{}); err == nil {
		test.Fatalf("nil database accepted")
	}
	database := openLegacyDatabase(test, "legacy-one")
	cases := []struct {
		name   string
		mutate func(*File)
	}{
		{name: "empty", mutate: func(mapping *File) { mapping.LegacyTenantIDs = nil; mapping.Tenants = nil }},
		{name: "blank expected", mutate: func(mapping *File) { mapping.LegacyTenantIDs[0] = " " }},
		{name: "duplicate expected", mutate: func(mapping *File) { mapping.LegacyTenantIDs[1] = "legacy-one" }},
		{name: "mapping count", mutate: func(mapping *File) { mapping.Tenants = mapping.Tenants[:1] }},
		{name: "unknown mapping", mutate: func(mapping *File) { mapping.Tenants[0].LegacyTenantID = "unknown" }},
		{name: "duplicate mapping", mutate: func(mapping *File) { mapping.Tenants[1].LegacyTenantID = "legacy-one" }},
		{name: "invalid owner", mutate: func(mapping *File) { mapping.Tenants[0].Owner.UserAccountID = "bad" }},
		{name: "invalid identity", mutate: func(mapping *File) { mapping.Tenants[0].Owner.AuthUserID = "" }},
		{name: "identity multiple ids", mutate: func(mapping *File) { mapping.Tenants[1].Owner.UserAccountID = migrationOtherOwnerID }},
		{name: "id multiple identities", mutate: func(mapping *File) { mapping.Tenants[1].Owner.AuthUserID = "different" }},
		{name: "invalid tenant", mutate: func(mapping *File) { mapping.Tenants[0].TenantID = "bad" }},
		{name: "duplicate tenant", mutate: func(mapping *File) { mapping.Tenants[1].TenantID = migrationTenantOne }},
		{name: "invalid name", mutate: func(mapping *File) { mapping.Tenants[0].Name = "" }},
		{name: "invalid credential", mutate: func(mapping *File) { mapping.Tenants[0].CredentialSecret = "bad" }},
		{name: "credential mismatch", mutate: func(mapping *File) { mapping.Tenants[0].CredentialID = migrationCredentialTwo }},
		{name: "duplicate credential", mutate: func(mapping *File) {
			mapping.Tenants[1].CredentialID = migrationCredentialOne
			mapping.Tenants[1].CredentialSecret = migrationSecret(migrationCredentialOne, 1)
		}},
	}
	for _, testCase := range cases {
		test.Run(testCase.name, func(test *testing.T) {
			mapping := validMapping()
			testCase.mutate(&mapping)
			if _, err := preflight(context.Background(), database, mapping); err == nil {
				test.Fatalf("invalid mapping accepted")
			}
		})
	}

	unknownDatabase := openLegacyDatabase(test, "unknown")
	if _, err := preflight(context.Background(), unknownDatabase, validMapping()); err == nil {
		test.Fatalf("persisted tenant without mapping accepted")
	}
	noLegacy, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		test.Fatalf("open no-legacy database: %v", err)
	}
	if _, err := preflight(context.Background(), noLegacy, validMapping()); err == nil {
		test.Fatalf("database without legacy table accepted")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preflight(canceled, database, validMapping()); err == nil {
		test.Fatalf("canceled preflight succeeded")
	}
}

func TestMigrationApplyStageFailures(test *testing.T) {
	type stageCase struct {
		name    string
		install func(*testing.T, *gorm.DB)
	}
	createFailure := func(table string, eventType string) func(*testing.T, *gorm.DB) {
		return func(test *testing.T, database *gorm.DB) {
			test.Helper()
			name := "force_create_" + strings.ReplaceAll(table+eventType, ".", "_")
			if err := database.Callback().Create().Before("gorm:create").Register(name, func(transaction *gorm.DB) {
				if transaction.Statement.Table != table {
					return
				}
				if eventType != "" {
					event, ok := transaction.Statement.Dest.(*gormstore.ControlEvent)
					if !ok || event.EventType != eventType {
						return
					}
				}
				transaction.AddError(errors.New("forced create error"))
			}); err != nil {
				test.Fatalf("register create callback: %v", err)
			}
		}
	}
	cases := []stageCase{
		{name: "auto migrate", install: func(test *testing.T, database *gorm.DB) {
			if err := database.Exec(`CREATE VIEW user_accounts AS SELECT account_id AS user_account_id FROM accounts`).Error; err != nil {
				test.Fatalf("create view: %v", err)
			}
		}},
		{name: "rename", install: func(test *testing.T, database *gorm.DB) {
			if err := database.Exec(`CREATE TABLE ledger_accounts (account_id text PRIMARY KEY)`).Error; err != nil {
				test.Fatalf("create canonical table: %v", err)
			}
		}},
		{name: "user account", install: createFailure("user_accounts", "")},
		{name: "user event", install: createFailure("control_events", "user_account.provisioned")},
		{name: "tenant", install: createFailure("tenants", "")},
		{name: "tenant event", install: createFailure("control_events", "tenant.created")},
		{name: "credential", install: createFailure("tenant_credentials", "")},
		{name: "credential event", install: createFailure("control_events", "tenant_credential.created")},
		{name: "account update", install: func(test *testing.T, database *gorm.DB) {
			if err := database.Callback().Update().Before("gorm:update").Register("force_account_update", func(transaction *gorm.DB) {
				if transaction.Statement.Table == "ledger_accounts" {
					transaction.AddError(errors.New("forced update error"))
				}
			}); err != nil {
				test.Fatalf("register update callback: %v", err)
			}
		}},
		{name: "account tenant type", install: func(test *testing.T, database *gorm.DB) {
			if err := database.Exec(`CREATE TABLE ledger_accounts__temp (account_id text PRIMARY KEY)`).Error; err != nil {
				test.Fatalf("create migration conflict table: %v", err)
			}
		}},
	}
	for _, testCase := range cases {
		test.Run(testCase.name, func(test *testing.T) {
			database := openLegacyDatabase(test, "legacy-one")
			prepared, err := preflight(context.Background(), database, validMapping())
			if err != nil {
				test.Fatalf("preflight: %v", err)
			}
			testCase.install(test, database)
			if err := applyPrepared(context.Background(), database, prepared); err == nil {
				test.Fatalf("stage failure did not stop migration")
			}
			if !database.Migrator().HasTable("accounts") {
				test.Fatalf("failed migration did not roll back accounts")
			}
		})
	}
}

func TestControlEventError(test *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		test.Fatalf("open: %v", err)
	}
	if err := database.AutoMigrate(&gormstore.UserAccount{}, &gormstore.ControlEvent{}); err != nil {
		test.Fatalf("migrate: %v", err)
	}
	if err := database.Exec("PRAGMA foreign_keys=ON;").Error; err != nil {
		test.Fatalf("foreign keys: %v", err)
	}
	if err := createEvent(database, migrationAccountID, "event", "tenant", migrationTenantOne, time.Now()); err == nil {
		test.Fatalf("event without actor succeeded")
	}

}
