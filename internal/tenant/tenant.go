package tenant

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/google/uuid"
)

const (
	credentialPrefix     = "ledger"
	credentialSecretSize = 32
	maximumNameLength    = 80
)

var (
	ErrCredentialInvalid   = errors.New("tenant_credential_invalid")
	ErrCredentialRevoked   = errors.New("tenant_credential_revoked")
	ErrIdempotencyConflict = errors.New("idempotency_conflict")
	ErrInvalidCursor       = errors.New("tenant_invalid_cursor")
	ErrInvalidID           = errors.New("tenant_invalid_id")
	ErrInvalidName         = errors.New("tenant_invalid_name")
	ErrNotFound            = errors.New("tenant_not_found")
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

type CredentialID struct {
	value string
}

func NewCredentialID(raw string) (CredentialID, error) {
	value := strings.TrimSpace(raw)
	if _, err := uuid.Parse(value); err != nil {
		return CredentialID{}, fmt.Errorf("%w: %v", ErrCredentialInvalid, err)
	}
	return CredentialID{value: value}, nil
}

func (id CredentialID) String() string {
	return id.value
}

type Name struct {
	value string
}

func NewName(raw string) (Name, error) {
	value := strings.TrimSpace(raw)
	if value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > maximumNameLength {
		return Name{}, ErrInvalidName
	}
	return Name{value: value}, nil
}

func (name Name) String() string {
	return name.value
}

type IdempotencyKey struct {
	digest [sha256.Size]byte
}

func NewIdempotencyKey(raw string) (IdempotencyKey, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > 255 {
		return IdempotencyKey{}, errors.New("idempotency_key_invalid")
	}
	return IdempotencyKey{digest: sha256.Sum256([]byte(value))}, nil
}

func (key IdempotencyKey) Digest() []byte {
	result := make([]byte, len(key.digest))
	copy(result, key.digest[:])
	return result
}

type RequestDigest struct {
	value [sha256.Size]byte
}

func NewRequestDigest(canonical string) RequestDigest {
	return RequestDigest{value: sha256.Sum256([]byte(canonical))}
}

func (digest RequestDigest) Bytes() []byte {
	result := make([]byte, len(digest.value))
	copy(result, digest.value[:])
	return result
}

type Tenant struct {
	id        ID
	ownerID   useraccount.ID
	name      Name
	createdAt time.Time
}

func NewTenant(id ID, ownerID useraccount.ID, name Name, createdAt time.Time) (Tenant, error) {
	if id.String() == "" || ownerID.String() == "" || name.String() == "" || createdAt.IsZero() {
		return Tenant{}, errors.New("tenant_invalid")
	}
	return Tenant{id: id, ownerID: ownerID, name: name, createdAt: createdAt.UTC()}, nil
}

func (item Tenant) ID() ID {
	return item.id
}

func (item Tenant) OwnerID() useraccount.ID {
	return item.ownerID
}

func (item Tenant) Name() Name {
	return item.name
}

func (item Tenant) CreatedAt() time.Time {
	return item.createdAt
}

type Cursor struct {
	CreatedAt time.Time
	TenantID  ID
}

type Credential struct {
	id        CredentialID
	tenantID  ID
	createdAt time.Time
	revokedAt *time.Time
}

func NewCredential(id CredentialID, tenantID ID, createdAt time.Time, revokedAt *time.Time) (Credential, error) {
	if id.String() == "" || tenantID.String() == "" || createdAt.IsZero() {
		return Credential{}, ErrCredentialInvalid
	}
	var normalizedRevokedAt *time.Time
	if revokedAt != nil {
		value := revokedAt.UTC()
		normalizedRevokedAt = &value
	}
	return Credential{id: id, tenantID: tenantID, createdAt: createdAt.UTC(), revokedAt: normalizedRevokedAt}, nil
}

func (credential Credential) ID() CredentialID {
	return credential.id
}

func (credential Credential) TenantID() ID {
	return credential.tenantID
}

func (credential Credential) CreatedAt() time.Time {
	return credential.createdAt
}

func (credential Credential) RevokedAt() (time.Time, bool) {
	if credential.revokedAt == nil {
		return time.Time{}, false
	}
	return *credential.revokedAt, true
}

type CreateInput struct {
	Tenant      Tenant
	Idempotency IdempotencyKey
	Request     RequestDigest
	RequestID   string
}

type CreateCredentialInput struct {
	Credential   Credential
	SecretDigest []byte
	Idempotency  IdempotencyKey
	Request      RequestDigest
	RequestID    string
}

type StoredCredential struct {
	Credential Credential
	Digest     []byte
}

type authenticatedTenantKey struct{}

func WithAuthenticatedID(ctx context.Context, tenantID ID) context.Context {
	return context.WithValue(ctx, authenticatedTenantKey{}, tenantID)
}

func AuthenticatedID(ctx context.Context) (ID, bool) {
	tenantID, ok := ctx.Value(authenticatedTenantKey{}).(ID)
	return tenantID, ok
}

type Store interface {
	Create(context.Context, CreateInput) (Tenant, bool, error)
	List(context.Context, useraccount.ID, *Cursor, int) ([]Tenant, error)
	GetTenant(context.Context, useraccount.ID, ID) (Tenant, error)
	CreateCredential(context.Context, useraccount.ID, CreateCredentialInput) (Credential, bool, error)
	ListCredentials(context.Context, useraccount.ID, ID) ([]Credential, error)
	RevokeCredential(context.Context, useraccount.ID, ID, CredentialID, time.Time, string) (Credential, error)
	FindCredential(context.Context, CredentialID) (StoredCredential, error)
}

type UUIDGenerator func() string
type Clock func() time.Time

type Service struct {
	store   Store
	now     Clock
	newUUID UUIDGenerator
	random  io.Reader
}

func NewService(store Store, now Clock, newUUID UUIDGenerator, random io.Reader) (*Service, error) {
	if store == nil || now == nil || newUUID == nil || random == nil {
		return nil, errors.New("tenant_service_invalid_dependency")
	}
	return &Service{store: store, now: now, newUUID: newUUID, random: random}, nil
}

func NewDefaultService(store Store) (*Service, error) {
	return NewService(store, func() time.Time { return time.Now().UTC() }, uuid.NewString, rand.Reader)
}

func (service *Service) Create(ctx context.Context, ownerID useraccount.ID, name Name, key IdempotencyKey, requestID string) (Tenant, bool, error) {
	tenantID, err := NewID(service.newUUID())
	if err != nil {
		return Tenant{}, false, err
	}
	item, err := NewTenant(tenantID, ownerID, name, service.now())
	if err != nil {
		return Tenant{}, false, err
	}
	return service.store.Create(ctx, CreateInput{
		Tenant:      item,
		Idempotency: key,
		Request:     NewRequestDigest(name.String()),
		RequestID:   requestID,
	})
}

func (service *Service) List(ctx context.Context, ownerID useraccount.ID, cursor *Cursor, limit int) ([]Tenant, error) {
	return service.store.List(ctx, ownerID, cursor, limit)
}

func (service *Service) Get(ctx context.Context, ownerID useraccount.ID, tenantID ID) (Tenant, error) {
	return service.store.GetTenant(ctx, ownerID, tenantID)
}

func (service *Service) CreateCredential(ctx context.Context, ownerID useraccount.ID, tenantID ID, key IdempotencyKey, requestID string) (Credential, string, bool, error) {
	credentialID, err := NewCredentialID(service.newUUID())
	if err != nil {
		return Credential{}, "", false, err
	}
	secretBytes := make([]byte, credentialSecretSize)
	if _, err := io.ReadFull(service.random, secretBytes); err != nil {
		return Credential{}, "", false, fmt.Errorf("tenant_credential_generate: %w", err)
	}
	secretPart := base64.RawURLEncoding.EncodeToString(secretBytes)
	secret := credentialPrefix + "_" + credentialID.String() + "_" + secretPart
	digest := sha256.Sum256([]byte(secretPart))
	credential, err := NewCredential(credentialID, tenantID, service.now(), nil)
	if err != nil {
		return Credential{}, "", false, err
	}
	persisted, created, err := service.store.CreateCredential(ctx, ownerID, CreateCredentialInput{
		Credential:   credential,
		SecretDigest: digest[:],
		Idempotency:  key,
		Request:      NewRequestDigest(tenantID.String()),
		RequestID:    requestID,
	})
	if err != nil {
		return Credential{}, "", false, err
	}
	if !created {
		return persisted, "", false, nil
	}
	return persisted, secret, true, nil
}

func (service *Service) ListCredentials(ctx context.Context, ownerID useraccount.ID, tenantID ID) ([]Credential, error) {
	return service.store.ListCredentials(ctx, ownerID, tenantID)
}

func (service *Service) RevokeCredential(ctx context.Context, ownerID useraccount.ID, tenantID ID, credentialID CredentialID, requestID string) (Credential, error) {
	return service.store.RevokeCredential(ctx, ownerID, tenantID, credentialID, service.now(), requestID)
}

func (service *Service) Authenticate(ctx context.Context, rawSecret string) (ID, error) {
	credentialID, digest, err := ParseCredentialSecret(rawSecret)
	if err != nil {
		return ID{}, err
	}
	stored, err := service.store.FindCredential(ctx, credentialID)
	if err != nil {
		if errors.Is(err, ErrCredentialInvalid) {
			return ID{}, ErrCredentialInvalid
		}
		return ID{}, fmt.Errorf("tenant credential authenticate: %w", err)
	}
	if _, revoked := stored.Credential.RevokedAt(); revoked {
		return ID{}, ErrCredentialRevoked
	}
	if len(stored.Digest) != sha256.Size || subtle.ConstantTimeCompare(stored.Digest, digest) != 1 {
		return ID{}, ErrCredentialInvalid
	}
	return stored.Credential.TenantID(), nil
}

func ParseCredentialSecret(raw string) (CredentialID, []byte, error) {
	parts := strings.SplitN(raw, "_", 3)
	if len(parts) != 3 || parts[0] != credentialPrefix || parts[2] == "" {
		return CredentialID{}, nil, ErrCredentialInvalid
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(decoded) != credentialSecretSize {
		return CredentialID{}, nil, ErrCredentialInvalid
	}
	credentialID, err := NewCredentialID(parts[1])
	if err != nil {
		return CredentialID{}, nil, ErrCredentialInvalid
	}
	digest := sha256.Sum256([]byte(parts[2]))
	return credentialID, digest[:], nil
}
