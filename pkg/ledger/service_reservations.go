package ledger

import "context"

// ReservationState is a computed view of a reservation suitable for introspection APIs.
type ReservationState struct {
	ReservationID    ReservationID
	AmountCents      PositiveAmountCents
	Status           ReservationStatus
	ExpiresAtUnixUTC int64
	CreatedUnixUTC   int64
	UpdatedUnixUTC   int64
	Expired          bool
	HeldCents        AmountCents
	CapturedCents    AmountCents
}

// GetReservationState returns the computed state for a reservation.
func (service *Service) GetReservationState(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, reservationID ReservationID) (ReservationState, error) {
	accountID, err := service.store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
	if err != nil {
		return ReservationState{}, err
	}
	reservation, err := service.store.GetReservation(ctx, accountID, reservationID)
	if err != nil {
		return ReservationState{}, err
	}
	nowUnixUTC := service.nowFn()
	return reservationStateFromReservation(reservation, nowUnixUTC), nil
}

// ListReservationStates returns the computed states for reservations matching the supplied filters.
func (service *Service) ListReservationStates(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, beforeCreatedUnixUTC int64, limit int, filter ListReservationsFilter) ([]ReservationState, error) {
	accountID, err := service.store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
	if err != nil {
		return nil, err
	}
	reservations, err := service.store.ListReservations(ctx, accountID, beforeCreatedUnixUTC, limit, filter)
	if err != nil {
		return nil, err
	}
	nowUnixUTC := service.nowFn()
	states := make([]ReservationState, 0, len(reservations))
	for _, reservation := range reservations {
		states = append(states, reservationStateFromReservation(reservation, nowUnixUTC))
	}
	return states, nil
}

func reservationStateFromReservation(reservation Reservation, nowUnixUTC int64) ReservationState {
	expired := reservation.ExpiresAtUnixUTC() != 0 && reservation.ExpiresAtUnixUTC() <= nowUnixUTC
	amount := reservation.AmountCents()
	held := AmountCents(0)
	if reservation.Status() == ReservationStatusActive && !expired {
		held = amount.ToAmountCents()
	}
	captured := AmountCents(0)
	if reservation.Status() == ReservationStatusCaptured {
		captured = amount.ToAmountCents()
	}
	return ReservationState{
		ReservationID:    reservation.ReservationID(),
		AmountCents:      amount,
		Status:           reservation.Status(),
		ExpiresAtUnixUTC: reservation.ExpiresAtUnixUTC(),
		CreatedUnixUTC:   reservation.CreatedUnixUTC(),
		UpdatedUnixUTC:   reservation.UpdatedUnixUTC(),
		Expired:          expired,
		HeldCents:        held,
		CapturedCents:    captured,
	}
}
