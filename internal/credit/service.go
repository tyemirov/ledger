package credit

import "context"

// Service contains the domain logic over a Store.
type Service struct {
	store Store
	nowFn func() int64
}

// NewService wires a Service.
func NewService(store Store, now func() int64) *Service {
	if now == nil {
		now = func() int64 { return 0 }
	}
	return &Service{store: store, nowFn: now}
}

// Balance returns total and available (total + active holds, holds are negative).
func (s *Service) Balance(ctx context.Context, userID string) (Balance, error) {
	accountID, err := s.store.GetOrCreateAccountID(ctx, userID)
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
		AvailableCents: total + holds, // holds are negative
	}, nil
}

// Grant appends a positive grant (optionally expiring).
func (s *Service) Grant(ctx context.Context, userID string, amount AmountCents, idem string, expiresAtUnixUTC int64, metadataJSON string) error {
	return s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:        accountID,
			Type:             EntryGrant,
			AmountCents:      amount,
			IdempotencyKey:   idem,
			ExpiresAtUnixUTC: expiresAtUnixUTC,
			MetadataJSON:     metadataJSON,
			CreatedUnixUTC:   s.nowFn(),
		})
	})
}

// Reserve appends a negative hold if sufficient available balance.
func (s *Service) Reserve(ctx context.Context, userID string, amount AmountCents, reservationID, idem, metadataJSON string) error {
	return s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID)
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
		if total+holds < amount {
			return ErrInsufficientFunds
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntryHold,
			AmountCents:    -amount,
			ReservationID:  reservationID,
			IdempotencyKey: idem,
			MetadataJSON:   metadataJSON,
			CreatedUnixUTC: now,
		})
	})
}

// Capture spends against a reservation (basic existence check).
func (s *Service) Capture(ctx context.Context, userID, reservationID, idem string, amount AmountCents, metadataJSON string) error {
	return s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		ok, err := txStore.ReservationExists(ctx, accountID, reservationID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnknownReservation
		}
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntrySpend,
			AmountCents:    -amount,
			ReservationID:  reservationID,
			IdempotencyKey: idem,
			MetadataJSON:   metadataJSON,
			CreatedUnixUTC: s.nowFn(),
		})
	})
}

// Release cancels a reservation by writing a reverse-hold entry.
// (This simple version doesn’t compute the exact held amount — adjust as needed.)
func (s *Service) Release(ctx context.Context, userID, reservationID, idem, metadataJSON string) error {
	return s.store.WithTx(ctx, func(ctx context.Context, txStore Store) error {
		accountID, err := txStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		ok, err := txStore.ReservationExists(ctx, accountID, reservationID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrUnknownReservation
		}
		// Write a zero-amount marker to keep the API flow working.
		// You can upgrade this to add back the held amount if you later store per-reservation sums.
		return txStore.InsertEntry(ctx, Entry{
			AccountID:      accountID,
			Type:           EntryReverseHold,
			AmountCents:    0,
			ReservationID:  reservationID,
			IdempotencyKey: idem,
			MetadataJSON:   metadataJSON,
			CreatedUnixUTC: s.nowFn(),
		})
	})
}
