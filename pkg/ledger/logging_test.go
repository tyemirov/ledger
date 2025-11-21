package ledger

import (
	"context"
	"errors"
	"testing"
)

type recorderLogger struct {
	entries []OperationLog
}

func (r *recorderLogger) LogOperation(_ context.Context, entry OperationLog) {
	r.entries = append(r.entries, entry)
}

func TestServiceLogsGrantOperation(t *testing.T) {
	baseStore := newMockStore()
	logger := &recorderLogger{}
	service, err := NewService(baseStore, func() int64 { return 42 }, WithOperationLogger(logger))
	if err != nil {
		t.Fatalf("service init failed: %v", err)
	}
	user := mustUserID(t, "user-1")
	amount := mustAmount(t, 100)
	idem := mustIdempotencyKey(t, "grant-1")
	metadata := mustMetadata(t, `{"action":"test"}`)
	if err := service.Grant(context.Background(), user, amount, idem, 0, metadata); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logger.entries))
	}
	entry := logger.entries[0]
	if entry.Operation != "grant" || entry.UserID != user || entry.Amount != amount || entry.IdempotencyKey != idem {
		t.Fatalf("unexpected log entry: %+v", entry)
	}
	if entry.Error != nil || entry.Status != "ok" {
		t.Fatalf("expected successful log entry, got %+v", entry)
	}
}

func TestServiceLogsErrorStatus(t *testing.T) {
	failingStore := &failingStore{err: errors.New("boom")}
	logger := &recorderLogger{}
	service, err := NewService(failingStore, func() int64 { return 1 }, WithOperationLogger(logger))
	if err != nil {
		t.Fatalf("service init failed: %v", err)
	}
	user := mustUserID(t, "user-1")
	amount := mustAmount(t, 100)
	idem := mustIdempotencyKey(t, "grant-1")
	metadata := mustMetadata(t, `{"action":"test"}`)
	err = service.Grant(context.Background(), user, amount, idem, 0, metadata)
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(logger.entries) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logger.entries))
	}
	if logger.entries[0].Status != "error" || logger.entries[0].Error == nil {
		t.Fatalf("expected error log entry, got %+v", logger.entries[0])
	}
}

// helper constructors reuse existing helpers defined in service_reservation_test.go.
