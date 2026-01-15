package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestReserveCreatesReservationAndHoldEntry(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 100))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-123")
	reservationID := mustReservationID(test, "res-1")
	idempotencyKey := mustIdempotencyKey(test, "idem-1")
	metadata := mustMetadata(test, `{"foo":"bar"}`)
	amount := mustPositiveAmount(test, 40)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}

	if len(store.entries) != 1 {
		test.Fatalf("expected 1 ledger entry, got %d", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Type() != EntryHold {
		test.Fatalf("expected hold entry, got %s", entry.Type())
	}
	expectedAmount := amount.ToEntryAmountCents().Negated()
	if entry.AmountCents() != expectedAmount {
		test.Fatalf("expected hold entry %d, got %d", expectedAmount, entry.AmountCents())
	}
	reservation := store.mustReservation(test, reservationID)
	if reservation.Status() != ReservationStatusActive {
		test.Fatalf("expected reservation active, got %s", reservation.Status())
	}
}

func TestBalanceComputesAvailableFunds(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 200))
	reservationID := mustReservationID(test, "active-hold")
	reservation := mustReservationRecord(test, store.accountID, reservationID, mustPositiveAmount(test, 50), ReservationStatusActive)
	store.reservations[reservationID] = reservation
	service := mustNewService(test, store)
	userID := mustUserID(test, "availability-user")

	balance, err := service.Balance(context.Background(), userID)
	if err != nil {
		test.Fatalf("balance: %v", err)
	}
	if balance.TotalCents != 200 {
		test.Fatalf("expected total 200, got %d", balance.TotalCents)
	}
	if balance.AvailableCents != 150 {
		test.Fatalf("expected available 150, got %d", balance.AvailableCents)
	}
}

func TestGrantAppendsGrantEntry(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 0))
	service := mustNewService(test, store)
	userID := mustUserID(test, "grant-user")
	idempotencyKey := mustIdempotencyKey(test, "grant-idem")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 75)

	if err := service.Grant(context.Background(), userID, amount, idempotencyKey, 0, metadata); err != nil {
		test.Fatalf("grant: %v", err)
	}
	if len(store.entries) != 1 {
		test.Fatalf("expected grant entry, got %d entries", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Type() != EntryGrant {
		test.Fatalf("unexpected grant entry type: %s", entry.Type())
	}
	if entry.AmountCents() != amount.ToEntryAmountCents() {
		test.Fatalf("unexpected grant amount: %d", entry.AmountCents())
	}
}

func TestReserveInsufficientFunds(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 10))
	service := mustNewService(test, store)
	userID := mustUserID(test, "reserve-low")
	reservationID := mustReservationID(test, "reserve-low")
	idempotencyKey := mustIdempotencyKey(test, "reserve-low")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 50)

	err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata)
	if !errors.Is(err, ErrInsufficientFunds) {
		test.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestCaptureMovesReservationToCaptured(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-456")
	reservationID := mustReservationID(test, "res-9")
	idempotencyKey := mustIdempotencyKey(test, "idem-9")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 60)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	if err := service.Capture(context.Background(), userID, reservationID, idempotencyKey, amount, metadata); err != nil {
		test.Fatalf("capture: %v", err)
	}

	if got := len(store.entries); got != 3 {
		test.Fatalf("expected 3 ledger entries (hold, reverse, spend), got %d", got)
	}
	reverse := store.entries[1]
	if reverse.Type() != EntryReverseHold {
		test.Fatalf("expected reverse hold, got %s", reverse.Type())
	}
	if reverse.AmountCents() != amount.ToEntryAmountCents() {
		test.Fatalf("expected reverse hold of %d, got %d", amount, reverse.AmountCents())
	}
	spend := store.entries[2]
	if spend.Type() != EntrySpend {
		test.Fatalf("expected spend, got %s", spend.Type())
	}
	if spend.AmountCents() != amount.ToEntryAmountCents().Negated() {
		test.Fatalf("expected spend of %d, got %d", amount.ToEntryAmountCents().Negated(), spend.AmountCents())
	}
	reservation := store.mustReservation(test, reservationID)
	if reservation.Status() != ReservationStatusCaptured {
		test.Fatalf("expected captured reservation, got %s", reservation.Status())
	}
}

func TestCaptureAmountMismatch(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 200))
	service := mustNewService(test, store)
	userID := mustUserID(test, "capture-mismatch")
	reservationID := mustReservationID(test, "capture-mismatch")
	idempotencyKey := mustIdempotencyKey(test, "capture-mismatch")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 60)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	err := service.Capture(context.Background(), userID, reservationID, idempotencyKey, mustPositiveAmount(test, 10), metadata)
	if !errors.Is(err, ErrInvalidAmountCents) {
		test.Fatalf("expected ErrInvalidAmountCents, got %v", err)
	}
}

