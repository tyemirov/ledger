package main

import (
	"context"
	"fmt"
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
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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

func main() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "ledgerd: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cfg := &runtimeConfig{}
	cmd := &cobra.Command{
		Use:           "ledgerd",
		Short:         "Ledger gRPC server",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig(cmd, cfg)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return runServer(ctx, cfg)
		},
	}

	cmd.Flags().String(flagDatabaseURL, defaultDatabaseURL, "PostgreSQL connection string")
	cmd.Flags().String(flagListenAddr, defaultGRPCListenAddr, "gRPC listen address")

	return cmd
}

func loadConfig(cmd *cobra.Command, cfg *runtimeConfig) error {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	if err := viper.BindEnv(configKeyDatabaseURL, "DATABASE_URL"); err != nil {
		return err
	}
	if err := viper.BindEnv(configKeyListenAddr, "GRPC_LISTEN_ADDR"); err != nil {
		return err
	}

	if err := viper.BindPFlag(configKeyDatabaseURL, cmd.Flags().Lookup(flagDatabaseURL)); err != nil {
		return err
	}
	if err := viper.BindPFlag(configKeyListenAddr, cmd.Flags().Lookup(flagListenAddr)); err != nil {
		return err
	}

	cfg.DatabaseURL = viper.GetString(configKeyDatabaseURL)
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = defaultDatabaseURL
	}
	cfg.ListenAddr = viper.GetString(configKeyListenAddr)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultGRPCListenAddr
	}
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("database url is required")
	}
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen addr is required")
	}
	return nil
}

func runServer(ctx context.Context, cfg *runtimeConfig) error {
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("logger init: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	gormDB, cleanup, driver, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer cleanup()

	if err := prepareSchema(gormDB, driver); err != nil {
		return err
	}

	store := gormstore.New(gormDB)
	clock := func() int64 { return time.Now().UTC().Unix() }
	opLogger := &zapOperationLogger{logger: logger}
	creditService, err := ledger.NewService(store, clock, ledger.WithOperationLogger(opLogger))
	if err != nil {
		return fmt.Errorf("ledger service init: %w", err)
	}

	lis, err := net.Listen("tcp", cfg.ListenAddr)
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

	var (
		db  *gorm.DB
		cfg *gorm.Config
	)
	cfg = &gorm.Config{}
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
	if driver != "sqlite" {
		return nil
	}
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
