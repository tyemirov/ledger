package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

var errUnimplemented = errors.New("unimplemented")

// Run boots the HTTP façade using the supplied configuration.
func Run(ctx context.Context, cfg Config) error {
	logger, err := zap.NewProduction()
	if err != nil {
		return fmt.Errorf("zap init: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	dialOptions := []grpc.DialOption{}
	if cfg.LedgerInsecure {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		dialOptions = append(dialOptions, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}
	conn, err := grpc.NewClient(cfg.LedgerAddress, dialOptions...)
	if err != nil {
		return fmt.Errorf("connect ledger: %w", err)
	}
	conn.Connect()
	if err := waitForClientReady(ctx, conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("connect ledger: %w", err)
	}
	defer conn.Close()

	ledgerClient := creditv1.NewCreditServiceClient(conn)
	sessionValidator, err := sessionvalidator.New(sessionvalidator.Config{
		SigningKey: []byte(cfg.SessionSigningKey),
		Issuer:     cfg.SessionIssuer,
		CookieName: cfg.SessionCookieName,
	})
	if err != nil {
		return fmt.Errorf("session validator: %w", err)
	}

	handler := &httpHandler{
		logger:       logger,
		ledgerClient: ledgerClient,
		cfg:          cfg,
	}

	router := setupRouter(cfg, handler, sessionValidator)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: router,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("demoapi listening", zap.String("addr", cfg.ListenAddr))
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Warn("server shutdown error", zap.Error(shutdownErr))
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func setupRouter(cfg Config, handler *httpHandler, validator *sessionvalidator.Validator) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.AllowedOrigins,
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Origin", "Accept"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/healthz", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := router.Group("/api")
	api.Use(validator.GinMiddleware("auth_claims"))

	api.GET("/session", handler.handleSession)
	api.POST("/bootstrap", handler.handleBootstrap)
	api.GET("/wallet", handler.handleWallet)
	api.POST("/transactions", handler.handleTransaction)
	api.POST("/purchases", handler.handlePurchase)
	api.GET("/reservations", handler.handleListReservations)
	api.POST("/reservations", handler.handleReserve)
	api.GET("/reservations/:reservationId", handler.handleGetReservation)
	api.POST("/reservations/:reservationId/capture", handler.handleCaptureReservation)
	api.POST("/reservations/:reservationId/release", handler.handleReleaseReservation)
	api.POST("/refunds", handler.handleRefund)
	api.POST("/batch/spend", handler.handleBatchSpend)
	api.POST("/batch/refund", handler.handleBatchRefund)

	return router
}

type httpHandler struct {
	logger       *zap.Logger
	ledgerClient creditv1.CreditServiceClient
	cfg          Config
	bootstrapped sync.Map
}

func (handler *httpHandler) handleSession(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"user_id":    claims.GetUserID(),
		"email":      claims.GetUserEmail(),
		"display":    claims.GetUserDisplayName(),
		"avatar_url": claims.GetUserAvatarURL(),
		"roles":      claims.GetUserRoles(),
		"expires":    claims.GetExpiresAt().Unix(),
	})
}

func (handler *httpHandler) handleBootstrap(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()

	if err := handler.ensureBootstrap(requestCtx, claims.GetUserID()); err != nil {
		handler.logger.Error("bootstrap grant failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "grant failed"))
		return
	}
	handler.respondWithWallet(ctx, claims.GetUserID())
}

func (handler *httpHandler) ensureBootstrap(ctx context.Context, userID string) error {
	if _, ok := handler.bootstrapped.Load(userID); ok {
		return nil
	}
	_, err := handler.ledgerClient.Grant(ctx, &creditv1.GrantRequest{
		UserId:           userID,
		AmountCents:      BootstrapAmountCents(),
		IdempotencyKey:   fmt.Sprintf("bootstrap:%s", userID),
		MetadataJson:     marshalMetadata(map[string]string{"action": "bootstrap"}),
		ExpiresAtUnixUtc: 0,
		LedgerId:         handler.cfg.DefaultLedgerID,
		TenantId:         handler.cfg.DefaultTenantID,
	})
	if err != nil && !isGRPCAlreadyExists(err) {
		return err
	}
	handler.bootstrapped.Store(userID, struct{}{})
	return nil
}

func (handler *httpHandler) handleWallet(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	handler.respondWithWallet(ctx, claims.GetUserID())
}

func (handler *httpHandler) handleTransaction(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request spendRequest
	if err := ctx.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()

	metadata := request.Metadata
	if metadata == nil {
		metadata = map[string]any{"action": "spend"}
	}

	_, err := handler.ledgerClient.Spend(requestCtx, &creditv1.SpendRequest{
		UserId:         claims.GetUserID(),
		AmountCents:    TransactionAmountCents(),
		IdempotencyKey: fmt.Sprintf("spend:%s", uuid.NewString()),
		MetadataJson:   marshalMetadata(metadata),
		LedgerId:       handler.cfg.DefaultLedgerID,
		TenantId:       handler.cfg.DefaultTenantID,
	})
	if err != nil {
		if isGRPCInsufficientFunds(err) {
			handler.respondTransactionStatus(ctx, "insufficient_funds", claims.GetUserID())
			return
		}
		handler.logger.Error("spend failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "spend failed"))
		return
	}
	handler.respondTransactionStatus(ctx, "success", claims.GetUserID())
}

func (handler *httpHandler) handlePurchase(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request purchaseRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}
	if request.Coins < MinimumPurchaseCoins() || request.Coins%PurchaseIncrementCoins() != 0 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_coins", fmt.Sprintf("coins must be >= %d and in steps of %d", MinimumPurchaseCoins(), PurchaseIncrementCoins())))
		return
	}

	metadata := request.Metadata
	if metadata == nil {
		metadata = map[string]any{"action": "purchase"}
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()

	_, err := handler.ledgerClient.Grant(requestCtx, &creditv1.GrantRequest{
		UserId:         claims.GetUserID(),
		AmountCents:    request.Coins * CoinValueCents(),
		IdempotencyKey: fmt.Sprintf("purchase:%s", uuid.NewString()),
		MetadataJson:   marshalMetadata(metadata),
		LedgerId:       handler.cfg.DefaultLedgerID,
		TenantId:       handler.cfg.DefaultTenantID,
	})
	if err != nil {
		handler.logger.Error("purchase grant failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "grant failed"))
		return
	}
	handler.respondWithWallet(ctx, claims.GetUserID())
}

func (handler *httpHandler) handleReserve(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request reserveRequest
	if err := ctx.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}
	if request.Coins <= 0 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_coins", "coins must be greater than zero"))
		return
	}
	if request.TTLSeconds < 0 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_ttl", "ttl_seconds must be >= 0"))
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()

	reservationID := uuid.NewString()
	expiresAt := int64(0)
	if request.TTLSeconds > 0 {
		expiresAt = time.Now().UTC().Add(time.Duration(request.TTLSeconds) * time.Second).Unix()
	}

	metadata := request.Metadata
	if metadata == nil {
		metadata = map[string]any{"action": "reserve", "source": "demo"}
	}

	_, err := handler.ledgerClient.Reserve(requestCtx, &creditv1.ReserveRequest{
		UserId:           claims.GetUserID(),
		AmountCents:      request.Coins * CoinValueCents(),
		ReservationId:    reservationID,
		IdempotencyKey:   fmt.Sprintf("reserve:%s", reservationID),
		MetadataJson:     marshalMetadata(metadata),
		LedgerId:         handler.cfg.DefaultLedgerID,
		TenantId:         handler.cfg.DefaultTenantID,
		ExpiresAtUnixUtc: expiresAt,
	})
	if err != nil {
		if isGRPCInsufficientFunds(err) {
			handler.respondReservationStatus(ctx, "insufficient_funds", claims.GetUserID(), nil)
			return
		}
		handler.logger.Error("reserve failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reserve failed"))
		return
	}
	reservation, reservationErr := handler.fetchReservation(ctx.Request.Context(), claims.GetUserID(), reservationID)
	if reservationErr != nil {
		handler.logger.Error("reservation fetch failed", zap.Error(reservationErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservation unavailable"))
		return
	}
	handler.respondReservationStatus(ctx, "reserved", claims.GetUserID(), &reservation)
}

func (handler *httpHandler) handleGetReservation(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	reservationID := ctx.Param("reservationId")
	if reservationID == "" {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_reservation_id", "reservationId is required"))
		return
	}
	reservation, err := handler.fetchReservation(ctx.Request.Context(), claims.GetUserID(), reservationID)
	if err != nil {
		if isGRPCUnknownReservation(err) {
			ctx.JSON(http.StatusNotFound, errorResponse("unknown_reservation", "reservation not found"))
			return
		}
		handler.logger.Error("reservation fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservation unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"reservation": reservation})
}

func (handler *httpHandler) handleListReservations(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()
	response, err := handler.ledgerClient.ListReservations(requestCtx, &creditv1.ListReservationsRequest{
		UserId:               claims.GetUserID(),
		LedgerId:             handler.cfg.DefaultLedgerID,
		TenantId:             handler.cfg.DefaultTenantID,
		BeforeCreatedUnixUtc: time.Now().UTC().Add(time.Second).Unix(),
		Limit:                25,
	})
	if err != nil {
		handler.logger.Error("list reservations failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservations unavailable"))
		return
	}
	payload := make([]reservationPayload, 0, len(response.GetReservations()))
	for _, entry := range response.GetReservations() {
		payload = append(payload, reservationPayloadFromProto(entry))
	}
	ctx.JSON(http.StatusOK, gin.H{"reservations": payload})
}

func (handler *httpHandler) handleCaptureReservation(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	reservationID := ctx.Param("reservationId")
	if reservationID == "" {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_reservation_id", "reservationId is required"))
		return
	}
	reservation, err := handler.fetchReservation(ctx.Request.Context(), claims.GetUserID(), reservationID)
	if err != nil {
		if isGRPCUnknownReservation(err) {
			ctx.JSON(http.StatusNotFound, errorResponse("unknown_reservation", "reservation not found"))
			return
		}
		handler.logger.Error("reservation fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservation unavailable"))
		return
	}
	if reservation.Status != "active" || reservation.Expired {
		ctx.JSON(http.StatusConflict, errorResponse("reservation_closed", "reservation is not active"))
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()
	_, captureErr := handler.ledgerClient.Capture(requestCtx, &creditv1.CaptureRequest{
		UserId:         claims.GetUserID(),
		ReservationId:  reservationID,
		IdempotencyKey: fmt.Sprintf("capture:%s", reservationID),
		AmountCents:    reservation.AmountCents,
		MetadataJson:   marshalMetadata(map[string]any{"action": "capture", "source": "demo"}),
		LedgerId:       handler.cfg.DefaultLedgerID,
		TenantId:       handler.cfg.DefaultTenantID,
	})
	if captureErr != nil && !isGRPCAlreadyExists(captureErr) {
		if isGRPCReservationClosed(captureErr) {
			ctx.JSON(http.StatusConflict, errorResponse("reservation_closed", "reservation closed"))
			return
		}
		handler.logger.Error("capture failed", zap.Error(captureErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "capture failed"))
		return
	}

	updated, updatedErr := handler.fetchReservation(ctx.Request.Context(), claims.GetUserID(), reservationID)
	if updatedErr != nil {
		handler.logger.Error("reservation fetch failed", zap.Error(updatedErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservation unavailable"))
		return
	}
	handler.respondReservationStatus(ctx, "captured", claims.GetUserID(), &updated)
}

func (handler *httpHandler) handleReleaseReservation(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	reservationID := ctx.Param("reservationId")
	if reservationID == "" {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_reservation_id", "reservationId is required"))
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()
	_, releaseErr := handler.ledgerClient.Release(requestCtx, &creditv1.ReleaseRequest{
		UserId:         claims.GetUserID(),
		ReservationId:  reservationID,
		IdempotencyKey: fmt.Sprintf("release:%s", reservationID),
		MetadataJson:   marshalMetadata(map[string]any{"action": "release", "source": "demo"}),
		LedgerId:       handler.cfg.DefaultLedgerID,
		TenantId:       handler.cfg.DefaultTenantID,
	})
	if releaseErr != nil && !isGRPCAlreadyExists(releaseErr) {
		if isGRPCReservationClosed(releaseErr) {
			ctx.JSON(http.StatusConflict, errorResponse("reservation_closed", "reservation closed"))
			return
		}
		if isGRPCUnknownReservation(releaseErr) {
			ctx.JSON(http.StatusNotFound, errorResponse("unknown_reservation", "reservation not found"))
			return
		}
		handler.logger.Error("release failed", zap.Error(releaseErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "release failed"))
		return
	}
	updated, updatedErr := handler.fetchReservation(ctx.Request.Context(), claims.GetUserID(), reservationID)
	if updatedErr != nil {
		if isGRPCUnknownReservation(updatedErr) {
			handler.respondReservationStatus(ctx, "released", claims.GetUserID(), nil)
			return
		}
		handler.logger.Error("reservation fetch failed", zap.Error(updatedErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "reservation unavailable"))
		return
	}
	handler.respondReservationStatus(ctx, "released", claims.GetUserID(), &updated)
}

func (handler *httpHandler) handleRefund(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request refundRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}
	amountCents := request.AmountCents
	if amountCents <= 0 && request.AmountCoins > 0 {
		amountCents = request.AmountCoins * CoinValueCents()
	}
	originalEntryID := request.OriginalEntryID
	originalIdempotencyKey := request.OriginalIdempotencyKey
	if amountCents <= 0 || (originalEntryID == "" && originalIdempotencyKey == "") || (originalEntryID != "" && originalIdempotencyKey != "") {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_refund", "provide exactly one original reference and a positive amount"))
		return
	}

	metadata := request.Metadata
	if metadata == nil {
		metadata = map[string]any{"action": "refund", "source": "demo"}
	}

	idempotencyTarget := originalEntryID
	if idempotencyTarget == "" {
		idempotencyTarget = originalIdempotencyKey
	}
	idempotencyKey := fmt.Sprintf("refund:%s:%d", idempotencyTarget, amountCents)

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()

	refundReq := &creditv1.RefundRequest{
		UserId:         claims.GetUserID(),
		LedgerId:       handler.cfg.DefaultLedgerID,
		TenantId:       handler.cfg.DefaultTenantID,
		AmountCents:    amountCents,
		IdempotencyKey: idempotencyKey,
		MetadataJson:   marshalMetadata(metadata),
	}
	if originalEntryID != "" {
		refundReq.Original = &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: originalEntryID}
	} else {
		refundReq.Original = &creditv1.RefundRequest_OriginalIdempotencyKey{OriginalIdempotencyKey: originalIdempotencyKey}
	}

	response, err := handler.ledgerClient.Refund(requestCtx, refundReq)
	if err != nil {
		if isGRPCAlreadyExists(err) {
			handler.respondRefundStatus(ctx, "duplicate", claims.GetUserID(), "", 0)
			return
		}
		if isGRPCRefundExceedsDebit(err) {
			ctx.JSON(http.StatusConflict, errorResponse("refund_exceeds_debit", "refund exceeds debit"))
			return
		}
		if isGRPCInvalidRefundOriginal(err) {
			ctx.JSON(http.StatusBadRequest, errorResponse("invalid_refund_original", "invalid refund original"))
			return
		}
		handler.logger.Error("refund failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "refund failed"))
		return
	}
	handler.respondRefundStatus(ctx, "refunded", claims.GetUserID(), response.GetEntryId(), response.GetCreatedUnixUtc())
}

func (handler *httpHandler) handleBatchSpend(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request batchSpendRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}
	if request.Count <= 0 || request.Count > 5000 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_count", "count must be between 1 and 5000"))
		return
	}
	if request.Coins <= 0 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_coins", "coins must be greater than zero"))
		return
	}
	batchID := uuid.NewString()
	operations := make([]*creditv1.BatchOperation, 0, request.Count)
	for index := 0; index < request.Count; index++ {
		operationID := fmt.Sprintf("spend-%d", index)
		idempotencyKey := fmt.Sprintf("spend:batch:%s:%d", batchID, index)
		operations = append(operations, &creditv1.BatchOperation{
			OperationId: operationID,
			Operation: &creditv1.BatchOperation_Spend{
				Spend: &creditv1.BatchSpendOp{
					AmountCents:    request.Coins * CoinValueCents(),
					IdempotencyKey: idempotencyKey,
					MetadataJson: marshalMetadata(map[string]any{
						"action":   "batch_spend",
						"source":   "demo",
						"batch_id": batchID,
						"index":    index,
					}),
				},
			},
		})
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()
	response, err := handler.ledgerClient.Batch(requestCtx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{
			UserId:   claims.GetUserID(),
			LedgerId: handler.cfg.DefaultLedgerID,
			TenantId: handler.cfg.DefaultTenantID,
		},
		Operations: operations,
		Atomic:     request.Atomic,
	})
	if err != nil {
		handler.logger.Error("batch spend failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "batch spend failed"))
		return
	}
	summary := summarizeBatchResults(response.GetResults())
	wallet, walletErr := handler.fetchWallet(ctx.Request.Context(), claims.GetUserID())
	if walletErr != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(walletErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"batch": summary, "wallet": wallet})
}

func (handler *httpHandler) handleBatchRefund(ctx *gin.Context) {
	claims := getClaims(ctx)
	if claims == nil {
		ctx.JSON(http.StatusUnauthorized, errorResponse("unauthorized", "missing session"))
		return
	}
	var request batchRefundRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_payload", "expected JSON body"))
		return
	}
	if len(request.Items) == 0 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_items", "items is required"))
		return
	}
	if len(request.Items) > 5000 {
		ctx.JSON(http.StatusBadRequest, errorResponse("invalid_items", "items must be <= 5000"))
		return
	}
	batchID := uuid.NewString()
	operations := make([]*creditv1.BatchOperation, 0, len(request.Items))
	for index, item := range request.Items {
		if item.AmountCents <= 0 {
			ctx.JSON(http.StatusBadRequest, errorResponse("invalid_amount", "amount_cents must be > 0"))
			return
		}
		if (item.OriginalEntryID == "" && item.OriginalIdempotencyKey == "") || (item.OriginalEntryID != "" && item.OriginalIdempotencyKey != "") {
			ctx.JSON(http.StatusBadRequest, errorResponse("invalid_original", "provide exactly one original reference"))
			return
		}
		operationID := fmt.Sprintf("refund-%d", index)
		idempotencyTarget := item.OriginalEntryID
		if idempotencyTarget == "" {
			idempotencyTarget = item.OriginalIdempotencyKey
		}
		idempotencyKey := fmt.Sprintf("refund:batch:%s:%s:%d", batchID, idempotencyTarget, item.AmountCents)
		refund := &creditv1.BatchRefundOp{
			AmountCents:    item.AmountCents,
			IdempotencyKey: idempotencyKey,
			MetadataJson: marshalMetadata(map[string]any{
				"action":   "batch_refund",
				"source":   "demo",
				"batch_id": batchID,
			}),
		}
		if item.OriginalEntryID != "" {
			refund.Original = &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: item.OriginalEntryID}
		} else {
			refund.Original = &creditv1.BatchRefundOp_OriginalIdempotencyKey{OriginalIdempotencyKey: item.OriginalIdempotencyKey}
		}
		operations = append(operations, &creditv1.BatchOperation{
			OperationId: operationID,
			Operation:   &creditv1.BatchOperation_Refund{Refund: refund},
		})
	}

	requestCtx, cancel := context.WithTimeout(ctx.Request.Context(), handler.cfg.LedgerTimeout)
	defer cancel()
	response, err := handler.ledgerClient.Batch(requestCtx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{
			UserId:   claims.GetUserID(),
			LedgerId: handler.cfg.DefaultLedgerID,
			TenantId: handler.cfg.DefaultTenantID,
		},
		Operations: operations,
		Atomic:     request.Atomic,
	})
	if err != nil {
		handler.logger.Error("batch refund failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "batch refund failed"))
		return
	}
	summary := summarizeBatchResults(response.GetResults())
	wallet, walletErr := handler.fetchWallet(ctx.Request.Context(), claims.GetUserID())
	if walletErr != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(walletErr))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"batch": summary, "wallet": wallet})
}