func TestCaptureUsesDistinctIdempotencyKeys(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 120))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-456")
	reservationID := mustReservationID(test, "res-10")
	idempotencyKey := mustIdempotencyKey(test, "idem-10")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 30)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, idempotencyKey, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	if err := service.Capture(context.Background(), userID, reservationID, idempotencyKey, amount, metadata); err != nil {
		test.Fatalf("capture: %v", err)
	}

	if got := len(store.entries); got != 3 {
		test.Fatalf("expected 3 ledger entries (grant not required), got %d", got)
	}

	keys := make(map[IdempotencyKey]struct{}, len(store.entries))
	for _, entry := range store.entries {
		keys[entry.IdempotencyKey()] = struct{}{}
	}
	if len(keys) != len(store.entries) {
		test.Fatalf("expected unique idempotency keys, got %v", keys)
	}

	reverse := store.entries[1]
	spend := store.entries[2]
	if reverse.IdempotencyKey() == spend.IdempotencyKey() {
		test.Fatalf("expected distinct keys, got reverse=%s spend=%s", reverse.IdempotencyKey().String(), spend.IdempotencyKey().String())
	}
}

func TestReleaseUnlocksReservation(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 150))
	service := mustNewService(test, store)
	userID := mustUserID(test, "user-789")
	reservationID := mustReservationID(test, "res-77")
	holdIdempotencyKey := mustIdempotencyKey(test, "idem-77")
	releaseIdempotencyKey := mustIdempotencyKey(test, "idem-77-release")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 50)

	if err := service.Reserve(context.Background(), userID, amount, reservationID, holdIdempotencyKey, metadata); err != nil {
		test.Fatalf("reserve: %v", err)
	}
	if err := service.Release(context.Background(), userID, reservationID, releaseIdempotencyKey, metadata); err != nil {
		test.Fatalf("release: %v", err)
	}
	if got := len(store.entries); got != 2 {
		test.Fatalf("expected 2 entries (hold + reverse hold), got %d", got)
	}
	reverse := store.entries[1]
	if reverse.Type() != EntryReverseHold {
		test.Fatalf("expected reverse hold, got %s", reverse.Type())
	}
	if reverse.AmountCents() != amount.ToEntryAmountCents() {
		test.Fatalf("expected reverse hold of %d, got %d", amount, reverse.AmountCents())
	}
	reservation := store.mustReservation(test, reservationID)
	if reservation.Status() != ReservationStatusReleased {
		test.Fatalf("expected released reservation, got %s", reservation.Status())
	}
}

func TestListEntriesDelegatesToStore(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 0))
	accountID := store.accountID
	entryIDOne := mustEntryID(test, "e1")
	entryIDTwo := mustEntryID(test, "e2")
	idempotencyKey := mustIdempotencyKey(test, "list-idem")
	metadata := mustMetadata(test, "{}")
	entryOne := mustEntry(test, entryIDOne, accountID, EntryGrant, mustEntryAmount(test, 10), idempotencyKey, metadata)
	entryTwo := mustEntry(test, entryIDTwo, accountID, EntryGrant, mustEntryAmount(test, 20), idempotencyKey, metadata)
	store.listEntries = []Entry{entryOne, entryTwo}
	service := mustNewService(test, store)
	userID := mustUserID(test, "list-user")

	out, err := service.ListEntries(context.Background(), userID, 0, 5)
	if err != nil {
		test.Fatalf("list entries: %v", err)
	}
	if len(out) != 2 {
		test.Fatalf("unexpected entries: %+v", out)
	}
	if out[0].EntryID().String() != "e1" || out[1].EntryID().String() != "e2" {
		test.Fatalf("unexpected entries: %+v", out)
	}
}

func TestNewServiceRequiresDependencies(test *testing.T) {
	test.Parallel()
	_, err := NewService(nil, func() int64 { return 0 })
	if !errors.Is(err, ErrInvalidServiceConfig) {
		test.Fatalf("expected invalid service config error, got %v", err)
	}
	store := newStubStore(test, mustAmountCents(test, 0))
	_, err = NewService(store, nil)
	if !errors.Is(err, ErrInvalidServiceConfig) {
		test.Fatalf("expected invalid service config error, got %v", err)
	}
}

