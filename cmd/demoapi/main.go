package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/demoapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	flagListenAddr     = "listen-addr"
	flagLedgerAddr     = "ledger-addr"
	flagLedgerInsecure = "ledger-insecure"
	flagLedgerTimeout  = "ledger-timeout"
	flagAllowedOrigins = "allowed-origins"
	flagJWTSigningKey  = "jwt-signing-key"
	flagJWTIssuer      = "jwt-issuer"
	flagJWTCookieName  = "jwt-cookie-name"
	flagTAuthBaseURL   = "tauth-base-url"
	envPrefix          = "DEMOAPI"
)

func main() {
	rootCmd := newRootCommand()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "demoapi: %v\n", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	cfg := demoapi.Config{}
	cmd := &cobra.Command{
		Use:           "demoapi",
		Short:         "HTTP façade for the credit demo",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return loadConfig(cmd, &cfg)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			return demoapi.Run(ctx, cfg)
		},
	}

	cmd.Flags().String(flagListenAddr, ":9090", "HTTP listen address")
	cmd.Flags().String(flagLedgerAddr, "localhost:7000", "creditd gRPC address")
	cmd.Flags().Bool(flagLedgerInsecure, true, "use insecure gRPC transport")
	cmd.Flags().Duration(flagLedgerTimeout, 3*time.Second, "ledger RPC timeout (e.g. 3s)")
	cmd.Flags().String(flagAllowedOrigins, "http://localhost:8000", "comma-separated list of allowed CORS origins")
	cmd.Flags().String(flagJWTSigningKey, "", "TAuth JWT signing key")
	cmd.Flags().String(flagJWTIssuer, "tauth", "expected JWT issuer")
	cmd.Flags().String(flagJWTCookieName, "app_session", "JWT cookie name")
	cmd.Flags().String(flagTAuthBaseURL, "http://localhost:8080", "base URL of TAuth (for documentation links)")

	return cmd
}

func loadConfig(cmd *cobra.Command, cfg *demoapi.Config) error {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	for _, flagName := range []string{flagListenAddr, flagLedgerAddr, flagLedgerInsecure, flagLedgerTimeout, flagAllowedOrigins, flagJWTSigningKey, flagJWTIssuer, flagJWTCookieName, flagTAuthBaseURL} {
		if err := v.BindPFlag(flagName, cmd.Flags().Lookup(flagName)); err != nil {
			return err
		}
	}

	cfg.ListenAddr = strings.TrimSpace(v.GetString(flagListenAddr))
	cfg.LedgerAddress = strings.TrimSpace(v.GetString(flagLedgerAddr))
	cfg.LedgerInsecure = v.GetBool(flagLedgerInsecure)
	cfg.LedgerTimeout = v.GetDuration(flagLedgerTimeout)
	cfg.AllowedOrigins = demoapi.ParseAllowedOrigins(v.GetString(flagAllowedOrigins))
	cfg.SessionSigningKey = v.GetString(flagJWTSigningKey)
	cfg.SessionIssuer = strings.TrimSpace(v.GetString(flagJWTIssuer))
	cfg.SessionCookieName = strings.TrimSpace(v.GetString(flagJWTCookieName))
	cfg.TAuthBaseURL = strings.TrimSpace(v.GetString(flagTAuthBaseURL))

	return cfg.Validate()
}
