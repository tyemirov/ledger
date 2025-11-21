package ledger

import (
	"context"
	"fmt"
)

// Service contains the domain logic over a Store.
type Service struct {
	store  Store
	nowFn  func() int64
	logger OperationLogger
}

// NewService wires a Service.
func NewService(store Store, now func() int64, options ...ServiceOption) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("%w: store dependency is nil", ErrInvalidServiceConfig)
	}
	if now == nil {
		return nil, fmt.Errorf("%w: clock dependency is nil", ErrInvalidServiceConfig)
	}
	service := &Service{store: store, nowFn: now}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service, nil
}

// Balance returns total and available (total minus active holds).
func (s *Service) Balance(ctx context.Context, userID UserID) (Balance, error) {
	accountID, err := s.store.GetOrCreateAccountID(ctx, userID.String())
	if err != nil {
		return Balance{}, err
	}
	now := s.nowFn()
	total, err := s.store.SumTotal(ctx, accountID, now)
	if err != nil {
		return Balance{}, err
	}
	holds, err := s.store.SumActiveHolds(ctx, accountID, now)
	if err != nil {
		return Balance{}, err
	}
	return Balance{
		TotalCents:     total,
		AvailableCents: total - holds,
	}, nil
}

// Grant appends a positive grant (optionally expiring).
func (s *Service) Grant(ctx context.Context, userID UserID, amount AmountCents, idem IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) error {
	err := s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID.String())
		if err != nil {
			return err
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:        accountID,
			Type:             EntryGrant,
			AmountCents:      amount,
			IdempotencyKey:   idem.String(),
			ExpiresAtUnixUTC: expiresAtUnixUTC,
			MetadataJSON:     metadata.String(),
			CreatedUnixUTC:   s.nowFn(),
		})
	})
	s.logOperation(ctx, OperationLog{
		Operation:      "grant",
		UserID:         userID,
		Amount:         amount,
		IdempotencyKey: idem,
		Metadata:       metadata,
		Error:          err,
	})
	return err
}

// Reserve appends a negative hold if sufficient available balance.
func (s *Service) Reserve(ctx context.Context, userID UserID, amount AmountCents, reservationID ReservationID, idem IdempotencyKey, metadata MetadataJSON) error {
	err := s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID.String())
		if err != nil {
			return err
		}
		now := s.nowFn()
		total, err := txStore.SumTotal(ctx, accountID, now)
		if err != nil {
			return err
		}
		holds, err := txStore.SumActiveHolds(ctx, accountID, now)
		if err != nil {
			return err
		}
		if total-holds < amount {
			return ErrInsufficientFunds
		}
		if err := txStore.CreateReservation(ctx, Reservation{
			AccountID:     accountID,
			ReservationID: reservationID.String(),
			AmountCents:   amount,
			Status:        ReservationStatusActive,
		}); err != nil {
			return err
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntryHold,
			AmountCents:    -amount,
			ReservationID:  reservationID.String(),
			IdempotencyKey: idem.String(),
			MetadataJSON:   metadata.String(),
			CreatedUnixUTC: now,
		})
	})
	s.logOperation(ctx, OperationLog{
		Operation:      "reserve",
		UserID:         userID,
		Amount:         amount,
		ReservationID:  reservationID,
		IdempotencyKey: idem,
		Metadata:       metadata,
		Error:          err,
	})
	return err
}

// Capture finalizes a reservation by reversing the hold and spending the funds with distinct idempotency keys.
func (s *Service) Capture(ctx context.Context, userID UserID, reservationID ReservationID, idem IdempotencyKey, amount AmountCents, metadata MetadataJSON) error {
	err := s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID.String())
		if err != nil {
			return err
		}
		reservation, err := txStore.GetReservation(ctx, accountID, reservationID.String())
		if err != nil {
			return err
		}
		if reservation.Status != ReservationStatusActive {
			return ErrReservationClosed
		}
		if reservation.AmountCents != amount {
			return fmt.Errorf("%w: capture amount mismatch", ErrInvalidAmountCents)
		}
		if err := txStore.UpdateReservationStatus(ctx, accountID, reservationID.String(), ReservationStatusActive, ReservationStatusCaptured); err != nil {
			return err
		}
		now := s.nowFn()
		reverseIdempotencyKey := fmt.Sprintf("%s:reverse", idem.String())
		if err := txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntryReverseHold,
			AmountCents:    reservation.AmountCents,
			ReservationID:  reservationID.String(),
			IdempotencyKey: reverseIdempotencyKey,
			MetadataJSON:   metadata.String(),
			CreatedUnixUTC: now,
		}); err != nil {
			return err
		}
		spendIdempotencyKey := fmt.Sprintf("%s:spend", idem.String())
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntrySpend,
			AmountCents:    -amount,
			ReservationID:  reservationID.String(),
			IdempotencyKey: spendIdempotencyKey,
			MetadataJSON:   metadata.String(),
			CreatedUnixUTC: now,
		})
	})
	s.logOperation(ctx, OperationLog{
		Operation:      "capture",
		UserID:         userID,
		ReservationID:  reservationID,
		Amount:         amount,
		IdempotencyKey: idem,
		Metadata:       metadata,
		Error:          err,
	})
	return err
}

// Release cancels a reservation by writing a reverse-hold entry.
// (This simple version doesn’t compute the exact held amount — adjust as needed.)
func (s *Service) Release(ctx context.Context, userID UserID, reservationID ReservationID, idem IdempotencyKey, metadata MetadataJSON) error {
	err := s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID.String())
		if err != nil {
			return err
		}
		reservation, err := txStore.GetReservation(ctx, accountID, reservationID.String())
		if err != nil {
			return err
		}
		if reservation.Status != ReservationStatusActive {
			return ErrReservationClosed
		}
		if err := txStore.UpdateReservationStatus(ctx, accountID, reservationID.String(), ReservationStatusActive, ReservationStatusReleased); err != nil {
			return err
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntryReverseHold,
			AmountCents:    reservation.AmountCents,
			ReservationID:  reservationID.String(),
			IdempotencyKey: idem.String(),
			MetadataJSON:   metadata.String(),
			CreatedUnixUTC: s.nowFn(),
		})
	})
	s.logOperation(ctx, OperationLog{
		Operation:      "release",
		UserID:         userID,
		ReservationID:  reservationID,
		IdempotencyKey: idem,
		Metadata:       metadata,
		Error:          err,
	})
	return err
}

func (s *Service) logOperation(ctx context.Context, entry OperationLog) {
	if s.logger == nil {
		return
	}
	if entry.Status == "" {
		if entry.Error != nil {
			entry.Status = "error"
		} else {
			entry.Status = "ok"
		}
	}
	s.logger.LogOperation(ctx, entry)
}
