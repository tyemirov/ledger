package walletapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	creditv1 "github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/ledger_demo/backend/internal/walletapi"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

const (
	healthPath              = "/healthz"
	sessionPath             = "/api/session"
	bootstrapPath           = "/api/bootstrap"
	walletPath              = "/api/wallet"
	transactionsPath        = "/api/transactions"
	purchasesPath           = "/api/purchases"
	contentTypeHeader       = "Content-Type"
	contentTypeJSON         = "application/json"
	metadataSourceKey       = "source"
	metadataSourceValue     = "integration_test"
	metadataCoinsKey        = "coins"
	spendStatusSuccess      = "success"
	spendStatusInsufficient = "insufficient_funds"
	sessionIssuer           = "tauth"
	sessionUserID           = "demo-user"
	sessionUserEmail        = "demo@example.com"
	sessionUserDisplayName  = "Demo User"
	sessionUserAvatarURL    = "https://example.com/avatar.png"
)

type integrationState struct {
	walletSnapshot walletapi.WalletEnvelope
}

func TestRun_WalletFlowIntegration(t *testing.T) {
	ledgerAddress, ledgerCleanup := startLedgerServer(t)
	defer ledgerCleanup()

	listenAddress := allocateListenAddress(t)
	configuration := walletapi.Config{
		ListenAddr:        listenAddress,
		LedgerAddress:     ledgerAddress,
		LedgerInsecure:    true,
		LedgerTimeout:     2 * time.Second,
		AllowedOrigins:    []string{"http://localhost:8000"},
		SessionSigningKey: "secret-key",
		SessionIssuer:     sessionIssuer,
		SessionCookieName: "app_session",
		TAuthBaseURL:      "http://localhost:8080",
	}

	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	runErrors := make(chan error, 1)
	go func() { runErrors <- walletapi.Run(runContext, configuration) }()

	waitForServerHealthy(t, configuration.ListenAddr)

	sessionCookie := buildSessionCookie(t, configuration)
	httpClient := &http.Client{Timeout: 2 * time.Second}
	baseURL := fmt.Sprintf("http://%s", configuration.ListenAddr)

	state := &integrationState{}
	testCases := []struct {
		name   string
		action func(*testing.T, *http.Client, string, *http.Cookie, *integrationState)
	}{
		{
			name: "bootstrap wallet",
			action: func(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie, state *integrationState) {
				walletEnvelope := executeWalletRequest(t, client, apiBaseURL, http.MethodPost, bootstrapPath, cookie, nil)
				expectedCoins := walletapi.BootstrapAmountCents() / walletapi.CoinValueCents()
				if walletEnvelope.Wallet.Balance.TotalCoins != expectedCoins {
					t.Fatalf("expected %d coins after bootstrap, received %d", expectedCoins, walletEnvelope.Wallet.Balance.TotalCoins)
				}
				state.walletSnapshot = walletEnvelope
			},
		},
		{
			name: "spend coins until insufficient funds",
			action: func(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie, state *integrationState) {
				for attemptIndex := 0; attemptIndex < 4; attemptIndex++ {
					transactionEnvelope := executeTransactionRequest(t, client, apiBaseURL, cookie)
					if transactionEnvelope.Status != spendStatusSuccess {
						t.Fatalf("expected success status, received %s", transactionEnvelope.Status)
					}
					state.walletSnapshot.Wallet = transactionEnvelope.Wallet
				}
				insufficientEnvelope := executeTransactionRequest(t, client, apiBaseURL, cookie)
				if insufficientEnvelope.Status != spendStatusInsufficient {
					t.Fatalf("expected insufficient funds status, received %s", insufficientEnvelope.Status)
				}
				state.walletSnapshot.Wallet = insufficientEnvelope.Wallet
			},
		},
		{
			name: "purchase coins to restore balance",
			action: func(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie, state *integrationState) {
				purchasePayload := map[string]any{
					metadataCoinsKey: int64(10),
					"metadata": map[string]any{
						metadataSourceKey: metadataSourceValue,
						metadataCoinsKey:  int64(10),
					},
				}
				walletEnvelope := executeWalletRequest(t, client, apiBaseURL, http.MethodPost, purchasesPath, cookie, purchasePayload)
				expectedCoins := int64(10)
				if walletEnvelope.Wallet.Balance.AvailableCoins != expectedCoins {
					t.Fatalf("expected %d coins after purchase, received %d", expectedCoins, walletEnvelope.Wallet.Balance.AvailableCoins)
				}
				state.walletSnapshot = walletEnvelope
			},
		},
		{
			name: "wallet endpoint returns history",
			action: func(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie, state *integrationState) {
				walletEnvelope := executeWalletRequest(t, client, apiBaseURL, http.MethodGet, walletPath, cookie, nil)
				if len(walletEnvelope.Wallet.Entries) == 0 {
					t.Fatalf("expected wallet entries to be populated")
				}
				if walletEnvelope.Wallet.Balance.AvailableCoins != state.walletSnapshot.Wallet.Balance.AvailableCoins {
					t.Fatalf("expected available coins to remain at %d, received %d", state.walletSnapshot.Wallet.Balance.AvailableCoins, walletEnvelope.Wallet.Balance.AvailableCoins)
				}
			},
		},
		{
			name: "session endpoint returns profile",
			action: func(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie, state *integrationState) {
				session := executeSessionRequest(t, client, apiBaseURL, cookie)
				if session.UserID != sessionUserID {
					t.Fatalf("expected session user %s, received %s", sessionUserID, session.UserID)
				}
				if session.Email != sessionUserEmail {
					t.Fatalf("expected session email %s, received %s", sessionUserEmail, session.Email)
				}
				if session.Expires == 0 {
					t.Fatalf("expected session expiry to be populated")
				}
			},
		},
	}

	t.Run("session endpoint rejects missing cookie", func(t *testing.T) {
		client := &http.Client{Timeout: 2 * time.Second}
		request, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s%s", configuration.ListenAddr, sessionPath), nil)
		if err != nil {
			t.Fatalf("session request init failed: %v", err)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("session request failed: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected unauthorized status, received %d", response.StatusCode)
		}
	})

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.action(t, httpClient, baseURL, sessionCookie, state)
		})
	}

	cancelRun()
	if err := <-runErrors; err != nil {
		t.Fatalf("walletapi run returned error: %v", err)
	}
}

