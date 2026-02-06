package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func TestResolveDriver(test *testing.T) {
	testCases := []struct {
		name           string
		input          string
		wantDriver     string
		wantSQLitePath bool
		wantErr        bool
	}{
		{name: "postgres", input: "postgres://postgres:postgres@localhost:5432/credit?sslmode=disable", wantDriver: "postgres"},
		{name: "postgresql", input: "postgresql://postgres:postgres@localhost:5432/credit?sslmode=disable", wantDriver: "postgres"},
		{name: "unknown url scheme", input: "mysql://root:pass@localhost:3306/db", wantDriver: "mysql"},
		{name: "sqlite url", input: "sqlite:///tmp/ledger.db", wantDriver: "sqlite", wantSQLitePath: true},
		{name: "sqlite raw path", input: "ledger.db", wantDriver: "sqlite", wantSQLitePath: true},
		{name: "sqlite url parse error", input: "sqlite://%zz", wantErr: true},
		{name: "database url parse error", input: "http://%zz", wantErr: true},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			gotDriver, gotSQLitePath, err := resolveDriver(testCase.input)
			if testCase.wantErr {
				if err == nil {
					test.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if gotDriver != testCase.wantDriver {
				test.Fatalf("expected driver %q, got %q", testCase.wantDriver, gotDriver)
			}
			if testCase.wantSQLitePath && gotSQLitePath == "" {
				test.Fatalf("expected sqlite path, got empty")
			}
		})
	}
}

func TestLoadConfigUsesDefaults(test *testing.T) {
	viper.Reset()
	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	if err := loadConfig(cmd, cfg); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL == "" {
		test.Fatalf("expected database url, got empty")
	}
	if cfg.ListenAddr == "" {
		test.Fatalf("expected listen addr, got empty")
	}
}

func TestLoadConfigRespectsEnvOverrides(test *testing.T) {
	viper.Reset()
	test.Setenv("DATABASE_URL", "sqlite://:memory:")
	test.Setenv("GRPC_LISTEN_ADDR", "127.0.0.1:9999")

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	if err := loadConfig(cmd, cfg); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "sqlite://:memory:" {
		test.Fatalf("expected env database url, got %q", cfg.DatabaseURL)
	}
	if cfg.ListenAddr != "127.0.0.1:9999" {
		test.Fatalf("expected env listen addr, got %q", cfg.ListenAddr)
	}
}

func TestNormalizeSQLitePath(test *testing.T) {
	tempDir := test.TempDir()
	absolutePath := filepath.Join(tempDir, "ledger.db")
	relativePath := "ledger.db"

	memoryPath, err := normalizeSQLitePath(":memory:")
	if err != nil {
		test.Fatalf("memory path: %v", err)
	}
	if memoryPath != ":memory:" {
		test.Fatalf("expected :memory:, got %q", memoryPath)
	}

	normalizedAbsolute, err := normalizeSQLitePath(absolutePath)
	if err != nil {
		test.Fatalf("absolute path: %v", err)
	}
	if normalizedAbsolute != absolutePath {
		test.Fatalf("expected %q, got %q", absolutePath, normalizedAbsolute)
	}

	normalizedRelative, err := normalizeSQLitePath(relativePath)
	if err != nil {
		test.Fatalf("relative path: %v", err)
	}
	if filepath.IsAbs(normalizedRelative) {
		test.Fatalf("expected relative path, got %q", normalizedRelative)
	}
}

