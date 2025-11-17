package credit

import (
	"context"
	"testing"
)

func TestReserveCreatesReservationAndHoldEntry(t *testing.T) {
	t.Parallel()
	store := newStubStore(AmountCents(100))
	service := mustNewService(t, store)
	userID := mustUserID(t, "user-123")
	reservationID := mustReservationID(t, "res-1")
	idem := mustIdempotencyKey(t, "idem-1")
	metadata := mustMetadata(t, `{"foo":"bar"}`)
	amount := mustAmount(t, 40)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idem, metadata); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	if len(store.entries) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Type != EntryHold || entry.AmountCents != -amount {
		t.Fatalf("expected hold entry -%d, got %+v", amount, entry)
	}
	res := store.mustReservation(t, reservationID.String())
	if res.Status != ReservationStatusActive {
		t.Fatalf("expected reservation active, got %s", res.Status)
	}
}

func TestCaptureMovesReservationToCaptured(t *testing.T) {
	t.Parallel()
	store := newStubStore(AmountCents(200))
	service := mustNewService(t, store)
	userID := mustUserID(t, "user-456")
	resID := mustReservationID(t, "res-9")
	idem := mustIdempotencyKey(t, "idem-9")
	meta := mustMetadata(t, "{}")
	amount := mustAmount(t, 60)

	if err := service.Reserve(context.Background(), userID, amount, resID, idem, meta); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Capture(context.Background(), userID, resID, idem, amount, meta); err != nil {
		t.Fatalf("capture: %v", err)
	}

	if got := len(store.entries); got != 3 {
		t.Fatalf("expected 3 ledger entries (hold, reverse, spend), got %d", got)
	}
	reverse := store.entries[1]
	if reverse.Type != EntryReverseHold || reverse.AmountCents != amount {
		t.Fatalf("expected reverse hold of %d, got %+v", amount, reverse)
	}
	spend := store.entries[2]
	if spend.Type != EntrySpend || spend.AmountCents != -amount {
		t.Fatalf("expected spend of -%d, got %+v", amount, spend)
	}
	res := store.mustReservation(t, resID.String())
	if res.Status != ReservationStatusCaptured {
		t.Fatalf("expected captured reservation, got %s", res.Status)
	}
}

func TestCaptureUsesDistinctIdempotencyKeys(t *testing.T) {
	t.Parallel()
	store := newStubStore(AmountCents(120))
	service := mustNewService(t, store)
	userID := mustUserID(t, "user-456")
	reservationID := mustReservationID(t, "res-10")
	idempotencyKey := mustIdempotencyKey(t, "idem-10")
	metadata := mustMetadata(t, "{}")
	amount := mustAmount(t, 30)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Capture(context.Background(), userID, reservationID, idempotencyKey, amount, metadata); err != nil {
		t.Fatalf("capture: %v", err)
	}

	if got := len(store.entries); got != 3 {
		t.Fatalf("expected 3 ledger entries (grant not required), got %d", got)
	}

	keys := make(map[string]struct{}, len(store.entries))
	for _, entry := range store.entries {
		keys[entry.IdempotencyKey] = struct{}{}
	}
	if len(keys) != len(store.entries) {
		t.Fatalf("expected unique idempotency keys, got %v", keys)
	}

	reverse := store.entries[1]
	spend := store.entries[2]
	if reverse.IdempotencyKey == spend.IdempotencyKey {
		t.Fatalf("expected distinct keys, got reverse=%s spend=%s", reverse.IdempotencyKey, spend.IdempotencyKey)
	}
}

func TestReleaseUnlocksReservation(t *testing.T) {
	t.Parallel()
	store := newStubStore(AmountCents(150))
	service := mustNewService(t, store)
	userID := mustUserID(t, "user-789")
	resID := mustReservationID(t, "res-77")
	holdIdempotencyKey := mustIdempotencyKey(t, "idem-77")
	releaseIdempotencyKey := mustIdempotencyKey(t, "idem-77-release")
	meta := mustMetadata(t, "{}")
	amount := mustAmount(t, 50)

	if err := service.Reserve(context.Background(), userID, amount, resID, holdIdempotencyKey, meta); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := service.Release(context.Background(), userID, resID, releaseIdempotencyKey, meta); err != nil {
		t.Fatalf("release: %v", err)
	}
	if got := len(store.entries); got != 2 {
		t.Fatalf("expected 2 entries (hold + reverse hold), got %d", got)
	}
	reverse := store.entries[1]
	if reverse.Type != EntryReverseHold || reverse.AmountCents != amount {
		t.Fatalf("expected reverse hold of %d, got %+v", amount, reverse)
	}
	res := store.mustReservation(t, resID.String())
	if res.Status != ReservationStatusReleased {
		t.Fatalf("expected released reservation, got %s", res.Status)
	}
}

