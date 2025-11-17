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
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/glebarez/sqlite"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	flagDatabaseURL       = "database-url"
	flagListenAddr        = "listen-addr"
	configKeyDatabaseURL  = "database_url"
	configKeyListenAddr   = "listen_addr"
	defaultDatabaseURL    = "sqlite:///tmp/ledger.db"
	defaultGRPCListenAddr = ":7000"
)

type runtimeConfig struct {
	DatabaseURL string
	ListenAddr  string
}

func main() {
	cmd := newRootCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "creditd: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cfg := &runtimeConfig{}
	cmd := &cobra.Command{
		Use:           "creditd",
		Short:         "Credit ledger gRPC server",
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
	creditService, err := credit.NewService(store, clock)
	if err != nil {
		return fmt.Errorf("credit service init: %w", err)
	}

	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	grpcServer := grpc.NewServer()
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
	// Treat everything else as a direct sqlite path.
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
