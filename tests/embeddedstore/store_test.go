package embeddedstore_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/pkg/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCallerTransactionOwnsApplicationAndLedgerEffects(t *testing.T) {
	database, clock, tenant, user, namespace, metadata := newEmbeddedStore(t)
	service := embeddedService(t, database, clock)
	amount := embeddedAmount(t, 500)
	if err := service.Grant(t.Context(), tenant, user, namespace, amount, embeddedKey(t, "funding"), 0, metadata); err != nil {
		t.Fatal(err)
	}
	reservation, err := ledger.NewReservationID("request-reservation")
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("controlled application failure")
	admit := func(transaction *gorm.DB) error {
		if err := transaction.Exec("INSERT INTO application_requests (id) VALUES (?)", "request").Error; err != nil {
			return err
		}
		return embeddedService(t, transaction, clock).Reserve(t.Context(), tenant, user, namespace, embeddedAmount(t, 400), reservation, embeddedKey(t, "reserve"), 0, metadata)
	}
	err = database.Transaction(func(transaction *gorm.DB) error {
		if err := admit(transaction); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("outer rollback error: %v", err)
	}
	assertEmbeddedBalance(t, service, tenant, user, namespace, 500, 500)
	var requests int64
	if err := database.Table("application_requests").Count(&requests).Error; err != nil || requests != 0 {
		t.Fatalf("rolled back application requests=%d error=%v", requests, err)
	}
	if err := database.Transaction(admit); err != nil {
		t.Fatal(err)
	}
	assertEmbeddedBalance(t, service, tenant, user, namespace, 500, 100)
	if err := database.Table("application_requests").Count(&requests).Error; err != nil || requests != 1 {
		t.Fatalf("committed application requests=%d error=%v", requests, err)
	}
	entries, err := service.ListEntries(t.Context(), tenant, user, namespace, clock()+1, 100, ledger.ListEntriesFilter{})
	if err != nil || len(entries) != 2 {
		t.Fatalf("grant and hold entries=%v error=%v", entries, err)
	}
}

func TestPermanentReservationAndAtomicSmallerSettlement(t *testing.T) {
	database, clock, tenant, user, namespace, metadata := newEmbeddedStore(t)
	service := embeddedService(t, database, clock)
	if err := service.Grant(t.Context(), tenant, user, namespace, embeddedAmount(t, 500), embeddedKey(t, "funding"), 0, metadata); err != nil {
		t.Fatal(err)
	}
	reservation, err := ledger.NewReservationID("uncertain-request")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Reserve(t.Context(), tenant, user, namespace, embeddedAmount(t, 400), reservation, embeddedKey(t, "reserve"), 0, metadata); err != nil {
		t.Fatal(err)
	}
	// Reopen the database and advance the clock beyond any request lifetime.
	connection, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gorm.Open(database.Dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeEmbeddedDatabase(t, reopened) })
	later := func() int64 { return clock() + int64((365*24*time.Hour)/time.Second) }
	service = embeddedService(t, reopened, later)
	assertEmbeddedBalance(t, service, tenant, user, namespace, 500, 100)
	state, err := service.GetReservationState(t.Context(), tenant, user, namespace, reservation)
	if err != nil || state.Expired || state.HeldCents.Int64() != 400 {
		t.Fatalf("uncertain reservation expired: %+v %v", state, err)
	}
	settle := func(amount int64) ([]ledger.BatchOperationResult, error) {
		return service.Batch(t.Context(), tenant, user, namespace, []ledger.BatchOperation{
			{OperationID: "release", Release: &ledger.BatchReleaseOperation{ReservationID: reservation, IdempotencyKey: embeddedKey(t, "settlement-release"), Metadata: metadata}},
			{OperationID: "charge", Spend: &ledger.BatchSpendOperation{Amount: embeddedAmount(t, amount), IdempotencyKey: embeddedKey(t, "settlement-charge"), Metadata: metadata}},
		}, true)
	}
	results, err := settle(600)
	if err != nil || len(results) != 2 || !errors.Is(results[1].Error, ledger.ErrInsufficientFunds) || !results[0].RolledBack {
		t.Fatalf("failed settlement: %+v %v", results, err)
	}
	assertEmbeddedBalance(t, service, tenant, user, namespace, 500, 100)
	results, err = settle(125)
	if err != nil || len(results) != 2 || results[0].Error != nil || results[1].Error != nil || results[0].RolledBack || results[1].RolledBack {
		t.Fatalf("smaller settlement: %+v %v", results, err)
	}
	assertEmbeddedBalance(t, service, tenant, user, namespace, 375, 375)
	entries, err := service.ListEntries(t.Context(), tenant, user, namespace, later()+1, 100, ledger.ListEntriesFilter{})
	if err != nil || len(entries) != 4 {
		t.Fatalf("funding, hold, release, and spend entries=%v error=%v", entries, err)
	}
	var holdDeltas int64
	for _, entry := range entries {
		if entry.Type() == ledger.EntryHold || entry.Type() == ledger.EntryReverseHold {
			holdDeltas += entry.AmountCents().Int64()
		}
	}
	if holdDeltas != 0 {
		t.Fatalf("reservation entries do not balance: %d", holdDeltas)
	}
}

func newEmbeddedStore(t *testing.T) (*gorm.DB, func() int64, ledger.TenantID, ledger.UserID, ledger.LedgerID, ledger.MetadataJSON) {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "embedded.db")+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeEmbeddedDatabase(t, database) })
	if err := database.AutoMigrate(&gormstore.LedgerAccount{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Exec("CREATE TABLE application_requests (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	tenant, err := ledger.NewTenantID("embedded-application")
	if err != nil {
		t.Fatal(err)
	}
	user, err := ledger.NewUserID("billing-account")
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := ledger.NewLedgerID("USD")
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := ledger.NewMetadataJSON(`{"request_id":"request"}`)
	if err != nil {
		t.Fatal(err)
	}
	return database, func() int64 { return 1800000000 }, tenant, user, namespace, metadata
}

func embeddedService(t *testing.T, database *gorm.DB, clock func() int64) *ledger.Service {
	t.Helper()
	service, err := ledger.NewService(gormstore.New(database), clock)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func embeddedAmount(t *testing.T, value int64) ledger.PositiveAmountCents {
	t.Helper()
	amount, err := ledger.NewPositiveAmountCents(value)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}

func embeddedKey(t *testing.T, value string) ledger.IdempotencyKey {
	t.Helper()
	key, err := ledger.NewIdempotencyKey(value)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func assertEmbeddedBalance(t *testing.T, service *ledger.Service, tenant ledger.TenantID, user ledger.UserID, namespace ledger.LedgerID, total, available int64) {
	t.Helper()
	balance, err := service.Balance(t.Context(), tenant, user, namespace)
	if err != nil || balance.TotalCents.Int64() != total || balance.AvailableCents.Int64() != available {
		t.Fatalf("balance=%+v want total=%d available=%d error=%v", balance, total, available, err)
	}
}

func closeEmbeddedDatabase(t *testing.T, database *gorm.DB) {
	t.Helper()
	connection, err := database.DB()
	if err != nil {
		t.Error(err)
		return
	}
	if err := connection.Close(); err != nil {
		t.Error(err)
	}
}