func TestSpendInsufficientFunds(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 10))
	service := mustNewService(test, store)
	userID := mustUserID(test, "spend-low")
	idempotencyKey := mustIdempotencyKey(test, "spend-low")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 40)

	err := service.Spend(context.Background(), userID, amount, idempotencyKey, metadata)
	if !errors.Is(err, ErrInsufficientFunds) {
		test.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestSpendAppendsSpendEntry(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustAmountCents(test, 150))
	service := mustNewService(test, store)
	userID := mustUserID(test, "spend-user")
	idempotencyKey := mustIdempotencyKey(test, "spend-idem")
	metadata := mustMetadata(test, "{}")
	amount := mustPositiveAmount(test, 25)

	if err := service.Spend(context.Background(), userID, amount, idempotencyKey, metadata); err != nil {
		test.Fatalf("spend: %v", err)
	}
	if len(store.entries) != 1 {
		test.Fatalf("expected 1 entry, got %d", len(store.entries))
	}
	entry := store.entries[0]
	if entry.Type() != EntrySpend {
		test.Fatalf("unexpected spend entry type: %s", entry.Type())
	}
	if entry.AmountCents() != amount.ToEntryAmountCents().Negated() {
		test.Fatalf("unexpected spend entry amount: %d", entry.AmountCents())
	}
}

type stubStore struct {
	accountID    AccountID
	total        AmountCents
	reservations map[ReservationID]Reservation
	entries      []EntryInput
	listEntries  []Entry
	listErr      error
	idempotency  map[IdempotencyKey]struct{}
}

func newStubStore(test *testing.T, initialTotal AmountCents) *stubStore {
	test.Helper()
	return &stubStore{
		accountID:    mustAccountID(test, "acct-1"),
		total:        initialTotal,
		reservations: make(map[ReservationID]Reservation),
		idempotency:  make(map[IdempotencyKey]struct{}),
	}
}

func (store *stubStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	return fn(ctx, store)
}

func (store *stubStore) GetOrCreateAccountID(ctx context.Context, userID UserID) (AccountID, error) {
	return store.accountID, nil
}

func (store *stubStore) InsertEntry(ctx context.Context, entryInput EntryInput) error {
	if _, exists := store.idempotency[entryInput.IdempotencyKey()]; exists {
		return ErrDuplicateIdempotencyKey
	}
	store.idempotency[entryInput.IdempotencyKey()] = struct{}{}
	store.entries = append(store.entries, entryInput)
	switch entryInput.Type() {
	case EntryGrant, EntrySpend:
		updatedTotal, err := applyEntryDelta(store.total, entryInput.AmountCents())
		if err != nil {
			return err
		}
		store.total = updatedTotal
	}
	return nil
}

func (store *stubStore) SumTotal(ctx context.Context, accountID AccountID, _ int64) (AmountCents, error) {
	return store.total, nil
}

func (store *stubStore) SumActiveHolds(ctx context.Context, accountID AccountID, _ int64) (AmountCents, error) {
	var sum int64
	for _, reservation := range store.reservations {
		if reservation.Status() == ReservationStatusActive {
			sum += reservation.AmountCents().Int64()
		}
	}
	return NewAmountCents(sum)
}

func (store *stubStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	reservationID := reservation.ReservationID()
	if _, exists := store.reservations[reservationID]; exists {
		return ErrReservationExists
	}
	store.reservations[reservationID] = reservation
	return nil
}

func (store *stubStore) GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error) {
	reservation, ok := store.reservations[reservationID]
	if !ok {
		return Reservation{}, ErrUnknownReservation
	}
	return reservation, nil
}

func (store *stubStore) UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from, to ReservationStatus) error {
	reservation, ok := store.reservations[reservationID]
	if !ok {
		return ErrUnknownReservation
	}
	if reservation.Status() != from {
		return ErrReservationClosed
	}
	updatedReservation, err := NewReservation(reservation.AccountID(), reservation.ReservationID(), reservation.AmountCents(), to)
	if err != nil {
		return err
	}
	store.reservations[reservationID] = updatedReservation
	return nil
}

func (store *stubStore) ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int) ([]Entry, error) {
	if store.listErr != nil {
		return nil, store.listErr
	}
	return append([]Entry(nil), store.listEntries...), nil
}

func (store *stubStore) mustReservation(test *testing.T, reservationID ReservationID) Reservation {
	test.Helper()
	reservation, ok := store.reservations[reservationID]
	if !ok {
		test.Fatalf("reservation %s not found", reservationID.String())
	}
	return reservation
}

func applyEntryDelta(total AmountCents, delta EntryAmountCents) (AmountCents, error) {
	updated := total.Int64() + delta.Int64()
	return NewAmountCents(updated)
}

func mustNewService(test *testing.T, store Store) *Service {
	test.Helper()
	service, err := NewService(store, func() int64 { return 100 })
	if err != nil {
		test.Fatalf("new service: %v", err)
	}
	return service
}

