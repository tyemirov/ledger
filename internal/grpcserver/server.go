package grpcserver

import (
	"context"
	"errors"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	errorInsufficientFunds       = "insufficient_funds"
	errorUnknownReservation      = "unknown_reservation"
	errorDuplicateIdempotencyKey = "duplicate_idempotency_key"
)

// CreditServiceServer exposes the credit ledger over gRPC.
type CreditServiceServer struct {
	creditv1.UnimplementedCreditServiceServer
	creditService *credit.Service
}

// NewCreditServiceServer constructs a gRPC server for the credit service.
func NewCreditServiceServer(creditService *credit.Service) *CreditServiceServer {
	return &CreditServiceServer{creditService: creditService}
}

func (service *CreditServiceServer) GetBalance(ctx context.Context, request *creditv1.BalanceRequest) (*creditv1.BalanceResponse, error) {
	balance, operationError := service.creditService.Balance(ctx, request.GetUserId())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.BalanceResponse{
		TotalCents:     int64(balance.TotalCents),
		AvailableCents: int64(balance.AvailableCents),
	}, nil
}

func (service *CreditServiceServer) Grant(ctx context.Context, request *creditv1.GrantRequest) (*creditv1.Empty, error) {
	operationError := service.creditService.Grant(ctx, request.GetUserId(), credit.AmountCents(request.GetAmountCents()), request.GetIdempotencyKey(), request.GetExpiresAtUnixUtc(), request.GetMetadataJson())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Reserve(ctx context.Context, request *creditv1.ReserveRequest) (*creditv1.Empty, error) {
	operationError := service.creditService.Reserve(ctx, request.GetUserId(), credit.AmountCents(request.GetAmountCents()), request.GetReservationId(), request.GetIdempotencyKey(), request.GetMetadataJson())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Capture(ctx context.Context, request *creditv1.CaptureRequest) (*creditv1.Empty, error) {
	operationError := service.creditService.Capture(ctx, request.GetUserId(), request.GetReservationId(), request.GetIdempotencyKey(), credit.AmountCents(request.GetAmountCents()), request.GetMetadataJson())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Release(ctx context.Context, request *creditv1.ReleaseRequest) (*creditv1.Empty, error) {
	operationError := service.creditService.Release(ctx, request.GetUserId(), request.GetReservationId(), request.GetIdempotencyKey(), request.GetMetadataJson())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) Spend(ctx context.Context, request *creditv1.SpendRequest) (*creditv1.Empty, error) {
	operationError := service.creditService.Spend(ctx, request.GetUserId(), credit.AmountCents(request.GetAmountCents()), request.GetIdempotencyKey(), request.GetMetadataJson())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{}, nil
}

func (service *CreditServiceServer) ListEntries(ctx context.Context, request *creditv1.ListEntriesRequest) (*creditv1.ListEntriesResponse, error) {
	entries, operationError := service.creditService.ListEntries(ctx, request.GetUserId(), request.GetBeforeUnixUtc(), int(request.GetLimit()))
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	response := &creditv1.ListEntriesResponse{Entries: make([]*creditv1.Entry, 0, len(entries))}
	for _, entry := range entries {
		response.Entries = append(response.Entries, &creditv1.Entry{
			EntryId:          entry.EntryID,
			AccountId:        entry.AccountID,
			Type:             string(entry.Type),
			AmountCents:      int64(entry.AmountCents),
			ReservationId:    entry.ReservationID,
			IdempotencyKey:   entry.IdempotencyKey,
			ExpiresAtUnixUtc: entry.ExpiresAtUnixUTC,
			MetadataJson:     entry.MetadataJSON,
			CreatedUnixUtc:   entry.CreatedUnixUTC,
		})
	}
	return response, nil
}

func mapToGRPCError(source error) error {
	if errors.Is(source, credit.ErrInsufficientFunds) {
		return status.Error(codes.FailedPrecondition, errorInsufficientFunds)
	}
	if errors.Is(source, credit.ErrUnknownReservation) {
		return status.Error(codes.NotFound, errorUnknownReservation)
	}
	if errors.Is(source, credit.ErrDuplicateIdempotencyKey) {
		return status.Error(codes.AlreadyExists, errorDuplicateIdempotencyKey)
	}
	return status.Error(codes.Internal, source.Error())
}
