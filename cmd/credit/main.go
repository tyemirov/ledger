package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	flagDatabaseURL       = "database-url"
	flagListenAddr        = "listen-addr"
	configKeyDatabaseURL  = "database_url"
	configKeyListenAddr   = "listen_addr"
	defaultDatabaseURL    = "sqlite:///tmp/ledger.db"
	defaultGRPCListenAddr = ":50051"
)

type runtimeConfig struct {
	DatabaseURL string
	ListenAddr  string
}

var (
	exitFunc               = os.Exit
	stderrWriter io.Writer = os.Stderr
)

func main() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		exitCode := 1
		if _, writeErr := fmt.Fprintf(stderrWriter, "ledgerd: %v\n", err); writeErr != nil {
			if _, fallbackErr := fmt.Fprintf(os.Stderr, "ledgerd: %v\n", err); fallbackErr != nil {
				exitCode = 2
			}
		}
		exitFunc(exitCode)
	}
}

func newRootCommand() *cobra.Command {
	cfg := &runtimeConfig{}
	cmd := &cobra.Command{
		Use:           "ledgerd",
		Short:         "Ledger gRPC server",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig(cmd, cfg)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runServer(ctx, cfg)
		},
	}

	cmd.PersistentFlags().String(flagDatabaseURL, defaultDatabaseURL, "PostgreSQL connection string")
	cmd.PersistentFlags().String(flagListenAddr, defaultGRPCListenAddr, "gRPC listen address")

	return cmd
}

func loadConfig(cmd *cobra.Command, cfg *runtimeConfig) error {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	// viper.BindEnv errors only when the key is missing (len(input)==0).
	// These keys are compile-time constants, so BindEnv cannot fail here.
	_ = viper.BindEnv(configKeyDatabaseURL, "DATABASE_URL")
	_ = viper.BindEnv(configKeyListenAddr, "GRPC_LISTEN_ADDR")

	databaseFlag := lookupFlag(cmd, flagDatabaseURL)
	if databaseFlag == nil {
		return fmt.Errorf("flag for %q is nil", configKeyDatabaseURL)
	}
	_ = viper.BindPFlag(configKeyDatabaseURL, databaseFlag)

	listenFlag := lookupFlag(cmd, flagListenAddr)
	if listenFlag == nil {
		return fmt.Errorf("flag for %q is nil", configKeyListenAddr)
	}
	_ = viper.BindPFlag(configKeyListenAddr, listenFlag)

	cfg.DatabaseURL = viper.GetString(configKeyDatabaseURL)
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = defaultDatabaseURL
	}
	cfg.ListenAddr = viper.GetString(configKeyListenAddr)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultGRPCListenAddr
	}
	return nil
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if flag := cmd.Flags().Lookup(name); flag != nil {
		return flag
	}
	if flag := cmd.PersistentFlags().Lookup(name); flag != nil {
		return flag
	}
	if flag := cmd.InheritedFlags().Lookup(name); flag != nil {
		return flag
	}
	return nil
}

type listenFunc func(network, address string) (net.Listener, error)

func runServer(ctx context.Context, cfg *runtimeConfig) error {
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("logger init: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	return runServerWithListen(ctx, cfg, logger, net.Listen)
}

func runServerWithListen(ctx context.Context, cfg *runtimeConfig, logger *zap.Logger, listen listenFunc) error {
	gormDB, cleanup, driver, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			logger.Error("database cleanup failed", zap.Error(cleanupErr))
		}
	}()

	if err := prepareSchema(gormDB, driver); err != nil {
		return err
	}

	store := gormstore.New(gormDB)
	clock := func() int64 { return time.Now().UTC().Unix() }
	opLogger := &zapOperationLogger{logger: logger}
	creditService, err := ledger.NewService(
		store,
		clock,
		ledger.WithOperationLogger(opLogger),
	)
	if err != nil {
		return fmt.Errorf("ledger service init: %w", err)
	}

	lis, err := listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(newLoggingInterceptor(logger)),
	)
	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(creditService))

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gRPC server starting", zap.String("listen_addr", cfg.ListenAddr))
		errCh <- grpcServer.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
		grpcServer.GracefulStop()
		if serveErr := <-errCh; serveErr != nil && serveErr != grpc.ErrServerStopped {
			return serveErr
		}
		return nil
	case serveErr := <-errCh:
		if serveErr == grpc.ErrServerStopped {
			return nil
		}
		return serveErr
	}
}

type userIDGetter interface {
	GetUserId() string
}

type ledgerIDGetter interface {
	GetLedgerId() string
}

type tenantIDGetter interface {
	GetTenantId() string
}

func extractUserID(request interface{}) string {
	getter, ok := request.(userIDGetter)
	if !ok {
		return ""
	}
	userID := strings.TrimSpace(getter.GetUserId())
	return userID
}

func extractLedgerID(request interface{}) string {
	getter, ok := request.(ledgerIDGetter)
	if !ok {
		return ""
	}
	ledgerID := strings.TrimSpace(getter.GetLedgerId())
	return ledgerID
}

func extractTenantID(request interface{}) string {
	getter, ok := request.(tenantIDGetter)
	if !ok {
		return ""
	}
	tenantID := strings.TrimSpace(getter.GetTenantId())
	return tenantID
}

