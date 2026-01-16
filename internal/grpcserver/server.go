package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	errorInsufficientFunds       = "insufficient_funds"
	errorUnknownReservation      = "unknown_reservation"
	errorDuplicateIdempotencyKey = "duplicate_idempotency_key"
	errorInvalidUserID           = "invalid_user_id"
	errorInvalidLedgerID         = "invalid_ledger_id"
	errorInvalidTenantID         = "invalid_tenant_id"
	errorInvalidReservationID    = "invalid_reservation_id"
	errorInvalidIdempotencyKey   = "invalid_idempotency_key"
	errorInvalidAmount           = "invalid_amount_cents"
	errorInvalidMetadata         = "invalid_metadata_json"
	errorInvalidListLimit        = "invalid_list_limit"
	errorReservationExists       = "reservation_exists"
	errorReservationClosed       = "reservation_closed"

	defaultListEntriesLimit = 50
	maxListEntriesLimit     = 200
)

// CreditServiceServer exposes the credit ledger over gRPC.
type CreditServiceServer struct {
	creditv1.UnimplementedCreditServiceServer
	creditService *ledger.Service
}

// NewCreditServiceServer constructs a gRPC server for the ledger service.
func NewCreditServiceServer(creditService *ledger.Service) *CreditServiceServer {
	return &CreditServiceServer{creditService: creditService}
}

func (service *CreditServiceServer) GetBalance(ctx context.Context, request *creditv1.BalanceRequest) (*creditv1.BalanceResponse, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	balance, operationError := service.creditService.Balance(ctx, tenantID, userID, ledgerID)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.BalanceResponse{
		TotalCents:     balance.TotalCents.Int64(),
		AvailableCents: balance.AvailableCents.Int64(),
	}, nil
}

