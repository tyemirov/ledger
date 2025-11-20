package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

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

	cmd.Flags().String(flagListenAddr, "", "HTTP listen address (required)")
	cmd.Flags().String(flagLedgerAddr, "", "ledgerd gRPC address (required)")
	cmd.Flags().Bool(flagLedgerInsecure, false, "set true when connecting to an insecure ledger endpoint (required)")
	cmd.Flags().Duration(flagLedgerTimeout, 0, "ledger RPC timeout (e.g. 3s, required)")
	cmd.Flags().String(flagAllowedOrigins, "", "comma-separated list of allowed CORS origins (required)")
	cmd.Flags().String(flagJWTSigningKey, "", "TAuth JWT signing key (required)")
	cmd.Flags().String(flagJWTIssuer, "", "expected JWT issuer (required)")
	cmd.Flags().String(flagJWTCookieName, "", "JWT cookie name (required)")
	cmd.Flags().String(flagTAuthBaseURL, "", "base URL of TAuth for documentation/metadata (required)")

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

	if !v.IsSet(flagListenAddr) {
		return fmt.Errorf("%s is required", flagListenAddr)
	}
	if !v.IsSet(flagLedgerAddr) {
		return fmt.Errorf("%s is required", flagLedgerAddr)
	}
	if !v.IsSet(flagLedgerInsecure) {
		return fmt.Errorf("%s is required", flagLedgerInsecure)
	}
	if !v.IsSet(flagLedgerTimeout) {
		return fmt.Errorf("%s is required", flagLedgerTimeout)
	}
	if !v.IsSet(flagAllowedOrigins) {
		return fmt.Errorf("%s is required", flagAllowedOrigins)
	}
	if !v.IsSet(flagJWTSigningKey) {
		return fmt.Errorf("%s is required", flagJWTSigningKey)
	}
	if !v.IsSet(flagJWTIssuer) {
		return fmt.Errorf("%s is required", flagJWTIssuer)
	}
	if !v.IsSet(flagJWTCookieName) {
		return fmt.Errorf("%s is required", flagJWTCookieName)
	}
	if !v.IsSet(flagTAuthBaseURL) {
		return fmt.Errorf("%s is required", flagTAuthBaseURL)
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
