package demo

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
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

	authClaims := &sessionvalidator.Claims{
		UserID:          "demo-user",
		UserEmail:       "demo@example.com",
		UserDisplayName: "Demo User",
	}

	bootstrapCtx, bootstrapRecorder := newTestContext(http.MethodPost, "/api/bootstrap", nil)
	bootstrapCtx.Set("auth_claims", authClaims)
	handler.handleBootstrap(bootstrapCtx)
	if bootstrapRecorder.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", bootstrapRecorder.Code, bootstrapRecorder.Body.String())
	}
	wallet := readWallet(t, handler, authClaims.GetUserID())
	if wallet.Balance.TotalCoins != 20 {
		t.Fatalf("expected 20 coins after bootstrap, got %d", wallet.Balance.TotalCoins)
	}

	for i := 0; i < 4; i++ {
		txCtx, txRecorder := newTestContext(http.MethodPost, "/api/transactions", map[string]any{"metadata": map[string]any{"source": "test"}})
		txCtx.Set("auth_claims", authClaims)
		handler.handleTransaction(txCtx)
		if txRecorder.Code != http.StatusOK {
			t.Fatalf("transaction %d status=%d body=%s", i, txRecorder.Code, txRecorder.Body.String())
		}
		wallet = readWallet(t, handler, authClaims.GetUserID())
		expectedCoins := 20 - 5*(i+1)
		if wallet.Balance.AvailableCoins != int64(expectedCoins) {
			t.Fatalf("expected %d coins after transaction %d, got %d", expectedCoins, i, wallet.Balance.AvailableCoins)
		}
	}

	insufficientCtx, insufficientRecorder := newTestContext(http.MethodPost, "/api/transactions", map[string]any{"metadata": map[string]any{"source": "test"}})
	insufficientCtx.Set("auth_claims", authClaims)
	handler.handleTransaction(insufficientCtx)
	if insufficientRecorder.Code != http.StatusOK {
		t.Fatalf("insufficient funds request status=%d body=%s", insufficientRecorder.Code, insufficientRecorder.Body.String())
	}
	wallet = readWallet(t, handler, authClaims.GetUserID())
	if wallet.Balance.AvailableCoins != 0 {
		t.Fatalf("expected balance to stay at 0 after insufficient funds attempt, got %d", wallet.Balance.AvailableCoins)
	}

	purchaseCtx, purchaseRecorder := newTestContext(http.MethodPost, "/api/purchases", map[string]any{"coins": 10, "metadata": map[string]any{"source": "test"}})
	purchaseCtx.Set("auth_claims", authClaims)
	handler.handlePurchase(purchaseCtx)
	if purchaseRecorder.Code != http.StatusOK {
		t.Fatalf("purchase status=%d body=%s", purchaseRecorder.Code, purchaseRecorder.Body.String())
	}
	wallet = readWallet(t, handler, authClaims.GetUserID())
	if wallet.Balance.AvailableCoins != 10 {
		t.Fatalf("expected 10 coins after purchase, got %d", wallet.Balance.AvailableCoins)
	}
	if len(wallet.Entries) == 0 {
		t.Fatalf("expected ledger entries to be populated")
	}
}

func readWallet(t *testing.T, handler *httpHandler, userID string) *walletResponse {
	t.Helper()
	wallet, err := handler.fetchWallet(context.Background(), userID)
	if err != nil {
		t.Fatalf("wallet fetch failed: %v", err)
	}
	return wallet
}

func startLedgerClient(t *testing.T) (creditv1.CreditServiceClient, func()) {
	t.Helper()
	listener := bufconn.Listen(bufconnSize)
	grpcServer := grpc.NewServer()
	creditv1.RegisterCreditServiceServer(grpcServer, newFakeLedgerServer())

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

type fakeLedgerServer struct {
	creditv1.UnimplementedCreditServiceServer
	balanceCents    int64
	entries         []*creditv1.Entry
	seenIdempotency map[string]struct{}
}

func newFakeLedgerServer() *fakeLedgerServer {
	return &fakeLedgerServer{
		seenIdempotency: make(map[string]struct{}),
		entries:         []*creditv1.Entry{},
	}
}

func (srv *fakeLedgerServer) Grant(_ context.Context, request *creditv1.GrantRequest) (*creditv1.Empty, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}
	srv.balanceCents += request.AmountCents
	srv.entries = append(srv.entries, &creditv1.Entry{
		AmountCents:    request.AmountCents,
		Type:           "grant",
		IdempotencyKey: request.IdempotencyKey,
		CreatedUnixUtc: time.Now().UTC().Unix(),
	})
	return &creditv1.Empty{}, nil
}

func (srv *fakeLedgerServer) Spend(_ context.Context, request *creditv1.SpendRequest) (*creditv1.Empty, error) {
	if srv.balanceCents < request.AmountCents {
		return nil, status.Error(codes.FailedPrecondition, "insufficient_funds")
	}
	srv.balanceCents -= request.AmountCents
	srv.entries = append(srv.entries, &creditv1.Entry{
		AmountCents:    -request.AmountCents,
		Type:           "spend",
		IdempotencyKey: request.IdempotencyKey,
		CreatedUnixUtc: time.Now().UTC().Unix(),
	})
	return &creditv1.Empty{}, nil
}

func (srv *fakeLedgerServer) GetBalance(context.Context, *creditv1.BalanceRequest) (*creditv1.BalanceResponse, error) {
	return &creditv1.BalanceResponse{
		TotalCents:     srv.balanceCents,
		AvailableCents: srv.balanceCents,
	}, nil
}

func (srv *fakeLedgerServer) ListEntries(context.Context, *creditv1.ListEntriesRequest) (*creditv1.ListEntriesResponse, error) {
	entries := make([]*creditv1.Entry, len(srv.entries))
	copy(entries, srv.entries)
	return &creditv1.ListEntriesResponse{Entries: entries}, nil
}

func (srv *fakeLedgerServer) Reserve(context.Context, *creditv1.ReserveRequest) (*creditv1.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "reserve not implemented")
}

func (srv *fakeLedgerServer) Capture(context.Context, *creditv1.CaptureRequest) (*creditv1.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "capture not implemented")
}

func (srv *fakeLedgerServer) Release(context.Context, *creditv1.ReleaseRequest) (*creditv1.Empty, error) {
	return nil, status.Error(codes.Unimplemented, "release not implemented")
}

type walletEnvelope struct {
	Wallet walletResponse `json:"wallet"`
}

type transactionEnvelope struct {
	Status string         `json:"status"`
	Wallet walletResponse `json:"wallet"`
}