func newLoggingInterceptor(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		response, err := handler(ctx, request)
		code := status.Code(err)

		fields := []zap.Field{
			zap.String("method", info.FullMethod),
			zap.Duration("duration", time.Since(start)),
			zap.String("code", code.String()),
		}
		if userID := extractUserID(request); userID != "" {
			fields = append(fields, zap.String("user_id", userID))
		}
		if ledgerID := extractLedgerID(request); ledgerID != "" {
			fields = append(fields, zap.String("ledger_id", ledgerID))
		}
		if tenantID := extractTenantID(request); tenantID != "" {
			fields = append(fields, zap.String("tenant_id", tenantID))
		}
		if err != nil {
			logger.Error("grpc request failed", append(fields, zap.Error(err))...)
		} else {
			logger.Info("grpc request completed", fields...)
		}
		return response, err
	}
}

type zapOperationLogger struct {
	logger *zap.Logger
}

func (logger *zapOperationLogger) LogOperation(_ context.Context, entry ledger.OperationLog) {
	if logger == nil || logger.logger == nil {
		return
	}
	const logEventLedgerOperation = "ledger.operation"
	status := entry.Status
	if status == "" {
		if entry.Error != nil {
			status = "error"
		} else {
			status = "ok"
		}
	}
	fields := []zap.Field{
		zap.String("operation", entry.Operation),
		zap.String("status", status),
	}
	if user := entry.UserID.String(); user != "" {
		fields = append(fields, zap.String("user_id", user))
	}
	if ledgerID := entry.LedgerID.String(); ledgerID != "" {
		fields = append(fields, zap.String("ledger_id", ledgerID))
	}
	if tenantID := entry.TenantID.String(); tenantID != "" {
		fields = append(fields, zap.String("tenant_id", tenantID))
	}
	if entry.Amount != 0 {
		fields = append(fields, zap.Int64("amount_cents", entry.Amount.Int64()))
	}
	if entry.ReservationID != nil {
		if reservation := entry.ReservationID.String(); reservation != "" {
			fields = append(fields, zap.String("reservation_id", reservation))
		}
	}
	if key := entry.IdempotencyKey.String(); key != "" {
		fields = append(fields, zap.String("idempotency_key", key))
	}
	if metadata := entry.Metadata.String(); metadata != "" && metadata != "{}" {
		fields = append(fields, zap.String("metadata", metadata))
	}
	if entry.Error != nil {
		fields = append(fields, zap.Error(entry.Error))
		logger.logger.Error(logEventLedgerOperation, fields...)
		return
	}
	logger.logger.Info(logEventLedgerOperation, fields...)
}

func openDatabase(ctx context.Context, dsn string) (*gorm.DB, func() error, string, error) {
	driver, sqlitePath, err := resolveDriver(dsn)
	if err != nil {
		return nil, nil, "", err
	}

	var db *gorm.DB
	cfg := &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	}
	switch driver {
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), cfg)
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(sqlitePath), cfg)
	default:
		return nil, nil, "", fmt.Errorf("unsupported database scheme %q", driver)
	}
	if err != nil {
		return nil, nil, "", err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, "", err
	}
	cleanup := func() error { return sqlDB.Close() }
	return db.WithContext(ctx), cleanup, driver, nil
}

func resolveDriver(dsn string) (string, string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		return "postgres", "", nil
	}
	if strings.HasPrefix(dsn, "file:") {
		sqliteDSN, err := normalizeSQLiteFileDSN(dsn)
		if err != nil {
			return "", "", err
		}
		return "sqlite", sqliteDSN, nil
	}
	if strings.Contains(dsn, "://") && !strings.HasPrefix(dsn, "sqlite://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", fmt.Errorf("parse database url: %w", err)
		}
		if u.Scheme != "" && u.Scheme != "postgres" && u.Scheme != "postgresql" {
			return u.Scheme, "", nil
		}
	}
	if strings.HasPrefix(dsn, "sqlite://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", "", fmt.Errorf("parse sqlite url: %w", err)
		}
		path := u.Path
		if path == "" {
			path = u.Host
		}
		if path == "" || path == "/" {
			path = "ledger.db"
		}
		sqlitePath, err := normalizeSQLitePath(path)
		return "sqlite", sqlitePath, err
	}
	sqlitePath, err := normalizeSQLitePath(dsn)
	return "sqlite", sqlitePath, err
}

func normalizeSQLiteFileDSN(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parse sqlite file url: %w", err)
	}
	if parsed.Opaque != "" {
		return dsn, nil
	}
	if parsed.Host != "" && parsed.Host != "localhost" {
		return "", fmt.Errorf("parse sqlite file url: unsupported host %q", parsed.Host)
	}
	if strings.TrimSpace(parsed.Path) == "" {
		return "", fmt.Errorf("parse sqlite file url: missing path")
	}
	if _, err := normalizeSQLitePath(parsed.Path); err != nil {
		return "", err
	}
	return dsn, nil
}

func normalizeSQLitePath(path string) (string, error) {
	if path == ":memory:" {
		return path, nil
	}
	if strings.HasPrefix(path, "/") {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		return path, nil
	}
	abs := filepath.Join(".", path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

func prepareSchema(db *gorm.DB, driver string) error {
	if driver == "sqlite" {
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("sql database: %w", err)
		}
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)

		if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
			return fmt.Errorf("pragma journal_mode: %w", err)
		}
		if err := db.Exec("PRAGMA busy_timeout=5000;").Error; err != nil {
			return fmt.Errorf("pragma busy_timeout: %w", err)
		}
		if err := db.Exec("PRAGMA foreign_keys=ON;").Error; err != nil {
			return fmt.Errorf("pragma foreign_keys: %w", err)
		}
	}
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
