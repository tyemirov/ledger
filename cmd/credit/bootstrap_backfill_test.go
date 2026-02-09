package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestRunBootstrapBackfillAppliesMissingBootstrapGrant(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("db handle: %v", err)
	}
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		_ = sqlDB.Close()
		test.Fatalf("migrate: %v", err)
	}
	store := gormstore.New(db)

	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("tenant: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("ledger: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(1000)
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("amount: %v", err)
	}
	idempotencyKeyBase, err := ledger.NewIdempotencyKey("bootstrap")
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON(`{"reason":"account_bootstrap"}`)
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("metadata: %v", err)
	}
	rule, err := ledger.NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("rule: %v", err)
	}
	bootstrapKey, err := rule.BootstrapIdempotencyKey()
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("bootstrap key: %v", err)
	}
	existingAmount, err := ledger.NewEntryAmountCents(1)
	if err != nil {
		_ = sqlDB.Close()
		test.Fatalf("existing amount: %v", err)
	}

	const accountCount = 1200
	const alreadyBootstrappedEvery = 4

	ctx := context.Background()
	preApplied := 0
	for i := 0; i < accountCount; i++ {
		userID, err := ledger.NewUserID(fmt.Sprintf("user-%05d", i))
		if err != nil {
			_ = sqlDB.Close()
			test.Fatalf("user: %v", err)
		}
		accountID, err := store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			_ = sqlDB.Close()
			test.Fatalf("account id: %v", err)
		}

		existingKey, err := ledger.NewIdempotencyKey(fmt.Sprintf("existing-%05d", i))
		if err != nil {
			_ = sqlDB.Close()
			test.Fatalf("existing key: %v", err)
		}
		existingInput, err := ledger.NewEntryInput(
			accountID,
			ledger.EntryGrant,
			existingAmount,
			nil,
			nil,
			existingKey,
			0,
			metadata,
			1700000000,
		)
		if err != nil {
			_ = sqlDB.Close()
			test.Fatalf("existing input: %v", err)
		}
		if _, err := store.InsertEntry(ctx, existingInput); err != nil {
			_ = sqlDB.Close()
			test.Fatalf("insert existing entry: %v", err)
		}

		if i%alreadyBootstrappedEvery == 0 {
			bootstrapInput, err := ledger.NewEntryInput(
				accountID,
				ledger.EntryGrant,
				amount.ToEntryAmountCents(),
				nil,
				nil,
				bootstrapKey,
				0,
				metadata,
				1700000000,
			)
			if err != nil {
				_ = sqlDB.Close()
				test.Fatalf("bootstrap input: %v", err)
			}
			if _, err := store.InsertEntry(ctx, bootstrapInput); err != nil {
				_ = sqlDB.Close()
				test.Fatalf("insert bootstrap entry: %v", err)
			}
			preApplied++
		}
	}

	if err := sqlDB.Close(); err != nil {
		test.Fatalf("close seed db: %v", err)
	}

	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://" + sqlitePath,
		BootstrapGrantsJSON: `[{"tenant_id":"default","ledger_id":"default","amount_cents":1000,"idempotency_key_prefix":"bootstrap","metadata_json":"{\"reason\":\"account_bootstrap\"}"}]`,
	}

	logger := zap.NewNop()
	summary, err := runBootstrapBackfill(ctx, cfg, logger, 200)
	if err != nil {
		test.Fatalf("backfill: %v", err)
	}
	if summary.Scopes != 1 {
		test.Fatalf("expected 1 scope, got %d", summary.Scopes)
	}
	if summary.AccountsSeen != accountCount {
		test.Fatalf("expected accounts seen %d, got %d", accountCount, summary.AccountsSeen)
	}
	expectedApplied := accountCount - preApplied
	if summary.Applied != expectedApplied {
		test.Fatalf("expected applied %d, got %d", expectedApplied, summary.Applied)
	}
	if summary.Skipped != preApplied {
		test.Fatalf("expected skipped %d, got %d", preApplied, summary.Skipped)
	}

	verifyDB, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("reopen db: %v", err)
	}
	verifySQLDB, err := verifyDB.DB()
	if err != nil {
		test.Fatalf("verify db handle: %v", err)
	}
	test.Cleanup(func() { _ = verifySQLDB.Close() })

	var bootstrapCount int64
	if err := verifyDB.Model(&gormstore.LedgerEntry{}).Where("idempotency_key = ?", bootstrapKey.String()).Count(&bootstrapCount).Error; err != nil {
		test.Fatalf("count bootstrap entries: %v", err)
	}
	if bootstrapCount != accountCount {
		test.Fatalf("expected bootstrap entries %d, got %d", accountCount, bootstrapCount)
	}

	summary, err = runBootstrapBackfill(ctx, cfg, logger, 200)
	if err != nil {
		test.Fatalf("backfill second run: %v", err)
	}
	if summary.Applied != 0 {
		test.Fatalf("expected applied 0 on second run, got %d", summary.Applied)
	}
	if summary.Skipped != accountCount {
		test.Fatalf("expected skipped %d on second run, got %d", accountCount, summary.Skipped)
	}
}