// --- helpers ---

type stubStore struct {
	accountID    string
	total        AmountCents
	reservations map[string]Reservation
	entries      []Entry
	idempotency  map[string]struct{}
}

func newStubStore(initialTotal AmountCents) *stubStore {
	return &stubStore{
		accountID:    "acct-1",
		total:        initialTotal,
		reservations: make(map[string]Reservation),
		idempotency:  make(map[string]struct{}),
	}
}

func (s *stubStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	return fn(ctx, s)
}

func (s *stubStore) GetOrCreateAccountID(ctx context.Context, userID string) (string, error) {
	return s.accountID, nil
}

func (s *stubStore) InsertEntry(ctx context.Context, entry Entry) error {
	if _, exists := s.idempotency[entry.IdempotencyKey]; exists {
		return ErrDuplicateIdempotencyKey
	}
	s.idempotency[entry.IdempotencyKey] = struct{}{}
	s.entries = append(s.entries, entry)
	switch entry.Type {
	case EntryGrant, EntrySpend:
		s.total += entry.AmountCents
	}
	return nil
}

func (s *stubStore) SumTotal(ctx context.Context, accountID string, _ int64) (AmountCents, error) {
	return s.total, nil
}

func (s *stubStore) SumActiveHolds(ctx context.Context, accountID string, _ int64) (AmountCents, error) {
	var sum AmountCents
	for _, res := range s.reservations {
		if res.Status == ReservationStatusActive {
			sum += res.AmountCents
		}
	}
	return sum, nil
}

func (s *stubStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	key := reservation.ReservationID
	if _, exists := s.reservations[key]; exists {
		return ErrReservationExists
	}
	s.reservations[key] = reservation
	return nil
}

func (s *stubStore) GetReservation(ctx context.Context, accountID string, reservationID string) (Reservation, error) {
	res, ok := s.reservations[reservationID]
	if !ok {
		return Reservation{}, ErrUnknownReservation
	}
	return res, nil
}

func (s *stubStore) UpdateReservationStatus(ctx context.Context, accountID string, reservationID string, from, to ReservationStatus) error {
	res, ok := s.reservations[reservationID]
	if !ok {
		return ErrUnknownReservation
	}
	if res.Status != from {
		return ErrReservationClosed
	}
	res.Status = to
	s.reservations[reservationID] = res
	return nil
}

func (s *stubStore) ListEntries(ctx context.Context, accountID string, beforeUnixUTC int64, limit int) ([]Entry, error) {
	return nil, nil
}

func (s *stubStore) mustReservation(t *testing.T, reservationID string) Reservation {
	t.Helper()
	res, ok := s.reservations[reservationID]
	if !ok {
		t.Fatalf("reservation %s not found", reservationID)
	}
	return res
}

// domain helper constructors
func mustNewService(t *testing.T, store Store) *Service {
	t.Helper()
	service, err := NewService(store, func() int64 { return 100 })
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func mustUserID(t *testing.T, raw string) UserID {
	t.Helper()
	value, err := NewUserID(raw)
	if err != nil {
		t.Fatalf("user id: %v", err)
	}
	return value
}

func mustReservationID(t *testing.T, raw string) ReservationID {
	t.Helper()
	value, err := NewReservationID(raw)
	if err != nil {
		t.Fatalf("reservation id: %v", err)
	}
	return value
}

func mustIdempotencyKey(t *testing.T, raw string) IdempotencyKey {
	t.Helper()
	value, err := NewIdempotencyKey(raw)
	if err != nil {
		t.Fatalf("idempotency key: %v", err)
	}
	return value
}

func mustMetadata(t *testing.T, raw string) MetadataJSON {
	t.Helper()
	value, err := NewMetadataJSON(raw)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	return value
}

func mustAmount(t *testing.T, raw int64) AmountCents {
	t.Helper()
	value, err := NewAmountCents(raw)
	if err != nil {
		t.Fatalf("amount: %v", err)
	}
	return value
}
