package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/controlplane"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/migration"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
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
	flagMigrationMap  = "mapping"
	defaultConfigFile = "config.yml"
)

type runtimeConfig struct {
	Service struct {
		DatabaseURL    string `mapstructure:"database_url"`
		GRPCListenAddr string `mapstructure:"grpc_listen_addr"`
		HTTPListenAddr string `mapstructure:"http_listen_addr"`
	} `mapstructure:"service"`
	Auth struct {
		JWTSigningKey     string `mapstructure:"jwt_signing_key"`
		JWTIssuer         string `mapstructure:"jwt_issuer"`
		TAuthTenantID     string `mapstructure:"tauth_tenant_id"`
		SessionCookieName string `mapstructure:"session_cookie_name"`
		PublicOrigin      string `mapstructure:"public_origin"`
	} `mapstructure:"auth"`
	UI struct {
		Description    string `mapstructure:"description"`
		TAuthURL       string `mapstructure:"tauth_url"`
		GoogleClientID string `mapstructure:"google_client_id"`
		LoginPath      string `mapstructure:"login_path"`
		LogoutPath     string `mapstructure:"logout_path"`
		NoncePath      string `mapstructure:"nonce_path"`
		SessionPath    string `mapstructure:"session_path"`
	} `mapstructure:"ui"`
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
	cmd.AddCommand(newMigrationCommand(cfg))

	return cmd
}

func newMigrationCommand(cfg *runtimeConfig) *cobra.Command {
	command := &cobra.Command{
		Use:   "migrate-user-accounts",
		Short: "Migrate the legacy static tenant data once",
		RunE: func(command *cobra.Command, _ []string) error {
			mappingPath, _ := command.Flags().GetString(flagMigrationMap)
			return runUserAccountMigration(command.Context(), cfg, mappingPath)
		},
	}
	command.Flags().String(flagMigrationMap, "", "Path to the mode-0600 migration mapping")
	_ = command.MarkFlagRequired(flagMigrationMap)
	return command
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

	if err := v.UnmarshalExact(cfg); err != nil {
		return fmt.Errorf("unmarshal config: %w", err)
	}

	if strings.TrimSpace(cfg.Service.DatabaseURL) == "" {
		return fmt.Errorf("service.database_url is required in %q", configFile)
	}
	if strings.TrimSpace(cfg.Service.GRPCListenAddr) == "" {
		return fmt.Errorf("service.grpc_listen_addr is required in %q", configFile)
	}
	if strings.TrimSpace(cfg.Service.HTTPListenAddr) == "" {
		return fmt.Errorf("service.http_listen_addr is required in %q", configFile)
	}
	for field, value := range map[string]string{
		"auth.jwt_signing_key":     cfg.Auth.JWTSigningKey,
		"auth.jwt_issuer":          cfg.Auth.JWTIssuer,
		"auth.tauth_tenant_id":     cfg.Auth.TAuthTenantID,
		"auth.session_cookie_name": cfg.Auth.SessionCookieName,
		"auth.public_origin":       cfg.Auth.PublicOrigin,
		"ui.description":           cfg.UI.Description,
		"ui.tauth_url":             cfg.UI.TAuthURL,
		"ui.google_client_id":      cfg.UI.GoogleClientID,
		"ui.login_path":            cfg.UI.LoginPath,
		"ui.logout_path":           cfg.UI.LogoutPath,
		"ui.nonce_path":            cfg.UI.NoncePath,
		"ui.session_path":          cfg.UI.SessionPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required in %q", field, configFile)
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

func runUserAccountMigration(ctx context.Context, cfg *runtimeConfig, mappingPath string) error {
	mapping, err := migration.Load(mappingPath)
	if err != nil {
		return err
	}
	database, cleanup, _, err := openDatabaseFunc(ctx, cfg.Service.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database open: %w", err)
	}
	defer func() { _ = cleanup() }()
	if err := migration.Apply(ctx, database, mapping); err != nil {
		return err
	}
	return nil
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
	accountService, _ := useraccount.NewDefaultService(store)
	tenantService, _ := tenant.NewDefaultService(store)
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

	grpcListener, err := listen("tcp", cfg.Service.GRPCListenAddr)
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	httpListener, err := listen("tcp", cfg.Service.HTTPListenAddr)
	if err != nil {
		_ = grpcListener.Close()
		return fmt.Errorf("http listen: %w", err)
	}

	sessions, _ := sessionvalidator.New(sessionvalidator.Config{
		SigningKey: []byte(cfg.Auth.JWTSigningKey),
		Issuer:     cfg.Auth.JWTIssuer,
		CookieName: cfg.Auth.SessionCookieName,
	})
	httpHandler, _ := controlplane.NewHandler(accountService, tenantService, sessions, cfg.Auth.TAuthTenantID, cfg.Auth.PublicOrigin, controlplane.BrowserConfig{
		Description:    cfg.UI.Description,
		TAuthURL:       cfg.UI.TAuthURL,
		GoogleClientID: cfg.UI.GoogleClientID,
		LoginPath:      cfg.UI.LoginPath,
		LogoutPath:     cfg.UI.LogoutPath,
		NoncePath:      cfg.UI.NoncePath,
		SessionPath:    cfg.UI.SessionPath,
	}, opLogger)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			newLoggingInterceptor(logger),
			newAuthInterceptor(tenantService),
		),
	)
	httpServer := &http.Server{Handler: httpHandler, ReadHeaderTimeout: 5 * time.Second}

	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(creditService))

	errCh := make(chan error, 2)
	go func() {
		logger.Info("gRPC server starting", zap.String("listen_addr", cfg.Service.GRPCListenAddr))
		errCh <- grpcServer.Serve(grpcListener)
	}()
	go func() {
		logger.Info("HTTP server starting", zap.String("listen_addr", cfg.Service.HTTPListenAddr))
		errCh <- httpServer.Serve(httpListener)
	}()

	return awaitServers(ctx, grpcServer, httpServer, errCh, logger)
}

