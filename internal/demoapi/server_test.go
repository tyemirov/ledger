package demoapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

const bufconnSize = 1 << 20

func TestDemoAPITransactionsAndPurchases(t *testing.T) {
	ledgerClient, cleanup := startLedgerClient(t)
	defer cleanup()

	cfg := Config{
		ListenAddr:        ":0",
		LedgerAddress:     "bufnet",
		LedgerInsecure:    true,
		LedgerTimeout:     2 * time.Second,
		AllowedOrigins:    []string{"http://localhost:8000"},
		SessionSigningKey: "secret-key",
		SessionIssuer:     "tauth",
		SessionCookieName: "app_session",
		TAuthBaseURL:      "http://localhost:8080",
	}

	handler := &httpHandler{
		logger:       zap.NewNop(),
		ledgerClient: ledgerClient,
		cfg:          cfg,
	}

	validator, err := sessionvalidator.New(sessionvalidator.Config{
		SigningKey: []byte(cfg.SessionSigningKey),
		Issuer:     cfg.SessionIssuer,
		CookieName: cfg.SessionCookieName,
	})
	if err != nil {
		t.Fatalf("validator init failed: %v", err)
	}

	router := setupRouter(cfg, handler, validator)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	cookie := buildSessionCookie(t, cfg)

	// Bootstrap wallet (20 coins)
	walletEnvelope := execRequest(t, server, http.MethodPost, "/api/bootstrap", cookie, nil)
	if walletEnvelope.Wallet.Balance.TotalCoins != 20 {
		t.Fatalf("expected 20 coins after bootstrap, got %d", walletEnvelope.Wallet.Balance.TotalCoins)
	}

	// Spend coins four times successfully (brings balance to zero)
	for i := 0; i < 4; i++ {
		txEnvelope := execTransactionRequest(t, server, cookie)
		if txEnvelope.Status != "success" {
			t.Fatalf("expected success status, got %s", txEnvelope.Status)
		}
	}

	// Fifth attempt should be rejected with insufficient funds
	insufficient := execTransactionRequest(t, server, cookie)
	if insufficient.Status != "insufficient_funds" {
		t.Fatalf("expected insufficient funds status, got %s", insufficient.Status)
	}

	// Purchase 10 coins
	purchasePayload := map[string]any{"coins": 10, "metadata": map[string]any{"source": "test"}}
	walletAfterPurchase := execRequest(t, server, http.MethodPost, "/api/purchases", cookie, purchasePayload)
	if walletAfterPurchase.Wallet.Balance.AvailableCoins != 10 {
		t.Fatalf("expected 10 coins after purchase, got %d", walletAfterPurchase.Wallet.Balance.AvailableCoins)
	}

	// Fetch wallet to ensure entries exist
	walletFinal := execRequest(t, server, http.MethodGet, "/api/wallet", cookie, nil)
	if len(walletFinal.Wallet.Entries) == 0 {
		t.Fatalf("expected ledger entries to be populated")
	}
}

func execTransactionRequest(t *testing.T, server *httptest.Server, cookie *http.Cookie) transactionEnvelope {
	payload := map[string]any{"metadata": map[string]any{"source": "test", "coins": TransactionAmountCents() / CoinValueCents()}}
	body := bytes.NewBuffer(mustJSONMarshal(t, payload))
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/transactions", body)
	if err != nil {
		t.Fatalf("request init failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
	var envelope transactionEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return envelope
}

func execRequest(t *testing.T, server *httptest.Server, method, path string, cookie *http.Cookie, payload map[string]any) walletEnvelope {
	var body *bytes.Reader
	if payload != nil {
		body = bytes.NewReader(mustJSONMarshal(t, payload))
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, server.URL+path, body)
	if err != nil {
		t.Fatalf("request init failed: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(cookie)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: %d", resp.StatusCode)
	}
	var envelope walletEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return envelope
}

func startLedgerClient(t *testing.T) (creditv1.CreditServiceClient, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ledger.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open failed: %v", err)
	}
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}
	store := gormstore.New(db)
	clock := func() int64 { return time.Now().UTC().Unix() }
	service, err := credit.NewService(store, clock)
	if err != nil {
		t.Fatalf("ledger service init failed: %v", err)
	}

	listener := bufconn.Listen(bufconnSize)
	grpcServer := grpc.NewServer()
	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(service))

	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			t.Logf("gRPC server error: %v", serveErr)
		}
	}()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("gRPC client init failed: %v", err)
	}
	conn.Connect()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	if err := waitForClientReady(waitCtx, conn); err != nil {
		t.Fatalf("gRPC client failed to connect: %v", err)
	}

	cleanup := func() {
		grpcServer.Stop()
		_ = conn.Close()
	}
	return creditv1.NewCreditServiceClient(conn), cleanup
}

func buildSessionCookie(t *testing.T, cfg Config) *http.Cookie {
	claims := &sessionvalidator.Claims{
		UserID:          "demo-user",
		UserEmail:       "demo@example.com",
		UserDisplayName: "Demo",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.SessionIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.SessionSigningKey))
	if err != nil {
		t.Fatalf("token signing failed: %v", err)
	}
	return &http.Cookie{Name: cfg.SessionCookieName, Value: signed}
}

func mustJSONMarshal(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return raw
}

type walletEnvelope struct {
	Wallet walletResponse `json:"wallet"`
}

type transactionEnvelope struct {
	Status string         `json:"status"`
	Wallet walletResponse `json:"wallet"`
}
