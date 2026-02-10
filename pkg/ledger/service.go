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
func (service *Service) Balance(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID) (Balance, error) {
	accountID, err := service.store.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
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
	available := calculateAvailable(total, holds)
	return Balance{
		TotalCents:     total,
		AvailableCents: available,
	}, nil
}

// Grant appends a positive grant (optionally expiring).
func (service *Service) Grant(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) error {
	_, err := service.GrantEntry(ctx, tenantID, userID, ledgerID, amount, idempotencyKey, expiresAtUnixUTC, metadata)
	return err
}

// GrantEntry appends a positive grant (optionally expiring) and returns the persisted entry.
func (service *Service) GrantEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) (Entry, error) {
	var persistedEntry Entry
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		entryInput, err := NewEntryInput(
			accountID,
			EntryGrant,
			amount.ToEntryAmountCents(),
			nil,
			nil,
			idempotencyKey,
			expiresAtUnixUTC,
			metadata,
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, entryInput)
		return err
	})
	service.logOperation(ctx, OperationLog{
		Operation:      operationGrant,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	if operationError != nil {
		return Entry{}, operationError
	}
	return persistedEntry, nil
}

// Reserve appends a negative hold if sufficient available balance.
func (service *Service) Reserve(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, reservationID ReservationID, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) error {
	_, err := service.ReserveEntry(ctx, tenantID, userID, ledgerID, amount, reservationID, idempotencyKey, expiresAtUnixUTC, metadata)
	return err
}

// ReserveEntry appends a negative hold if sufficient available balance and returns the persisted hold entry.
func (service *Service) ReserveEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, amount PositiveAmountCents, reservationID ReservationID, idempotencyKey IdempotencyKey, expiresAtUnixUTC int64, metadata MetadataJSON) (Entry, error) {
	var persistedEntry Entry
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
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
		available := calculateAvailable(total, holds)
		amountCents := amount.ToAmountCents()
		if available.Int64() < amountCents.Int64() {
			return ErrInsufficientFunds
		}
		reservation, err := NewReservation(accountID, reservationID, amount, ReservationStatusActive, expiresAtUnixUTC)
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
			nil,
			idempotencyKey,
			expiresAtUnixUTC,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, entryInput)
		return err
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationReserve,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		ReservationID:  &reservationRef,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	if operationError != nil {
		return Entry{}, operationError
	}
	return persistedEntry, nil
}

// Capture finalizes a reservation by reversing the hold and spending the funds with distinct idempotency keys.
func (service *Service) Capture(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, reservationID ReservationID, idempotencyKey IdempotencyKey, amount PositiveAmountCents, metadata MetadataJSON) error {
	_, err := service.CaptureDebitEntry(ctx, tenantID, userID, ledgerID, reservationID, idempotencyKey, amount, metadata)
	return err
}

// CaptureDebitEntry finalizes a reservation and returns the persisted debit entry.
func (service *Service) CaptureDebitEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, reservationID ReservationID, idempotencyKey IdempotencyKey, amount PositiveAmountCents, metadata MetadataJSON) (Entry, error) {
	var persistedEntry Entry
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
		if err != nil {
			return err
		}
		nowUnixUTC := service.nowFn()
		reservation, err := transactionStore.GetReservation(ctx, accountID, reservationID)
		if err != nil {
			return err
		}
		if reservation.Status() != ReservationStatusActive {
			return ErrReservationClosed
		}
		if reservation.ExpiresAtUnixUTC() != 0 && reservation.ExpiresAtUnixUTC() <= nowUnixUTC {
			return ErrReservationClosed
		}
		if reservation.AmountCents() != amount {
			return fmt.Errorf("%w: capture amount mismatch", ErrInvalidAmountCents)
		}
		if err := transactionStore.UpdateReservationStatus(ctx, accountID, reservationID, ReservationStatusActive, ReservationStatusCaptured); err != nil {
			return err
		}
		reverseKey, err := deriveIdempotencyKey(idempotencyKey, idempotencySuffixReverse)
		if err != nil {
			return err
		}
		reverseEntry, err := NewEntryInput(
			accountID,
			EntryReverseHold,
			reservation.AmountCents().ToEntryAmountCents(),
			&reservationID,
			nil,
			reverseKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		if _, err := transactionStore.InsertEntry(ctx, reverseEntry); err != nil {
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
			nil,
			spendKey,
			0,
			metadata,
			nowUnixUTC,
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, spendEntry)
		return err
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationCapture,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		ReservationID:  &reservationRef,
		Amount:         amount.ToAmountCents(),
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	if operationError != nil {
		return Entry{}, operationError
	}
	return persistedEntry, nil
}

// Release cancels a reservation by writing a reverse-hold entry.
func (service *Service) Release(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, reservationID ReservationID, idempotencyKey IdempotencyKey, metadata MetadataJSON) error {
	_, err := service.ReleaseEntry(ctx, tenantID, userID, ledgerID, reservationID, idempotencyKey, metadata)
	return err
}

// ReleaseEntry cancels a reservation by writing a reverse-hold entry and returns the persisted entry.
func (service *Service) ReleaseEntry(ctx context.Context, tenantID TenantID, userID UserID, ledgerID LedgerID, reservationID ReservationID, idempotencyKey IdempotencyKey, metadata MetadataJSON) (Entry, error) {
	var reservationAmount AmountCents
	var persistedEntry Entry
	operationError := service.store.WithTx(ctx, func(ctx context.Context, transactionStore Store) error {
		accountID, err := transactionStore.GetOrCreateAccountID(ctx, tenantID, userID, ledgerID)
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
			nil,
			idempotencyKey,
			0,
			metadata,
			service.nowFn(),
		)
		if err != nil {
			return err
		}
		persistedEntry, err = transactionStore.InsertEntry(ctx, entryInput)
		return err
	})
	reservationRef := reservationID
	service.logOperation(ctx, OperationLog{
		Operation:      operationRelease,
		TenantID:       tenantID,
		UserID:         userID,
		LedgerID:       ledgerID,
		ReservationID:  &reservationRef,
		Amount:         reservationAmount,
		IdempotencyKey: idempotencyKey,
		Metadata:       metadata,
		Error:          operationError,
	})
	if operationError != nil {
		return Entry{}, operationError
	}
	return persistedEntry, nil
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

func calculateAvailable(total SignedAmountCents, holds AmountCents) SignedAmountCents {
	availableRaw := total.Int64() - holds.Int64()
	available, _ := NewSignedAmountCents(availableRaw)
	return available
}
