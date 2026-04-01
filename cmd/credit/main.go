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
	"regexp"
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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	flagConfigFile    = "config"
	defaultConfigFile = "config.yml"
)

type tenantConfig struct {
	ID        string `mapstructure:"id"`
	Name      string `mapstructure:"name"`
	SecretKey string `mapstructure:"secret_key"`
}

type runtimeConfig struct {
	Service struct {
		DatabaseURL string `mapstructure:"database_url"`
		ListenAddr  string `mapstructure:"listen_addr"`
	} `mapstructure:"service"`
	Tenants []tenantConfig `mapstructure:"tenants"`
}

var (
	exitFunc                                                       = os.Exit
	stderrWriter          io.Writer                                = os.Stderr
	newLogger             func(...zap.Option) (*zap.Logger, error) = zap.NewProduction
	openDatabaseFunc                                               = openDatabase
	prepareSchemaFunc                                              = prepareSchema
	newServiceFunc                                                 = ledger.NewService
	gormOpenFunc                                                   = gorm.Open
	configVariablePattern                                          = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)
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

	cmd.PersistentFlags().String(flagConfigFile, defaultConfigFile, "Path to mandatory configuration file")

	return cmd
}

func loadConfig(cmd *cobra.Command, cfg *runtimeConfig) error {
	v := viper.New()

	configFile, _ := cmd.Flags().GetString(flagConfigFile)
	if configFile == "" {
		configFile = defaultConfigFile
	}

	if _, err := os.Stat(configFile); err != nil {
		return fmt.Errorf("configuration file %q is mandatory but missing: %w", configFile, err)
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}

	expanded := expandConfigVariables(string(content))

	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(expanded)); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}

	if err := v.Unmarshal(cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	// Strict validation: DatabaseURL and ListenAddr must be provided in the config
	if strings.TrimSpace(cfg.Service.DatabaseURL) == "" {
		return fmt.Errorf("service.database_url is required in %q", configFile)
	}
	if strings.TrimSpace(cfg.Service.ListenAddr) == "" {
		return fmt.Errorf("service.listen_addr is required in %q", configFile)
	}

	for _, tenant := range cfg.Tenants {
		if strings.TrimSpace(tenant.ID) == "" {
			return fmt.Errorf("tenant id is required in %q", configFile)
		}
		if strings.TrimSpace(tenant.SecretKey) == "" {
			return fmt.Errorf("tenant %q secret_key is required in %q", tenant.ID, configFile)
		}
	}

	return nil
}

func expandConfigVariables(content string) string {
	return configVariablePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := configVariablePattern.FindStringSubmatch(match)
		value, isSet := os.LookupEnv(parts[1])
		if parts[2] != "" {
			if !isSet || value == "" {
				return parts[3]
			}
			return value
		}
		if !isSet {
			return ""
		}
		return value
	})
}

type listenFunc func(network, address string) (net.Listener, error)

func runServer(ctx context.Context, cfg *runtimeConfig) error {
	logger, err := newLogger()
	if err != nil {
		return fmt.Errorf("logger init: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	return runServerWithListen(ctx, cfg, logger, net.Listen)
}

func runServerWithListen(ctx context.Context, cfg *runtimeConfig, logger *zap.Logger, listen listenFunc) error {
	gormDB, cleanup, driver, err := openDatabaseFunc(ctx, cfg.Service.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			logger.Error("database cleanup failed", zap.Error(cleanupErr))
		}
	}()

	if err := prepareSchemaFunc(gormDB, driver); err != nil {
		return err
	}

	store := gormstore.New(gormDB)
	clock := func() int64 { return time.Now().UTC().Unix() }
	opLogger := &zapOperationLogger{logger: logger}
	creditService, err := newServiceFunc(
		store,
		clock,
		ledger.WithOperationLogger(opLogger),
	)
	if err != nil {
		return fmt.Errorf("ledger service init: %w", err)
	}

	lis, err := listen("tcp", cfg.Service.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	tenantSecrets := make(map[string]string, len(cfg.Tenants))
	tenantIDs := make([]string, 0, len(cfg.Tenants))
	for _, tenant := range cfg.Tenants {
		tenantSecrets[tenant.ID] = tenant.SecretKey
		tenantIDs = append(tenantIDs, tenant.ID)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			newLoggingInterceptor(logger),
			newAuthInterceptor(tenantSecrets),
		),
	)

	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(creditService, tenantIDs))

	errCh := make(chan error, 1)
	go func() {
		logger.Info("gRPC server starting", zap.String("listen_addr", cfg.Service.ListenAddr))
		errCh <- grpcServer.Serve(lis)
	}()

	return awaitServer(ctx, grpcServer, errCh, logger)
}

type userIDGetter interface {
	GetUserId() string
}

type ledgerIDGetter interface {
	GetLedgerId() string
}

func awaitServer(ctx context.Context, grpcServer *grpc.Server, errCh <-chan error, logger *zap.Logger) error {
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

func newAuthInterceptor(tenantSecrets map[string]string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		tenantID := extractTenantID(request)
		if tenantID == "" {
			return nil, status.Error(codes.Unauthenticated, "missing tenant_id")
		}

		expectedSecret, ok := tenantSecrets[tenantID]
		if !ok {
			return nil, status.Errorf(codes.PermissionDenied, "tenant %q is not authorized", tenantID)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		authHeader := md.Get("authorization")
		if len(authHeader) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		const bearerPrefix = "Bearer "
		token := authHeader[0]
		if !strings.HasPrefix(token, bearerPrefix) {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
		}

		providedSecret := strings.TrimPrefix(token, bearerPrefix)
		if providedSecret != expectedSecret {
			return nil, status.Error(codes.Unauthenticated, "invalid secret key")
		}

		return handler(ctx, request)
	}
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
		db, err = gormOpenFunc(postgres.Open(dsn), cfg)
	case "sqlite":
		db, err = gormOpenFunc(sqlite.Open(sqlitePath), cfg)
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