func executeTransactionRequest(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie) walletapi.TransactionEnvelope {
	transactionMetadata := map[string]any{
		metadataSourceKey: metadataSourceValue,
		metadataCoinsKey:  walletapi.TransactionAmountCents() / walletapi.CoinValueCents(),
	}
	payload := map[string]any{"metadata": transactionMetadata}
	body := bytes.NewBuffer(mustJSONMarshal(t, payload))
	request, err := http.NewRequest(http.MethodPost, apiBaseURL+transactionsPath, body)
	if err != nil {
		t.Fatalf("transaction request init failed: %v", err)
	}
	request.Header.Set(contentTypeHeader, contentTypeJSON)
	request.AddCookie(cookie)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("transaction request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code for transaction: %d", response.StatusCode)
	}
	var envelope walletapi.TransactionEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("transaction response decode failed: %v", err)
	}
	return envelope
}

func executeWalletRequest(t *testing.T, client *http.Client, apiBaseURL string, method string, path string, cookie *http.Cookie, payload map[string]any) walletapi.WalletEnvelope {
	var requestBody *bytes.Reader
	if payload != nil {
		requestBody = bytes.NewReader(mustJSONMarshal(t, payload))
	} else {
		requestBody = bytes.NewReader(nil)
	}
	request, err := http.NewRequest(method, apiBaseURL+path, requestBody)
	if err != nil {
		t.Fatalf("request init failed for %s: %v", path, err)
	}
	if payload != nil {
		request.Header.Set(contentTypeHeader, contentTypeJSON)
	}
	request.AddCookie(cookie)

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("request failed for %s: %v", path, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code for %s: %d", path, response.StatusCode)
	}
	var envelope walletapi.WalletEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("response decode failed for %s: %v", path, err)
	}
	return envelope
}

func executeSessionRequest(t *testing.T, client *http.Client, apiBaseURL string, cookie *http.Cookie) walletapi.SessionEnvelope {
	request, err := http.NewRequest(http.MethodGet, apiBaseURL+sessionPath, nil)
	if err != nil {
		t.Fatalf("session request init failed: %v", err)
	}
	request.AddCookie(cookie)
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("session request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected session status: %d", response.StatusCode)
	}
	var envelope walletapi.SessionEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("session response decode failed: %v", err)
	}
	return envelope
}

func mustJSONMarshal(t *testing.T, payload map[string]any) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return raw
}

func waitForServerHealthy(t *testing.T, listenAddress string) {
	t.Helper()
	healthURL := fmt.Sprintf("http://%s%s", listenAddress, healthPath)
	httpClient := &http.Client{Timeout: 500 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := httpClient.Get(healthURL)
		if err == nil && response.StatusCode == http.StatusOK {
			response.Body.Close()
			return
		}
		if response != nil {
			response.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy at %s", healthURL)
}

func buildSessionCookie(t *testing.T, configuration walletapi.Config) *http.Cookie {
	claims := &sessionvalidator.Claims{
		UserID:          sessionUserID,
		UserEmail:       sessionUserEmail,
		UserDisplayName: sessionUserDisplayName,
		UserAvatarURL:   sessionUserAvatarURL,
		UserRoles:       []string{"member"},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    configuration.SessionIssuer,
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(configuration.SessionSigningKey))
	if err != nil {
		t.Fatalf("token signing failed: %v", err)
	}
	return &http.Cookie{Name: configuration.SessionCookieName, Value: signedToken}
}

func startLedgerServer(t *testing.T) (string, func()) {
	t.Helper()

	database, err := gorm.Open(sqlite.Open(t.TempDir()+"/ledger.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open failed: %v", err)
	}
	if err := database.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		t.Fatalf("automigrate failed: %v", err)
	}
	store := gormstore.New(database)
	currentTime := func() int64 { return time.Now().UTC().Unix() }
	service, err := credit.NewService(store, currentTime)
	if err != nil {
		t.Fatalf("credit service init failed: %v", err)
	}

	grpcServer := grpc.NewServer()
	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(service))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ledger listener init failed: %v", err)
	}
	go func() {
		if serveErr := grpcServer.Serve(listener); serveErr != nil {
			t.Logf("gRPC server error: %v", serveErr)
		}
	}()

	cleanup := func() {
		grpcServer.Stop()
		_ = listener.Close()
	}
	return listener.Addr().String(), cleanup
}

func allocateListenAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen address allocation failed: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}
