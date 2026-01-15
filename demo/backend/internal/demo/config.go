package demo

import (
	"fmt"
	"strings"
	"time"
)

const (
	coinValueCents     int64 = 100
	transactionCoins   int64 = 5
	bootstrapCoins     int64 = 20
	minPurchaseCoins   int64 = 5
	purchaseStep       int64 = 5
	walletHistoryLimit       = 10
)

// Config aggregates runtime settings for the demo API.
type Config struct {
	ListenAddr        string
	LedgerAddress     string
	LedgerInsecure    bool
	LedgerTimeout     time.Duration
	DefaultTenantID   string
	DefaultLedgerID   string
	AllowedOrigins    []string
	SessionSigningKey string
	SessionIssuer     string
	SessionCookieName string
	TAuthBaseURL      string
}

// Validate ensures the configuration contains sane values.
func (cfg *Config) Validate() error {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return fmt.Errorf("listen addr is required")
	}
	if strings.TrimSpace(cfg.LedgerAddress) == "" {
		return fmt.Errorf("ledger address is required")
	}
	if cfg.LedgerTimeout <= 0 {
		return fmt.Errorf("ledger timeout must be greater than zero")
	}
	if strings.TrimSpace(cfg.DefaultTenantID) == "" {
		return fmt.Errorf("default tenant id is required")
	}
	if strings.TrimSpace(cfg.DefaultLedgerID) == "" {
		return fmt.Errorf("default ledger id is required")
	}
	if len(cfg.AllowedOrigins) == 0 {
		return fmt.Errorf("at least one allowed origin is required")
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
	if strings.TrimSpace(cfg.TAuthBaseURL) == "" {
		return fmt.Errorf("tauth base url is required")
	}
	return nil
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
