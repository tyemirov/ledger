package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/pgstore"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const (
	flagDatabaseURL       = "database-url"
	flagListenAddr        = "listen-addr"
	configKeyDatabaseURL  = "database_url"
	configKeyListenAddr   = "listen_addr"
	defaultDatabaseURL    = "postgres://postgres:postgres@localhost:5432/credit?sslmode=disable"
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

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pgx pool: %w", err)
	}
	defer pool.Close()

	store := pgstore.New(pool)
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