func TestRunBootstrapBackfillRejectsEmptyPolicy(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://:memory:",
		BootstrapGrantsJSON: " ",
	}
	_, err := runBootstrapBackfill(context.Background(), cfg, zap.NewNop(), 10)
	if !errors.Is(err, ledger.ErrInvalidServiceConfig) {
		test.Fatalf("expected invalid service config, got %v", err)
	}
}

func TestRunBootstrapBackfillRejectsNonPositivePageSize(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://:memory:",
		BootstrapGrantsJSON: `[{"tenant_id":"default","ledger_id":"default","amount_cents":1000,"idempotency_key_prefix":"bootstrap","metadata_json":"{}"}]`,
	}
	_, err := runBootstrapBackfill(context.Background(), cfg, zap.NewNop(), 0)
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestRunBootstrapBackfillReturnsDatabaseOpenErrorForUnsupportedScheme(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "mysql://root:pass@localhost:3306/db",
		BootstrapGrantsJSON: `[{"tenant_id":"default","ledger_id":"default","amount_cents":1000,"idempotency_key_prefix":"bootstrap","metadata_json":"{}"}]`,
	}
	_, err := runBootstrapBackfill(context.Background(), cfg, zap.NewNop(), 10)
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestRunBootstrapBackfillReturnsErrorForInvalidBootstrapJSON(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://:memory:",
		BootstrapGrantsJSON: "{",
	}
	_, err := runBootstrapBackfill(context.Background(), cfg, zap.NewNop(), 10)
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestNewBootstrapBackfillCommandRegistersPageSizeFlag(test *testing.T) {
	cmd := newBootstrapBackfillCommand(&runtimeConfig{})
	flag := cmd.Flags().Lookup("page-size")
	if flag == nil {
		test.Fatalf("expected page-size flag")
	}
	if flag.DefValue != fmt.Sprintf("%d", defaultBootstrapBackfillPageSize) {
		test.Fatalf("expected default %d, got %q", defaultBootstrapBackfillPageSize, flag.DefValue)
	}
}

func TestNewBootstrapBackfillCommandPropagatesBackfillErrors(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://:memory:",
		BootstrapGrantsJSON: " ",
	}

	cmd := newBootstrapBackfillCommand(cfg)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--page-size", "10"})

	if err := cmd.Execute(); err == nil {
		test.Fatalf("expected error")
	}
}

func TestBootstrapBackfillCommandExecutesOnEmptyDatabase(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://" + sqlitePath,
		BootstrapGrantsJSON: `[{"tenant_id":"default","ledger_id":"default","amount_cents":1000,"idempotency_key_prefix":"bootstrap","metadata_json":"{}"}]`,
	}

	cmd := newBootstrapBackfillCommand(cfg)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--page-size", "10"})

	if err := cmd.Execute(); err != nil {
		test.Fatalf("execute: %v", err)
	}
}

func TestRunBootstrapBackfillReturnsErrorWhenPrepareSchemaFails(test *testing.T) {
	cfg := &runtimeConfig{
		DatabaseURL:         "sqlite://:memory:",
		BootstrapGrantsJSON: `[{"tenant_id":"default","ledger_id":"default","amount_cents":1000,"idempotency_key_prefix":"bootstrap","metadata_json":"{}"}]`,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runBootstrapBackfill(ctx, cfg, zap.NewNop(), 10)
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestBackfillBootstrapRuleReturnsErrorWhenAccountListingFails(test *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		test.Fatalf("open db: %v", err)
	}

	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(1000)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKeyBase, err := ledger.NewIdempotencyKey("bootstrap")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON(`{}`)
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	rule, err := ledger.NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		test.Fatalf("rule: %v", err)
	}

	// Intentionally avoid schema migration so the underlying query fails.
	store := gormstore.New(db)
	_, _, _, err = backfillBootstrapRule(context.Background(), store, rule, func() int64 { return 1700000000 }, 10)
	if err == nil {
		test.Fatalf("expected error")
	}
}
