package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	errorInsufficientFunds       = "insufficient_funds"
	errorUnknownReservation      = "unknown_reservation"
	errorUnknownEntry            = "unknown_entry"
	errorDuplicateIdempotencyKey = "duplicate_idempotency_key"
	errorInvalidUserID           = "invalid_user_id"
	errorInvalidLedgerID         = "invalid_ledger_id"
	errorInvalidTenantID         = "invalid_tenant_id"
	errorInvalidReservationID    = "invalid_reservation_id"
	errorInvalidEntryID          = "invalid_entry_id"
	errorInvalidIdempotencyKey   = "invalid_idempotency_key"
	errorInvalidAmount           = "invalid_amount_cents"
	errorInvalidMetadata         = "invalid_metadata_json"
	errorInvalidEntryType        = "invalid_entry_type"
	errorInvalidListLimit        = "invalid_list_limit"
	errorInvalidAccountContext   = "invalid_account_context"
	errorInvalidOperationID      = "invalid_operation_id"
	errorMissingBatchOperation   = "missing_batch_operation"
	errorBatchTooLarge           = "batch_too_large"
	errorRolledBack              = "rolled_back"
	errorInternal                = "internal"
	errorReservationExists       = "reservation_exists"
	errorReservationClosed       = "reservation_closed"
	errorMissingRefundOriginal   = "missing_refund_original"
	errorInvalidRefundOriginal   = "invalid_refund_original"
	errorRefundExceedsDebit      = "refund_exceeds_debit"

	defaultListEntriesLimit = 50
	maxListEntriesLimit     = 200
	maxBatchOperations      = 5000
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
	entry, operationError := service.creditService.GrantEntry(ctx, tenantID, userID, ledgerID, amount, idem, request.GetExpiresAtUnixUtc(), metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
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
	entry, operationError := service.creditService.ReserveEntry(ctx, tenantID, userID, ledgerID, amount, reservationID, idem, request.GetExpiresAtUnixUtc(), metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
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
	entry, operationError := service.creditService.CaptureDebitEntry(ctx, tenantID, userID, ledgerID, reservationID, idem, amount, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
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
	entry, operationError := service.creditService.ReleaseEntry(ctx, tenantID, userID, ledgerID, reservationID, idem, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
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
	entry, operationError := service.creditService.SpendEntry(ctx, tenantID, userID, ledgerID, amount, idem, metadata)
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}
	return &creditv1.Empty{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
}

func (service *CreditServiceServer) Refund(ctx context.Context, request *creditv1.RefundRequest) (*creditv1.RefundResponse, error) {
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

	if request.GetOriginalEntryId() != "" {
		originalEntryID, err := ledger.NewEntryID(request.GetOriginalEntryId())
		if err != nil {
			return nil, mapToGRPCError(err)
		}
		entry, operationError := service.creditService.RefundByEntryIDEntry(ctx, tenantID, userID, ledgerID, originalEntryID, amount, idem, metadata)
		if operationError != nil {
			return nil, mapToGRPCError(operationError)
		}
		return &creditv1.RefundResponse{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
	}

	if request.GetOriginalIdempotencyKey() != "" {
		originalIdempotencyKey, err := ledger.NewIdempotencyKey(request.GetOriginalIdempotencyKey())
		if err != nil {
			return nil, mapToGRPCError(err)
		}
		entry, operationError := service.creditService.RefundByOriginalIdempotencyKeyEntry(ctx, tenantID, userID, ledgerID, originalIdempotencyKey, amount, idem, metadata)
		if operationError != nil {
			return nil, mapToGRPCError(operationError)
		}
		return &creditv1.RefundResponse{EntryId: entry.EntryID().String(), CreatedUnixUtc: entry.CreatedUnixUTC()}, nil
	}

	return nil, status.Error(codes.InvalidArgument, errorMissingRefundOriginal)
}

func (service *CreditServiceServer) Batch(ctx context.Context, request *creditv1.BatchRequest) (*creditv1.BatchResponse, error) {
	account := request.GetAccount()
	if account == nil {
		return nil, status.Error(codes.InvalidArgument, errorInvalidAccountContext)
	}

	userID, err := ledger.NewUserID(account.GetUserId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	ledgerID, err := ledger.NewLedgerID(account.GetLedgerId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}
	tenantID, err := ledger.NewTenantID(account.GetTenantId())
	if err != nil {
		return nil, mapToGRPCError(err)
	}

	rawOperations := request.GetOperations()
	if len(rawOperations) > maxBatchOperations {
		return nil, status.Error(codes.InvalidArgument, errorBatchTooLarge)
	}

	operations := make([]ledger.BatchOperation, len(rawOperations))
	for operationIndex, operation := range rawOperations {
		operationID := strings.TrimSpace(operation.GetOperationId())
		if operationID == "" {
			return nil, status.Error(codes.InvalidArgument, errorInvalidOperationID)
		}

		var parsedOperation ledger.BatchOperation
		parsedOperation.OperationID = operationID

		switch operationValue := operation.GetOperation().(type) {
		case *creditv1.BatchOperation_Grant:
			if operationValue.Grant == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			amount, err := ledger.NewPositiveAmountCents(operationValue.Grant.GetAmountCents())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Grant.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Grant.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			parsedOperation.Grant = &ledger.BatchGrantOperation{
				Amount:           amount,
				IdempotencyKey:   idem,
				ExpiresAtUnixUTC: operationValue.Grant.GetExpiresAtUnixUtc(),
				Metadata:         metadata,
			}
		case *creditv1.BatchOperation_Spend:
			if operationValue.Spend == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			amount, err := ledger.NewPositiveAmountCents(operationValue.Spend.GetAmountCents())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Spend.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Spend.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			parsedOperation.Spend = &ledger.BatchSpendOperation{
				Amount:         amount,
				IdempotencyKey: idem,
				Metadata:       metadata,
			}
		case *creditv1.BatchOperation_Reserve:
			if operationValue.Reserve == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			amount, err := ledger.NewPositiveAmountCents(operationValue.Reserve.GetAmountCents())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			reservationID, err := ledger.NewReservationID(operationValue.Reserve.GetReservationId())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Reserve.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Reserve.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			parsedOperation.Reserve = &ledger.BatchReserveOperation{
				Amount:           amount,
				ReservationID:    reservationID,
				IdempotencyKey:   idem,
				ExpiresAtUnixUTC: operationValue.Reserve.GetExpiresAtUnixUtc(),
				Metadata:         metadata,
			}
		case *creditv1.BatchOperation_Capture:
			if operationValue.Capture == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			reservationID, err := ledger.NewReservationID(operationValue.Capture.GetReservationId())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Capture.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			amount, err := ledger.NewPositiveAmountCents(operationValue.Capture.GetAmountCents())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Capture.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			parsedOperation.Capture = &ledger.BatchCaptureOperation{
				ReservationID:  reservationID,
				IdempotencyKey: idem,
				Amount:         amount,
				Metadata:       metadata,
			}
		case *creditv1.BatchOperation_Release:
			if operationValue.Release == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			reservationID, err := ledger.NewReservationID(operationValue.Release.GetReservationId())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Release.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Release.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			parsedOperation.Release = &ledger.BatchReleaseOperation{
				ReservationID:  reservationID,
				IdempotencyKey: idem,
				Metadata:       metadata,
			}
		case *creditv1.BatchOperation_Refund:
			if operationValue.Refund == nil {
				return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
			}
			amount, err := ledger.NewPositiveAmountCents(operationValue.Refund.GetAmountCents())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			idem, err := ledger.NewIdempotencyKey(operationValue.Refund.GetIdempotencyKey())
			if err != nil {
				return nil, mapToGRPCError(err)
			}
			metadata, err := ledger.NewMetadataJSON(operationValue.Refund.GetMetadataJson())
			if err != nil {
				return nil, mapToGRPCError(err)
			}

			var originalEntryID *ledger.EntryID
			var originalIdempotencyKey *ledger.IdempotencyKey
			if operationValue.Refund.GetOriginalEntryId() != "" {
				parsedOriginalEntryID, err := ledger.NewEntryID(operationValue.Refund.GetOriginalEntryId())
				if err != nil {
					return nil, mapToGRPCError(err)
				}
				originalEntryID = &parsedOriginalEntryID
			} else if operationValue.Refund.GetOriginalIdempotencyKey() != "" {
				parsedOriginalIdempotencyKey, err := ledger.NewIdempotencyKey(operationValue.Refund.GetOriginalIdempotencyKey())
				if err != nil {
					return nil, mapToGRPCError(err)
				}
				originalIdempotencyKey = &parsedOriginalIdempotencyKey
			} else {
				return nil, status.Error(codes.InvalidArgument, errorMissingRefundOriginal)
			}

			parsedOperation.Refund = &ledger.BatchRefundOperation{
				OriginalEntryID:        originalEntryID,
				OriginalIdempotencyKey: originalIdempotencyKey,
				Amount:                 amount,
				IdempotencyKey:         idem,
				Metadata:               metadata,
			}
		default:
			return nil, status.Error(codes.InvalidArgument, errorMissingBatchOperation)
		}

		operations[operationIndex] = parsedOperation
	}

	results, operationError := service.creditService.Batch(ctx, tenantID, userID, ledgerID, operations, request.GetAtomic())
	if operationError != nil {
		return nil, mapToGRPCError(operationError)
	}

	response := &creditv1.BatchResponse{Results: make([]*creditv1.BatchOperationResult, len(results))}
	for resultIndex, result := range results {
		resultMessage := &creditv1.BatchOperationResult{OperationId: result.OperationID}

		if result.Duplicate {
			resultMessage.Ok = true
			resultMessage.Duplicate = true
			response.Results[resultIndex] = resultMessage
			continue
		}

		if result.RolledBack {
			resultMessage.Ok = false
			resultMessage.ErrorCode = errorRolledBack
			resultMessage.ErrorMessage = errorRolledBack
			response.Results[resultIndex] = resultMessage
			continue
		}

		if result.Error != nil {
			resultMessage.Ok = false
			resultMessage.ErrorCode = mapToBatchErrorCode(result.Error)
			resultMessage.ErrorMessage = result.Error.Error()
			response.Results[resultIndex] = resultMessage
			continue
		}

		if result.Entry == nil {
			resultMessage.Ok = false
			resultMessage.ErrorCode = errorInternal
			resultMessage.ErrorMessage = errorInternal
			response.Results[resultIndex] = resultMessage
			continue
		}

		resultMessage.Ok = true
		resultMessage.EntryId = result.Entry.EntryID().String()
		resultMessage.CreatedUnixUtc = result.Entry.CreatedUnixUTC()
		response.Results[resultIndex] = resultMessage
	}

	return response, nil
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
	entryTypes := make([]ledger.EntryType, 0, len(request.GetTypes()))
	for _, entryTypeValue := range request.GetTypes() {
		parsedEntryType, err := ledger.ParseEntryType(entryTypeValue)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, errorInvalidEntryType)
		}
		entryTypes = append(entryTypes, parsedEntryType)
	}

	var reservationID *ledger.ReservationID
	if request.GetReservationId() != "" {
		parsedReservationID, err := ledger.NewReservationID(request.GetReservationId())
		if err != nil {
			return nil, mapToGRPCError(err)
		}
		reservationID = &parsedReservationID
	}

	var idempotencyKeyPrefix *ledger.IdempotencyKey
	if request.GetIdempotencyKeyPrefix() != "" {
		parsedIdempotencyKey, err := ledger.NewIdempotencyKey(request.GetIdempotencyKeyPrefix())
		if err != nil {
			return nil, mapToGRPCError(err)
		}
		idempotencyKeyPrefix = &parsedIdempotencyKey
	}

	entries, operationError := service.creditService.ListEntries(ctx, tenantID, userID, ledgerID, before, int(limit), ledger.ListEntriesFilter{
		Types:                entryTypes,
		ReservationID:        reservationID,
		IdempotencyKeyPrefix: idempotencyKeyPrefix,
	})
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
		refundOfEntryIDValue := ""
		refundOfEntryID, hasRefundOf := entryRecord.RefundOfEntryID()
		if hasRefundOf {
			refundOfEntryIDValue = refundOfEntryID.String()
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
			RefundOfEntryId:  refundOfEntryIDValue,
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

func mapToBatchErrorCode(source error) string {
	if errors.Is(source, ledger.ErrInvalidUserID) {
		return errorInvalidUserID
	}
	if errors.Is(source, ledger.ErrInvalidLedgerID) {
		return errorInvalidLedgerID
	}
	if errors.Is(source, ledger.ErrInvalidTenantID) {
		return errorInvalidTenantID
	}
	if errors.Is(source, ledger.ErrInvalidReservationID) {
		return errorInvalidReservationID
	}
	if errors.Is(source, ledger.ErrInvalidEntryID) {
		return errorInvalidEntryID
	}
	if errors.Is(source, ledger.ErrInvalidIdempotencyKey) {
		return errorInvalidIdempotencyKey
	}
	if errors.Is(source, ledger.ErrInvalidAmountCents) {
		return errorInvalidAmount
	}
	if errors.Is(source, ledger.ErrInvalidMetadataJSON) {
		return errorInvalidMetadata
	}
	if errors.Is(source, ledger.ErrInvalidEntryType) {
		return errorInvalidEntryType
	}
	if errors.Is(source, ledger.ErrInsufficientFunds) {
		return errorInsufficientFunds
	}
	if errors.Is(source, ledger.ErrUnknownReservation) {
		return errorUnknownReservation
	}
	if errors.Is(source, ledger.ErrUnknownEntry) {
		return errorUnknownEntry
	}
	if errors.Is(source, ledger.ErrDuplicateIdempotencyKey) {
		return errorDuplicateIdempotencyKey
	}
	if errors.Is(source, ledger.ErrIdempotencyKeyConflict) {
		return errorDuplicateIdempotencyKey
	}
	if errors.Is(source, ledger.ErrReservationExists) {
		return errorReservationExists
	}
	if errors.Is(source, ledger.ErrReservationClosed) {
		return errorReservationClosed
	}
	if errors.Is(source, ledger.ErrInvalidRefundOriginal) {
		return errorInvalidRefundOriginal
	}
	if errors.Is(source, ledger.ErrRefundExceedsDebit) {
		return errorRefundExceedsDebit
	}

	var operationError ledger.OperationError
	if errors.As(source, &operationError) {
		return operationError.Operation() + "." + operationError.Subject() + "." + operationError.Code()
	}
	return errorInternal
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
	if errors.Is(source, ledger.ErrInvalidEntryID) {
		return status.Error(codes.InvalidArgument, errorInvalidEntryID)
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
	if errors.Is(source, ledger.ErrInvalidEntryType) {
		return status.Error(codes.InvalidArgument, errorInvalidEntryType)
	}
	if errors.Is(source, ledger.ErrInsufficientFunds) {
		return status.Error(codes.FailedPrecondition, errorInsufficientFunds)
	}
	if errors.Is(source, ledger.ErrUnknownReservation) {
		return status.Error(codes.NotFound, errorUnknownReservation)
	}
	if errors.Is(source, ledger.ErrUnknownEntry) {
		return status.Error(codes.NotFound, errorUnknownEntry)
	}
	if errors.Is(source, ledger.ErrDuplicateIdempotencyKey) {
		return status.Error(codes.AlreadyExists, errorDuplicateIdempotencyKey)
	}
	if errors.Is(source, ledger.ErrIdempotencyKeyConflict) {
		return status.Error(codes.AlreadyExists, errorDuplicateIdempotencyKey)
	}
	if errors.Is(source, ledger.ErrReservationExists) {
		return status.Error(codes.AlreadyExists, errorReservationExists)
	}
	if errors.Is(source, ledger.ErrReservationClosed) {
		return status.Error(codes.FailedPrecondition, errorReservationClosed)
	}
	if errors.Is(source, ledger.ErrInvalidRefundOriginal) {
		return status.Error(codes.FailedPrecondition, errorInvalidRefundOriginal)
	}
	if errors.Is(source, ledger.ErrRefundExceedsDebit) {
		return status.Error(codes.FailedPrecondition, errorRefundExceedsDebit)
	}
	return status.Error(codes.Internal, source.Error())
}
