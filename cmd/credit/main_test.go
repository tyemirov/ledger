package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/controlplane"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
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
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	testRuntimeTenantID = "0196f0ec-3e80-7a54-bd2b-56cfe90bf801"
	testRuntimeOrigin   = "https://ledger.example.test"
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
		{name: "sqlite file url", input: "file:///tmp/ledger.db", wantDriver: "sqlite", wantSQLitePath: true},
		{name: "sqlite file shared memory", input: "file::memory:?cache=shared", wantDriver: "sqlite", wantSQLitePath: true},
		{name: "sqlite file url unsupported host", input: "file://remotehost/tmp/ledger.db", wantErr: true},
		{name: "sqlite file url missing path", input: "file://localhost", wantErr: true},
		{name: "sqlite file url parse error", input: "file://%zz", wantErr: true},
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

func TestLoadConfigErrorsWhenFileMissing(test *testing.T) {
	viper.Reset()
	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, "missing.yml", "config")
	_ = cmd.Flags().Set(flagConfigFile, "missing.yml")

	if err := loadConfig(cmd, cfg); err == nil {
		test.Fatalf("expected error for missing config file, got nil")
	}
}

func TestLoadConfigErrorsWhenRequiredFieldsMissing(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "invalid.yml")
	content := `
service:
  grpc_listen_addr: ":50051"
  http_listen_addr: ":8080"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err == nil {
		test.Fatalf("expected error for missing database_url, got nil")
	}
}

func TestLoadConfigErrorsWhenAuthMissing(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "invalid_tenant.yml")
	content := `
service:
  database_url: "sqlite://test.db"
  grpc_listen_addr: ":50051"
  http_listen_addr: ":8080"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err == nil {
		test.Fatalf("expected error for missing auth, got nil")
	}
}

func TestLoadConfigErrorsWhenPublicOriginMissing(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "invalid_tenant_id.yml")
	content := `
service:
  database_url: "sqlite://test.db"
  grpc_listen_addr: ":50051"
  http_listen_addr: ":8080"
auth:
  jwt_signing_key: "secret"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err == nil {
		test.Fatalf("expected error for missing public origin, got nil")
	}
}

func TestLoadConfigWithFileAndExpansion(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "config.yml")
	content := `
service:
  database_url: "${TEST_DB_URL}"
  grpc_listen_addr: ":8888"
  http_listen_addr: ":8080"
auth:
  jwt_signing_key: "secret"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
  public_origin: "https://ledger.example.test"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	test.Setenv("TEST_DB_URL", "sqlite://test.db")

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}

	if cfg.Service.DatabaseURL != "sqlite://test.db" {
		test.Fatalf("expected expanded database url, got %q", cfg.Service.DatabaseURL)
	}
	if cfg.Service.GRPCListenAddr != ":8888" {
		test.Fatalf("expected gRPC listen addr :8888, got %q", cfg.Service.GRPCListenAddr)
	}
	if cfg.Auth.PublicOrigin != testRuntimeOrigin {
		test.Fatalf("expected public origin, got %q", cfg.Auth.PublicOrigin)
	}
}

func TestLoadConfigWithDefaultExpansion(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "config.yml")
	content := `
service:
  database_url: "${TEST_DB_URL:-sqlite://default.db}"
  grpc_listen_addr: "${TEST_LISTEN_ADDR:-:9999}"
  http_listen_addr: ":8080"
auth:
  jwt_signing_key: "${TEST_SIGNING_KEY:-default-secret}"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
  public_origin: "https://ledger.example.test"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}

	if cfg.Service.DatabaseURL != "sqlite://default.db" {
		test.Fatalf("expected default database url, got %q", cfg.Service.DatabaseURL)
	}
	if cfg.Service.GRPCListenAddr != ":9999" {
		test.Fatalf("expected default listen addr, got %q", cfg.Service.GRPCListenAddr)
	}
	if cfg.Auth.JWTSigningKey != "default-secret" {
		test.Fatalf("expected default signing key, got %q", cfg.Auth.JWTSigningKey)
	}
}

func TestLoadConfigDefaultExpansionUsesEnvironmentOverride(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "config.yml")
	content := `
service:
  database_url: "${TEST_DB_URL:-sqlite://default.db}"
  grpc_listen_addr: "${TEST_LISTEN_ADDR:-:9999}"
  http_listen_addr: ":8080"
auth:
  jwt_signing_key: "${TEST_SIGNING_KEY:-default-secret}"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
  public_origin: "https://ledger.example.test"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	test.Setenv("TEST_DB_URL", "sqlite://override.db")
	test.Setenv("TEST_LISTEN_ADDR", ":7777")
	test.Setenv("TEST_SIGNING_KEY", "override-secret")

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	if err := loadConfig(cmd, cfg); err != nil {
		test.Fatalf("unexpected error: %v", err)
	}

	if cfg.Service.DatabaseURL != "sqlite://override.db" {
		test.Fatalf("expected overridden database url, got %q", cfg.Service.DatabaseURL)
	}
	if cfg.Service.GRPCListenAddr != ":7777" {
		test.Fatalf("expected overridden listen addr, got %q", cfg.Service.GRPCListenAddr)
	}
	if cfg.Auth.JWTSigningKey != "override-secret" {
		test.Fatalf("expected overridden signing key, got %q", cfg.Auth.JWTSigningKey)
	}
}

