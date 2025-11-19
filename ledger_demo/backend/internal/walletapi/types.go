package walletapi

import "encoding/json"

// WalletEnvelope wraps wallet payloads returned by the API endpoints.
type WalletEnvelope struct {
	Wallet WalletPayload `json:"wallet"`
}

// WalletPayload describes the wallet balance and entry history.
type WalletPayload struct {
	Balance WalletBalance  `json:"balance"`
	Entries []EntryPayload `json:"entries"`
}

// WalletBalance normalizes cents/coins for the UI.
type WalletBalance struct {
	TotalCents     int64 `json:"total_cents"`
	AvailableCents int64 `json:"available_cents"`
	TotalCoins     int64 `json:"total_coins"`
	AvailableCoins int64 `json:"available_coins"`
}

// EntryPayload mirrors the ledger entry contract for the UI.
type EntryPayload struct {
	EntryID        string          `json:"entry_id"`
	Type           string          `json:"type"`
	AmountCents    int64           `json:"amount_cents"`
	AmountCoins    int64           `json:"amount_coins"`
	ReservationID  string          `json:"reservation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedUnixUTC int64           `json:"created_unix_utc"`
}

// TransactionEnvelope includes status plus the updated wallet payload.
type TransactionEnvelope struct {
	Status string        `json:"status"`
	Wallet WalletPayload `json:"wallet"`
}

// SessionEnvelope represents the session payload returned to the UI.
type SessionEnvelope struct {
	UserID  string   `json:"user_id"`
	Email   string   `json:"email"`
	Display string   `json:"display"`
	Avatar  string   `json:"avatar_url"`
	Roles   []string `json:"roles"`
	Expires int64    `json:"expires"`
}

// ErrorEnvelope encodes API errors.
type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

// ErrorPayload contains the code and message for user-visible errors.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
