package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/glebarez/sqlite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func TestNormalizeListLimit(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		input     int32
		wantValue int32
		wantErr   bool
	}{
		{name: "default when zero", input: 0, wantValue: defaultListEntriesLimit},
		{name: "default when negative", input: -10, wantValue: defaultListEntriesLimit},
		{name: "accept within range", input: 1, wantValue: 1},
		{name: "reject too large", input: maxListEntriesLimit + 1, wantErr: true},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			value, err := normalizeListLimit(testCase.input)
			if testCase.wantErr {
				if err == nil {
					test.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				test.Fatalf("unexpected error: %v", err)
			}
			if value != testCase.wantValue {
				test.Fatalf("expected %d, got %d", testCase.wantValue, value)
			}
		})
	}
}

func TestMapToGRPCError(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name        string
		input       error
		wantCode    codes.Code
		wantMessage string
	}{
		{name: "invalid user id", input: ledger.ErrInvalidUserID, wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID},
		{name: "invalid ledger id", input: ledger.ErrInvalidLedgerID, wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID},
		{name: "invalid tenant id", input: ledger.ErrInvalidTenantID, wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID},
		{name: "invalid reservation id", input: ledger.ErrInvalidReservationID, wantCode: codes.InvalidArgument, wantMessage: errorInvalidReservationID},
		{name: "invalid entry id", input: ledger.ErrInvalidEntryID, wantCode: codes.InvalidArgument, wantMessage: errorInvalidEntryID},
		{name: "invalid idempotency key", input: ledger.ErrInvalidIdempotencyKey, wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey},
		{name: "invalid amount", input: ledger.ErrInvalidAmountCents, wantCode: codes.InvalidArgument, wantMessage: errorInvalidAmount},
		{name: "invalid metadata", input: ledger.ErrInvalidMetadataJSON, wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata},
		{name: "invalid entry type", input: ledger.ErrInvalidEntryType, wantCode: codes.InvalidArgument, wantMessage: errorInvalidEntryType},
		{name: "insufficient funds", input: ledger.ErrInsufficientFunds, wantCode: codes.FailedPrecondition, wantMessage: errorInsufficientFunds},
		{name: "unknown reservation", input: ledger.ErrUnknownReservation, wantCode: codes.NotFound, wantMessage: errorUnknownReservation},
		{name: "unknown entry", input: ledger.ErrUnknownEntry, wantCode: codes.NotFound, wantMessage: errorUnknownEntry},
		{name: "duplicate idempotency", input: ledger.ErrDuplicateIdempotencyKey, wantCode: codes.AlreadyExists, wantMessage: errorDuplicateIdempotencyKey},
		{name: "reservation exists", input: ledger.ErrReservationExists, wantCode: codes.AlreadyExists, wantMessage: errorReservationExists},
		{name: "reservation closed", input: ledger.ErrReservationClosed, wantCode: codes.FailedPrecondition, wantMessage: errorReservationClosed},
		{name: "invalid refund original", input: ledger.ErrInvalidRefundOriginal, wantCode: codes.FailedPrecondition, wantMessage: errorInvalidRefundOriginal},
		{name: "refund exceeds debit", input: ledger.ErrRefundExceedsDebit, wantCode: codes.FailedPrecondition, wantMessage: errorRefundExceedsDebit},
		{name: "fallback", input: errors.New("boom"), wantCode: codes.Internal, wantMessage: "boom"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			err := mapToGRPCError(testCase.input)
			gotStatus, ok := status.FromError(err)
			if !ok {
				test.Fatalf("expected grpc status error")
			}
			if gotStatus.Code() != testCase.wantCode {
				test.Fatalf("expected code %v, got %v", testCase.wantCode, gotStatus.Code())
			}
			if gotStatus.Message() != testCase.wantMessage {
				test.Fatalf("expected message %q, got %q", testCase.wantMessage, gotStatus.Message())
			}
		})
	}
}

func TestMapToBatchErrorCode(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name     string
		input    error
		wantCode string
	}{
		{name: "invalid user id", input: ledger.ErrInvalidUserID, wantCode: errorInvalidUserID},
		{name: "invalid ledger id", input: ledger.ErrInvalidLedgerID, wantCode: errorInvalidLedgerID},
		{name: "invalid tenant id", input: ledger.ErrInvalidTenantID, wantCode: errorInvalidTenantID},
		{name: "invalid reservation id", input: ledger.ErrInvalidReservationID, wantCode: errorInvalidReservationID},
		{name: "invalid entry id", input: ledger.ErrInvalidEntryID, wantCode: errorInvalidEntryID},
		{name: "invalid idempotency key", input: ledger.ErrInvalidIdempotencyKey, wantCode: errorInvalidIdempotencyKey},
		{name: "invalid amount", input: ledger.ErrInvalidAmountCents, wantCode: errorInvalidAmount},
		{name: "invalid metadata", input: ledger.ErrInvalidMetadataJSON, wantCode: errorInvalidMetadata},
		{name: "invalid entry type", input: ledger.ErrInvalidEntryType, wantCode: errorInvalidEntryType},
		{name: "insufficient funds", input: ledger.ErrInsufficientFunds, wantCode: errorInsufficientFunds},
		{name: "unknown reservation", input: ledger.ErrUnknownReservation, wantCode: errorUnknownReservation},
		{name: "unknown entry", input: ledger.ErrUnknownEntry, wantCode: errorUnknownEntry},
		{name: "duplicate idempotency", input: ledger.ErrDuplicateIdempotencyKey, wantCode: errorDuplicateIdempotencyKey},
		{name: "reservation exists", input: ledger.ErrReservationExists, wantCode: errorReservationExists},
		{name: "reservation closed", input: ledger.ErrReservationClosed, wantCode: errorReservationClosed},
		{name: "invalid refund original", input: ledger.ErrInvalidRefundOriginal, wantCode: errorInvalidRefundOriginal},
		{name: "refund exceeds debit", input: ledger.ErrRefundExceedsDebit, wantCode: errorRefundExceedsDebit},
		{name: "operation error", input: ledger.WrapError("store", "entry", "insert", errors.New("boom")), wantCode: "store.entry.insert"},
		{name: "fallback", input: errors.New("boom"), wantCode: errorInternal},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			code := mapToBatchErrorCode(testCase.input)
			if code != testCase.wantCode {
				test.Fatalf("expected %q, got %q", testCase.wantCode, code)
			}
		})
	}
}

