package useraccount

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testID = "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"

type testStore struct {
	Store
	provisionInput ProvisionInput
	provisioned    Account
	created        bool
	provisionErr   error
	gotIdentity    ExternalIdentity
	got            Account
	getErr         error
}

func (store *testStore) ProvisionUserAccount(_ context.Context, input ProvisionInput) (Account, bool, error) {
	store.provisionInput = input
	return store.provisioned, store.created, store.provisionErr
}

func (store *testStore) GetUserAccount(_ context.Context, identity ExternalIdentity) (Account, error) {
	store.gotIdentity = identity
	return store.got, store.getErr
}

func TestUserAccountContract(test *testing.T) {
	identity, err := NewExternalIdentity(" tauth ", " mprlab ", " user-1 ")
	if err != nil {
		test.Fatalf("identity: %v", err)
	}
	if identity.Issuer() != "tauth" || identity.TenantID() != "mprlab" || identity.UserID() != "user-1" {
		test.Fatalf("identity was not normalized")
	}
	for _, fields := range [][3]string{{"", "mprlab", "user"}, {"tauth", "", "user"}, {"tauth", "mprlab", ""}} {
		if _, err := NewExternalIdentity(fields[0], fields[1], fields[2]); !errors.Is(err, ErrInvalidIdentity) {
			test.Fatalf("expected invalid identity for %q", fields)
		}
	}

	id, err := NewID(" " + testID + " ")
	if err != nil || id.String() != testID {
		test.Fatalf("id: %v %q", err, id.String())
	}
	if _, err := NewID("invalid"); !errors.Is(err, ErrInvalidID) {
		test.Fatalf("expected invalid id, got %v", err)
	}

	createdAt := time.Date(2026, 9, 1, 1, 2, 3, 0, time.FixedZone("offset", 3600))
	account, err := NewAccount(id, identity, createdAt)
	if err != nil {
		test.Fatalf("account: %v", err)
	}
	if account.ID() != id || account.Identity() != identity || !account.CreatedAt().Equal(createdAt.UTC()) || account.CreatedAt().Location() != time.UTC {
		test.Fatalf("account accessors returned unexpected values")
	}
	if _, err := NewAccount(ID{}, identity, createdAt); !errors.Is(err, ErrInvalidID) {
		test.Fatalf("expected invalid account id, got %v", err)
	}
	if _, err := NewAccount(id, ExternalIdentity{}, createdAt); !errors.Is(err, ErrInvalidIdentity) {
		test.Fatalf("expected invalid account identity, got %v", err)
	}
	if _, err := NewAccount(id, identity, time.Time{}); err == nil {
		test.Fatalf("expected invalid account time")
	}
}

func TestUserAccountService(test *testing.T) {
	store := &testStore{}
	fixedTime := time.Date(2026, 9, 1, 2, 3, 4, 0, time.UTC)
	service, err := NewService(store, func() time.Time { return fixedTime }, func() string { return testID })
	if err != nil {
		test.Fatalf("service: %v", err)
	}
	identity, _ := NewExternalIdentity("tauth", "mprlab", "user")
	id, _ := NewID(testID)
	wantAccount, _ := NewAccount(id, identity, fixedTime)
	store.provisioned = wantAccount
	store.created = true

	got, created, err := service.Provision(context.Background(), identity, "request-1")
	if err != nil || !created || got != wantAccount {
		test.Fatalf("provision: account=%v created=%v err=%v", got, created, err)
	}
	if store.provisionInput.Account != wantAccount || store.provisionInput.RequestID != "request-1" {
		test.Fatalf("unexpected provision input")
	}
	if _, _, err := service.Provision(context.Background(), identity, " "); err == nil {
		test.Fatalf("expected invalid request id")
	}

	badIDService, _ := NewService(store, func() time.Time { return fixedTime }, func() string { return "bad" })
	if _, _, err := badIDService.Provision(context.Background(), identity, "request"); !errors.Is(err, ErrInvalidID) {
		test.Fatalf("expected invalid generated id, got %v", err)
	}
	badTimeService, _ := NewService(store, func() time.Time { return time.Time{} }, func() string { return testID })
	if _, _, err := badTimeService.Provision(context.Background(), identity, "request"); err == nil {
		test.Fatalf("expected invalid generated time")
	}

	store.got = wantAccount
	got, err = service.Get(context.Background(), identity)
	if err != nil || got != wantAccount || store.gotIdentity != identity {
		test.Fatalf("get did not forward")
	}

	defaultService, err := NewDefaultService(store)
	if err != nil {
		test.Fatalf("default service: %v", err)
	}
	if _, _, err := defaultService.Provision(context.Background(), identity, "default-request"); err != nil {
		test.Fatalf("default service provision: %v", err)
	}
	for _, constructor := range []func() (*Service, error){
		func() (*Service, error) {
			return NewService(nil, func() time.Time { return fixedTime }, func() string { return testID })
		},
		func() (*Service, error) { return NewService(store, nil, func() string { return testID }) },
		func() (*Service, error) { return NewService(store, func() time.Time { return fixedTime }, nil) },
	} {
		if _, err := constructor(); err == nil {
			test.Fatalf("expected invalid service dependency")
		}
	}
}