func mustUserID(test *testing.T, raw string) UserID {
	test.Helper()
	value, err := NewUserID(raw)
	if err != nil {
		test.Fatalf("user id: %v", err)
	}
	return value
}

func mustReservationID(test *testing.T, raw string) ReservationID {
	test.Helper()
	value, err := NewReservationID(raw)
	if err != nil {
		test.Fatalf("reservation id: %v", err)
	}
	return value
}

func mustIdempotencyKey(test *testing.T, raw string) IdempotencyKey {
	test.Helper()
	value, err := NewIdempotencyKey(raw)
	if err != nil {
		test.Fatalf("idempotency key: %v", err)
	}
	return value
}

func mustMetadata(test *testing.T, raw string) MetadataJSON {
	test.Helper()
	value, err := NewMetadataJSON(raw)
	if err != nil {
		test.Fatalf("metadata: %v", err)
	}
	return value
}

func mustPositiveAmount(test *testing.T, raw int64) PositiveAmountCents {
	test.Helper()
	value, err := NewPositiveAmountCents(raw)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	return value
}

func mustAmountCents(test *testing.T, raw int64) AmountCents {
	test.Helper()
	value, err := NewAmountCents(raw)
	if err != nil {
		test.Fatalf("amount: %v", err)
	}
	return value
}

func mustEntryAmount(test *testing.T, raw int64) EntryAmountCents {
	test.Helper()
	value, err := NewEntryAmountCents(raw)
	if err != nil {
		test.Fatalf("entry amount: %v", err)
	}
	return value
}

func mustAccountID(test *testing.T, raw string) AccountID {
	test.Helper()
	value, err := NewAccountID(raw)
	if err != nil {
		test.Fatalf("account id: %v", err)
	}
	return value
}

func mustEntryID(test *testing.T, raw string) EntryID {
	test.Helper()
	value, err := NewEntryID(raw)
	if err != nil {
		test.Fatalf("entry id: %v", err)
	}
	return value
}

func mustReservationRecord(test *testing.T, accountID AccountID, reservationID ReservationID, amount PositiveAmountCents, status ReservationStatus) Reservation {
	test.Helper()
	reservation, err := NewReservation(accountID, reservationID, amount, status)
	if err != nil {
		test.Fatalf("reservation: %v", err)
	}
	return reservation
}

func mustEntry(test *testing.T, entryID EntryID, accountID AccountID, entryType EntryType, amount EntryAmountCents, idempotencyKey IdempotencyKey, metadata MetadataJSON) Entry {
	test.Helper()
	entry, err := NewEntry(entryID, accountID, entryType, amount, nil, idempotencyKey, 0, metadata, 100)
	if err != nil {
		test.Fatalf("entry: %v", err)
	}
	return entry
}

type mockStore struct {
	*stubStore
}

func newMockStore(test *testing.T) *mockStore {
	return &mockStore{stubStore: newStubStore(test, mustAmountCents(test, 0))}
}

type failingStore struct {
	Store
	err         error
	accountID   AccountID
	total       AmountCents
	activeHolds AmountCents
}

func newFailingStore(test *testing.T, err error) *failingStore {
	test.Helper()
	return &failingStore{
		err:         err,
		accountID:   mustAccountID(test, "acct"),
		total:       mustAmountCents(test, 1000),
		activeHolds: mustAmountCents(test, 0),
	}
}

func (store *failingStore) WithTx(ctx context.Context, fn func(ctx context.Context, txStore Store) error) error {
	if fn == nil {
		return nil
	}
	return fn(ctx, store)
}

func (store *failingStore) GetOrCreateAccountID(ctx context.Context, userID UserID) (AccountID, error) {
	return store.accountID, nil
}

func (store *failingStore) InsertEntry(ctx context.Context, entry EntryInput) error {
	return store.err
}

func (store *failingStore) SumTotal(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error) {
	return store.total, nil
}

func (store *failingStore) SumActiveHolds(ctx context.Context, accountID AccountID, atUnixUTC int64) (AmountCents, error) {
	return store.activeHolds, nil
}

func (store *failingStore) CreateReservation(ctx context.Context, reservation Reservation) error {
	return nil
}

func (store *failingStore) GetReservation(ctx context.Context, accountID AccountID, reservationID ReservationID) (Reservation, error) {
	return Reservation{}, nil
}

func (store *failingStore) UpdateReservationStatus(ctx context.Context, accountID AccountID, reservationID ReservationID, from, to ReservationStatus) error {
	return nil
}

func (store *failingStore) ListEntries(ctx context.Context, accountID AccountID, beforeUnixUTC int64, limit int) ([]Entry, error) {
	return nil, nil
}