func (service *CreditServiceServer) Grant(ctx context.Context, request *creditv1.GrantRequest) (*creditv1.Empty, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	amount, err := ledger.NewPositiveAmountCents(request.GetAmountCents())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	idem, err := ledger.NewIdempotencyKey(request.GetIdempotencyKey())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	metadata, err := ledger.NewMetadataJSON(request.GetMetadataJson())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	operationError := service.creditService.Grant(ctx, tenantID, userID, ledgerID, amount, idem, request.GetExpiresAtUnixUtc(), metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Reserve(ctx context.Context, request *creditv1.ReserveRequest) (*creditv1.Empty, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	amount, err := ledger.NewPositiveAmountCents(request.GetAmountCents())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	reservationID, err := ledger.NewReservationID(request.GetReservationId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	idem, err := ledger.NewIdempotencyKey(request.GetIdempotencyKey())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	metadata, err := ledger.NewMetadataJSON(request.GetMetadataJson())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	operationError := service.creditService.Reserve(ctx, tenantID, userID, ledgerID, amount, reservationID, idem, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Capture(ctx context.Context, request *creditv1.CaptureRequest) (*creditv1.Empty, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	reservationID, err := ledger.NewReservationID(request.GetReservationId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	idem, err := ledger.NewIdempotencyKey(request.GetIdempotencyKey())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	amount, err := ledger.NewPositiveAmountCents(request.GetAmountCents())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	metadata, err := ledger.NewMetadataJSON(request.GetMetadataJson())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	operationError := service.creditService.Capture(ctx, tenantID, userID, ledgerID, reservationID, idem, amount, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Release(ctx context.Context, request *creditv1.ReleaseRequest) (*creditv1.Empty, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	reservationID, err := ledger.NewReservationID(request.GetReservationId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	idem, err := ledger.NewIdempotencyKey(request.GetIdempotencyKey())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	metadata, err := ledger.NewMetadataJSON(request.GetMetadataJson())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	operationError := service.creditService.Release(ctx, tenantID, userID, ledgerID, reservationID, idem, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Spend(ctx context.Context, request *creditv1.SpendRequest) (*creditv1.Empty, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	amount, err := ledger.NewPositiveAmountCents(request.GetAmountCents())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	idem, err := ledger.NewIdempotencyKey(request.GetIdempotencyKey())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	metadata, err := ledger.NewMetadataJSON(request.GetMetadataJson())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	operationError := service.creditService.Spend(ctx, tenantID, userID, ledgerID, amount, idem, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) ListEntries(ctx context.Context, request *creditv1.ListEntriesRequest) (*creditv1.ListEntriesResponse, error) {
	userID, err := ledger.NewUserID(request.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(request.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(request.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	limit, err := normalizeListLimit(request.GetLimit())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, errorInvalidListLimit)
	}
	before := request.GetBeforeUnixUtc()
	if before == 0 {
		before = time.Now().UTC().Unix()
	}
	entries, operationError := service.creditService.ListEntries(ctx, tenantID, userID, ledgerID, before, int(limit))
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	response := &creditv1.ListEntriesResponse{Entries: make([]*creditv1.Entry, 0, len(entries))}
	for _, entryRecord := range entries {
		reservationIDValue := ""
		reservationID, hasReservation := entryRecord.ReservationID()
		if hasReservation {
			reservationIDValue = reservationID.String()
		}
		response.Entries = append(response.Entries, &creditv1.Entry{
			EntryId:          entryRecord.EntryID().String(),
			AccountId:        entryRecord.AccountID().String(),
			Type:             entryRecord.Type().String(),
			AmountCents:      entryRecord.AmountCents().Int64(),
			ReservationId:    reservationIDValue,
			IdempotencyKey:   entryRecord.IdempotencyKey().String(),
			ExpiresAtUnixUtc: entryRecord.ExpiresAtUnixUTC(),
			MetadataJson:     entryRecord.MetadataJSON().String(),
			CreatedUnixUtc:   entryRecord.CreatedUnixUTC(),
		})
	}
	return response, nil
}

func normalizeListLimit(limit int32) (int32, error) {
	if limit <= 0 {
		return defaultListEntriesLimit, nil
	}
	if limit > maxListEntriesLimit {
		return 0, fmt.Errorf("limit exceeds maximum: %d > %d", limit, maxListEntriesLimit)
	}
	return limit, nil
}

func mapToGRPCError(source error) error {
	if errors.Is(source, ledger.ErrInvalidUserID) {
		return status.Error(codes.InvalidArgument, errorInvalidUserID)
	}
	if errors.Is(source, ledger.ErrInvalidLedgerID) {
		return status.Error(codes.InvalidArgument, errorInvalidLedgerID)
	}
	if errors.Is(source, ledger.ErrInvalidTenantID) {
		return status.Error(codes.InvalidArgument, errorInvalidTenantID)
	}
	if errors.Is(source, ledger.ErrInvalidReservationID) {
		return status.Error(codes.InvalidArgument, errorInvalidReservationID)
	}
	if errors.Is(source, ledger.ErrInvalidIdempotencyKey) {
		return status.Error(codes.InvalidArgument, errorInvalidIdempotencyKey)
	}
	if errors.Is(source, ledger.ErrInvalidAmountCents) {
		return status.Error(codes.InvalidArgument, errorInvalidAmount)
	}
	if errors.Is(source, ledger.ErrInvalidMetadataJSON) {
		return status.Error(codes.InvalidArgument, errorInvalidMetadata)
	}
	if errors.Is(source, ledger.ErrInsufficientFunds) {
		return status.Error(codes.FailedPrecondition, errorInsufficientFunds)
	}
	if errors.Is(source, ledger.ErrUnknownReservation) {
		return status.Error(codes.NotFound, errorUnknownReservation)
	}
	if errors.Is(source, ledger.ErrDuplicateIdempotencyKey) {
		return status.Error(codes.AlreadyExists, errorDuplicateIdempotencyKey)
	}
	if errors.Is(source, ledger.ErrReservationExists) {
		return status.Error(codes.AlreadyExists, errorReservationExists)
	}
	if errors.Is(source, ledger.ErrReservationClosed) {
		return status.Error(codes.FailedPrecondition, errorReservationClosed)
	}
	return status.Error(codes.Internal, source.Error())
}