func TestExpandConfigVariablesReturnsEmptyStringForUnsetVariableWithoutDefault(test *testing.T) {
	const environmentName = "LEDGER_TEST_EXPAND_CONFIG_UNSET"

	originalValue, wasSet := os.LookupEnv(environmentName)
	if err := os.Unsetenv(environmentName); err != nil {
		test.Fatalf("unset env: %v", err)
	}
	defer func() {
		if !wasSet {
			return
		}
		if err := os.Setenv(environmentName, originalValue); err != nil {
			test.Fatalf("restore env: %v", err)
		}
	}()

	input := fmt.Sprintf("prefix-${%s}-suffix", environmentName)
	if got := expandConfigVariables(input); got != "prefix--suffix" {
		test.Fatalf("expected empty expansion for unset variable, got %q", got)
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

func TestNormalizeSQLitePathReturnsErrorWhenDirectoryCreationFails(test *testing.T) {
	tempDir := test.TempDir()
	blockingFile := filepath.Join(tempDir, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		test.Fatalf("write file: %v", err)
	}
	_, err := normalizeSQLitePath(filepath.Join(blockingFile, "ledger.db"))
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestNormalizeSQLitePathReturnsErrorForRelativePathsWhenDirectoryCreationFails(test *testing.T) {
	tempDir := test.TempDir()
	workingDir, err := os.Getwd()
	if err != nil {
		test.Fatalf("getwd: %v", err)
	}
	test.Cleanup(func() { _ = os.Chdir(workingDir) })
	if err := os.Chdir(tempDir); err != nil {
		test.Fatalf("chdir: %v", err)
	}

	if err := os.WriteFile("not-a-directory", []byte("x"), 0o644); err != nil {
		test.Fatalf("write file: %v", err)
	}
	_, err = normalizeSQLitePath(filepath.Join("not-a-directory", "ledger.db"))
	if err == nil {
		test.Fatalf("expected error")
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

	db, cleanup, driver, err = openDatabase(ctx, "file::memory:?cache=shared")
	if err != nil {
		test.Fatalf("open sqlite file: %v", err)
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

	tempDir := test.TempDir()
	fileDSN := "file://" + filepath.Join(tempDir, "ledger.db")
	db, cleanup, driver, err = openDatabase(ctx, fileDSN)
	if err != nil {
		test.Fatalf("open sqlite file url: %v", err)
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

func TestOpenDatabaseReturnsErrorWhenDriverCannotBeResolved(test *testing.T) {
	ctx := context.Background()
	_, _, _, err := openDatabase(ctx, "http://%zz")
	if err == nil {
		test.Fatalf("expected error")
	}
}

func TestOpenDatabasePostgresReturnsErrorWhenUnavailable(test *testing.T) {
	ctx := context.Background()
	_, _, _, err := openDatabase(ctx, "postgres://postgres:postgres@127.0.0.1:1/credit?sslmode=disable")
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
	if !db.Migrator().HasTable(&gormstore.LedgerAccount{}) {
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

func TestPrepareSchemaRejectsLegacyAccounts(test *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		test.Fatalf("open: %v", err)
	}
	if err := database.Exec(`CREATE TABLE accounts (account_id text PRIMARY KEY)`).Error; err != nil {
		test.Fatalf("create legacy accounts: %v", err)
	}
	if err := prepareSchema(database, "sqlite"); err == nil || !strings.Contains(err.Error(), "migration") {
		test.Fatalf("expected migration requirement, got %v", err)
	}
}

func TestPrepareSchemaReturnsErrorWhenDatabaseIsClosed(test *testing.T) {
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
	if err := sqlDB.Close(); err != nil {
		test.Fatalf("close sql db: %v", err)
	}

	if err := prepareSchema(db, "sqlite"); err == nil {
		test.Fatalf("expected error")
	}
	if err := prepareSchema(db, "postgres"); err == nil {
		test.Fatalf("expected error")
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
	batch := &creditv1.BatchRequest{Account: &creditv1.AccountContext{TenantId: " batch-tenant "}}
	if got := extractTenantID(batch); got != "batch-tenant" {
		test.Fatalf("expected nested batch tenant, got %q", got)
	}
	if got := extractTenantID(&creditv1.BatchRequest{}); got != "" {
		test.Fatalf("expected empty nested tenant, got %q", got)
	}
}

func TestZapOperationLoggerIsNilSafe(test *testing.T) {
	var operationLogger *zapOperationLogger
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant"})
	operationLogger = &zapOperationLogger{logger: zap.NewNop()}
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant"})
	operationLogger.LogOperation(context.Background(), ledger.OperationLog{Operation: "grant", Error: grpc.ErrServerStopped})
}

func TestAuthInterceptor(test *testing.T) {
	interceptor := newAuthInterceptor(stubTenantAuthenticator{tenantID: testRuntimeTenantID, secret: "s1"})
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		authenticatedID, ok := tenant.AuthenticatedID(ctx)
		if !ok {
			return nil, errors.New("authenticated tenant missing")
		}
		addressedID, _ := tenant.NewID(extractTenantID(req))
		if authenticatedID != addressedID {
			return nil, status.Error(codes.PermissionDenied, "tenant is not authorized")
		}
		return "ok", nil
	}

	testCases := []struct {
		name     string
		request  interface{}
		metadata metadata.MD
		wantCode codes.Code
	}{
		{
			name:     "missing tenant_id",
			request:  struct{}{},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "invalid tenant id",
			request:  testIDRequest{tenantID: "unknown"},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "missing metadata",
			request:  testIDRequest{tenantID: testRuntimeTenantID},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "missing authorization header",
			request:  testIDRequest{tenantID: testRuntimeTenantID},
			metadata: metadata.MD{"foo": []string{"bar"}},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "invalid authorization header format",
			request:  testIDRequest{tenantID: testRuntimeTenantID},
			metadata: metadata.MD{"authorization": []string{"Basic s1"}},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "invalid secret key",
			request:  testIDRequest{tenantID: testRuntimeTenantID},
			metadata: metadata.MD{"authorization": []string{"Bearer wrong"}},
			wantCode: codes.Unauthenticated,
		},
		{
			name:     "success",
			request:  testIDRequest{tenantID: testRuntimeTenantID},
			metadata: metadata.MD{"authorization": []string{"Bearer s1"}},
			wantCode: codes.OK,
		},
		{
			name:     "tenant mismatch",
			request:  testIDRequest{tenantID: "0196f0ec-3e80-7a54-bd2b-56cfe90bf899"},
			metadata: metadata.MD{"authorization": []string{"Bearer s1"}},
			wantCode: codes.PermissionDenied,
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			ctx := context.Background()
			if testCase.metadata != nil {
				ctx = metadata.NewIncomingContext(ctx, testCase.metadata)
			}
			_, err := interceptor(ctx, testCase.request, &grpc.UnaryServerInfo{}, handler)
			if status.Code(err) != testCase.wantCode {
				test.Fatalf("expected code %v, got %v: %v", testCase.wantCode, status.Code(err), err)
			}
		})
	}
}

func TestLoadConfigErrorsOnInvalidYAML(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "invalid.yml")
	if err := os.WriteFile(configFile, []byte("service: : :"), 0o644); err != nil {
		test.Fatalf("write config: %v", err)
	}
	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)
	if err := loadConfig(cmd, cfg); err == nil {
		test.Fatalf("expected error")
	}
}

func TestRunServerWithListenReturnsErrorOnListenFailure(test *testing.T) {
	logger := zap.NewNop()
	cfg := configuredRuntime("sqlite://:memory:", ":1", ":2")
	err := runServerWithListen(context.Background(), cfg, logger, func(n, a string) (net.Listener, error) {
		return nil, errors.New("listen failed")
	})
	if err == nil || !strings.Contains(err.Error(), "listen failed") {
		test.Fatalf("expected listen failure, got %v", err)
	}
}

func TestRunServerWithListenReturnsErrorOnHTTPListenFailure(test *testing.T) {
	logger := zap.NewNop()
	cfg := configuredRuntime("sqlite://:memory:", "127.0.0.1:0", "127.0.0.1:0")
	callCount := 0
	err := runServerWithListen(context.Background(), cfg, logger, func(network string, address string) (net.Listener, error) {
		callCount++
		if callCount == 2 {
			return nil, errors.New("http listen failed")
		}
		return net.Listen(network, address)
	})
	if err == nil || !strings.Contains(err.Error(), "http listen failed") {
		test.Fatalf("expected HTTP listen failure, got %v", err)
	}
}

func TestRunServerWithListenReturnsErrorOnDatabaseFailure(test *testing.T) {
	logger := zap.NewNop()
	cfg := &runtimeConfig{}
	cfg.Service.DatabaseURL = "http://%zz" // Fails url.Parse in openDatabase
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := runServerWithListen(ctx, cfg, logger, net.Listen)
	if err == nil {
		test.Fatalf("expected database failure")
	}
}

func TestRunServerWithListenReturnsErrorOnSchemaFailure(test *testing.T) {
	logger := zap.NewNop()
	cfg := &runtimeConfig{}
	cfg.Service.DatabaseURL = "sqlite://:memory:"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := runServerWithListen(ctx, cfg, logger, func(n, a string) (net.Listener, error) {
		return nil, errors.New("stop")
	})
	if err == nil {
		test.Fatalf("expected listen failure")
	}
}

func TestNormalizeSQLiteFileDSNValidation(test *testing.T) {
	testCases := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{name: "valid opaque", dsn: "file:ledger.db", wantErr: false},
		{name: "unsupported host", dsn: "file://remote/ledger.db", wantErr: true},
		{name: "missing path", dsn: "file://localhost", wantErr: true},
	}
	for _, tc := range testCases {
		test.Run(tc.name, func(test *testing.T) {
			_, err := normalizeSQLiteFileDSN(tc.dsn)
			if (err != nil) != tc.wantErr {
				test.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestRunServerErrorsOnLoggerInit(test *testing.T) {
	originalNewLogger := newLogger
	test.Cleanup(func() { newLogger = originalNewLogger })

	newLogger = func(_ ...zap.Option) (*zap.Logger, error) {
		return nil, errors.New("logger init failed")
	}

	cfg := &runtimeConfig{}
	cfg.Service.DatabaseURL = "sqlite://:memory:"

	err := runServer(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "logger init") {
		test.Fatalf("expected logger init error, got %v", err)
	}
}

func TestRunServerSucceedsAndSyncsLogger(test *testing.T) {
	originalNewLogger := newLogger
	test.Cleanup(func() { newLogger = originalNewLogger })

	syncCalled := false
	core, _ := observer.New(zapcore.DebugLevel)
	testLogger := zap.New(core)

	// Wrap the logger to track Sync calls
	newLogger = func(_ ...zap.Option) (*zap.Logger, error) {
		return testLogger.WithOptions(zap.Hooks(func(_ zapcore.Entry) error { return nil })), nil
	}

	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	listenAddress := reserveLocalAddress(test)
	httpAddress := reserveLocalAddress(test)
	cfg := configuredRuntime("sqlite://"+sqlitePath, listenAddress, httpAddress)

	ctx, cancel := context.WithCancel(context.Background())

	// Use a custom newLogger that detects Sync
	newLogger = func(_ ...zap.Option) (*zap.Logger, error) {
		return zap.New(core, zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return &syncTrackingCore{Core: c, syncCalled: &syncCalled}
		})), nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- runServer(ctx, cfg)
	}()

	conn := waitForGRPCServer(test, listenAddress)
	_ = conn.Close()
	cancel()

	if err := <-errCh; err != nil {
		test.Fatalf("runServer: %v", err)
	}

	if !syncCalled {
		test.Fatalf("expected logger.Sync() to be called")
	}
}

func TestAuthInterceptorMissingMetadata(test *testing.T) {
	interceptor := newAuthInterceptor(stubTenantAuthenticator{tenantID: testRuntimeTenantID, secret: "s1"})
	_, err := interceptor(context.Background(), testIDRequest{tenantID: testRuntimeTenantID}, &grpc.UnaryServerInfo{}, nil)
	if status.Code(err) != codes.Unauthenticated {
		test.Fatalf("expected unauthenticated, got %v", status.Code(err))
	}
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
	operationLogger.LogControlRequest(controlplane.RequestLog{Operation: "GET /api/tenants", StatusCode: http.StatusOK})
	operationLogger.LogControlRequest(controlplane.RequestLog{Operation: "GET /api/tenants/id", StatusCode: http.StatusOK, UserAccountID: "account", ResourceID: "tenant"})
	if observedLogs.FilterMessage("control.request").Len() != 2 {
		test.Fatalf("expected control request logs")
	}

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
	grpcAddress := reserveLocalAddress(test)
	httpAddress := reserveLocalAddress(test)
	cfg := configuredRuntime("sqlite://"+sqlitePath, grpcAddress, httpAddress)
	tenantSecret := seedRuntimeTenant(test, sqlitePath)

	core, observedLogs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	ctx, cancel := context.WithCancel(context.Background())
	serverResultCh := make(chan error, 1)
	go func() {
		serverResultCh <- runServerWithListen(ctx, cfg, logger, net.Listen)
	}()

	conn := waitForGRPCServer(test, cfg.Service.GRPCListenAddr)
	client := creditv1.NewCreditServiceClient(conn)

	requestContext, cancelRequests := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelRequests()

	// Add authentication header
	requestContext = metadata.NewOutgoingContext(requestContext, metadata.Pairs("authorization", "Bearer "+tenantSecret))

	if _, err := client.GetBalance(requestContext, &creditv1.BalanceRequest{UserId: " user-123 ", TenantId: testRuntimeTenantID, LedgerId: " default "}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("get balance: %v", err)
	}

	if _, err := client.GetBalance(requestContext, &creditv1.BalanceRequest{
		UserId: "user-123", TenantId: "0196f0ec-3e80-7a54-bd2b-56cfe90bf899", LedgerId: "default",
	}); status.Code(err) != codes.PermissionDenied {
		_ = conn.Close()
		cancel()
		test.Fatalf("expected tenant mismatch denial, got %v", status.Code(err))
	}

	if _, err := client.Grant(requestContext, &creditv1.GrantRequest{
		UserId:         "user-123",
		TenantId:       testRuntimeTenantID,
		LedgerId:       "default",
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{\"reason\":\"signup\"}",
	}); err != nil {
		_ = conn.Close()
		cancel()
		test.Fatalf("grant: %v", err)
	}

	_, err := client.Grant(requestContext, &creditv1.GrantRequest{
		UserId:         "user-123",
		TenantId:       testRuntimeTenantID,
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
		TenantId:       testRuntimeTenantID,
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
		TenantId:       testRuntimeTenantID,
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
		TenantId:       testRuntimeTenantID,
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
		TenantId:       testRuntimeTenantID,
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
	httpAddress := reserveLocalAddress(test)

	configFile := filepath.Join(tempDir, "config.yml")
	content := fmt.Sprintf(`
service:
  database_url: "sqlite://%s"
  grpc_listen_addr: "%s"
  http_listen_addr: "%s"
auth:
  jwt_signing_key: "test-signing-key"
  jwt_issuer: "tauth"
  tauth_tenant_id: "mprlab"
  session_cookie_name: "app_session"
  public_origin: "https://ledger.example.test"
`, sqlitePath, listenAddress, httpAddress)
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config file: %v", err)
	}

	cmd := newRootCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"--" + flagConfigFile, configFile,
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

func TestLedgerdMainHelpReturns(test *testing.T) {
	originalArgs := os.Args
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	test.Cleanup(func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	})

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		test.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		test.Fatalf("stderr pipe: %v", err)
	}
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	os.Args = []string{"ledgerd", "--help"}

	main()

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	_, _ = io.ReadAll(stdoutReader)
	_, _ = io.ReadAll(stderrReader)
	_ = stdoutReader.Close()
	_ = stderrReader.Close()
}

func TestLedgerdMainExitsOnCommandError(test *testing.T) {
	originalArgs := os.Args
	originalExitFunc := exitFunc
	originalStderrWriter := stderrWriter
	test.Cleanup(func() {
		os.Args = originalArgs
		exitFunc = originalExitFunc
		stderrWriter = originalStderrWriter
	})

	exitCalled := false
	exitCode := 0
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}
	stderrWriter = io.Discard
	os.Args = []string{"ledgerd", "--unknown-flag"}

	main()

	if !exitCalled {
		test.Fatalf("expected exit to be called")
	}
	if exitCode != 1 {
		test.Fatalf("expected exit code 1, got %d", exitCode)
	}
}

type alwaysErrorWriter struct{}

func (alwaysErrorWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestLedgerdMainExitsWithCodeTwoWhenErrorOutputFails(test *testing.T) {
	originalArgs := os.Args
	originalExitFunc := exitFunc
	originalStderrWriter := stderrWriter
	originalStderr := os.Stderr
	test.Cleanup(func() {
		os.Args = originalArgs
		exitFunc = originalExitFunc
		stderrWriter = originalStderrWriter
		os.Stderr = originalStderr
	})

	stderrReader, stderrPipeWriter, err := os.Pipe()
	if err != nil {
		test.Fatalf("stderr pipe: %v", err)
	}
	if err := stderrReader.Close(); err != nil {
		test.Fatalf("stderr reader close: %v", err)
	}
	os.Stderr = stderrPipeWriter
	if err := stderrPipeWriter.Close(); err != nil {
		test.Fatalf("stderr writer close: %v", err)
	}

	exitCalled := false
	exitCode := 0
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}
	stderrWriter = alwaysErrorWriter{}
	os.Args = []string{"ledgerd", "--unknown-flag"}

	main()

	if !exitCalled {
		test.Fatalf("expected exit to be called")
	}
	if exitCode != 2 {
		test.Fatalf("expected exit code 2, got %d", exitCode)
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

type stubTenantAuthenticator struct {
	tenantID string
	secret   string
}

func (authenticator stubTenantAuthenticator) Authenticate(_ context.Context, secret string) (tenant.ID, error) {
	tenantID, _ := tenant.NewID(authenticator.tenantID)
	if secret != authenticator.secret {
		return tenant.ID{}, tenant.ErrCredentialInvalid
	}
	return tenantID, nil
}

func configuredRuntime(databaseURL string, grpcAddress string, httpAddress string) *runtimeConfig {
	configuration := &runtimeConfig{}
	configuration.Service.DatabaseURL = databaseURL
	configuration.Service.GRPCListenAddr = grpcAddress
	configuration.Service.HTTPListenAddr = httpAddress
	configuration.Auth.JWTSigningKey = "test-signing-key"
	configuration.Auth.JWTIssuer = "tauth"
	configuration.Auth.TAuthTenantID = "mprlab"
	configuration.Auth.SessionCookieName = "app_session"
	configuration.Auth.PublicOrigin = testRuntimeOrigin
	return configuration
}

func seedRuntimeTenant(test *testing.T, sqlitePath string) string {
	test.Helper()
	database, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open seed database: %v", err)
	}
	if err := prepareSchema(database, "sqlite"); err != nil {
		test.Fatalf("prepare seed database: %v", err)
	}
	createdAt := time.Now().UTC()
	userAccountID := "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"
	credentialID := "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
	secretPart := base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdefghijklmnopqrstuv"))
	digest := sha256.Sum256([]byte(secretPart))
	if err := database.Create(&gormstore.UserAccount{
		UserAccountID: userAccountID,
		AuthIssuer:    "tauth",
		AuthTenantID:  "mprlab",
		AuthUserID:    "test-user",
		CreatedAt:     createdAt,
	}).Error; err != nil {
		test.Fatalf("seed user account: %v", err)
	}
	if err := database.Create(&gormstore.LedgerTenant{
		TenantID:           testRuntimeTenantID,
		OwnerUserAccountID: userAccountID,
		Name:               "Test tenant",
		CreatedAt:          createdAt,
	}).Error; err != nil {
		test.Fatalf("seed tenant: %v", err)
	}
	if err := database.Create(&gormstore.TenantCredential{
		CredentialID: credentialID,
		TenantID:     testRuntimeTenantID,
		SecretDigest: digest[:],
		CreatedAt:    createdAt,
	}).Error; err != nil {
		test.Fatalf("seed credential: %v", err)
	}
	return "ledger_" + credentialID + "_" + secretPart
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

// syncTrackingCore wraps a zapcore.Core to detect Sync calls.
type syncTrackingCore struct {
	zapcore.Core
	syncCalled *bool
}

func (c *syncTrackingCore) Sync() error {
	*c.syncCalled = true
	return c.Core.Sync()
}

func (c *syncTrackingCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return c.Core.Check(entry, ce)
}

func TestLoadConfigFallsBackToDefaultConfigFile(test *testing.T) {
	viper.Reset()
	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	// Set the flag to an empty string so that the fallback to defaultConfigFile triggers
	_ = cmd.Flags().Set(flagConfigFile, "")

	err := loadConfig(cmd, cfg)
	// Since the default config.yml is unlikely to exist in the test dir, we expect a "missing" error
	if err == nil {
		test.Fatalf("expected error when falling back to default config file")
	}
	if !strings.Contains(err.Error(), defaultConfigFile) {
		test.Fatalf("expected error to reference default config file %q, got: %v", defaultConfigFile, err)
	}
}

func TestLoadConfigErrorsWhenFileUnreadable(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "unreadable.yml")
	if err := os.WriteFile(configFile, []byte("service:\n  database_url: x\n"), 0o000); err != nil {
		test.Fatalf("write config: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	err := loadConfig(cmd, cfg)
	if err == nil {
		test.Fatalf("expected error for unreadable config file")
	}
	if !strings.Contains(err.Error(), "read config file") {
		test.Fatalf("expected read config file error, got: %v", err)
	}
}

func TestLoadConfigErrorsOnUnmarshalFailure(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "bad_unmarshal.yml")
	// YAML that parses but cannot unmarshal to runtimeConfig:
	// service should be a struct but we give it a list
	content := `
service:
  database_url:
    - one
    - two
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	err := loadConfig(cmd, cfg)
	if err == nil {
		test.Fatalf("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshal config") {
		test.Fatalf("expected unmarshal config error, got: %v", err)
	}
}

func TestLoadConfigErrorsWhenListenAddrMissing(test *testing.T) {
	viper.Reset()
	tempDir := test.TempDir()
	configFile := filepath.Join(tempDir, "no_listen.yml")
	content := `
service:
  database_url: "sqlite://test.db"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config: %v", err)
	}

	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)

	err := loadConfig(cmd, cfg)
	if err == nil {
		test.Fatalf("expected error for missing listen_addr")
	}
	if !strings.Contains(err.Error(), "listen_addr") {
		test.Fatalf("expected listen_addr error, got: %v", err)
	}
}

func TestLoadConfigErrorsWhenHTTPListenAddrMissing(test *testing.T) {
	viper.Reset()
	configFile := filepath.Join(test.TempDir(), "no_http_listen.yml")
	content := `
service:
  database_url: "sqlite://test.db"
  grpc_listen_addr: ":50051"
`
	if err := os.WriteFile(configFile, []byte(content), 0o644); err != nil {
		test.Fatalf("write config: %v", err)
	}
	cfg := &runtimeConfig{}
	cmd := newRootCommand()
	cmd.Flags().String(flagConfigFile, configFile, "config")
	_ = cmd.Flags().Set(flagConfigFile, configFile)
	if err := loadConfig(cmd, cfg); err == nil || !strings.Contains(err.Error(), "http_listen_addr") {
		test.Fatalf("expected HTTP listen error, got %v", err)
	}
}

func TestRunUserAccountMigration(test *testing.T) {
	if err := runUserAccountMigration(context.Background(), &runtimeConfig{}, filepath.Join(test.TempDir(), "missing")); err == nil {
		test.Fatalf("missing mapping succeeded")
	}
	mappingPath := filepath.Join(test.TempDir(), "mapping.yml")
	credentialID := "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
	secretPart := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	content := `legacy_tenant_ids: [legacy]
tenants:
  - legacy_tenant_id: legacy
    tenant_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf801
    name: Migrated
    owner:
      user_account_id: 0196f0ec-3e80-7a54-bd2b-56cfe90bf810
      auth_issuer: tauth
      auth_tenant_id: mprlab
      auth_user_id: owner
    credential_id: ` + credentialID + `
    credential_secret: ledger_` + credentialID + `_` + secretPart + "\n"
	if err := os.WriteFile(mappingPath, []byte(content), 0o600); err != nil {
		test.Fatalf("write mapping: %v", err)
	}
	badDatabaseConfig := configuredRuntime("http://%zz", ":1", ":2")
	if err := runUserAccountMigration(context.Background(), badDatabaseConfig, mappingPath); err == nil || !strings.Contains(err.Error(), "database open") {
		test.Fatalf("expected database open error, got %v", err)
	}

	emptyDatabasePath := filepath.Join(test.TempDir(), "empty.db")
	emptyConfig := configuredRuntime("sqlite://"+emptyDatabasePath, ":1", ":2")
	if err := runUserAccountMigration(context.Background(), emptyConfig, mappingPath); err == nil {
		test.Fatalf("migration without legacy schema succeeded")
	}

	databasePath := filepath.Join(test.TempDir(), "legacy.db")
	database, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open legacy database: %v", err)
	}
	if err := database.Exec(`CREATE TABLE accounts (account_id text PRIMARY KEY, tenant_id text NOT NULL, user_id text NOT NULL, ledger_id text NOT NULL, created_at datetime NOT NULL)`).Error; err != nil {
		test.Fatalf("create legacy accounts: %v", err)
	}
	if err := database.Exec(`INSERT INTO accounts(account_id, tenant_id, user_id, ledger_id, created_at) VALUES (?, ?, ?, ?, ?)`, "0196f0ec-3e80-7a54-bd2b-56cfe90bf900", "legacy", "user", "default", time.Now().UTC()).Error; err != nil {
		test.Fatalf("insert legacy account: %v", err)
	}
	sqlDatabase, _ := database.DB()
	_ = sqlDatabase.Close()
	configuration := configuredRuntime("sqlite://"+databasePath, ":1", ":2")
	if err := runUserAccountMigration(context.Background(), configuration, mappingPath); err != nil {
		test.Fatalf("run migration: %v", err)
	}
	verified, cleanup, _, err := openDatabase(context.Background(), configuration.Service.DatabaseURL)
	if err != nil {
		test.Fatalf("reopen migrated database: %v", err)
	}
	defer func() { _ = cleanup() }()
	if !verified.Migrator().HasTable("ledger_accounts") || verified.Migrator().HasTable("accounts") {
		test.Fatalf("migration did not rename accounts")
	}

	command := newMigrationCommand(configuration)
	_ = command.Flags().Set(flagMigrationMap, filepath.Join(test.TempDir(), "missing"))
	if err := command.RunE(command, nil); err == nil {
		test.Fatalf("migration command ignored mapping error")
	}
}

func TestRunServerWithListenLogsCleanupError(test *testing.T) {
	originalOpenDB := openDatabaseFunc
	test.Cleanup(func() { openDatabaseFunc = originalOpenDB })

	core, observedLogs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	cleanupErr := errors.New("cleanup forced failure")
	openDatabaseFunc = func(ctx context.Context, dsn string) (*gorm.DB, func() error, string, error) {
		db, cleanup, driver, err := openDatabase(ctx, dsn)
		if err != nil {
			return nil, nil, "", err
		}
		// Wrap cleanup to always return an error
		failingCleanup := func() error {
			_ = cleanup()
			return cleanupErr
		}
		return db, failingCleanup, driver, nil
	}

	cfg := configuredRuntime("sqlite://:memory:", reserveLocalAddress(test), reserveLocalAddress(test))

	ctx, cancel := context.WithCancel(context.Background())
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- runServerWithListen(ctx, cfg, logger, net.Listen)
	}()

	conn := waitForGRPCServer(test, cfg.Service.GRPCListenAddr)
	_ = conn.Close()
	cancel()

	if err := <-serverDone; err != nil {
		test.Fatalf("unexpected server error: %v", err)
	}

	cleanupLogs := observedLogs.FilterMessage("database cleanup failed")
	if cleanupLogs.Len() == 0 {
		test.Fatalf("expected database cleanup failed log entry")
	}
}

func TestRunServerWithListenPrepareSchemaErrorAfterDBOpen(test *testing.T) {
	originalPrepareSchema := prepareSchemaFunc
	test.Cleanup(func() { prepareSchemaFunc = originalPrepareSchema })

	schemaErr := errors.New("schema migration failed")
	prepareSchemaFunc = func(_ *gorm.DB, _ string) error {
		return schemaErr
	}

	core, _ := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	cfg := configuredRuntime("sqlite://:memory:", reserveLocalAddress(test), reserveLocalAddress(test))

	err := runServerWithListen(context.Background(), cfg, logger, net.Listen)
	if !errors.Is(err, schemaErr) {
		test.Fatalf("expected schema error, got: %v", err)
	}
}

func TestRunServerWithListenNewServiceError(test *testing.T) {
	originalNewService := newServiceFunc
	test.Cleanup(func() { newServiceFunc = originalNewService })

	serviceErr := errors.New("service init failed")
	newServiceFunc = func(_ ledger.Store, _ func() int64, _ ...ledger.ServiceOption) (*ledger.Service, error) {
		return nil, serviceErr
	}

	core, _ := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	cfg := configuredRuntime("sqlite://:memory:", reserveLocalAddress(test), reserveLocalAddress(test))

	err := runServerWithListen(context.Background(), cfg, logger, net.Listen)
	if err == nil || !strings.Contains(err.Error(), "ledger service init") {
		test.Fatalf("expected ledger service init error, got: %v", err)
	}
}

func TestRunServerWithListenServeErrorNotServerStopped(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "serve_err.db")

	core, _ := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	cfg := configuredRuntime("sqlite://"+sqlitePath, reserveLocalAddress(test), reserveLocalAddress(test))

	// Provide a listener that is already closed, causing Serve to fail immediately
	// with an error that is NOT ErrServerStopped.
	err := runServerWithListen(context.Background(), cfg, logger, func(network, address string) (net.Listener, error) {
		lis, listenErr := net.Listen(network, address)
		if listenErr != nil {
			return nil, listenErr
		}
		// Close the listener before Serve gets to use it
		_ = lis.Close()
		return lis, nil
	})
	if err == nil {
		test.Fatalf("expected serve error")
	}
	if errors.Is(err, grpc.ErrServerStopped) {
		test.Fatalf("expected error other than ErrServerStopped, got %v", err)
	}
}

func TestResolveDriverSQLiteURLEmptyPathDefaultsToLedgerDB(test *testing.T) {
	driver, sqlitePath, err := resolveDriver("sqlite://")
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if driver != "sqlite" {
		test.Fatalf("expected sqlite driver, got %q", driver)
	}
	if !strings.Contains(sqlitePath, "ledger.db") {
		test.Fatalf("expected path containing ledger.db, got %q", sqlitePath)
	}
}

func TestResolveDriverSQLiteURLRootPathDefaultsToLedgerDB(test *testing.T) {
	driver, sqlitePath, err := resolveDriver("sqlite:///")
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if driver != "sqlite" {
		test.Fatalf("expected sqlite driver, got %q", driver)
	}
	// sqlite:/// has path "/" which should default to ledger.db
	if !strings.Contains(sqlitePath, "ledger.db") {
		test.Fatalf("expected path containing ledger.db, got %q", sqlitePath)
	}
}

func TestResolveDriverSQLiteURLWithHostNoPath(test *testing.T) {
	// sqlite://hostname with no path: Host="hostname", Path=""
	// path = u.Host = "hostname" which is not empty, so it's used as a path.
	driver, sqlitePath, err := resolveDriver("sqlite://hostname")
	if err != nil {
		test.Fatalf("unexpected error: %v", err)
	}
	if driver != "sqlite" {
		test.Fatalf("expected sqlite driver, got %q", driver)
	}
	if !strings.Contains(sqlitePath, "hostname") {
		test.Fatalf("expected path containing hostname, got %q", sqlitePath)
	}
}

func TestNormalizeSQLiteFileDSNParseError(test *testing.T) {
	_, err := normalizeSQLiteFileDSN("file://%zz")
	if err == nil {
		test.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse sqlite file url") {
		test.Fatalf("expected parse sqlite file url error, got: %v", err)
	}
}

func TestNormalizeSQLiteFileDSNPathNormalizationError(test *testing.T) {
	tempDir := test.TempDir()
	// Create a file that blocks directory creation
	blockingFile := filepath.Join(tempDir, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		test.Fatalf("write file: %v", err)
	}
	// file:///path/not-a-directory/sub/ledger.db - the parent dir creation should fail
	dsn := "file://" + filepath.Join(blockingFile, "sub", "ledger.db")
	_, err := normalizeSQLiteFileDSN(dsn)
	if err == nil {
		test.Fatalf("expected normalization error")
	}
}

func TestOpenDatabaseSQLiteWithDirCreation(test *testing.T) {
	tempDir := test.TempDir()
	// Use a path that requires creating a subdirectory
	sqlitePath := filepath.Join(tempDir, "subdir", "deep", "ledger.db")
	ctx := context.Background()
	db, cleanup, driver, err := openDatabase(ctx, "sqlite://"+sqlitePath)
	if err != nil {
		test.Fatalf("open sqlite with dir creation: %v", err)
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
}

func TestPrepareSchemaPragmaBusyTimeoutError(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "pragma_busy.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })

	// Register a callback that fails on busy_timeout
	db.Callback().Raw().Before("*").Register("fail_busy_timeout", func(gormDB *gorm.DB) {
		if gormDB.Statement != nil && strings.Contains(gormDB.Statement.SQL.String(), "busy_timeout") {
			gormDB.AddError(errors.New("busy_timeout pragma failed"))
		}
	})

	err = prepareSchema(db, "sqlite")
	if err == nil {
		test.Fatalf("expected busy_timeout error")
	}
	if !strings.Contains(err.Error(), "busy_timeout") {
		test.Fatalf("expected busy_timeout in error, got: %v", err)
	}
}

func TestPrepareSchemaPragmaForeignKeysError(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "pragma_fk.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })

	// Register a callback that fails on foreign_keys
	db.Callback().Raw().Before("*").Register("fail_foreign_keys", func(gormDB *gorm.DB) {
		if gormDB.Statement != nil && strings.Contains(gormDB.Statement.SQL.String(), "foreign_keys") {
			gormDB.AddError(errors.New("foreign_keys pragma failed"))
		}
	})

	err = prepareSchema(db, "sqlite")
	if err == nil {
		test.Fatalf("expected foreign_keys error")
	}
	if !strings.Contains(err.Error(), "foreign_keys") {
		test.Fatalf("expected foreign_keys in error, got: %v", err)
	}
}

func TestPrepareSchemaReturnsErrorWhenSQLDBFails(test *testing.T) {
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "sqldb_fail.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		test.Fatalf("open sqlite: %v", err)
	}
	// Close the underlying sql.DB so that db.DB() returns it but subsequent
	// operations fail. Actually, db.DB() caches - we need it to fail.
	// Close db's sql.DB first, then call prepareSchema.
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("sql db: %v", err)
	}
	// db.DB() caches the result, so calling it again won't fail.
	// Instead, use a ConnPool that returns an error from GetDBConn.
	_ = sqlDB.Close()

	// The db.DB() call in prepareSchema will return the cached (but closed) connection,
	// so it won't error on db.DB() itself, but on the subsequent operations.
	// That means this test doesn't cover the db.DB() error path.
	// We need a different approach to cover line 488-489.
	err = prepareSchema(db, "sqlite")
	if err == nil {
		test.Fatalf("expected error when sql.DB is closed")
	}
}

func TestRunServerWithListenServeErrorNotServerStoppedViaErrCh(test *testing.T) {
	// Test lines 215-219: when Serve returns an error via errCh without ctx cancellation.
	// The case where errCh receives ErrServerStopped returns nil (line 217).
	// The case where errCh receives another error returns that error (line 219).
	// TestRunServerWithListenServeErrorNotServerStopped already covers the non-ErrServerStopped case.
	// To cover the ErrServerStopped case: use a listener that closes immediately
	// after Accept is called once. grpc.Server.Serve will return after listener is closed.
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "errstop.db")

	core, _ := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	cfg := configuredRuntime("sqlite://"+sqlitePath, reserveLocalAddress(test), reserveLocalAddress(test))

	// Use a listener wrapper that closes the listener after a short delay,
	// simulating the gRPC server stopping itself.
	err := runServerWithListen(context.Background(), cfg, logger, func(network, address string) (net.Listener, error) {
		lis, lisErr := net.Listen(network, address)
		if lisErr != nil {
			return nil, lisErr
		}
		return &autoCloseListener{Listener: lis, delay: 100 * time.Millisecond}, nil
	})
	// When the listener closes, grpc.Server.Serve returns a "use of closed network connection" error
	// which is NOT ErrServerStopped, so line 219 is hit.
	if err == nil {
		test.Fatalf("expected serve error from closed listener")
	}
}

func TestAwaitServersErrorPaths(test *testing.T) {
	logger := zap.NewNop()
	errCh := make(chan error, 2)
	errCh <- errors.New("forced serve error")
	errCh <- http.ErrServerClosed
	if err := shutdownServers(grpc.NewServer(), &http.Server{}, errCh); err == nil || !strings.Contains(err.Error(), "forced serve error") {
		test.Fatalf("expected shutdown serve error, got %v", err)
	}

	errCh = make(chan error, 1)
	errCh <- http.ErrServerClosed
	if err := awaitServers(context.Background(), grpc.NewServer(), &http.Server{}, errCh, logger); err != nil {
		test.Fatalf("expected normal server close, got %v", err)
	}

}

// autoCloseListener wraps a net.Listener and closes it after a delay.
type autoCloseListener struct {
	net.Listener
	delay time.Duration
}

func (l *autoCloseListener) Accept() (net.Conn, error) {
	go func() {
		time.Sleep(l.delay)
		_ = l.Listener.Close()
	}()
	return l.Listener.Accept()
}

// minimalConnPool satisfies gorm.ConnPool but not GetDBConnector,
// so gorm.DB.DB() returns gorm.ErrInvalidDB.
type minimalConnPool struct{}

func (minimalConnPool) PrepareContext(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (minimalConnPool) ExecContext(_ context.Context, _ string, _ ...interface{}) (sql.Result, error) {
	return nil, errors.New("not implemented")
}

func (minimalConnPool) QueryContext(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func (minimalConnPool) QueryRowContext(_ context.Context, _ string, _ ...interface{}) *sql.Row {
	return nil
}

func TestOpenDatabaseSQLDBError(test *testing.T) {
	// Cover line 401-403: db.DB() returns error after successful gorm.Open.
	// We inject a custom gormOpenFunc to create a gorm.DB whose ConnPool does not
	// implement GetDBConnector, causing db.DB() to return gorm.ErrInvalidDB.
	originalGormOpen := gormOpenFunc
	test.Cleanup(func() { gormOpenFunc = originalGormOpen })

	gormOpenFunc = func(_ gorm.Dialector, _ ...gorm.Option) (*gorm.DB, error) {
		db := &gorm.DB{Config: &gorm.Config{}}
		db.ConnPool = minimalConnPool{}
		return db, nil
	}

	_, _, _, err := openDatabase(context.Background(), "sqlite://:memory:")
	if err == nil {
		test.Fatalf("expected db.DB() error")
	}
}

func TestPrepareSchemaDBError(test *testing.T) {
	// Cover line 488-489: db.DB() returns error in prepareSchema.
	// Create a gorm.DB with a ConnPool that doesn't implement GetDBConnector.
	db := &gorm.DB{
		Config: &gorm.Config{},
	}
	db.ConnPool = minimalConnPool{}
	db.Statement = &gorm.Statement{}

	err := prepareSchema(db, "sqlite")
	if err == nil {
		test.Fatalf("expected sql database error")
	}
	if !strings.Contains(err.Error(), "database handle is invalid") {
		test.Fatalf("expected invalid database handle error, got: %v", err)
	}
}

func TestMain(test *testing.M) {
	viper.Reset()
	os.Exit(test.Run())
}