func (handler *httpHandler) respondTransactionStatus(ctx *gin.Context, status string, userID string) {
	wallet, err := handler.fetchWallet(ctx.Request.Context(), userID)
	if err != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"status": status,
		"wallet": wallet,
	})
}

func (handler *httpHandler) respondWithWallet(ctx *gin.Context, userID string) {
	wallet, err := handler.fetchWallet(ctx.Request.Context(), userID)
	if err != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"wallet": wallet})
}

func (handler *httpHandler) fetchWallet(ctx context.Context, userID string) (*walletResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, handler.cfg.LedgerTimeout)
	defer cancel()
	balanceResp, err := handler.ledgerClient.GetBalance(requestCtx, &creditv1.BalanceRequest{
		UserId:   userID,
		LedgerId: handler.cfg.DefaultLedgerID,
		TenantId: handler.cfg.DefaultTenantID,
	})
	if err != nil {
		return nil, err
	}

	entriesCtx, entriesCancel := context.WithTimeout(ctx, handler.cfg.LedgerTimeout)
	defer entriesCancel()
	entriesResp, err := handler.ledgerClient.ListEntries(entriesCtx, &creditv1.ListEntriesRequest{
		UserId:        userID,
		Limit:         WalletHistoryLimit(),
		BeforeUnixUtc: time.Now().UTC().Add(time.Second).Unix(),
		LedgerId:      handler.cfg.DefaultLedgerID,
		TenantId:      handler.cfg.DefaultTenantID,
	})
	if err != nil {
		return nil, err
	}

	entries := make([]entryPayload, 0, len(entriesResp.GetEntries()))
	for _, entry := range entriesResp.GetEntries() {
		metadataBytes := []byte(entry.GetMetadataJson())
		if !json.Valid(metadataBytes) {
			metadataBytes = []byte("{}")
		}
		entries = append(entries, entryPayload{
			EntryID:         entry.GetEntryId(),
			Type:            entry.GetType(),
			AmountCents:     entry.GetAmountCents(),
			AmountCoins:     entry.GetAmountCents() / CoinValueCents(),
			ReservationID:   entry.GetReservationId(),
			IdempotencyKey:  entry.GetIdempotencyKey(),
			RefundOfEntryID: entry.GetRefundOfEntryId(),
			Metadata:        json.RawMessage(metadataBytes),
			CreatedUnixUTC:  entry.GetCreatedUnixUtc(),
		})
	}

	return &walletResponse{
		Balance: balancePayload{
			TotalCents:     balanceResp.GetTotalCents(),
			AvailableCents: balanceResp.GetAvailableCents(),
			TotalCoins:     balanceResp.GetTotalCents() / CoinValueCents(),
			AvailableCoins: balanceResp.GetAvailableCents() / CoinValueCents(),
		},
		Entries: entries,
	}, nil
}

