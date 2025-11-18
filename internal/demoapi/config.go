package demoapi

import (
	"fmt"
	"strings"
	"time"
)

const (
	defaultListenAddr          = ":9090"
	defaultLedgerAddr          = "localhost:7000"
	defaultAllowedOrigin       = "http://localhost:8000"
	defaultSessionIssuer       = "tauth"
	defaultSessionCookie       = "app_session"
	coinValueCents       int64 = 100
	transactionCoins     int64 = 5
	bootstrapCoins       int64 = 20
	minPurchaseCoins     int64 = 5
	purchaseStep         int64 = 5
	walletHistoryLimit         = 10
)

// Config aggregates runtime settings for the demo API.
type Config struct {
	ListenAddr        string
	LedgerAddress     string
	LedgerInsecure    bool
	LedgerTimeout     time.Duration
	AllowedOrigins    []string
	SessionSigningKey string
	SessionIssuer     string
	SessionCookieName string
	TAuthBaseURL      string
}

// Validate ensures the configuration contains sane values.
func (cfg *Config) Validate() error {
	cfg.ListenAddr = defaultIfEmpty(cfg.ListenAddr, defaultListenAddr)
	cfg.LedgerAddress = defaultIfEmpty(cfg.LedgerAddress, defaultLedgerAddr)
	if cfg.LedgerTimeout <= 0 {
		cfg.LedgerTimeout = 3 * time.Second
	}
	if len(cfg.AllowedOrigins) == 0 {
		cfg.AllowedOrigins = []string{defaultAllowedOrigin}
	}
	cfg.SessionIssuer = defaultIfEmpty(cfg.SessionIssuer, defaultSessionIssuer)
	cfg.SessionCookieName = defaultIfEmpty(cfg.SessionCookieName, defaultSessionCookie)
	cfg.TAuthBaseURL = defaultIfEmpty(cfg.TAuthBaseURL, "http://localhost:8080")
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return fmt.Errorf("listen addr is required")
	}
	if strings.TrimSpace(cfg.LedgerAddress) == "" {
		return fmt.Errorf("ledger address is required")
	}
	if len(cfg.SessionSigningKey) == 0 {
		return fmt.Errorf("jwt signing key is required")
	}
	if strings.TrimSpace(cfg.SessionIssuer) == "" {
		return fmt.Errorf("jwt issuer is required")
	}
	if strings.TrimSpace(cfg.SessionCookieName) == "" {
		return fmt.Errorf("jwt cookie name is required")
	}
	return nil
}

func defaultIfEmpty(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// ParseAllowedOrigins splits comma-delimited origins into a slice.
func ParseAllowedOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

// CoinValueCents exposes the cents-per-coin conversion.
func CoinValueCents() int64 {
	return coinValueCents
}

// TransactionAmountCents returns the per-transaction spend amount in cents.
func TransactionAmountCents() int64 {
	return transactionCoins * coinValueCents
}

// BootstrapAmountCents returns the default bootstrap amount in cents.
func BootstrapAmountCents() int64 {
	return bootstrapCoins * coinValueCents
}

// MinimumPurchaseCoins returns the minimum purchasable coins per request.
func MinimumPurchaseCoins() int64 {
	return minPurchaseCoins
}

// PurchaseIncrementCoins returns the purchase step size.
func PurchaseIncrementCoins() int64 {
	return purchaseStep
}

// WalletHistoryLimit returns how many entries are fetched for the UI.
func WalletHistoryLimit() int32 {
	return walletHistoryLimit
}
