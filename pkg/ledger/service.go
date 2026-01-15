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
func (service *Service) Balance(ctx context.Context, userID UserID) (Balance, error) {
	accountID, err := service.store.GetOrCreateAccountID(ctx, userID)
	if err != nil {
		return Balance{}, err
	}
	nowUnixUTC := service.nowFn()
	total, err := service.store.SumTotal(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Balance{}, err
	}
	holds, err := service.store.SumActiveHolds(ctx, accountID, nowUnixUTC)
	if err != nil {
		return Balance{}, err
	}
	available, err := calculateAvailable(total, holds)
	if err != nil {
		return Balance{}, err
	}
	return Balance{
		TotalCents:     total,
		AvailableCents: available,
	}, nil
}

// Grant appends a positive grant (optionally expiring).
func (service *Service) Grant(ctx context.Context, userID UserID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) error {
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntryGrant,
			amount.ToEntryAmountCents(),
			nil,
			idempotencyKey,
			expiresAtUnixUTC,
			metadata,
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		return transactionStore.InsertEntry(ctx, entryInput)
	})
	service.logOperation(ctx, OperationLog{
		Operation:      operationGrant,
		UserID:         userID,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	return operationError
}

// Reserve appends a negative hold if sufficient available balance.
func (service *Service) Reserve(ctx context.Context, userID UserID, amount PositiveAmountCents, reservationID ReservationID, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		nowUnixUTC := service.nowFn()
		total, err := transactionStore.SumTotal(ctx, accountID, nowUnixUTC)
		if err != nil {
			return err
		}
		holds, err := transactionStore.SumActiveHolds(ctx, accountID, nowUnixUTC)
		if err != nil {
			return err
		}
		available, err := calculateAvailable(total, holds)
		if err != nil {
			return err
		}
		if available < amount.ToAmountCents() {
			return ErrInsufficientFunds
		}
		reservation, err := NewReservation(accountID, reservationID, amount, ReservationStatusActive)
		if err != nil {
			return err
		}
		if err := transactionStore.CreateReservation(ctx, reservation); err != nil {
			return err
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntryHold,
			amount.ToEntryAmountCents().Negated(),
			&reservationID,
			idempotencyKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		return transactionStore.InsertEntry(ctx, entryInput)
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationReserve,
		UserID:         userID,
		ReservationID:  &reservationRef,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	return operationError
}

// Capture finalizes a reservation by reversing the hold and spending the funds with distinct idempotency keys.
func (service *Service) Capture(ctx context.Context, userID UserID, reservationID ReservationID, idempotencyKey IdempotencyKey, amount PositiveAmountCents, metadata MetadataJSON) error {
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		reservation, err := transactionStore.GetReservation(ctx, accountID, reservationID)
		if err != nil {
			return err
		}
		if reservation.Status() != ReservationStatusActive {
			return ErrReservationClosed
		}
		if reservation.AmountCents() != amount {
			return fmt.Errorf("%w: capture amount mismatch", ErrInvalidAmountCents)
		}
		if err := transactionStore.UpdateReservationStatus(ctx, accountID, reservationID, ReservationStatusActive, ReservationStatusCaptured); err != nil {
			return err
		}
		nowUnixUTC := service.nowFn()
		reverseKey, err := deriveIdempotencyKey(idempotencyKey, idempotencySuffixReverse)
		if err != nil {
			return err
		}
		reverseEntry, err := NewEntryInput(
			accountID,
			EntryReverseHold,
			reservation.AmountCents().ToEntryAmountCents(),
			&reservationID,
			reverseKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		if err := transactionStore.InsertEntry(ctx, reverseEntry); err != nil {
			return err
		}
		spendKey, err := deriveIdempotencyKey(idempotencyKey, idempotencySuffixSpend)
		if err != nil {
			return err
		}
		spendEntry, err := NewEntryInput(
			accountID,
			EntrySpend,
			amount.ToEntryAmountCents().Negated(),
			&reservationID,
			spendKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		return transactionStore.InsertEntry(ctx, spendEntry)
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationCapture,
		UserID:         userID,
		ReservationID:  &reservationRef,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	return operationError
}

// Release cancels a reservation by writing a reverse-hold entry.
func (service *Service) Release(ctx context.Context, userID UserID, reservationID ReservationID, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	var reservationAmount AmountCents
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, userID)
		if err != nil {
			return err
		}
		reservation, err := transactionStore.GetReservation(ctx, accountID, reservationID)
		if err != nil {
			return err
		}
		if reservation.Status() != ReservationStatusActive {
			return ErrReservationClosed
		}
		reservationAmount = reservation.AmountCents().ToAmountCents()
		if err := transactionStore.UpdateReservationStatus(ctx, accountID, reservationID, ReservationStatusActive, ReservationStatusReleased); err != nil {
			return err
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntryReverseHold,
			reservation.AmountCents().ToEntryAmountCents(),
			&reservationID,
			idempotencyKey,
			0,
			metadata,
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		return transactionStore.InsertEntry(ctx, entryInput)
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationRelease,
		UserID:         userID,
		ReservationID:  &reservationRef,
		Amount:         reservationAmount,
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	return operationError
}

func (service *Service) logOperation(ctx context.Context, entry OperationLog) {
	if service.logger == nil {
		return
	}
	if entry.Status == "" {
		if entry.Error != nil {
			entry.Status = operationStatusError
		} else {
			entry.Status = operationStatusOK
		}
	}
	service.logger.LogOperation(ctx, entry)
}

func deriveIdempotencyKey(baseKey IdempotencyKey, suffix string) (IdempotencyKey, error) {
	combined := baseKey.String() + idempotencyKeyDelimiter + suffix
	return NewIdempotencyKey(combined)
}

func calculateAvailable(total AmountCents, holds AmountCents) (AmountCents, error) {
	availableRaw := total.Int64() - holds.Int64()
	available, err := NewAmountCents(availableRaw)
	if err != nil {
		return 0, WrapError("service", "balance", "negative_available", ErrInvalidBalance)
	}
	return available, nil
}
