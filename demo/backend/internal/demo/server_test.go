package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/gin-gonic/gin"
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
	ledgerClient, fakeServer, cleanup := startLedgerClient(t)
	defer cleanup()

	cfg := Config{
		ListenAddr:        ":0",
		LedgerAddress:     "bufnet",
		LedgerInsecure:    true,
		LedgerTimeout:     2 * time.Second,
		DefaultTenantID:   "default",
		DefaultLedgerID:   "default",
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
	// second bootstrap should no-op without hitting Grant again.
	secondCtx, secondRecorder := newTestContext(http.MethodPost, "/api/bootstrap", nil)
	secondCtx.Set("auth_claims", authClaims)
	handler.handleBootstrap(secondCtx)
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("second bootstrap status=%d body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if fakeServer.grantCalls != 1 {
		t.Fatalf("expected exactly one grant call, got %d", fakeServer.grantCalls)
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

func TestDemoAPIReservationsRefundsBatch(t *testing.T) {
	ledgerClient, fakeServer, cleanup := startLedgerClient(t)
	defer cleanup()

	cfg := Config{
		ListenAddr:        ":0",
		LedgerAddress:     "bufnet",
		LedgerInsecure:    true,
		LedgerTimeout:     2 * time.Second,
		DefaultTenantID:   "default",
		DefaultLedgerID:   "default",
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
	if fakeServer.grantCalls != 1 {
		t.Fatalf("expected exactly one grant call, got %d", fakeServer.grantCalls)
	}

	reserveCtx, reserveRecorder := newTestContext(http.MethodPost, "/api/reservations", map[string]any{"coins": 5, "ttl_seconds": 60})
	reserveCtx.Set("auth_claims", authClaims)
	handler.handleReserve(reserveCtx)
	if reserveRecorder.Code != http.StatusOK {
		t.Fatalf("reserve status=%d body=%s", reserveRecorder.Code, reserveRecorder.Body.String())
	}
	if reserveRecorder.Body.Len() == 0 {
		t.Fatalf("reserve returned empty body (ctx errors=%v)", reserveCtx.Errors)
	}
	var reserveResp reservationEnvelope
	if err := json.Unmarshal(reserveRecorder.Body.Bytes(), &reserveResp); err != nil {
		t.Fatalf("reserve json decode: %v body=%q", err, reserveRecorder.Body.String())
	}
	if reserveResp.Status != "reserved" || reserveResp.Reservation == nil || reserveResp.Reservation.ReservationID == "" {
		t.Fatalf("unexpected reserve response: %#v", reserveResp)
	}
	if reserveResp.Reservation.Status != "active" {
		t.Fatalf("expected active reservation, got %q", reserveResp.Reservation.Status)
	}
	if reserveResp.Wallet == nil || reserveResp.Wallet.Balance.TotalCoins != 20 || reserveResp.Wallet.Balance.AvailableCoins != 15 {
		t.Fatalf("unexpected wallet after reserve: %#v", reserveResp.Wallet)
	}

	reservationID := reserveResp.Reservation.ReservationID

	captureCtx, captureRecorder := newTestContext(http.MethodPost, fmt.Sprintf("/api/reservations/%s/capture", reservationID), nil)
	captureCtx.Set("auth_claims", authClaims)
	captureCtx.Params = gin.Params{{Key: "reservationId", Value: reservationID}}
	handler.handleCaptureReservation(captureCtx)
	if captureRecorder.Code != http.StatusOK {
		t.Fatalf("capture status=%d body=%s", captureRecorder.Code, captureRecorder.Body.String())
	}
	var captureResp reservationEnvelope
	if err := json.Unmarshal(captureRecorder.Body.Bytes(), &captureResp); err != nil {
		t.Fatalf("capture json decode: %v", err)
	}
	if captureResp.Status != "captured" || captureResp.Reservation == nil || captureResp.Reservation.Status != "captured" {
		t.Fatalf("unexpected capture response: %#v", captureResp)
	}
	if captureResp.Wallet == nil || captureResp.Wallet.Balance.TotalCoins != 15 || captureResp.Wallet.Balance.AvailableCoins != 15 {
		t.Fatalf("unexpected wallet after capture: %#v", captureResp.Wallet)
	}

	var capturedDebitEntryID string
	for _, entry := range captureResp.Wallet.Entries {
		if entry.Type == "spend" && entry.AmountCents == -500 && entry.ReservationID == reservationID {
			capturedDebitEntryID = entry.EntryID
		}
	}
	if capturedDebitEntryID == "" {
		t.Fatalf("expected captured spend entry with entry_id")
	}

	refundCtx, refundRecorder := newTestContext(http.MethodPost, "/api/refunds", map[string]any{
		"original_entry_id": capturedDebitEntryID,
		"amount_coins":      5,
	})
	refundCtx.Set("auth_claims", authClaims)
	handler.handleRefund(refundCtx)
	if refundRecorder.Code != http.StatusOK {
		t.Fatalf("refund status=%d body=%s", refundRecorder.Code, refundRecorder.Body.String())
	}
	var refundResp refundEnvelope
	if err := json.Unmarshal(refundRecorder.Body.Bytes(), &refundResp); err != nil {
		t.Fatalf("refund json decode: %v", err)
	}
	if refundResp.Status != "refunded" || refundResp.RefundEntryID == "" || refundResp.Wallet == nil {
		t.Fatalf("unexpected refund response: %#v", refundResp)
	}
	if refundResp.Wallet.Balance.TotalCoins != 20 || refundResp.Wallet.Balance.AvailableCoins != 20 {
		t.Fatalf("unexpected wallet after refund: %#v", refundResp.Wallet)
	}

	type batchEnvelope struct {
		Batch  batchSummary    `json:"batch"`
		Wallet *walletResponse `json:"wallet"`
	}

	batchSpendCtx, batchSpendRecorder := newTestContext(http.MethodPost, "/api/batch/spend", map[string]any{
		"count":  3,
		"coins":  5,
		"atomic": true,
	})
	batchSpendCtx.Set("auth_claims", authClaims)
	handler.handleBatchSpend(batchSpendCtx)
	if batchSpendRecorder.Code != http.StatusOK {
		t.Fatalf("batch spend status=%d body=%s", batchSpendRecorder.Code, batchSpendRecorder.Body.String())
	}
	var batchSpendResp batchEnvelope
	if err := json.Unmarshal(batchSpendRecorder.Body.Bytes(), &batchSpendResp); err != nil {
		t.Fatalf("batch spend json decode: %v", err)
	}
	if batchSpendResp.Batch.Ok != 3 || batchSpendResp.Batch.Failed != 0 {
		t.Fatalf("unexpected batch spend summary: %#v", batchSpendResp.Batch)
	}
	if batchSpendResp.Wallet == nil || batchSpendResp.Wallet.Balance.TotalCoins != 5 || batchSpendResp.Wallet.Balance.AvailableCoins != 5 {
		t.Fatalf("unexpected wallet after batch spend: %#v", batchSpendResp.Wallet)
	}

	atomicFailCtx, atomicFailRecorder := newTestContext(http.MethodPost, "/api/batch/spend", map[string]any{
		"count":  2,
		"coins":  5,
		"atomic": true,
	})
	atomicFailCtx.Set("auth_claims", authClaims)
	handler.handleBatchSpend(atomicFailCtx)
	if atomicFailRecorder.Code != http.StatusOK {
		t.Fatalf("atomic batch spend status=%d body=%s", atomicFailRecorder.Code, atomicFailRecorder.Body.String())
	}
	var atomicFailResp batchEnvelope
	if err := json.Unmarshal(atomicFailRecorder.Body.Bytes(), &atomicFailResp); err != nil {
		t.Fatalf("atomic batch spend json decode: %v", err)
	}
	if atomicFailResp.Batch.Ok != 0 || atomicFailResp.Batch.Failed != 2 {
		t.Fatalf("unexpected atomic batch spend summary: %#v", atomicFailResp.Batch)
	}
	if atomicFailResp.Wallet == nil || atomicFailResp.Wallet.Balance.TotalCoins != 5 || atomicFailResp.Wallet.Balance.AvailableCoins != 5 {
		t.Fatalf("unexpected wallet after atomic abort: %#v", atomicFailResp.Wallet)
	}

	bestEffortCtx, bestEffortRecorder := newTestContext(http.MethodPost, "/api/batch/spend", map[string]any{
		"count":  2,
		"coins":  5,
		"atomic": false,
	})
	bestEffortCtx.Set("auth_claims", authClaims)
	handler.handleBatchSpend(bestEffortCtx)
	if bestEffortRecorder.Code != http.StatusOK {
		t.Fatalf("best-effort batch spend status=%d body=%s", bestEffortRecorder.Code, bestEffortRecorder.Body.String())
	}
	var bestEffortResp batchEnvelope
	if err := json.Unmarshal(bestEffortRecorder.Body.Bytes(), &bestEffortResp); err != nil {
		t.Fatalf("best-effort batch spend json decode: %v", err)
	}
	if bestEffortResp.Batch.Ok != 1 || bestEffortResp.Batch.Failed != 1 {
		t.Fatalf("unexpected best-effort batch spend summary: %#v", bestEffortResp.Batch)
	}
	if bestEffortResp.Wallet == nil || bestEffortResp.Wallet.Balance.TotalCoins != 0 || bestEffortResp.Wallet.Balance.AvailableCoins != 0 {
		t.Fatalf("unexpected wallet after best-effort: %#v", bestEffortResp.Wallet)
	}

	var lastSpendEntryID string
	for i := len(bestEffortResp.Wallet.Entries) - 1; i >= 0; i-- {
		entry := bestEffortResp.Wallet.Entries[i]
		if entry.Type == "spend" && entry.AmountCents == -500 {
			lastSpendEntryID = entry.EntryID
			break
		}
	}
	if lastSpendEntryID == "" {
		t.Fatalf("expected spend entry_id after batch spend")
	}

	batchRefundCtx, batchRefundRecorder := newTestContext(http.MethodPost, "/api/batch/refund", map[string]any{
		"items": []map[string]any{
			{"original_entry_id": lastSpendEntryID, "amount_cents": int64(500)},
		},
		"atomic": true,
	})
	batchRefundCtx.Set("auth_claims", authClaims)
	handler.handleBatchRefund(batchRefundCtx)
	if batchRefundRecorder.Code != http.StatusOK {
		t.Fatalf("batch refund status=%d body=%s", batchRefundRecorder.Code, batchRefundRecorder.Body.String())
	}
	var batchRefundResp batchEnvelope
	if err := json.Unmarshal(batchRefundRecorder.Body.Bytes(), &batchRefundResp); err != nil {
		t.Fatalf("batch refund json decode: %v", err)
	}
	if batchRefundResp.Batch.Ok != 1 || batchRefundResp.Batch.Failed != 0 {
		t.Fatalf("unexpected batch refund summary: %#v", batchRefundResp.Batch)
	}
	if batchRefundResp.Wallet == nil || batchRefundResp.Wallet.Balance.TotalCoins != 5 || batchRefundResp.Wallet.Balance.AvailableCoins != 5 {
		t.Fatalf("unexpected wallet after batch refund: %#v", batchRefundResp.Wallet)
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

func startLedgerClient(t *testing.T) (creditv1.CreditServiceClient, *fakeLedgerServer, func()) {
	t.Helper()
	listener := bufconn.Listen(bufconnSize)
	grpcServer := grpc.NewServer()
	fakeServer := newFakeLedgerServer()
	creditv1.RegisterCreditServiceServer(grpcServer, fakeServer)

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
	return creditv1.NewCreditServiceClient(conn), fakeServer, cleanup
}

type fakeLedgerServer struct {
	creditv1.UnimplementedCreditServiceServer
	balanceCents    int64
	heldCents       int64
	entries         []*creditv1.Entry
	reservations    map[string]*creditv1.Reservation
	seenIdempotency map[string]struct{}
	grantCalls      int
	nextEntryID     int64
}

func newFakeLedgerServer() *fakeLedgerServer {
	return &fakeLedgerServer{
		seenIdempotency: make(map[string]struct{}),
		entries:         []*creditv1.Entry{},
		reservations:    make(map[string]*creditv1.Reservation),
	}
}

func (srv *fakeLedgerServer) newEntryID() string {
	srv.nextEntryID++
	return fmt.Sprintf("entry-%d", srv.nextEntryID)
}

func (srv *fakeLedgerServer) Grant(_ context.Context, request *creditv1.GrantRequest) (*creditv1.Empty, error) {
	srv.grantCalls++
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}
	srv.balanceCents += request.AmountCents
	created := time.Now().UTC().Unix()
	entryID := srv.newEntryID()
	srv.entries = append(srv.entries, &creditv1.Entry{
		EntryId:        entryID,
		AmountCents:    request.AmountCents,
		Type:           "grant",
		IdempotencyKey: request.IdempotencyKey,
		CreatedUnixUtc: created,
	})
	return &creditv1.Empty{EntryId: entryID, CreatedUnixUtc: created}, nil
}

func (srv *fakeLedgerServer) Spend(_ context.Context, request *creditv1.SpendRequest) (*creditv1.Empty, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	available := srv.balanceCents - srv.heldCents
	if available < request.AmountCents {
		return nil, status.Error(codes.FailedPrecondition, "insufficient_funds")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}
	srv.balanceCents -= request.AmountCents
	created := time.Now().UTC().Unix()
	entryID := srv.newEntryID()
	srv.entries = append(srv.entries, &creditv1.Entry{
		EntryId:        entryID,
		AmountCents:    -request.AmountCents,
		Type:           "spend",
		IdempotencyKey: request.IdempotencyKey,
		CreatedUnixUtc: created,
	})
	return &creditv1.Empty{EntryId: entryID, CreatedUnixUtc: created}, nil
}

func (srv *fakeLedgerServer) GetBalance(context.Context, *creditv1.BalanceRequest) (*creditv1.BalanceResponse, error) {
	return &creditv1.BalanceResponse{
		TotalCents:     srv.balanceCents,
		AvailableCents: srv.balanceCents - srv.heldCents,
	}, nil
}

func (srv *fakeLedgerServer) ListEntries(context.Context, *creditv1.ListEntriesRequest) (*creditv1.ListEntriesResponse, error) {
	entries := make([]*creditv1.Entry, len(srv.entries))
	copy(entries, srv.entries)
	return &creditv1.ListEntriesResponse{Entries: entries}, nil
}

func (srv *fakeLedgerServer) Reserve(_ context.Context, request *creditv1.ReserveRequest) (*creditv1.Empty, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	available := srv.balanceCents - srv.heldCents
	if available < request.AmountCents {
		return nil, status.Error(codes.FailedPrecondition, "insufficient_funds")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}
	now := time.Now().UTC().Unix()
	srv.heldCents += request.AmountCents
	srv.reservations[request.ReservationId] = &creditv1.Reservation{
		ReservationId:    request.ReservationId,
		AmountCents:      request.AmountCents,
		Status:           "active",
		ExpiresAtUnixUtc: request.ExpiresAtUnixUtc,
		CreatedUnixUtc:   now,
		UpdatedUnixUtc:   now,
		Expired:          false,
		HeldCents:        request.AmountCents,
		CapturedCents:    0,
	}
	return &creditv1.Empty{CreatedUnixUtc: now}, nil
}

func (srv *fakeLedgerServer) Capture(_ context.Context, request *creditv1.CaptureRequest) (*creditv1.Empty, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	reservation, ok := srv.reservations[request.ReservationId]
	if !ok {
		return nil, status.Error(codes.NotFound, "unknown_reservation")
	}
	if reservation.GetStatus() != "active" {
		return nil, status.Error(codes.FailedPrecondition, "reservation_closed")
	}
	if request.AmountCents <= 0 || request.AmountCents > reservation.GetHeldCents() {
		return nil, status.Error(codes.InvalidArgument, "invalid_capture_amount")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}

	now := time.Now().UTC().Unix()
	reservation.HeldCents -= request.AmountCents
	reservation.CapturedCents += request.AmountCents
	reservation.UpdatedUnixUtc = now
	if reservation.HeldCents == 0 {
		reservation.Status = "captured"
	}

	srv.heldCents -= request.AmountCents
	srv.balanceCents -= request.AmountCents

	entryID := srv.newEntryID()
	srv.entries = append(srv.entries, &creditv1.Entry{
		EntryId:        entryID,
		AmountCents:    -request.AmountCents,
		Type:           "spend",
		ReservationId:  request.ReservationId,
		IdempotencyKey: request.IdempotencyKey,
		CreatedUnixUtc: now,
	})
	return &creditv1.Empty{EntryId: entryID, CreatedUnixUtc: now}, nil
}

func (srv *fakeLedgerServer) Release(_ context.Context, request *creditv1.ReleaseRequest) (*creditv1.Empty, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	reservation, ok := srv.reservations[request.ReservationId]
	if !ok {
		return nil, status.Error(codes.NotFound, "unknown_reservation")
	}
	if reservation.GetStatus() != "active" {
		return nil, status.Error(codes.FailedPrecondition, "reservation_closed")
	}
	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}

	now := time.Now().UTC().Unix()
	srv.heldCents -= reservation.GetHeldCents()
	reservation.HeldCents = 0
	reservation.Status = "released"
	reservation.UpdatedUnixUtc = now
	return &creditv1.Empty{CreatedUnixUtc: now}, nil
}

func (srv *fakeLedgerServer) GetReservation(_ context.Context, request *creditv1.GetReservationRequest) (*creditv1.GetReservationResponse, error) {
	reservation, ok := srv.reservations[request.ReservationId]
	if !ok {
		return nil, status.Error(codes.NotFound, "unknown_reservation")
	}
	copyReservation := *reservation
	now := time.Now().UTC().Unix()
	if copyReservation.GetStatus() == "active" && copyReservation.GetExpiresAtUnixUtc() > 0 && now > copyReservation.GetExpiresAtUnixUtc() {
		copyReservation.Expired = true
	} else {
		copyReservation.Expired = false
	}
	return &creditv1.GetReservationResponse{Reservation: &copyReservation}, nil
}

func (srv *fakeLedgerServer) ListReservations(_ context.Context, request *creditv1.ListReservationsRequest) (*creditv1.ListReservationsResponse, error) {
	filter := make(map[string]struct{}, len(request.GetStatuses()))
	for _, statusValue := range request.GetStatuses() {
		filter[statusValue] = struct{}{}
	}

	before := request.GetBeforeCreatedUnixUtc()
	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = 25
	}

	reservations := make([]*creditv1.Reservation, 0, len(srv.reservations))
	for _, reservation := range srv.reservations {
		if before > 0 && reservation.GetCreatedUnixUtc() >= before {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[reservation.GetStatus()]; !ok {
				continue
			}
		}
		copyReservation := *reservation
		reservations = append(reservations, &copyReservation)
		if len(reservations) >= limit {
			break
		}
	}
	return &creditv1.ListReservationsResponse{Reservations: reservations}, nil
}

func (srv *fakeLedgerServer) Refund(_ context.Context, request *creditv1.RefundRequest) (*creditv1.RefundResponse, error) {
	if _, exists := srv.seenIdempotency[request.IdempotencyKey]; exists {
		return nil, status.Error(codes.AlreadyExists, "duplicate")
	}
	original := srv.resolveOriginalEntry(request.GetOriginalEntryId(), request.GetOriginalIdempotencyKey())
	if original == nil || original.GetAmountCents() >= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid_refund_original")
	}
	refundedCents := srv.refundedCentsForOriginal(original.GetEntryId())
	remainingCents := -original.GetAmountCents() - refundedCents
	if request.AmountCents > remainingCents {
		return nil, status.Error(codes.FailedPrecondition, "refund_exceeds_debit")
	}

	srv.seenIdempotency[request.IdempotencyKey] = struct{}{}
	srv.balanceCents += request.AmountCents

	now := time.Now().UTC().Unix()
	entryID := srv.newEntryID()
	srv.entries = append(srv.entries, &creditv1.Entry{
		EntryId:         entryID,
		AmountCents:     request.AmountCents,
		Type:            "refund",
		IdempotencyKey:  request.IdempotencyKey,
		RefundOfEntryId: original.GetEntryId(),
		CreatedUnixUtc:  now,
	})

	return &creditv1.RefundResponse{EntryId: entryID, CreatedUnixUtc: now}, nil
}

type fakeLedgerSnapshot struct {
	balanceCents    int64
	heldCents       int64
	entries         []*creditv1.Entry
	reservations    map[string]*creditv1.Reservation
	seenIdempotency map[string]struct{}
	nextEntryID     int64
}

func (srv *fakeLedgerServer) snapshot() fakeLedgerSnapshot {
	entries := make([]*creditv1.Entry, len(srv.entries))
	for i, entry := range srv.entries {
		copyEntry := *entry
		entries[i] = &copyEntry
	}
	reservations := make(map[string]*creditv1.Reservation, len(srv.reservations))
	for id, reservation := range srv.reservations {
		copyReservation := *reservation
		reservations[id] = &copyReservation
	}
	seen := make(map[string]struct{}, len(srv.seenIdempotency))
	for key := range srv.seenIdempotency {
		seen[key] = struct{}{}
	}
	return fakeLedgerSnapshot{
		balanceCents:    srv.balanceCents,
		heldCents:       srv.heldCents,
		entries:         entries,
		reservations:    reservations,
		seenIdempotency: seen,
		nextEntryID:     srv.nextEntryID,
	}
}

func (srv *fakeLedgerServer) restore(snapshot fakeLedgerSnapshot) {
	srv.balanceCents = snapshot.balanceCents
	srv.heldCents = snapshot.heldCents
	srv.entries = snapshot.entries
	srv.reservations = snapshot.reservations
	srv.seenIdempotency = snapshot.seenIdempotency
	srv.nextEntryID = snapshot.nextEntryID
}

func (srv *fakeLedgerServer) Batch(_ context.Context, request *creditv1.BatchRequest) (*creditv1.BatchResponse, error) {
	operations := request.GetOperations()
	results := make([]*creditv1.BatchOperationResult, 0, len(operations))

	if request.GetAccount() == nil {
		return nil, status.Error(codes.InvalidArgument, "missing_account")
	}

	if request.GetAtomic() {
		snapshot := srv.snapshot()
		failIndex := -1
		for i, operation := range operations {
			result := srv.applyBatchOperation(operation)
			results = append(results, result)
			if !result.GetOk() {
				failIndex = i
				break
			}
		}

		if failIndex >= 0 {
			for i := len(results); i < len(operations); i++ {
				results = append(results, &creditv1.BatchOperationResult{
					OperationId:  operations[i].GetOperationId(),
					Ok:           false,
					ErrorCode:    "atomic_aborted",
					ErrorMessage: "atomic_aborted",
				})
			}
			srv.restore(snapshot)

			for i := 0; i < len(results); i++ {
				if i == failIndex {
					continue
				}
				if results[i].GetDuplicate() {
					continue
				}
				results[i].Ok = false
				results[i].ErrorCode = "atomic_aborted"
				results[i].ErrorMessage = "atomic_aborted"
				results[i].EntryId = ""
				results[i].CreatedUnixUtc = 0
			}
		}

		return &creditv1.BatchResponse{Results: results}, nil
	}

	for _, operation := range operations {
		results = append(results, srv.applyBatchOperation(operation))
	}

	return &creditv1.BatchResponse{Results: results}, nil
}

func (srv *fakeLedgerServer) applyBatchOperation(operation *creditv1.BatchOperation) *creditv1.BatchOperationResult {
	result := &creditv1.BatchOperationResult{OperationId: operation.GetOperationId()}

	switch typed := operation.GetOperation().(type) {
	case *creditv1.BatchOperation_Spend:
		payload := typed.Spend
		if payload == nil {
			result.Ok = false
			result.ErrorCode = "invalid_operation"
			result.ErrorMessage = "invalid_operation"
			return result
		}
		if _, exists := srv.seenIdempotency[payload.GetIdempotencyKey()]; exists {
			result.Ok = true
			result.Duplicate = true
			return result
		}
		available := srv.balanceCents - srv.heldCents
		if available < payload.GetAmountCents() {
			result.Ok = false
			result.ErrorCode = "insufficient_funds"
			result.ErrorMessage = "insufficient_funds"
			return result
		}

		srv.seenIdempotency[payload.GetIdempotencyKey()] = struct{}{}
		srv.balanceCents -= payload.GetAmountCents()

		now := time.Now().UTC().Unix()
		entryID := srv.newEntryID()
		srv.entries = append(srv.entries, &creditv1.Entry{
			EntryId:        entryID,
			AmountCents:    -payload.GetAmountCents(),
			Type:           "spend",
			IdempotencyKey: payload.GetIdempotencyKey(),
			CreatedUnixUtc: now,
		})
		result.Ok = true
		result.EntryId = entryID
		result.CreatedUnixUtc = now
		return result

	case *creditv1.BatchOperation_Refund:
		payload := typed.Refund
		if payload == nil {
			result.Ok = false
			result.ErrorCode = "invalid_operation"
			result.ErrorMessage = "invalid_operation"
			return result
		}
		if _, exists := srv.seenIdempotency[payload.GetIdempotencyKey()]; exists {
			result.Ok = true
			result.Duplicate = true
			return result
		}
		original := srv.resolveOriginalEntry(payload.GetOriginalEntryId(), payload.GetOriginalIdempotencyKey())
		if original == nil || original.GetAmountCents() >= 0 {
			result.Ok = false
			result.ErrorCode = "invalid_refund_original"
			result.ErrorMessage = "invalid_refund_original"
			return result
		}
		refundedCents := srv.refundedCentsForOriginal(original.GetEntryId())
		remainingCents := -original.GetAmountCents() - refundedCents
		if payload.GetAmountCents() > remainingCents {
			result.Ok = false
			result.ErrorCode = "refund_exceeds_debit"
			result.ErrorMessage = "refund_exceeds_debit"
			return result
		}

		srv.seenIdempotency[payload.GetIdempotencyKey()] = struct{}{}
		srv.balanceCents += payload.GetAmountCents()

		now := time.Now().UTC().Unix()
		entryID := srv.newEntryID()
		srv.entries = append(srv.entries, &creditv1.Entry{
			EntryId:         entryID,
			AmountCents:     payload.GetAmountCents(),
			Type:            "refund",
			IdempotencyKey:  payload.GetIdempotencyKey(),
			RefundOfEntryId: original.GetEntryId(),
			CreatedUnixUtc:  now,
		})
		result.Ok = true
		result.EntryId = entryID
		result.CreatedUnixUtc = now
		return result

	default:
		result.Ok = false
		result.ErrorCode = "unsupported_operation"
		result.ErrorMessage = "unsupported_operation"
		return result
	}
}

func (srv *fakeLedgerServer) resolveOriginalEntry(originalEntryID string, originalIdempotencyKey string) *creditv1.Entry {
	if originalEntryID != "" {
		for _, entry := range srv.entries {
			if entry.GetEntryId() == originalEntryID {
				return entry
			}
		}
		return nil
	}
	if originalIdempotencyKey != "" {
		for _, entry := range srv.entries {
			if entry.GetIdempotencyKey() == originalIdempotencyKey {
				return entry
			}
		}
		return nil
	}
	return nil
}

func (srv *fakeLedgerServer) refundedCentsForOriginal(originalEntryID string) int64 {
	var refunded int64
	for _, entry := range srv.entries {
		if entry.GetRefundOfEntryId() == originalEntryID {
			refunded += entry.GetAmountCents()
		}
	}
	return refunded
}

type walletEnvelope struct {
	Wallet walletResponse `json:"wallet"`
}

type transactionEnvelope struct {
	Status string         `json:"status"`
	Wallet walletResponse `json:"wallet"`
}
