package grpcserver

import (
	"context"
	"errors"
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
		{name: "invalid idempotency key", input: ledger.ErrInvalidIdempotencyKey, wantCode: codes.InvalidArgument, wantMessage: errorInvalidIdempotencyKey},
		{name: "invalid amount", input: ledger.ErrInvalidAmountCents, wantCode: codes.InvalidArgument, wantMessage: errorInvalidAmount},
		{name: "invalid metadata", input: ledger.ErrInvalidMetadataJSON, wantCode: codes.InvalidArgument, wantMessage: errorInvalidMetadata},
		{name: "insufficient funds", input: ledger.ErrInsufficientFunds, wantCode: codes.FailedPrecondition, wantMessage: errorInsufficientFunds},
		{name: "unknown reservation", input: ledger.ErrUnknownReservation, wantCode: codes.NotFound, wantMessage: errorUnknownReservation},
		{name: "duplicate idempotency", input: ledger.ErrDuplicateIdempotencyKey, wantCode: codes.AlreadyExists, wantMessage: errorDuplicateIdempotencyKey},
		{name: "reservation exists", input: ledger.ErrReservationExists, wantCode: codes.AlreadyExists, wantMessage: errorReservationExists},
		{name: "reservation closed", input: ledger.ErrReservationClosed, wantCode: codes.FailedPrecondition, wantMessage: errorReservationClosed},
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

	if _, err := server.Grant(ctx, &creditv1.GrantRequest{
		UserId:           userID,
		TenantId:         tenantID,
		LedgerId:         ledgerID,
		AmountCents:      1000,
		IdempotencyKey:   "grant-1",
		ExpiresAtUnixUtc: 0,
		MetadataJson:     "{}",
	}); err != nil {
		test.Fatalf("grant: %v", err)
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

	if _, err := server.Capture(ctx, &creditv1.CaptureRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-1",
		IdempotencyKey: "capture-1",
		AmountCents:    300,
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("capture: %v", err)
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

	if _, err := server.Reserve(ctx, &creditv1.ReserveRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		AmountCents:    100,
		ReservationId:  "order-2",
		IdempotencyKey: "reserve-2",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("reserve order-2: %v", err)
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
	if _, err := server.Release(ctx, &creditv1.ReleaseRequest{
		UserId:         userID,
		TenantId:       tenantID,
		LedgerId:       ledgerID,
		ReservationId:  "order-2",
		IdempotencyKey: "release-1",
		MetadataJson:   "{}",
	}); err != nil {
		test.Fatalf("release order-2: %v", err)
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

func newSQLiteLedgerService(test *testing.T) (*ledger.Service, error) {
	test.Helper()
	tempDir := test.TempDir()
	sqlitePath := filepath.Join(tempDir, "ledger.db")
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
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

func (store *alwaysErrorStore) InsertEntry(ctx context.Context, entry ledger.EntryInput) error {
	return store.err
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

func (store *alwaysErrorStore) ListEntries(ctx context.Context, accountID ledger.AccountID, beforeUnixUTC int64, limit int) ([]ledger.Entry, error) {
	return nil, store.err
}