func (handler *httpHandler) fetchReservation(ctx context.Context, userID string, reservationID string) (reservationPayload, error) {
	requestCtx, cancel := context.WithTimeout(ctx, handler.cfg.LedgerTimeout)
	defer cancel()
	response, err := handler.ledgerClient.GetReservation(requestCtx, &creditv1.GetReservationRequest{
		UserId:        userID,
		LedgerId:      handler.cfg.DefaultLedgerID,
		TenantId:      handler.cfg.DefaultTenantID,
		ReservationId: reservationID,
	})
	if err != nil {
		return reservationPayload{}, err
	}
	return reservationPayloadFromProto(response.GetReservation()), nil
}

func (handler *httpHandler) respondReservationStatus(ctx *gin.Context, status string, userID string, reservation *reservationPayload) {
	wallet, err := handler.fetchWallet(ctx.Request.Context(), userID)
	if err != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, reservationEnvelope{
		Status:      status,
		Wallet:      wallet,
		Reservation: reservation,
	})
}

func (handler *httpHandler) respondRefundStatus(ctx *gin.Context, status string, userID string, entryID string, createdUnixUTC int64) {
	wallet, err := handler.fetchWallet(ctx.Request.Context(), userID)
	if err != nil {
		handler.logger.Error("wallet fetch failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "wallet unavailable"))
		return
	}
	ctx.JSON(http.StatusOK, refundEnvelope{
		Status:         status,
		Wallet:         wallet,
		RefundEntryID:  entryID,
		CreatedUnixUTC: createdUnixUTC,
	})
}

func marshalMetadata(metadata any) string {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func getClaims(ctx *gin.Context) *sessionvalidator.Claims {
	claimsValue, ok := ctx.Get("auth_claims")
	if !ok {
		return nil
	}
	claims, _ := claimsValue.(*sessionvalidator.Claims)
	return claims
}

func isGRPCAlreadyExists(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.AlreadyExists
}

func isGRPCInsufficientFunds(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.FailedPrecondition && statusInfo.Message() == "insufficient_funds"
}

func isGRPCUnknownReservation(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.NotFound && statusInfo.Message() == "unknown_reservation"
}

func isGRPCReservationClosed(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.FailedPrecondition && statusInfo.Message() == "reservation_closed"
}

func isGRPCInvalidRefundOriginal(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.InvalidArgument && statusInfo.Message() == "invalid_refund_original"
}

func isGRPCRefundExceedsDebit(err error) bool {
	statusInfo, ok := status.FromError(err)
	if !ok {
		return false
	}
	return statusInfo.Code() == codes.FailedPrecondition && statusInfo.Message() == "refund_exceeds_debit"
}

func errorResponse(code string, message string) gin.H {
	return gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	}
}

type spendRequest struct {
	Metadata map[string]any `json:"metadata"`
}

type purchaseRequest struct {
	Coins    int64          `json:"coins"`
	Metadata map[string]any `json:"metadata"`
}

type walletResponse struct {
	Balance balancePayload `json:"balance"`
	Entries []entryPayload `json:"entries"`
}

type balancePayload struct {
	TotalCents     int64 `json:"total_cents"`
	AvailableCents int64 `json:"available_cents"`
	TotalCoins     int64 `json:"total_coins"`
	AvailableCoins int64 `json:"available_coins"`
}

type entryPayload struct {
	EntryID         string          `json:"entry_id"`
	Type            string          `json:"type"`
	AmountCents     int64           `json:"amount_cents"`
	AmountCoins     int64           `json:"amount_coins"`
	ReservationID   string          `json:"reservation_id"`
	IdempotencyKey  string          `json:"idempotency_key"`
	RefundOfEntryID string          `json:"refund_of_entry_id,omitempty"`
	Metadata        json.RawMessage `json:"metadata"`
	CreatedUnixUTC  int64           `json:"created_unix_utc"`
}

type reservationPayload struct {
	ReservationID    string `json:"reservation_id"`
	AmountCents      int64  `json:"amount_cents"`
	AmountCoins      int64  `json:"amount_coins"`
	Status           string `json:"status"`
	ExpiresAtUnixUTC int64  `json:"expires_at_unix_utc"`
	CreatedUnixUTC   int64  `json:"created_unix_utc"`
	UpdatedUnixUTC   int64  `json:"updated_unix_utc"`
	Expired          bool   `json:"expired"`
	HeldCents        int64  `json:"held_cents"`
	HeldCoins        int64  `json:"held_coins"`
	CapturedCents    int64  `json:"captured_cents"`
	CapturedCoins    int64  `json:"captured_coins"`
}

func reservationPayloadFromProto(reservation *creditv1.Reservation) reservationPayload {
	if reservation == nil {
		return reservationPayload{}
	}
	return reservationPayload{
		ReservationID:    reservation.GetReservationId(),
		AmountCents:      reservation.GetAmountCents(),
		AmountCoins:      reservation.GetAmountCents() / CoinValueCents(),
		Status:           reservation.GetStatus(),
		ExpiresAtUnixUTC: reservation.GetExpiresAtUnixUtc(),
		CreatedUnixUTC:   reservation.GetCreatedUnixUtc(),
		UpdatedUnixUTC:   reservation.GetUpdatedUnixUtc(),
		Expired:          reservation.GetExpired(),
		HeldCents:        reservation.GetHeldCents(),
		HeldCoins:        reservation.GetHeldCents() / CoinValueCents(),
		CapturedCents:    reservation.GetCapturedCents(),
		CapturedCoins:    reservation.GetCapturedCents() / CoinValueCents(),
	}
}

type reserveRequest struct {
	Coins      int64          `json:"coins"`
	TTLSeconds int64          `json:"ttl_seconds"`
	Metadata   map[string]any `json:"metadata"`
}

type reservationEnvelope struct {
	Status      string              `json:"status"`
	Wallet      *walletResponse     `json:"wallet"`
	Reservation *reservationPayload `json:"reservation,omitempty"`
}

type refundRequest struct {
	OriginalEntryID        string         `json:"original_entry_id"`
	OriginalIdempotencyKey string         `json:"original_idempotency_key"`
	AmountCents            int64          `json:"amount_cents"`
	AmountCoins            int64          `json:"amount_coins"`
	Metadata               map[string]any `json:"metadata"`
}

type refundEnvelope struct {
	Status         string          `json:"status"`
	Wallet         *walletResponse `json:"wallet"`
	RefundEntryID  string          `json:"refund_entry_id,omitempty"`
	CreatedUnixUTC int64           `json:"created_unix_utc,omitempty"`
}

type batchSpendRequest struct {
	Count  int   `json:"count"`
	Coins  int64 `json:"coins"`
	Atomic bool  `json:"atomic"`
}

type batchRefundItem struct {
	OriginalEntryID        string `json:"original_entry_id"`
	OriginalIdempotencyKey string `json:"original_idempotency_key"`
	AmountCents            int64  `json:"amount_cents"`
}

type batchRefundRequest struct {
	Items  []batchRefundItem `json:"items"`
	Atomic bool              `json:"atomic"`
}

type batchSummary struct {
	Ok        int `json:"ok"`
	Duplicate int `json:"duplicate"`
	Failed    int `json:"failed"`
}

func summarizeBatchResults(results []*creditv1.BatchOperationResult) batchSummary {
	summary := batchSummary{}
	for _, result := range results {
		if result.GetDuplicate() {
			summary.Duplicate++
		}
		if result.GetOk() {
			summary.Ok++
		} else {
			summary.Failed++
		}
	}
	return summary
}

func waitForClientReady(ctx context.Context, conn *grpc.ClientConn) error {
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return errors.New("grpc connection shutdown before ready")
		}
		if !conn.WaitForStateChange(ctx, state) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return errors.New("grpc connection failed to reach ready state")
		}
	}
}