func TestOpenDatabaseSQLiteAndUnsupportedScheme(test *testing.T) {
	ctx := context.Background()

	db, cleanup, driver, err := openDatabase(ctx, "sqlite://:memory:")
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	if driver != "sqlite" {
		test.Fatalf("expected sqlite driver, got %q", driver)
	}
	if db == nil || cleanup == nil {
		test.Fatalf("expected db and cleanup")
	}
	if err := cleanup(); err != nil {
		test.Fatalf("cleanup: %v", err)
	}

	_, _, _, err = openDatabase(ctx, "mysql://root:pass@localhost:3306/db")
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestPrepareSchemaSQLiteEnablesForeignKeys(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Exec("PRAGMA foreign_keys=OFF;").Error; err != nil {
		test.Fatalf("pragma off: %v", err)
	}
	if err := prepareSchema(db, "postgres"); err != nil {
		test.Fatalf("prepare schema (postgres driver): %v", err)
	}
	if !db.Migrator().HasTable(&gormstore.Account{}) {
		test.Fatalf("expected accounts table")
	}
	if foreignKeysEnabled := readSQLiteForeignKeys(test, db); foreignKeysEnabled {
		test.Fatalf("expected foreign keys remain disabled for non-sqlite driver")
	}

	if err := prepareSchema(db, "sqlite"); err != nil {
		test.Fatalf("prepare schema (sqlite driver): %v", err)
	}
	if !readSQLiteForeignKeys(test, db) {
		test.Fatalf("expected foreign keys enabled")
	}
}

func TestExtractIDs(test *testing.T) {
	req := testIDRequest{userID: " user ", ledgerID: " ledger ", tenantID: " tenant "}
	if got := extractUserID(req); got != "user" {
		test.Fatalf("expected user, got %q", got)
	}
	if got := extractLedgerID(req); got != "ledger" {
		test.Fatalf("expected ledger, got %q", got)
	}
	if got := extractTenantID(req); got != "tenant" {
		test.Fatalf("expected tenant, got %q", got)
	}

	if got := extractUserID(struct{}{}); got != "" {
		test.Fatalf("expected empty user id, got %q", got)
	}
}

func TestZapOperationLoggerIsNilSafe(test *testing.T) {
	var operationLogger *zapOperationLogger
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant"})
	operationLogger = &zapOperationLogger{logger: zap.NewNop()}
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant"})
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant", Error: grpc.ErrServerStopped})
}

func TestLoggingInterceptorCallsHandler(test *testing.T) {
	logger := zap.NewNop()
	interceptor := newLoggingInterceptor(logger)

	type request struct{}
	response, err := interceptor(context.Background(), request{}, &grpc.UnaryServerInfo{FullMethod: "/credit.v1.CreditService/GetBalance"}, func(ctx context.Context, request interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if response != "ok" {
		test.Fatalf("expected ok, got %v", response)
	}
}

func TestLoggingInterceptorLogsFieldsAndError(test *testing.T) {
	core, observedLogs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	interceptor := newLoggingInterceptor(logger)
	request := testIDRequest{userID: " user ", ledgerID: " ledger ", tenantID: " tenant "}

	_, err := interceptor(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/credit.v1.CreditService/GetBalance"}, func(ctx context.Context, request interface{}) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		test.Fatalf("unexpected success error: %v", err)
	}

	logs := observedLogs.FilterMessage("grpc request completed")
	if logs.Len() == 0 {
		test.Fatalf("expected completed log entry")
	}
	if logs.FilterFieldKey("user_id").Len() == 0 {
		test.Fatalf("expected user_id field")
	}
	if logs.FilterFieldKey("ledger_id").Len() == 0 {
		test.Fatalf("expected ledger_id field")
	}
	if logs.FilterFieldKey("tenant_id").Len() == 0 {
		test.Fatalf("expected tenant_id field")
	}

	sentinelError := errors.New("rpc failed")
	_, err = interceptor(context.Background(), request, &grpc.UnaryServerInfo{FullMethod: "/credit.v1.CreditService/GetBalance"}, func(ctx context.Context, request interface{}) (interface{}, error) {
		return nil, sentinelError
	})
	if !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
	errorLogs := observedLogs.FilterMessage("grpc request failed").FilterLevelExact(zapcore.ErrorLevel)
	if errorLogs.Len() == 0 {
		test.Fatalf("expected failed log entry")
	}
}

func TestZapOperationLoggerEmitsInfoAndError(test *testing.T) {
	core, observedLogs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	operationLogger := &zapOperationLogger{logger: logger}

	reservationID, err := ledger.NewReservationID("order-1")
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant id: %v", err)
	}
	userID, err := ledger.NewUserID("user-123")
	if err != nil {
		test.Fatalf("user id: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger id: %v", err)
	}
	amount, err := ledger.NewAmountCents(100)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKey, err := ledger.NewIdempotencyKey("key-1")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON("{\"source\":\"test\"}")
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}

	operationLogger.LogOperation(context.Background(), ledger.OperationLog{
		Operation:      "grant",
		Status:         "ok",
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		Amount:         amount,
		ReservationID:  &reservationID,
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
	})
	if observedLogs.FilterMessage("ledger.operation").FilterLevelExact(zapcore.InfoLevel).Len() == 0 {
		test.Fatalf("expected info operation log")
	}

	operationLogger.LogOperation(context.Background(), ledger.OperationLog{
		Operation: "grant",
		Error:     errors.New("boom"),
	})
	if observedLogs.FilterMessage("ledger.operation").FilterLevelExact(zapcore.ErrorLevel).Len() == 0 {
		test.Fatalf("expected error operation log")
	}
}

func TestRunServerWithListenHandlesRequestsAndShutdown(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatalf("listen: %v", err)
	}

	cfg := &runtimeConfig{
		DatabaseURL: "sqlite://" + sqlitePath,
		ListenAddr:  listener.Addr().String(),
	}
	core, observedLogs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	ctx, cancel := context.WithCancel(context.Background())
	serverResultCh := make(chan error, 1)
	go func() {
		serverResultCh <- runServerWithListen(ctx, cfg, logger, func(network string, address string) (net.Listener, error) {
			return listener, nil
		})
	}()

	conn := waitForGRPCServer(test, cfg.ListenAddr)
	client := creditv1.NewCreditServiceClient(conn)

	requestContext, cancelRequests := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRequests()

	if _, err := client.GetBalance(requestContext, &creditv1.BalanceRequest{UserId: " user-123 ", TenantId: " default ", LedgerId: " default "}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("get balance: %v", err)
	}

	if _, err := client.Grant(requestContext, &creditv1.GrantRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{\"reason\":\"signup\"}",
	}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("grant: %v", err)
	}

	_, err = client.Grant(requestContext, &creditv1.GrantRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.AlreadyExists {
		_ = conn.Close()
		cancel()
		test.Fatalf("expected already exists, got %v", status.Code(err))
	}

	if _, err := client.Reserve(requestContext, &creditv1.ReserveRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    200,
		ReservationId:  "order-1",
		IdempotencyKey: "reserve-1",
		MetadataJson:   "{\"order\":1}",
	}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("reserve: %v", err)
	}

	if _, err := client.Release(requestContext, &creditv1.ReleaseRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		ReservationId:  "order-1",
		IdempotencyKey: "release-1",
		MetadataJson:   "{}",
	}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("release: %v", err)
	}

	_, err = client.Reserve(requestContext, &creditv1.ReserveRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    200,
		ReservationId:  "order-1",
		IdempotencyKey: "reserve-2",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.AlreadyExists {
		_ = conn.Close()
		cancel()
		test.Fatalf("expected already exists, got %v", status.Code(err))
	}

	_, err = client.Spend(requestContext, &creditv1.SpendRequest{
		UserId:         "user-123",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    9999,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.FailedPrecondition {
		_ = conn.Close()
		cancel()
		test.Fatalf("expected failed precondition, got %v", status.Code(err))
	}

	_ = conn.Close()
	cancel()
	if err := <-serverResultCh; err != nil {
		test.Fatalf("server: %v", err)
	}

	if observedLogs.FilterMessage("grpc request completed").Len() == 0 {
		test.Fatalf("expected grpc request completed logs")
	}
	if observedLogs.FilterMessage("grpc request failed").Len() == 0 {
		test.Fatalf("expected grpc request failed logs")
	}
	if observedLogs.FilterMessage("ledger.operation").Len() == 0 {
		test.Fatalf("expected operation logs")
	}
}

func TestRootCommandPreRunAndRun(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	listenAddress := reserveLocalAddress(test)

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--" + flagDatabaseURL, "sqlite://" + sqlitePath,
		"--" + flagListenAddr, listenAddress,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd.SetContext(ctx)

	runResultCh := make(chan error, 1)
	go func() {
		runResultCh <- cmd.Execute()
	}()

	conn := waitForGRPCServer(test, listenAddress)
	_ = conn.Close()

	cancel()
	if err := <-runResultCh; err != nil {
		test.Fatalf("run: %v", err)
	}
}

func readSQLiteForeignKeys(test *testing.T, db *gorm.DB) bool {
	test.Helper()
	var value int
	if err := db.Raw("PRAGMA foreign_keys;").Scan(&value).Error; err != nil {
		test.Fatalf("pragma foreign keys: %v", err)
	}
	return value != 0
}

type testIDRequest struct {
	userID   string
	ledgerID string
	tenantID string
}

func (req testIDRequest) GetUserId() string {
	return req.userID
}

func (req testIDRequest) GetLedgerId() string {
	return req.ledgerID
}

func (req testIDRequest) GetTenantId() string {
	return req.tenantID
}

func reserveLocalAddress(test *testing.T) string {
	test.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		test.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		test.Fatalf("close address listener: %v", err)
	}
	return address
}

func waitForGRPCServer(test *testing.T, address string) *grpc.ClientConn {
	test.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		test.Fatalf("grpc new client: %v", err)
	}
	conn.Connect()

	dialContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn
		}

		if !conn.WaitForStateChange(dialContext, state) {
			_ = conn.Close()
			test.Fatalf("wait for grpc server: state=%v", conn.GetState())
		}
	}
}

func TestMain(test *testing.M) {
	viper.Reset()
	os.Exit(test.Run())
}
