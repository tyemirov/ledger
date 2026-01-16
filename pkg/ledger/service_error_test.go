package ledger

import (
	"context"
	"errors"
	"testing"
)

const (
	userIDValue             = "user-1"
	errStoreMessage         = "store error"
	caseAccountLookupError  = "account lookup error"
	caseSumTotalError       = "sum total error"
	caseSumActiveHoldsError = "sum active holds error"
	caseInsertEntryError    = "insert entry error"
	caseReservationExists   = "reservation exists"
	caseReservationLookup   = "reservation lookup error"
	caseReservationClosed   = "reservation closed"
	caseUpdateReservation   = "update reservation error"
	caseReverseEntryError   = "reverse entry error"
	caseSpendEntryError     = "spend entry error"
	caseListEntriesError    = "list entries error"
	errorMismatchMessage    = "expected %v, got %v"
)

var errStoreFailure = errors.New(errStoreMessage)

func TestBalanceReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumTotalError,
			configure: func(test *testing.T, store *stubStore) {
				store.sumTotalError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumActiveHoldsError,
			configure: func(test *testing.T, store *stubStore) {
				store.sumActiveHoldsError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 100))
			testCase.configure(test, store)
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			_, err := service.Balance(context.Background(), tenantID, userID, ledgerID)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestGrantReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseInsertEntryError,
			configure: func(test *testing.T, store *stubStore) {
				store.insertEntryError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 100))
			testCase.configure(test, store)
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)
			idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
			metadata := mustMetadata(test, metadataValue)
			amount := mustPositiveAmount(test, 10)

			err := service.Grant(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, 0, metadata)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestReserveReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumTotalError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.sumTotalError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumActiveHoldsError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.sumActiveHoldsError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReservationExists,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: ErrReservationExists,
		},
		{
			name: caseInsertEntryError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.insertEntryError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 200))
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)
			reservationID := mustReservationID(test, reservationIDValue)
			idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
			metadata := mustMetadata(test, metadataValue)
			amount := mustPositiveAmount(test, 50)

			testCase.configure(test, store, reservationID, amount)

			err := service.Reserve(context.Background(), tenantID, userID, ledgerID, amount, reservationID, idempotencyKey, metadata)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestCaptureReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReservationLookup,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.getReservationError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReservationClosed,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusCaptured)
			},
			wantErr: ErrReservationClosed,
		},
		{
			name: caseUpdateReservation,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.updateReservationError = errStoreFailure
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReverseEntryError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.insertEntryError = errStoreFailure
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSpendEntryError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.insertEntryError = errStoreFailure
				store.insertEntryErrorAtCall = 2
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 0))
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)
			reservationID := mustReservationID(test, reservationIDValue)
			idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
			metadata := mustMetadata(test, metadataValue)
			amount := mustPositiveAmount(test, 30)

			testCase.configure(test, store, reservationID, amount)

			err := service.Capture(context.Background(), tenantID, userID, ledgerID, reservationID, idempotencyKey, amount, metadata)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestReleaseReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReservationLookup,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.getReservationError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseReservationClosed,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusCaptured)
			},
			wantErr: ErrReservationClosed,
		},
		{
			name: caseUpdateReservation,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.updateReservationError = errStoreFailure
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseInsertEntryError,
			configure: func(test *testing.T, store *stubStore, reservationID ReservationID, amount PositiveAmountCents) {
				store.insertEntryError = errStoreFailure
				store.reservations[reservationID] = mustReservationRecord(test, store.accountID, reservationID, amount, ReservationStatusActive)
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 0))
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)
			reservationID := mustReservationID(test, reservationIDValue)
			idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
			metadata := mustMetadata(test, metadataValue)
			amount := mustPositiveAmount(test, 25)

			testCase.configure(test, store, reservationID, amount)

			err := service.Release(context.Background(), tenantID, userID, ledgerID, reservationID, idempotencyKey, metadata)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestSpendReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumTotalError,
			configure: func(test *testing.T, store *stubStore) {
				store.sumTotalError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseSumActiveHoldsError,
			configure: func(test *testing.T, store *stubStore) {
				store.sumActiveHoldsError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseInsertEntryError,
			configure: func(test *testing.T, store *stubStore) {
				store.insertEntryError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 200))
			testCase.configure(test, store)
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)
			idempotencyKey := mustIdempotencyKey(test, idempotencyValue)
			metadata := mustMetadata(test, metadataValue)
			amount := mustPositiveAmount(test, 25)

			err := service.Spend(context.Background(), tenantID, userID, ledgerID, amount, idempotencyKey, metadata)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}

func TestListEntriesReturnsStoreErrors(test *testing.T) {
	test.Parallel()
	testCases := []struct {
		name      string
		configure func(test *testing.T, store *stubStore)
		wantErr   error
	}{
		{
			name: caseAccountLookupError,
			configure: func(test *testing.T, store *stubStore) {
				store.getAccountError = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
		{
			name: caseListEntriesError,
			configure: func(test *testing.T, store *stubStore) {
				store.listErr = errStoreFailure
			},
			wantErr: errStoreFailure,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		test.Run(testCase.name, func(test *testing.T) {
			test.Parallel()
			store := newStubStore(test, mustSignedAmount(test, 0))
			testCase.configure(test, store)
			service := mustNewService(test, store)
			userID := mustUserID(test, userIDValue)
			ledgerID := mustLedgerID(test, defaultLedgerIDValue)
			tenantID := mustTenantID(test, defaultTenantIDValue)

			_, err := service.ListEntries(context.Background(), tenantID, userID, ledgerID, 0, 5)
			if !errors.Is(err, testCase.wantErr) {
				test.Fatalf(errorMismatchMessage, testCase.wantErr, err)
			}
		})
	}
}