func TestCreditServiceServerFlow(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 0 || balanceResponse.GetAvailableCents() != 0 {
		test.Fatalf("expected zero balance, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	grantResponse, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:           userID,
		TenantId:         tenantID,
		LedgerId:         ledgerID,
		AmountCents:      1000,
		IdempotencyKey:   "grant-1",
		ExpiresAtUnixUtc: 0,
		MetadataJson:     "{}",
	})
	if err != nil {
		test.Fatalf("grant: %v", err)
	}
	if grantResponse.GetEntryId() == "" {
		test.Fatalf("expected grant entry id")
	}
	if grantResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", grantResponse.GetCreatedUnixUtc())
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance after grant: %v", err)
	}
	if balanceResponse.GetTotalCents() != 1000 || balanceResponse.GetAvailableCents() != 1000 {
		test.Fatalf("expected 1000/1000, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}
	if spendResponse.GetEntryId() == "" {
		test.Fatalf("expected spend entry id")
	}
	if spendResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", spendResponse.GetCreatedUnixUtc())
	}

	reserveResponse, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    300,
		ReservationId:  "order-1",
		IdempotencyKey: "reserve-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("reserve: %v", err)
	}
	if reserveResponse.GetEntryId() == "" {
		test.Fatalf("expected reserve entry id")
	}
	if reserveResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", reserveResponse.GetCreatedUnixUtc())
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance after reserve: %v", err)
	}
	if balanceResponse.GetTotalCents() != 800 || balanceResponse.GetAvailableCents() != 500 {
		test.Fatalf("expected 800/500, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	captureResponse, err := server.Capture(ctx, &creditv1.CaptureRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-1",
		IdempotencyKey: "capture-1",
		AmountCents:    300,
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("capture: %v", err)
	}
	if captureResponse.GetEntryId() == "" {
		test.Fatalf("expected capture debit entry id")
	}
	if captureResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", captureResponse.GetCreatedUnixUtc())
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance after capture: %v", err)
	}
	if balanceResponse.GetTotalCents() != 500 || balanceResponse.GetAvailableCents() != 500 {
		test.Fatalf("expected 500/500, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	reserveSecondResponse, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		ReservationId:  "order-2",
		IdempotencyKey: "reserve-2",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("reserve order-2: %v", err)
	}
	if reserveSecondResponse.GetEntryId() == "" {
		test.Fatalf("expected reserve-2 entry id")
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance after reserve order-2: %v", err)
	}
	if balanceResponse.GetTotalCents() != 500 || balanceResponse.GetAvailableCents() != 400 {
		test.Fatalf("expected 500/400, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
	releaseResponse, err := server.Release(ctx, &creditv1.ReleaseRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-2",
		IdempotencyKey: "release-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("release order-2: %v", err)
	}
	if releaseResponse.GetEntryId() == "" {
		test.Fatalf("expected release entry id")
	}
	if releaseResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", releaseResponse.GetCreatedUnixUtc())
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance after release order-2: %v", err)
	}
	if balanceResponse.GetTotalCents() != 500 || balanceResponse.GetAvailableCents() != 500 {
		test.Fatalf("expected 500/500 after release, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	listResponse, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:        userID,
		TenantId:      tenantID,
		LedgerId:      ledgerID,
		BeforeUnixUtc: 0,
		Limit:         0,
	})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(listResponse.GetEntries()) == 0 {
		test.Fatalf("expected entries, got none")
	}

	_, err = server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:        userID,
		TenantId:      tenantID,
		LedgerId:      ledgerID,
		BeforeUnixUtc: 0,
		Limit:         maxListEntriesLimit + 1,
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidListLimit {
		test.Fatalf("expected %q, got %q", errorInvalidListLimit, status.Convert(err).Message())
	}

	_, err = server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    9999,
		IdempotencyKey: "spend-2",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.FailedPrecondition {
		test.Fatalf("expected failed precondition, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInsufficientFunds {
		test.Fatalf("expected %q, got %q", errorInsufficientFunds, status.Convert(err).Message())
	}

	_, err = server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    0,
		IdempotencyKey: "grant-2",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidAmount {
		test.Fatalf("expected %q, got %q", errorInvalidAmount, status.Convert(err).Message())
	}

	_, err = server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.AlreadyExists {
		test.Fatalf("expected already exists, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorDuplicateIdempotencyKey {
		test.Fatalf("expected %q, got %q", errorDuplicateIdempotencyKey, status.Convert(err).Message())
	}

	_, err = server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		ReservationId:  "order-2",
		IdempotencyKey: "reserve-3",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.AlreadyExists {
		test.Fatalf("expected already exists, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorReservationExists {
		test.Fatalf("expected %q, got %q", errorReservationExists, status.Convert(err).Message())
	}

	if _, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		ReservationId:  "order-3",
		IdempotencyKey: "reserve-4",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("reserve order-3: %v", err)
	}

	_, err = server.Capture(ctx, &creditv1.CaptureRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-3",
		IdempotencyKey: "capture-2",
		AmountCents:    99,
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidAmount {
		test.Fatalf("expected %q, got %q", errorInvalidAmount, status.Convert(err).Message())
	}

	_, err = server.Release(ctx, &creditv1.ReleaseRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "unknown",
		IdempotencyKey: "release-2",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.NotFound {
		test.Fatalf("expected not found, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorUnknownReservation {
		test.Fatalf("expected %q, got %q", errorUnknownReservation, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundSpendFlow(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}

	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	refundResponse, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
		AmountCents:    50,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	if refundResponse.GetEntryId() == "" {
		test.Fatalf("expected refund entry id")
	}
	if refundResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", refundResponse.GetCreatedUnixUtc())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 850 || balanceResponse.GetAvailableCents() != 850 {
		test.Fatalf("expected 850/850, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	listRefunds, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
		Types:    []string{"refund"},
		Limit:    10,
	})
	if err != nil {
		test.Fatalf("list refunds: %v", err)
	}
	if len(listRefunds.GetEntries()) != 1 {
		test.Fatalf("expected one refund entry, got %d", len(listRefunds.GetEntries()))
	}
	if listRefunds.GetEntries()[0].GetRefundOfEntryId() != spendResponse.GetEntryId() {
		test.Fatalf("expected refund_of_entry_id %q, got %q", spendResponse.GetEntryId(), listRefunds.GetEntries()[0].GetRefundOfEntryId())
	}
}

func TestCreditServiceServerRefundByOriginalIdempotencyKeyFlow(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if _, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("spend: %v", err)
	}

	refundResponse, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalIdempotencyKey{OriginalIdempotencyKey: "spend-1"},
		AmountCents:    50,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	if refundResponse.GetEntryId() == "" {
		test.Fatalf("expected refund entry id")
	}
	if refundResponse.GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", refundResponse.GetCreatedUnixUtc())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 850 || balanceResponse.GetAvailableCents() != 850 {
		test.Fatalf("expected 850/850, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerRefundValidationInvalidOriginalEntryID(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	_, err = server.Refund(context.Background(), &creditv1.RefundRequest{
		UserId:         "user",
		TenantId:       "default",
		LedgerId:       "default",
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: " "},
		AmountCents:    1,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidEntryID {
		test.Fatalf("expected %q, got %q", errorInvalidEntryID, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundValidationInvalidOriginalIdempotencyKey(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	_, err = server.Refund(context.Background(), &creditv1.RefundRequest{
		UserId:         "user",
		TenantId:       "default",
		LedgerId:       "default",
		Original:       &creditv1.RefundRequest_OriginalIdempotencyKey{OriginalIdempotencyKey: " "},
		AmountCents:    1,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidIdempotencyKey {
		test.Fatalf("expected %q, got %q", errorInvalidIdempotencyKey, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundUnknownEntryRejected(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	_, err = server.Refund(context.Background(), &creditv1.RefundRequest{
		UserId:         "user",
		TenantId:       "default",
		LedgerId:       "default",
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "missing-entry"},
		AmountCents:    1,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.NotFound {
		test.Fatalf("expected not found, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorUnknownEntry {
		test.Fatalf("expected %q, got %q", errorUnknownEntry, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundRejectsNonDebitOriginal(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	grantResponse, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("grant: %v", err)
	}

	_, err = server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: grantResponse.GetEntryId()},
		AmountCents:    1,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.FailedPrecondition {
		test.Fatalf("expected failed precondition, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidRefundOriginal {
		test.Fatalf("expected %q, got %q", errorInvalidRefundOriginal, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundCaptureDebitFlow(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if _, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    300,
		ReservationId:  "order-1",
		IdempotencyKey: "reserve-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	captureResponse, err := server.Capture(ctx, &creditv1.CaptureRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-1",
		IdempotencyKey: "capture-1",
		AmountCents:    300,
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("capture: %v", err)
	}

	if _, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: captureResponse.GetEntryId()},
		AmountCents:    300,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("refund: %v", err)
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 1000 || balanceResponse.GetAvailableCents() != 1000 {
		test.Fatalf("expected 1000/1000, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerRefundOverRefundRejected(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}
	if _, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
		AmountCents:    80,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("refund: %v", err)
	}

	_, err = server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
		AmountCents:    30,
		IdempotencyKey: "refund-2",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.FailedPrecondition {
		test.Fatalf("expected failed precondition, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorRefundExceedsDebit {
		test.Fatalf("expected %q, got %q", errorRefundExceedsDebit, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundDuplicateIdempotencyNoop(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}

	firstRefund, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
		AmountCents:    50,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("refund: %v", err)
	}
	secondRefund, err := server.Refund(ctx, &creditv1.RefundRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
		AmountCents:    50,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("refund duplicate: %v", err)
	}
	if firstRefund.GetEntryId() != secondRefund.GetEntryId() {
		test.Fatalf("expected same refund entry id %q, got %q", firstRefund.GetEntryId(), secondRefund.GetEntryId())
	}

	listRefunds, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
		Types:    []string{"refund"},
		Limit:    10,
	})
	if err != nil {
		test.Fatalf("list refunds: %v", err)
	}
	if len(listRefunds.GetEntries()) != 1 {
		test.Fatalf("expected one refund entry, got %d", len(listRefunds.GetEntries()))
	}
}

func TestCreditServiceServerRefundValidationMissingOriginal(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	_, err = server.Refund(context.Background(), &creditv1.RefundRequest{
		UserId:         "user",
		TenantId:       "default",
		LedgerId:       "default",
		AmountCents:    1,
		IdempotencyKey: "refund-1",
		MetadataJson:   "{}",
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorMissingRefundOriginal {
		test.Fatalf("expected %q, got %q", errorMissingRefundOriginal, status.Convert(err).Message())
	}
}

func TestCreditServiceServerRefundValidationErrors(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)
	ctx := context.Background()

	testCases := []struct {
		name        string
		invoke      func() error
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name: "invalid user id",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "",
					TenantId:       "default",
					LedgerId:       "default",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    1,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidUserID,
		},
		{
			name: "invalid ledger id",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "user",
					TenantId:       "default",
					LedgerId:       "",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    1,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidLedgerID,
		},
		{
			name: "invalid tenant id",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "user",
					TenantId:       "",
					LedgerId:       "default",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    1,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidTenantID,
		},
		{
			name: "invalid amount",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "user",
					TenantId:       "default",
					LedgerId:       "default",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    0,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidAmount,
		},
		{
			name: "invalid idempotency key",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "user",
					TenantId:       "default",
					LedgerId:       "default",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    1,
					IdempotencyKey: " ",
					MetadataJson:   "{}",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "invalid metadata",
			invoke: func() error {
				_, err := server.Refund(ctx, &creditv1.RefundRequest{
					UserId:         "user",
					TenantId:       "default",
					LedgerId:       "default",
					Original:       &creditv1.RefundRequest_OriginalEntryId{OriginalEntryId: "entry-1"},
					AmountCents:    1,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{",
				})
				return err
			},
			wantCode:    codes.InvalidArgument,
			wantMessage: errorInvalidMetadata,
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			err := testCase.invoke()
			if status.Code(err) != testCase.wantCode {
				test.Fatalf("expected code %v, got %v", testCase.wantCode, status.Code(err))
			}
			if status.Convert(err).Message() != testCase.wantMessage {
				test.Fatalf("expected message %q, got %q", testCase.wantMessage, status.Convert(err).Message())
			}
		})
	}
}

func TestCreditServiceServerBatchValidationErrors(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)
	ctx := context.Background()

	_, err = server.Batch(ctx, &creditv1.BatchRequest{})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidAccountContext {
		test.Fatalf("expected %q, got %q", errorInvalidAccountContext, status.Convert(err).Message())
	}

	_, err = server.Batch(ctx, &creditv1.BatchRequest{
		Account:    &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: "default"},
		Operations: []*creditv1.BatchOperation{{OperationId: "   ", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 1, IdempotencyKey: "idem", MetadataJson: "{}"}}}},
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorInvalidOperationID {
		test.Fatalf("expected %q, got %q", errorInvalidOperationID, status.Convert(err).Message())
	}

	_, err = server.Batch(ctx, &creditv1.BatchRequest{
		Account:    &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: "default"},
		Operations: []*creditv1.BatchOperation{{OperationId: "op-1"}},
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorMissingBatchOperation {
		test.Fatalf("expected %q, got %q", errorMissingBatchOperation, status.Convert(err).Message())
	}

	_, err = server.Batch(ctx, &creditv1.BatchRequest{
		Account:    &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: "default"},
		Operations: []*creditv1.BatchOperation{{OperationId: "op-1", Operation: &creditv1.BatchOperation_Grant{Grant: nil}}},
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorMissingBatchOperation {
		test.Fatalf("expected %q, got %q", errorMissingBatchOperation, status.Convert(err).Message())
	}
}

func TestCreditServiceServerBatchAccountContextValidation(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)
	ctx := context.Background()

	testCases := []struct {
		name        string
		account     *creditv1.AccountContext
		wantMessage string
	}{
		{name: "invalid user id", account: &creditv1.AccountContext{UserId: "", TenantId: "default", LedgerId: "default"}, wantMessage: errorInvalidUserID},
		{name: "invalid ledger id", account: &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: ""}, wantMessage: errorInvalidLedgerID},
		{name: "invalid tenant id", account: &creditv1.AccountContext{UserId: "user", TenantId: "", LedgerId: "default"}, wantMessage: errorInvalidTenantID},
	}
	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := server.Batch(ctx, &creditv1.BatchRequest{
				Account:    testCase.account,
				Operations: []*creditv1.BatchOperation{{OperationId: "grant-1", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 1, IdempotencyKey: "grant-1", MetadataJson: "{}"}}}},
			})
			if status.Code(err) != codes.InvalidArgument {
				test.Fatalf("expected invalid argument, got %v", status.Code(err))
			}
			if status.Convert(err).Message() != testCase.wantMessage {
				test.Fatalf("expected %q, got %q", testCase.wantMessage, status.Convert(err).Message())
			}
		})
	}
}

func TestCreditServiceServerBatchOperationValidationErrors(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)
	ctx := context.Background()
	account := &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: "default"}

	testCases := []struct {
		name        string
		operation   *creditv1.BatchOperation
		wantMessage string
	}{
		{
			name:        "grant invalid amount",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 0, IdempotencyKey: "grant-1", MetadataJson: "{}"}}},
			wantMessage: errorInvalidAmount,
		},
		{
			name:        "grant invalid idempotency key",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 1, IdempotencyKey: "", MetadataJson: "{}"}}},
			wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name:        "grant invalid metadata",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 1, IdempotencyKey: "grant-1", MetadataJson: "{"}}},
			wantMessage: errorInvalidMetadata,
		},
		{
			name:        "spend invalid amount",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{AmountCents: 0, IdempotencyKey: "spend-1", MetadataJson: "{}"}}},
			wantMessage: errorInvalidAmount,
		},
		{
			name:        "reserve invalid reservation id",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Reserve{Reserve: &creditv1.BatchReserveOp{AmountCents: 1, ReservationId: "", IdempotencyKey: "reserve-1", MetadataJson: "{}"}}},
			wantMessage: errorInvalidReservationID,
		},
		{
			name:        "capture invalid amount",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Capture{Capture: &creditv1.BatchCaptureOp{ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 0, MetadataJson: "{}"}}},
			wantMessage: errorInvalidAmount,
		},
		{
			name:        "release invalid reservation id",
			operation:   &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Release{Release: &creditv1.BatchReleaseOp{ReservationId: "", IdempotencyKey: "release-1", MetadataJson: "{}"}}},
			wantMessage: errorInvalidReservationID,
		},
		{
			name: "refund missing original",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				AmountCents:    1,
				IdempotencyKey: "refund-1",
				MetadataJson:   "{}",
			}}},
			wantMessage: errorMissingRefundOriginal,
		},
		{
			name: "refund invalid amount",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: "entry-1"},
				AmountCents:    0,
				IdempotencyKey: "refund-1",
				MetadataJson:   "{}",
			}}},
			wantMessage: errorInvalidAmount,
		},
		{
			name: "refund invalid idempotency key",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: "entry-1"},
				AmountCents:    1,
				IdempotencyKey: " ",
				MetadataJson:   "{}",
			}}},
			wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "refund invalid metadata",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: "entry-1"},
				AmountCents:    1,
				IdempotencyKey: "refund-1",
				MetadataJson:   "{",
			}}},
			wantMessage: errorInvalidMetadata,
		},
		{
			name: "refund invalid original entry id",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: " "},
				AmountCents:    1,
				IdempotencyKey: "refund-1",
				MetadataJson:   "{}",
			}}},
			wantMessage: errorInvalidEntryID,
		},
		{
			name: "refund invalid original idempotency key",
			operation: &creditv1.BatchOperation{OperationId: "op-1", Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
				Original:       &creditv1.BatchRefundOp_OriginalIdempotencyKey{OriginalIdempotencyKey: " "},
				AmountCents:    1,
				IdempotencyKey: "refund-1",
				MetadataJson:   "{}",
			}}},
			wantMessage: errorInvalidIdempotencyKey,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			_, err := server.Batch(ctx, &creditv1.BatchRequest{
				Account:    account,
				Operations: []*creditv1.BatchOperation{testCase.operation},
			})
			if status.Code(err) != codes.InvalidArgument {
				test.Fatalf("expected invalid argument, got %v", status.Code(err))
			}
			if status.Convert(err).Message() != testCase.wantMessage {
				test.Fatalf("expected %q, got %q", testCase.wantMessage, status.Convert(err).Message())
			}
		})
	}
}

