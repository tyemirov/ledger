package demo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/gin-gonic/gin"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type stubLedgerClient struct {
	grantErr    error
	spendErr    error
	balanceErr  error
	entriesErr  error
	balanceResp *creditv1.BalanceResponse
	entriesResp *creditv1.ListEntriesResponse
}

func (stub *stubLedgerClient) Grant(ctx context.Context, request *creditv1.GrantRequest, options ...grpc.CallOption) (*creditv1.Empty, error) {
	if stub.grantErr != nil {
		return nil, stub.grantErr
	}
	return &creditv1.Empty{}, nil
}

func (stub *stubLedgerClient) Spend(ctx context.Context, request *creditv1.SpendRequest, options ...grpc.CallOption) (*creditv1.Empty, error) {
	if stub.spendErr != nil {
		return nil, stub.spendErr
	}
	return &creditv1.Empty{}, nil
}

func (stub *stubLedgerClient) GetBalance(ctx context.Context, request *creditv1.BalanceRequest, options ...grpc.CallOption) (*creditv1.BalanceResponse, error) {
	if stub.balanceErr != nil {
		return nil, stub.balanceErr
	}
	if stub.balanceResp != nil {
		return stub.balanceResp, nil
	}
	return &creditv1.BalanceResponse{}, nil
}

func (stub *stubLedgerClient) ListEntries(ctx context.Context, request *creditv1.ListEntriesRequest, options ...grpc.CallOption) (*creditv1.ListEntriesResponse, error) {
	if stub.entriesErr != nil {
		return nil, stub.entriesErr
	}
	if stub.entriesResp != nil {
		return stub.entriesResp, nil
	}
	return &creditv1.ListEntriesResponse{}, nil
}

func (stub *stubLedgerClient) Reserve(context.Context, *creditv1.ReserveRequest, ...grpc.CallOption) (*creditv1.Empty, error) {
	return nil, errUnimplemented
}

func (stub *stubLedgerClient) Capture(context.Context, *creditv1.CaptureRequest, ...grpc.CallOption) (*creditv1.Empty, error) {
	return nil, errUnimplemented
}

func (stub *stubLedgerClient) Release(context.Context, *creditv1.ReleaseRequest, ...grpc.CallOption) (*creditv1.Empty, error) {
	return nil, errUnimplemented
}

var errTestAssertion = errors.New("assertion_error")

func TestHandlePurchaseInvalidCoins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &httpHandler{
		logger:       zap.NewNop(),
		ledgerClient: &stubLedgerClient{},
		cfg: Config{
			LedgerTimeout:     time.Second,
			SessionSigningKey: "k",
			SessionIssuer:     "i",
			SessionCookieName: "c",
			ListenAddr:        ":0",
			LedgerAddress:     "ledger:50051",
			DefaultTenantID:   "default",
			DefaultLedgerID:   "default",
			AllowedOrigins:    []string{"http://localhost"},
			TAuthBaseURL:      "http://localhost:8080",
		},
	}

	ctx, recorder := newTestContext(http.MethodPost, "/api/purchases", map[string]any{"coins": 3})
	ctx.Set("auth_claims", &sessionvalidator.Claims{})

	handler.handlePurchase(ctx)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestHandleTransactionUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &httpHandler{
		logger:       zap.NewNop(),
		ledgerClient: &stubLedgerClient{},
		cfg:          Config{LedgerTimeout: time.Second},
	}

	ctx, recorder := newTestContext(http.MethodPost, "/api/transactions", nil)

	handler.handleTransaction(ctx)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestHandleWalletLedgerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &httpHandler{
		logger: zap.NewNop(),
		ledgerClient: &stubLedgerClient{
			balanceErr: errTestAssertion,
		},
		cfg: Config{
			LedgerTimeout:     time.Second,
			SessionSigningKey: "k",
			SessionIssuer:     "i",
			SessionCookieName: "c",
			ListenAddr:        ":0",
			LedgerAddress:     "ledger:50051",
			DefaultTenantID:   "default",
			DefaultLedgerID:   "default",
			AllowedOrigins:    []string{"http://localhost"},
			TAuthBaseURL:      "http://localhost:8080",
		},
	}

	ctx, recorder := newTestContext(http.MethodGet, "/api/wallet", nil)
	ctx.Set("auth_claims", &sessionvalidator.Claims{})

	handler.handleWallet(ctx)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", recorder.Code)
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	origins := ParseAllowedOrigins(" http://a.com , http://b.com ")
	if len(origins) != 2 || origins[0] != "http://a.com" || origins[1] != "http://b.com" {
		t.Fatalf("unexpected origins: %#v", origins)
	}
}

func TestConfigValidateMissingFields(t *testing.T) {
	cfg := Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func newTestContext(method, path string, payload map[string]any) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, payloadReader(payload))
	return ctx, recorder
}

func payloadReader(payload map[string]any) *bytes.Reader {
	if payload == nil {
		return bytes.NewReader(nil)
	}
	encoded, _ := json.Marshal(payload)
	return bytes.NewReader(encoded)
}
