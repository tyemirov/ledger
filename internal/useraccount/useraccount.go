package useraccount

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidID       = errors.New("user_account_invalid_id")
	ErrInvalidIdentity = errors.New("user_account_invalid_identity")
	ErrNotFound        = errors.New("user_account_not_found")
)

type ID struct {
	value string
}

func NewID(raw string) (ID, error) {
	value := strings.TrimSpace(raw)
	if _, err := uuid.Parse(value); err != nil {
		return ID{}, fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	return ID{value: value}, nil
}

func (id ID) String() string {
	return id.value
}

type ExternalIdentity struct {
	issuer   string
	tenantID string
	userID   string
}

func NewExternalIdentity(issuer string, tenantID string, userID string) (ExternalIdentity, error) {
	normalizedIssuer := strings.TrimSpace(issuer)
	normalizedTenantID := strings.TrimSpace(tenantID)
	normalizedUserID := strings.TrimSpace(userID)
	if normalizedIssuer == "" || normalizedTenantID == "" || normalizedUserID == "" {
		return ExternalIdentity{}, ErrInvalidIdentity
	}
	return ExternalIdentity{
		issuer:   normalizedIssuer,
		tenantID: normalizedTenantID,
		userID:   normalizedUserID,
	}, nil
}

func (identity ExternalIdentity) Issuer() string {
	return identity.issuer
}

func (identity ExternalIdentity) TenantID() string {
	return identity.tenantID
}

func (identity ExternalIdentity) UserID() string {
	return identity.userID
}

type Account struct {
	id        ID
	identity  ExternalIdentity
	createdAt time.Time
}

func NewAccount(id ID, identity ExternalIdentity, createdAt time.Time) (Account, error) {
	if id.String() == "" {
		return Account{}, ErrInvalidID
	}
	if identity.Issuer() == "" || identity.TenantID() == "" || identity.UserID() == "" {
		return Account{}, ErrInvalidIdentity
	}
	if createdAt.IsZero() {
		return Account{}, errors.New("user_account_invalid_created_at")
	}
	return Account{id: id, identity: identity, createdAt: createdAt.UTC()}, nil
}

func (account Account) ID() ID {
	return account.id
}

func (account Account) Identity() ExternalIdentity {
	return account.identity
}

func (account Account) CreatedAt() time.Time {
	return account.createdAt
}

type Store interface {
	ProvisionUserAccount(context.Context, ProvisionInput) (Account, bool, error)
	GetUserAccount(context.Context, ExternalIdentity) (Account, error)
}

type ProvisionInput struct {
	Account   Account
	RequestID string
}

type UUIDGenerator func() string
type Clock func() time.Time

type Service struct {
	store   Store
	now     Clock
	newUUID UUIDGenerator
}

func NewService(store Store, now Clock, newUUID UUIDGenerator) (*Service, error) {
	if store == nil || now == nil || newUUID == nil {
		return nil, errors.New("user_account_service_invalid_dependency")
	}
	return &Service{store: store, now: now, newUUID: newUUID}, nil
}

func NewDefaultService(store Store) (*Service, error) {
	return NewService(store, func() time.Time { return time.Now().UTC() }, uuid.NewString)
}

func (service *Service) Provision(ctx context.Context, identity ExternalIdentity, requestID string) (Account, bool, error) {
	if strings.TrimSpace(requestID) == "" {
		return Account{}, false, errors.New("user_account_invalid_request_id")
	}
	accountID, err := NewID(service.newUUID())
	if err != nil {
		return Account{}, false, err
	}
	account, err := NewAccount(accountID, identity, service.now())
	if err != nil {
		return Account{}, false, err
	}
	return service.store.ProvisionUserAccount(ctx, ProvisionInput{Account: account, RequestID: requestID})
}

func (service *Service) Get(ctx context.Context, identity ExternalIdentity) (Account, error) {
	return service.store.GetUserAccount(ctx, identity)
}
