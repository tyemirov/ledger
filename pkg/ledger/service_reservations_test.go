package ledger

import (
	"context"
	"errors"
	"testing"
)

func TestGetReservationStateComputesDerivedFields(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)

	ctx := context.Background()
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 40)

	activeID := mustReservationID(test, "res-active")
	activeReservation, err := NewReservationWithTimestamps(store.accountID, activeID, amount, ReservationStatusActive, 150, 10, 20)
	if err != nil {
		test.Fatalf("active reservation: %v", err)
	}
	store.reservations[activeID] = activeReservation

	capturedID := mustReservationID(test, "res-captured")
	capturedReservation, err := NewReservationWithTimestamps(store.accountID, capturedID, amount, ReservationStatusCaptured, 0, 11, 21)
	if err != nil {
		test.Fatalf("captured reservation: %v", err)
	}
	store.reservations[capturedID] = capturedReservation

	expiredID := mustReservationID(test, "res-expired")
	expiredReservation, err := NewReservationWithTimestamps(store.accountID, expiredID, amount, ReservationStatusActive, 100, 12, 22)
	if err != nil {
		test.Fatalf("expired reservation: %v", err)
	}
	store.reservations[expiredID] = expiredReservation

	releasedID := mustReservationID(test, "res-released")
	releasedReservation, err := NewReservationWithTimestamps(store.accountID, releasedID, amount, ReservationStatusReleased, 0, 13, 23)
	if err != nil {
		test.Fatalf("released reservation: %v", err)
	}
	store.reservations[releasedID] = releasedReservation

	activeState, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, activeID)
	if err != nil {
		test.Fatalf("get active state: %v", err)
	}
	if activeState.Status != ReservationStatusActive || activeState.Expired {
		test.Fatalf("unexpected active state: %+v", activeState)
	}
	if activeState.HeldCents.Int64() != 40 || activeState.CapturedCents.Int64() != 0 {
		test.Fatalf("unexpected active held/captured: %+v", activeState)
	}
	if activeState.CreatedUnixUTC != 10 || activeState.UpdatedUnixUTC != 20 {
		test.Fatalf("unexpected active timestamps: %+v", activeState)
	}

	capturedState, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, capturedID)
	if err != nil {
		test.Fatalf("get captured state: %v", err)
	}
	if capturedState.Status != ReservationStatusCaptured {
		test.Fatalf("unexpected captured status: %+v", capturedState)
	}
	if capturedState.HeldCents.Int64() != 0 || capturedState.CapturedCents.Int64() != 40 {
		test.Fatalf("unexpected captured held/captured: %+v", capturedState)
	}

	expiredState, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, expiredID)
	if err != nil {
		test.Fatalf("get expired state: %v", err)
	}
	if expiredState.Status != ReservationStatusActive || !expiredState.Expired {
		test.Fatalf("unexpected expired state: %+v", expiredState)
	}
	if expiredState.HeldCents.Int64() != 0 || expiredState.CapturedCents.Int64() != 0 {
		test.Fatalf("unexpected expired held/captured: %+v", expiredState)
	}

	releasedState, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, releasedID)
	if err != nil {
		test.Fatalf("get released state: %v", err)
	}
	if releasedState.Status != ReservationStatusReleased || releasedState.Expired {
		test.Fatalf("unexpected released state: %+v", releasedState)
	}
	if releasedState.HeldCents.Int64() != 0 || releasedState.CapturedCents.Int64() != 0 {
		test.Fatalf("unexpected released held/captured: %+v", releasedState)
	}
}

func TestListReservationStatesAppliesFilters(test *testing.T) {
	test.Parallel()
	store := newStubStore(test, mustSignedAmount(test, 0))
	service := mustNewService(test, store)

	ctx := context.Background()
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	amount := mustPositiveAmount(test, 50)

	activeID := mustReservationID(test, "res-active")
	activeReservation, err := NewReservationWithTimestamps(store.accountID, activeID, amount, ReservationStatusActive, 0, 10, 10)
	if err != nil {
		test.Fatalf("active reservation: %v", err)
	}
	store.reservations[activeID] = activeReservation

	capturedID := mustReservationID(test, "res-captured")
	capturedReservation, err := NewReservationWithTimestamps(store.accountID, capturedID, amount, ReservationStatusCaptured, 0, 11, 11)
	if err != nil {
		test.Fatalf("captured reservation: %v", err)
	}
	store.reservations[capturedID] = capturedReservation

	states, err := service.ListReservationStates(ctx, tenantID, userID, ledgerID, 0, 10, ListReservationsFilter{
		Statuses: []ReservationStatus{ReservationStatusCaptured},
	})
	if err != nil {
		test.Fatalf("list states: %v", err)
	}
	if len(states) != 1 || states[0].ReservationID != capturedID {
		test.Fatalf("unexpected states: %+v", states)
	}
	if states[0].CapturedCents.Int64() != 50 || states[0].HeldCents.Int64() != 0 {
		test.Fatalf("unexpected state values: %+v", states[0])
	}
}

func TestReservationStateMethodsPropagateStoreErrors(test *testing.T) {
	test.Parallel()
	ctx := context.Background()
	userID := mustUserID(test, "user-123")
	ledgerID := mustLedgerID(test, defaultLedgerIDValue)
	tenantID := mustTenantID(test, defaultTenantIDValue)
	reservationID := mustReservationID(test, "res-1")
	sentinelError := errors.New("boom")

	store := newStubStore(test, mustSignedAmount(test, 0))
	store.getAccountError = sentinelError
	service := mustNewService(test, store)

	if _, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, reservationID); !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}

	store.getAccountError = nil
	store.getReservationError = sentinelError
	if _, err := service.GetReservationState(ctx, tenantID, userID, ledgerID, reservationID); !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}

	store.getReservationError = nil
	store.listErr = sentinelError
	if _, err := service.ListReservationStates(ctx, tenantID, userID, ledgerID, 0, 10, ListReservationsFilter{}); !errors.Is(err, sentinelError) {
		test.Fatalf("expected sentinel error, got %v", err)
	}
}