type userIDGetter interface {
	GetUserId() string
}

type ledgerIDGetter interface {
	GetLedgerId() string
}

func awaitServers(ctx context.Context, grpcServer *grpc.Server, httpServer *http.Server, errCh <-chan error, logger *zap.Logger) error {
	select {
	case <-ctx.Done():
		logger.Info("shutdown requested")
		return shutdownServers(grpcServer, httpServer, errCh)
	case serveError := <-errCh:
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
		grpcServer.GracefulStop()
		if errors.Is(serveError, http.ErrServerClosed) || serveError == grpc.ErrServerStopped {
			return nil
		}
		return serveError
	}
}

func shutdownServers(grpcServer *grpc.Server, httpServer *http.Server, errCh <-chan error) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpError := httpServer.Shutdown(shutdownContext)
	grpcServer.GracefulStop()
	for range 2 {
		serveError := <-errCh
		if serveError != nil && !errors.Is(serveError, http.ErrServerClosed) && serveError != grpc.ErrServerStopped {
			return serveError
		}
	}
	return httpError
}

type tenantIDGetter interface {
	GetTenantId() string
}

type accountContextGetter interface {
	GetAccount() *creditv1.AccountContext
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
	if getter, ok := request.(tenantIDGetter); ok {
		return strings.TrimSpace(getter.GetTenantId())
	}
	if getter, ok := request.(accountContextGetter); ok && getter.GetAccount() != nil {
		return strings.TrimSpace(getter.GetAccount().GetTenantId())
	}
	return ""
}

type tenantAuthenticator interface {
	Authenticate(context.Context, string) (tenant.ID, error)
}

func newAuthInterceptor(authenticator tenantAuthenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		rawTenantID := extractTenantID(request)
		if rawTenantID == "" {
			return nil, status.Error(codes.Unauthenticated, "missing tenant_id")
		}
		_, err := tenant.NewID(rawTenantID)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid tenant_id")
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
		authenticatedTenantID, err := authenticator.Authenticate(ctx, providedSecret)
		if err != nil {
			if errors.Is(err, tenant.ErrCredentialInvalid) || errors.Is(err, tenant.ErrCredentialRevoked) {
				return nil, status.Error(codes.Unauthenticated, "invalid secret key")
			}
			return nil, status.Error(codes.Internal, "tenant authentication failed")
		}

		return handler(tenant.WithAuthenticatedID(ctx, authenticatedTenantID), request)
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

func (logger *zapOperationLogger) LogControlRequest(entry controlplane.RequestLog) {
	fields := []zap.Field{
		zap.String("operation", entry.Operation),
		zap.Int("status_code", entry.StatusCode),
		zap.Duration("duration", entry.Duration),
	}
	if entry.UserAccountID != "" {
		fields = append(fields, zap.String("user_account_id", entry.UserAccountID))
	}
	if entry.ResourceID != "" {
		fields = append(fields, zap.String("resource_id", entry.ResourceID))
	}
	if entry.Error != nil {
		logger.logger.Error("control.request", append(fields, zap.Error(entry.Error))...)
		return
	}
	logger.logger.Info("control.request", fields...)
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
	if db == nil || db.Config == nil || db.Dialector == nil {
		return errors.New("database handle is invalid")
	}
	if db.Migrator().HasTable("accounts") {
		return errors.New("legacy accounts table requires the user-account migration")
	}
	if driver == "sqlite" {
		sqlDB, _ := db.DB()
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
	if err := db.AutoMigrate(
		&gormstore.UserAccount{},
		&gormstore.LedgerTenant{},
		&gormstore.TenantCredential{},
		&gormstore.IdempotencyRecord{},
		&gormstore.ControlEvent{},
		&gormstore.LedgerAccount{},
		&gormstore.LedgerEntry{},
		&gormstore.Reservation{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	return nil
}
