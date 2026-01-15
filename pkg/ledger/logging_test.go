package ledger

import (
	"context"
	"errors"
	"testing"
)

type recorderLogger struct {
	entries []OperationLog
}

func (logger *recorderLogger) LogOperation(_ context.Context, entry OperationLog) {
	logger.entries = append(logger.entries, entry)
}

func TestServiceLogsGrantOperation(test *testing.T) {
	test.Parallel()
	baseStore := newMockStore(test)
	logger := &recorderLogger{}
	service, err := NewService(baseStore, func() int64 { return 42 }, WithOperationLogger(logger))
	if err != nil {
		test.Fatalf("service init failed: %v", err)
	}
	user := mustUserID(test, "user-1")
	amount := mustPositiveAmount(test, 100)
	idempotencyKey := mustIdempotencyKey(test, "grant-1")
	metadata := mustMetadata(test, `{"action":"test"}`)
	if err := service.Grant(context.Background(), user, amount, idempotencyKey, 0, metadata); err != nil {
		test.Fatalf("grant failed: %v", err)
	}
	if len(logger.entries) != 1 {
		test.Fatalf("expected one log entry, got %d", len(logger.entries))
	}
	entry := logger.entries[0]
	if entry.Operation != operationGrant || entry.UserID != user || entry.Amount != amount.ToAmountCents() || entry.IdempotencyKey != idempotencyKey {
		test.Fatalf("unexpected log entry: %+v", entry)
	}
	if entry.Error != nil || entry.Status != operationStatusOK {
		test.Fatalf("expected successful log entry, got %+v", entry)
	}
}

func TestServiceLogsErrorStatus(test *testing.T) {
	test.Parallel()
	failing := newFailingStore(test, errors.New("boom"))
	logger := &recorderLogger{}
	service, err := NewService(failing, func() int64 { return 1 }, WithOperationLogger(logger))
	if err != nil {
		test.Fatalf("service init failed: %v", err)
	}
	user := mustUserID(test, "user-1")
	amount := mustPositiveAmount(test, 100)
	idempotencyKey := mustIdempotencyKey(test, "grant-1")
	metadata := mustMetadata(test, `{"action":"test"}`)
	err = service.Grant(context.Background(), user, amount, idempotencyKey, 0, metadata)
	if err == nil {
		test.Fatalf("expected error")
	}
	if len(logger.entries) != 1 {
		test.Fatalf("expected one log entry, got %d", len(logger.entries))
	}
	if logger.entries[0].Status != operationStatusError || logger.entries[0].Error == nil {
		test.Fatalf("expected error log entry, got %+v", logger.entries[0])
	}
}
