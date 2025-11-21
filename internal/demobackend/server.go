package demobackend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	return router
}

type httpHandler struct {
	logger       *zap.Logger
	ledgerClient creditv1.CreditServiceClient
	cfg          Config
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

	_, err := handler.ledgerClient.Grant(requestCtx, &creditv1.GrantRequest{
		UserId:           claims.GetUserID(),
		AmountCents:      BootstrapAmountCents(),
		IdempotencyKey:   fmt.Sprintf("bootstrap:%s", claims.GetUserID()),
		MetadataJson:     marshalMetadata(map[string]string{"action": "bootstrap"}),
		ExpiresAtUnixUtc: 0,
	})
	if err != nil {
		if !isGRPCAlreadyExists(err) {
			handler.logger.Error("bootstrap grant failed", zap.Error(err))
			ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "grant failed"))
			return
		}
	}
	handler.respondWithWallet(ctx, claims.GetUserID())
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
	})
	if err != nil {
		handler.logger.Error("purchase grant failed", zap.Error(err))
		ctx.JSON(http.StatusBadGateway, errorResponse("ledger_error", "grant failed"))
		return
	}
	handler.respondWithWallet(ctx, claims.GetUserID())
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
	balanceResp, err := handler.ledgerClient.GetBalance(requestCtx, &creditv1.BalanceRequest{UserId: userID})
	if err != nil {
		return nil, err
	}

	entriesCtx, entriesCancel := context.WithTimeout(ctx, handler.cfg.LedgerTimeout)
	defer entriesCancel()
	entriesResp, err := handler.ledgerClient.ListEntries(entriesCtx, &creditv1.ListEntriesRequest{
		UserId:        userID,
		Limit:         WalletHistoryLimit(),
		BeforeUnixUtc: time.Now().UTC().Add(time.Second).Unix(),
	})
	if err != nil {
		return nil, err
	}

	entries := make([]entryPayload, 0, len(entriesResp.GetEntries()))
	for _, entry := range entriesResp.GetEntries() {
		metadataBytes := []byte(entry.GetMetadataJson())
		entries = append(entries, entryPayload{
			EntryID:        entry.GetEntryId(),
			Type:           entry.GetType(),
			AmountCents:    entry.GetAmountCents(),
			AmountCoins:    entry.GetAmountCents() / CoinValueCents(),
			ReservationID:  entry.GetReservationId(),
			IdempotencyKey: entry.GetIdempotencyKey(),
			Metadata:       json.RawMessage(metadataBytes),
			CreatedUnixUTC: entry.GetCreatedUnixUtc(),
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
	EntryID        string          `json:"entry_id"`
	Type           string          `json:"type"`
	AmountCents    int64           `json:"amount_cents"`
	AmountCoins    int64           `json:"amount_coins"`
	ReservationID  string          `json:"reservation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedUnixUTC int64           `json:"created_unix_utc"`
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