func TestCreditServiceServerBatchMapsServiceErrors(test *testing.T) {
	test.Parallel()
	clock := func() int64 { return 1700000000 }
	service, err := ledger.NewService(&alwaysErrorStore{err: errors.New("boom")}, clock)
	if err != nil {
		test.Fatalf("service init: %v", err)
	}
	server := NewCreditServiceServer(service)
	_, err = server.Batch(context.Background(), &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: "user", TenantId: "default", LedgerId: "default"},
		Operations: []*creditv1.BatchOperation{
			{OperationId: "grant-1", Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{AmountCents: 1, IdempotencyKey: "grant-1", MetadataJson: "{}"}}},
		},
	})
	if status.Code(err) != codes.Internal {
		test.Fatalf("expected internal, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != "boom" {
		test.Fatalf("expected boom, got %q", status.Convert(err).Message())
	}
}

func TestCreditServiceServerBatchSupportsReserveCaptureAndRelease(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-1",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    1000,
					IdempotencyKey: "grant-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "reserve-1",
				Operation: &creditv1.BatchOperation_Reserve{Reserve: &creditv1.BatchReserveOp{
					AmountCents:    300,
					ReservationId:  "order-1",
					IdempotencyKey: "reserve-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "capture-1",
				Operation: &creditv1.BatchOperation_Capture{Capture: &creditv1.BatchCaptureOp{
					ReservationId:  "order-1",
					IdempotencyKey: "capture-1",
					AmountCents:    300,
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "reserve-2",
				Operation: &creditv1.BatchOperation_Reserve{Reserve: &creditv1.BatchReserveOp{
					AmountCents:    100,
					ReservationId:  "order-2",
					IdempotencyKey: "reserve-2",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "release-1",
				Operation: &creditv1.BatchOperation_Release{Release: &creditv1.BatchReleaseOp{
					ReservationId:  "order-2",
					IdempotencyKey: "release-1",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 5 {
		test.Fatalf("expected 5 results, got %d", len(results))
	}
	for resultIndex, result := range results {
		if !result.GetOk() || result.GetDuplicate() || result.GetEntryId() == "" {
			test.Fatalf("unexpected result[%d]: ok=%v dup=%v entry_id=%q code=%q", resultIndex, result.GetOk(), result.GetDuplicate(), result.GetEntryId(), result.GetErrorCode())
		}
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: userID, TenantId: tenantID, LedgerId: ledgerID})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 700 || balanceResponse.GetAvailableCents() != 700 {
		test.Fatalf("expected 700/700, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchSupportsRefundByOriginalIdempotencyKey(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-1",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    1000,
					IdempotencyKey: "grant-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-1",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    200,
					IdempotencyKey: "spend-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "refund-1",
				Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
					Original:       &creditv1.BatchRefundOp_OriginalIdempotencyKey{OriginalIdempotencyKey: "spend-1"},
					AmountCents:    50,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 3 {
		test.Fatalf("expected 3 results, got %d", len(results))
	}
	for resultIndex, result := range results {
		if !result.GetOk() || result.GetDuplicate() || (result.GetOperationId() != "grant-1" && result.GetEntryId() == "") {
			test.Fatalf("unexpected result[%d]: ok=%v dup=%v entry_id=%q code=%q", resultIndex, result.GetOk(), result.GetDuplicate(), result.GetEntryId(), result.GetErrorCode())
		}
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: userID, TenantId: tenantID, LedgerId: ledgerID})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 850 || balanceResponse.GetAvailableCents() != 850 {
		test.Fatalf("expected 850/850, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchSupportsRefundByOriginalEntryID(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}
	if spendResponse.GetEntryId() == "" {
		test.Fatalf("expected spend entry id")
	}

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "refund-1",
				Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
					Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
					AmountCents:    50,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].GetOk() || results[0].GetDuplicate() || results[0].GetEntryId() == "" {
		test.Fatalf("unexpected result: ok=%v dup=%v entry_id=%q code=%q", results[0].GetOk(), results[0].GetDuplicate(), results[0].GetEntryId(), results[0].GetErrorCode())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: userID, TenantId: tenantID, LedgerId: ledgerID})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 850 || balanceResponse.GetAvailableCents() != 850 {
		test.Fatalf("expected 850/850, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchRefundRejectsIdempotencyKeyConflictWithNonRefundEntry(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "collision-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	spendResponse, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	})
	if err != nil {
		test.Fatalf("spend: %v", err)
	}
	if spendResponse.GetEntryId() == "" {
		test.Fatalf("expected spend entry id")
	}

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "refund-1",
				Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
					Original:       &creditv1.BatchRefundOp_OriginalEntryId{OriginalEntryId: spendResponse.GetEntryId()},
					AmountCents:    50,
					IdempotencyKey: "collision-1",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 1 {
		test.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].GetOk() || results[0].GetDuplicate() || results[0].GetErrorCode() != errorDuplicateIdempotencyKey {
		test.Fatalf("expected idempotency conflict (code=%q), got ok=%v dup=%v code=%q", errorDuplicateIdempotencyKey, results[0].GetOk(), results[0].GetDuplicate(), results[0].GetErrorCode())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: userID, TenantId: tenantID, LedgerId: ledgerID})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 800 || balanceResponse.GetAvailableCents() != 800 {
		test.Fatalf("expected 800/800, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchRefundOverRefundRejected(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-1",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    1000,
					IdempotencyKey: "grant-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-1",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    100,
					IdempotencyKey: "spend-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "refund-1",
				Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
					Original:       &creditv1.BatchRefundOp_OriginalIdempotencyKey{OriginalIdempotencyKey: "spend-1"},
					AmountCents:    80,
					IdempotencyKey: "refund-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "refund-2",
				Operation: &creditv1.BatchOperation_Refund{Refund: &creditv1.BatchRefundOp{
					Original:       &creditv1.BatchRefundOp_OriginalIdempotencyKey{OriginalIdempotencyKey: "spend-1"},
					AmountCents:    30,
					IdempotencyKey: "refund-2",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 4 {
		test.Fatalf("expected 4 results, got %d", len(results))
	}
	if !results[2].GetOk() || results[2].GetEntryId() == "" {
		test.Fatalf("unexpected refund-1 result: ok=%v entry_id=%q code=%q", results[2].GetOk(), results[2].GetEntryId(), results[2].GetErrorCode())
	}
	if results[3].GetOk() || results[3].GetErrorCode() != errorRefundExceedsDebit {
		test.Fatalf("expected refund-2 to fail with %q, got ok=%v code=%q", errorRefundExceedsDebit, results[3].GetOk(), results[3].GetErrorCode())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: userID, TenantId: tenantID, LedgerId: ledgerID})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 980 || balanceResponse.GetAvailableCents() != 980 {
		test.Fatalf("expected 980/980, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerListEntriesAppliesFilters(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    1000,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if _, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    200,
		ReservationId:  "order-1",
		IdempotencyKey: "reserve-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	if _, err := server.Spend(ctx, &creditv1.SpendRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		IdempotencyKey: "spend-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("spend: %v", err)
	}

	listResponse, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:               userID,
		TenantId:             tenantID,
		LedgerId:             ledgerID,
		BeforeUnixUtc:        0,
		Limit:                10,
		Types:                []string{"hold"},
		ReservationId:        "order-1",
		IdempotencyKeyPrefix: "reserve",
	})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(listResponse.GetEntries()) != 1 {
		test.Fatalf("expected 1 entry, got %d", len(listResponse.GetEntries()))
	}
	entry := listResponse.GetEntries()[0]
	if entry.GetType() != "hold" {
		test.Fatalf("expected hold entry, got %q", entry.GetType())
	}
	if entry.GetReservationId() != "order-1" {
		test.Fatalf("expected reservation order-1, got %q", entry.GetReservationId())
	}
	if entry.GetIdempotencyKey() != "reserve-1" {
		test.Fatalf("expected idempotency reserve-1, got %q", entry.GetIdempotencyKey())
	}
}

func TestCreditServiceServerBatchBestEffortReturnsPerItemResults(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-1",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    1000,
					IdempotencyKey: "grant-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-1",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    200,
					IdempotencyKey: "spend-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-2",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    2000,
					IdempotencyKey: "spend-2",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "reserve-1",
				Operation: &creditv1.BatchOperation_Reserve{Reserve: &creditv1.BatchReserveOp{
					AmountCents:    300,
					ReservationId:  "order-1",
					IdempotencyKey: "reserve-1",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 4 {
		test.Fatalf("expected 4 results, got %d", len(results))
	}

	if !results[0].GetOk() || results[0].GetDuplicate() {
		test.Fatalf("expected grant ok without duplicate, got ok=%v dup=%v code=%q", results[0].GetOk(), results[0].GetDuplicate(), results[0].GetErrorCode())
	}
	if results[0].GetEntryId() == "" {
		test.Fatalf("expected grant entry id")
	}
	if results[0].GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", results[0].GetCreatedUnixUtc())
	}

	if !results[1].GetOk() || results[1].GetDuplicate() {
		test.Fatalf("expected spend ok without duplicate, got ok=%v dup=%v code=%q", results[1].GetOk(), results[1].GetDuplicate(), results[1].GetErrorCode())
	}
	if results[1].GetEntryId() == "" {
		test.Fatalf("expected spend entry id")
	}
	if results[1].GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", results[1].GetCreatedUnixUtc())
	}

	if results[2].GetOk() {
		test.Fatalf("expected spend-2 to fail")
	}
	if results[2].GetErrorCode() != errorInsufficientFunds {
		test.Fatalf("expected error code %q, got %q", errorInsufficientFunds, results[2].GetErrorCode())
	}

	if !results[3].GetOk() || results[3].GetDuplicate() {
		test.Fatalf("expected reserve ok without duplicate, got ok=%v dup=%v code=%q", results[3].GetOk(), results[3].GetDuplicate(), results[3].GetErrorCode())
	}
	if results[3].GetEntryId() == "" {
		test.Fatalf("expected reserve entry id")
	}
	if results[3].GetCreatedUnixUtc() != 1700000000 {
		test.Fatalf("expected created unix utc 1700000000, got %d", results[3].GetCreatedUnixUtc())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId: userID, TenantId: tenantID, LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 800 || balanceResponse.GetAvailableCents() != 500 {
		test.Fatalf("expected 800/500, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchTreatsDuplicateIdempotencyAsSuccess(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchRequest := &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  false,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-a",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    100,
					IdempotencyKey: "grant-a",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "grant-b",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    200,
					IdempotencyKey: "grant-b",
					MetadataJson:   "{}",
				}},
			},
		},
	}

	firstResponse, err := server.Batch(ctx, batchRequest)
	if err != nil {
		test.Fatalf("first batch: %v", err)
	}
	for _, result := range firstResponse.GetResults() {
		if !result.GetOk() || result.GetDuplicate() {
			test.Fatalf("expected ok without duplicate, got ok=%v dup=%v code=%q", result.GetOk(), result.GetDuplicate(), result.GetErrorCode())
		}
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId: userID, TenantId: tenantID, LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 300 || balanceResponse.GetAvailableCents() != 300 {
		test.Fatalf("expected 300/300, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	secondResponse, err := server.Batch(ctx, batchRequest)
	if err != nil {
		test.Fatalf("second batch: %v", err)
	}
	for _, result := range secondResponse.GetResults() {
		if !result.GetOk() || !result.GetDuplicate() {
			test.Fatalf("expected ok with duplicate, got ok=%v dup=%v code=%q", result.GetOk(), result.GetDuplicate(), result.GetErrorCode())
		}
		if result.GetEntryId() != "" {
			test.Fatalf("expected empty entry id on duplicate, got %q", result.GetEntryId())
		}
	}
	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId: userID, TenantId: tenantID, LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 300 || balanceResponse.GetAvailableCents() != 300 {
		test.Fatalf("expected 300/300 after duplicate batch, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchAtomicRollsBackAllMutations(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account: &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:  true,
		Operations: []*creditv1.BatchOperation{
			{
				OperationId: "grant-1",
				Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
					AmountCents:    1000,
					IdempotencyKey: "grant-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-1",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    200,
					IdempotencyKey: "spend-1",
					MetadataJson:   "{}",
				}},
			},
			{
				OperationId: "spend-2",
				Operation: &creditv1.BatchOperation_Spend{Spend: &creditv1.BatchSpendOp{
					AmountCents:    2000,
					IdempotencyKey: "spend-2",
					MetadataJson:   "{}",
				}},
			},
		},
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	results := batchResponse.GetResults()
	if len(results) != 3 {
		test.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].GetOk() || results[0].GetErrorCode() != errorRolledBack {
		test.Fatalf("expected rolled back grant, got ok=%v code=%q", results[0].GetOk(), results[0].GetErrorCode())
	}
	if results[1].GetOk() || results[1].GetErrorCode() != errorRolledBack {
		test.Fatalf("expected rolled back spend, got ok=%v code=%q", results[1].GetOk(), results[1].GetErrorCode())
	}
	if results[2].GetOk() || results[2].GetErrorCode() != errorInsufficientFunds {
		test.Fatalf("expected insufficient funds, got ok=%v code=%q", results[2].GetOk(), results[2].GetErrorCode())
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId: userID, TenantId: tenantID, LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 0 || balanceResponse.GetAvailableCents() != 0 {
		test.Fatalf("expected 0/0 after atomic rollback, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerBatchSupportsLargeBatches(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "user-123"
	tenantID := "default"
	ledgerID := "default"

	operations := make([]*creditv1.BatchOperation, 0, maxBatchOperations)
	for operationIndex := 0; operationIndex < maxBatchOperations; operationIndex++ {
		operationID := fmt.Sprintf("grant-%d", operationIndex)
		operations = append(operations, &creditv1.BatchOperation{
			OperationId: operationID,
			Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
				AmountCents:    1,
				IdempotencyKey: operationID,
				MetadataJson:   "{}",
			}},
		})
	}

	batchResponse, err := server.Batch(ctx, &creditv1.BatchRequest{
		Account:    &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Atomic:     false,
		Operations: operations,
	})
	if err != nil {
		test.Fatalf("batch: %v", err)
	}
	if len(batchResponse.GetResults()) != maxBatchOperations {
		test.Fatalf("expected %d results, got %d", maxBatchOperations, len(batchResponse.GetResults()))
	}
	if !batchResponse.GetResults()[0].GetOk() {
		test.Fatalf("expected first result to succeed")
	}
	if !batchResponse.GetResults()[len(batchResponse.GetResults())-1].GetOk() {
		test.Fatalf("expected last result to succeed")
	}

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId: userID, TenantId: tenantID, LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != int64(maxBatchOperations) || balanceResponse.GetAvailableCents() != int64(maxBatchOperations) {
		test.Fatalf("expected %d/%d, got total=%d available=%d", maxBatchOperations, maxBatchOperations, balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	tooLargeOperations := append(operations, &creditv1.BatchOperation{
		OperationId: "grant-too-many",
		Operation: &creditv1.BatchOperation_Grant{Grant: &creditv1.BatchGrantOp{
			AmountCents:    1,
			IdempotencyKey: "grant-too-many",
			MetadataJson:   "{}",
		}},
	})
	_, err = server.Batch(ctx, &creditv1.BatchRequest{
		Account:    &creditv1.AccountContext{UserId: userID, TenantId: tenantID, LedgerId: ledgerID},
		Operations: tooLargeOperations,
	})
	if status.Code(err) != codes.InvalidArgument {
		test.Fatalf("expected invalid argument, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != errorBatchTooLarge {
		test.Fatalf("expected %q, got %q", errorBatchTooLarge, status.Convert(err).Message())
	}
}

func newSQLiteLedgerService(test *testing.T) (*ledger.Service, error) {
	test.Helper()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	sqliteDSN := fmt.Sprintf("file:%s?cache=shared&_pragma=busy_timeout=5000&_pragma=journal_mode=WAL", sqlitePath)
	db, err := gorm.Open(sqlite.Open(sqliteDSN), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	test.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		return nil, err
	}
	store := gormstore.New(db)
	clock := func() int64 { return 1700000000 }
	return ledger.NewService(store, clock)
}

func mustBootstrapPolicy(test *testing.T, amountCents int64) ledger.BootstrapGrantPolicy {
	test.Helper()
	tenantID, err := ledger.NewTenantID("default")
	if err != nil {
		test.Fatalf("tenant id: %v", err)
	}
	ledgerID, err := ledger.NewLedgerID("default")
	if err != nil {
		test.Fatalf("ledger id: %v", err)
	}
	amount, err := ledger.NewPositiveAmountCents(amountCents)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	idempotencyKeyBase, err := ledger.NewIdempotencyKey("bootstrap")
	if err != nil {
		test.Fatalf("idempotency: %v", err)
	}
	metadata, err := ledger.NewMetadataJSON(`{"reason":"account_bootstrap"}`)
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	rule, err := ledger.NewBootstrapGrantRule(tenantID, ledgerID, amount, idempotencyKeyBase, metadata)
	if err != nil {
		test.Fatalf("bootstrap rule: %v", err)
	}
	policy, err := ledger.NewBootstrapGrantPolicy([]ledger.BootstrapGrantRule{rule})
	if err != nil {
		test.Fatalf("bootstrap policy: %v", err)
	}
	return policy
}

func TestBootstrapGrantAppliedOnFirstBalance(test *testing.T) {
	test.Parallel()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	sqliteDSN := fmt.Sprintf("file:%s?cache=shared&_pragma=busy_timeout=5000&_pragma=journal_mode=WAL", sqlitePath)
	db, err := gorm.Open(sqlite.Open(sqliteDSN), &gorm.Config{})
	if err != nil {
		test.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("db handle: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		test.Fatalf("migrate: %v", err)
	}
	store := gormstore.New(db)
	clock := func() int64 { return 1700000000 }

	bootstrapPolicy := mustBootstrapPolicy(test, 1000)
	creditService, err := ledger.NewService(store, clock, ledger.WithBootstrapGrantPolicy(bootstrapPolicy))
	if err != nil {
		test.Fatalf("new service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "bootstrap-user"
	tenantID := "default"
	ledgerID := "default"

	balanceResponse, err := server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 1000 || balanceResponse.GetAvailableCents() != 1000 {
		test.Fatalf("expected 1000/1000, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	balanceResponse, err = server.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance second time: %v", err)
	}
	if balanceResponse.GetTotalCents() != 1000 || balanceResponse.GetAvailableCents() != 1000 {
		test.Fatalf("expected 1000/1000 after second call, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}

	entries, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
		UserId:        userID,
		TenantId:      tenantID,
		LedgerId:      ledgerID,
		BeforeUnixUtc: 1893456000,
		Limit:         10,
	})
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if got := len(entries.GetEntries()); got != 1 {
		test.Fatalf("expected 1 entry, got %d", got)
	}
	if entries.GetEntries()[0].GetType() != "grant" {
		test.Fatalf("expected grant entry, got %q", entries.GetEntries()[0].GetType())
	}
}

func TestBootstrapGrantDoesNotApplyToExistingAccounts(test *testing.T) {
	test.Parallel()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	sqliteDSN := fmt.Sprintf("file:%s?cache=shared&_pragma=busy_timeout=5000&_pragma=journal_mode=WAL", sqlitePath)
	db, err := gorm.Open(sqlite.Open(sqliteDSN), &gorm.Config{})
	if err != nil {
		test.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		test.Fatalf("db handle: %v", err)
	}
	test.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}, &gormstore.Reservation{}); err != nil {
		test.Fatalf("migrate: %v", err)
	}
	store := gormstore.New(db)
	clock := func() int64 { return 1700000000 }

	creditService, err := ledger.NewService(store, clock)
	if err != nil {
		test.Fatalf("new service: %v", err)
	}
	server := NewCreditServiceServer(creditService)

	ctx := context.Background()
	userID := "bootstrap-existing"
	tenantID := "default"
	ledgerID := "default"

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    500,
		IdempotencyKey: "grant-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
	}

	bootstrapPolicy := mustBootstrapPolicy(test, 1000)
	creditServiceWithBootstrap, err := ledger.NewService(store, clock, ledger.WithBootstrapGrantPolicy(bootstrapPolicy))
	if err != nil {
		test.Fatalf("new service with bootstrap: %v", err)
	}
	serverWithBootstrap := NewCreditServiceServer(creditServiceWithBootstrap)

	balanceResponse, err := serverWithBootstrap.GetBalance(ctx, &creditv1.BalanceRequest{
		UserId:   userID,
		TenantId: tenantID,
		LedgerId: ledgerID,
	})
	if err != nil {
		test.Fatalf("get balance: %v", err)
	}
	if balanceResponse.GetTotalCents() != 500 || balanceResponse.GetAvailableCents() != 500 {
		test.Fatalf("expected 500/500, got total=%d available=%d", balanceResponse.GetTotalCents(), balanceResponse.GetAvailableCents())
	}
}

func TestCreditServiceServerValidationErrors(test *testing.T) {
	test.Parallel()
	creditService, err := newSQLiteLedgerService(test)
	if err != nil {
		test.Fatalf("new ledger service: %v", err)
	}
	server := NewCreditServiceServer(creditService)
	ctx := context.Background()

	testCases := []struct {
		name        string
		invoke      func() error
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name: "get balance invalid user id",
			invoke: func() error {
				_, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: "", TenantId: "default", LedgerId: "default"})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "get balance invalid ledger id",
			invoke: func() error {
				_, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: "user", TenantId: "default", LedgerId: ""})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "get balance invalid tenant id",
			invoke: func() error {
				_, err := server.GetBalance(ctx, &creditv1.BalanceRequest{UserId: "user", TenantId: "", LedgerId: "default"})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "grant invalid idempotency key",
			invoke: func() error {
				_, err := server.Grant(ctx, &creditv1.GrantRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "grant invalid metadata",
			invoke: func() error {
				_, err := server.Grant(ctx, &creditv1.GrantRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "grant-1", MetadataJson: "{",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata,
		},
		{
			name: "reserve invalid reservation id",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, ReservationId: "", IdempotencyKey: "reserve-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidReservationID,
		},
		{
			name: "capture invalid reservation id",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "", IdempotencyKey: "capture-1", AmountCents: 100, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidReservationID,
		},
		{
			name: "capture invalid idempotency key",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "", AmountCents: 100, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "release invalid metadata",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "release-1", MetadataJson: "{",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata,
		},
		{
			name: "spend invalid amount",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 0, IdempotencyKey: "spend-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidAmount,
		},
		{
			name: "grant invalid user id",
			invoke: func() error {
				_, err := server.Grant(ctx, &creditv1.GrantRequest{
					UserId: "", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "grant-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "grant invalid ledger id",
			invoke: func() error {
				_, err := server.Grant(ctx, &creditv1.GrantRequest{
					UserId: "user", TenantId: "default", LedgerId: "", AmountCents: 100, IdempotencyKey: "grant-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "grant invalid tenant id",
			invoke: func() error {
				_, err := server.Grant(ctx, &creditv1.GrantRequest{
					UserId: "user", TenantId: "", LedgerId: "default", AmountCents: 100, IdempotencyKey: "grant-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "reserve invalid user id",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "", TenantId: "default", LedgerId: "default", AmountCents: 100, ReservationId: "order-1", IdempotencyKey: "reserve-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "reserve invalid ledger id",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "default", LedgerId: "", AmountCents: 100, ReservationId: "order-1", IdempotencyKey: "reserve-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "reserve invalid tenant id",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "", LedgerId: "default", AmountCents: 100, ReservationId: "order-1", IdempotencyKey: "reserve-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "reserve invalid amount",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 0, ReservationId: "order-1", IdempotencyKey: "reserve-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidAmount,
		},
		{
			name: "reserve invalid idempotency key",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, ReservationId: "order-1", IdempotencyKey: "", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "reserve invalid metadata",
			invoke: func() error {
				_, err := server.Reserve(ctx, &creditv1.ReserveRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, ReservationId: "order-1", IdempotencyKey: "reserve-1", MetadataJson: "{",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata,
		},
		{
			name: "capture invalid user id",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 100, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "capture invalid ledger id",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "default", LedgerId: "", ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 100, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "capture invalid tenant id",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 100, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "capture invalid amount",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 0, MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidAmount,
		},
		{
			name: "capture invalid metadata",
			invoke: func() error {
				_, err := server.Capture(ctx, &creditv1.CaptureRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "capture-1", AmountCents: 100, MetadataJson: "{",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata,
		},
		{
			name: "release invalid user id",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "release-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "release invalid ledger id",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "user", TenantId: "default", LedgerId: "", ReservationId: "order-1", IdempotencyKey: "release-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "release invalid tenant id",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "user", TenantId: "", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "release-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "release invalid reservation id",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "", IdempotencyKey: "release-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidReservationID,
		},
		{
			name: "release invalid idempotency key",
			invoke: func() error {
				_, err := server.Release(ctx, &creditv1.ReleaseRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", ReservationId: "order-1", IdempotencyKey: "", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "spend invalid user id",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "spend-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "spend invalid ledger id",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "user", TenantId: "default", LedgerId: "", AmountCents: 100, IdempotencyKey: "spend-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "spend invalid tenant id",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "user", TenantId: "", LedgerId: "default", AmountCents: 100, IdempotencyKey: "spend-1", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "spend invalid idempotency key",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "", MetadataJson: "{}",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
		{
			name: "spend invalid metadata",
			invoke: func() error {
				_, err := server.Spend(ctx, &creditv1.SpendRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", AmountCents: 100, IdempotencyKey: "spend-1", MetadataJson: "{",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata,
		},
		{
			name: "list entries invalid user id",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "", TenantId: "default", LedgerId: "default", BeforeUnixUtc: 0, Limit: 1,
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidUserID,
		},
		{
			name: "list entries invalid ledger id",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "user", TenantId: "default", LedgerId: "", BeforeUnixUtc: 0, Limit: 1,
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidLedgerID,
		},
		{
			name: "list entries invalid tenant id",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "user", TenantId: "", LedgerId: "default", BeforeUnixUtc: 0, Limit: 1,
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidTenantID,
		},
		{
			name: "list entries invalid entry type",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", BeforeUnixUtc: 0, Limit: 1, Types: []string{"not-a-type"},
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidEntryType,
		},
		{
			name: "list entries invalid reservation id",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", BeforeUnixUtc: 0, Limit: 1, ReservationId: "   ",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidReservationID,
		},
		{
			name: "list entries invalid idempotency key prefix",
			invoke: func() error {
				_, err := server.ListEntries(ctx, &creditv1.ListEntriesRequest{
					UserId: "user", TenantId: "default", LedgerId: "default", BeforeUnixUtc: 0, Limit: 1, IdempotencyKeyPrefix: "   ",
				})
				return err
			},
			wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			err := testCase.invoke()
			gotStatus, ok := status.FromError(err)
			if !ok {
				test.Fatalf("expected grpc status error, got %v", err)
			}
			if gotStatus.Code() != testCase.wantCode {
				test.Fatalf("expected code %v, got %v", testCase.wantCode, gotStatus.Code())
			}
			if gotStatus.Message() != testCase.wantMessage {
				test.Fatalf("expected message %q, got %q", testCase.wantMessage, gotStatus.Message())
			}
		})
	}
}

func TestGetBalanceMapsServiceErrors(test *testing.T) {
	test.Parallel()
	clock := func() int64 { return 1700000000 }
	service, err := ledger.NewService(&alwaysErrorStore{err: errors.New("boom")}, clock)
	if err != nil {
		test.Fatalf("service init: %v", err)
	}
	server := NewCreditServiceServer(service)
	_, err = server.GetBalance(context.Background(), &creditv1.BalanceRequest{
		UserId: "user", TenantId: "default", LedgerId: "default",
	})
	if status.Code(err) != codes.Internal {
		test.Fatalf("expected internal, got %v", status.Code(err))
	}
	if status.Convert(err).Message() != "boom" {
		test.Fatalf("expected boom, got %q", status.Convert(err).Message())
	}
}

type alwaysErrorStore struct {
	err error
}

func (store *alwaysErrorStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore ledger.Store) error) error {
	return store.err
}

func (store *alwaysErrorStore) GetOrCreateAccountID(ctx context.Context, tenantID ledger.TenantID, userID ledger.UserID, ledgerID ledger.LedgerID) (ledger.AccountID, error) {
	return ledger.AccountID{}, store.err
}

func (store *alwaysErrorStore) InsertEntry(ctx context.Context, entry ledger.EntryInput) (ledger.Entry, error) {
	return ledger.Entry{}, store.err
}

func (store *alwaysErrorStore) GetEntry(ctx context.Context, accountID ledger.AccountID, entryID ledger.EntryID) (ledger.Entry, error) {
	return ledger.Entry{}, store.err
}

func (store *alwaysErrorStore) GetEntryByIdempotencyKey(ctx context.Context, accountID ledger.AccountID, idempotencyKey ledger.IdempotencyKey) (ledger.Entry, error) {
	return ledger.Entry{}, store.err
}

func (store *alwaysErrorStore) SumRefunds(ctx context.Context, accountID ledger.AccountID, originalEntryID ledger.EntryID) (ledger.AmountCents, error) {
	return 0, store.err
}

func (store *alwaysErrorStore) SumTotal(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.SignedAmountCents, error) {
	return 0, store.err
}

func (store *alwaysErrorStore) SumActiveHolds(ctx context.Context, accountID ledger.AccountID, atUnixUTC int64) (ledger.AmountCents, error) {
	return 0, store.err
}

func (store *alwaysErrorStore) CreateReservation(ctx context.Context, reservation ledger.Reservation) error {
	return store.err
}

func (store *alwaysErrorStore) GetReservation(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID) (ledger.Reservation, error) {
	return ledger.Reservation{}, store.err
}

func (store *alwaysErrorStore) UpdateReservationStatus(ctx context.Context, accountID ledger.AccountID, reservationID ledger.ReservationID, from, to ledger.ReservationStatus) error {
	return store.err
}

func (store *alwaysErrorStore) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int, filter ledger.ListEntriesFilter) ([]ledger.Entry, error) {
	return nil, store.err
}
