package tenant

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
)

const (
	tenantIDValue     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf801"
	credentialIDValue = "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
	ownerIDValue      = "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"
)

type storeStub struct {
	createInput        CreateInput
	createdTenant      Tenant
	createCreated      bool
	createErr          error
	listOwner          useraccount.ID
	listCursor         *Cursor
	listLimit          int
	listed             []Tenant
	listErr            error
	getOwner           useraccount.ID
	getTenantID        ID
	gotTenant          Tenant
	getErr             error
	credentialOwner    useraccount.ID
	credentialInput    CreateCredentialInput
	createdCredential  Credential
	credentialCreated  bool
	credentialErr      error
	listedCredentials  []Credential
	listCredentialsErr error
	revokedCredential  Credential
	revokeErr          error
	revokedAt          time.Time
	revokeRequestID    string
	storedCredential   StoredCredential
	findErr            error
}

func (store *storeStub) Create(_ context.Context, input CreateInput) (Tenant, bool, error) {
	store.createInput = input
	return store.createdTenant, store.createCreated, store.createErr
}

func (store *storeStub) List(_ context.Context, owner useraccount.ID, cursor *Cursor, limit int) ([]Tenant, error) {
	store.listOwner, store.listCursor, store.listLimit = owner, cursor, limit
	return store.listed, store.listErr
}

func (store *storeStub) GetTenant(_ context.Context, owner useraccount.ID, tenantID ID) (Tenant, error) {
	store.getOwner, store.getTenantID = owner, tenantID
	return store.gotTenant, store.getErr
}

func (store *storeStub) CreateCredential(_ context.Context, owner useraccount.ID, input CreateCredentialInput) (Credential, bool, error) {
	store.credentialOwner, store.credentialInput = owner, input
	return store.createdCredential, store.credentialCreated, store.credentialErr
}

func (store *storeStub) ListCredentials(_ context.Context, owner useraccount.ID, tenantID ID) ([]Credential, error) {
	store.getOwner, store.getTenantID = owner, tenantID
	return store.listedCredentials, store.listCredentialsErr
}

func (store *storeStub) RevokeCredential(_ context.Context, owner useraccount.ID, tenantID ID, _ CredentialID, revokedAt time.Time, requestID string) (Credential, error) {
	store.getOwner, store.getTenantID, store.revokedAt, store.revokeRequestID = owner, tenantID, revokedAt, requestID
	return store.revokedCredential, store.revokeErr
}

func (store *storeStub) FindCredential(_ context.Context, _ CredentialID) (StoredCredential, error) {
	return store.storedCredential, store.findErr
}

func TestTenantValueContracts(test *testing.T) {
	tenantID, err := NewID(" " + tenantIDValue + " ")
	if err != nil || tenantID.String() != tenantIDValue {
		test.Fatalf("tenant id: %v", err)
	}
	if _, err := NewID("bad"); !errors.Is(err, ErrInvalidID) {
		test.Fatalf("expected invalid tenant id")
	}
	credentialID, err := NewCredentialID(" " + credentialIDValue + " ")
	if err != nil || credentialID.String() != credentialIDValue {
		test.Fatalf("credential id: %v", err)
	}
	if _, err := NewCredentialID("bad"); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid credential id")
	}

	name, err := NewName(" Test tenant ")
	if err != nil || name.String() != "Test tenant" {
		test.Fatalf("name: %v", err)
	}
	for _, raw := range []string{" ", strings.Repeat("x", maximumNameLength+1), string([]byte{0xff})} {
		if _, err := NewName(raw); !errors.Is(err, ErrInvalidName) {
			test.Fatalf("expected invalid name for %q", raw)
		}
	}

	key, err := NewIdempotencyKey(" retry-1 ")
	if err != nil || len(key.Digest()) != sha256.Size || bytes.Equal(key.Digest(), make([]byte, sha256.Size)) {
		test.Fatalf("idempotency key: %v", err)
	}
	for _, raw := range []string{" ", strings.Repeat("x", 256)} {
		if _, err := NewIdempotencyKey(raw); err == nil {
			test.Fatalf("expected invalid idempotency key")
		}
	}
	digest := NewRequestDigest("canonical")
	if len(digest.Bytes()) != sha256.Size {
		test.Fatalf("request digest length")
	}

	ownerID, _ := useraccount.NewID(ownerIDValue)
	createdAt := time.Date(2026, 9, 1, 1, 2, 3, 0, time.FixedZone("offset", 3600))
	item, err := NewTenant(tenantID, ownerID, name, createdAt)
	if err != nil {
		test.Fatalf("tenant: %v", err)
	}
	if item.ID() != tenantID || item.OwnerID() != ownerID || item.Name() != name || !item.CreatedAt().Equal(createdAt.UTC()) || item.CreatedAt().Location() != time.UTC {
		test.Fatalf("tenant accessors")
	}
	for _, input := range []struct {
		id      ID
		owner   useraccount.ID
		name    Name
		created time.Time
	}{{owner: ownerID, name: name, created: createdAt}, {id: tenantID, name: name, created: createdAt}, {id: tenantID, owner: ownerID, created: createdAt}, {id: tenantID, owner: ownerID, name: name}} {
		if _, err := NewTenant(input.id, input.owner, input.name, input.created); err == nil {
			test.Fatalf("expected invalid tenant")
		}
	}

	revokedAt := createdAt.Add(time.Hour)
	credential, err := NewCredential(credentialID, tenantID, createdAt, &revokedAt)
	if err != nil {
		test.Fatalf("credential: %v", err)
	}
	gotRevokedAt, revoked := credential.RevokedAt()
	if credential.ID() != credentialID || credential.TenantID() != tenantID || !credential.CreatedAt().Equal(createdAt.UTC()) || !revoked || !gotRevokedAt.Equal(revokedAt.UTC()) {
		test.Fatalf("credential accessors")
	}
	active, _ := NewCredential(credentialID, tenantID, createdAt, nil)
	if _, revoked := active.RevokedAt(); revoked {
		test.Fatalf("active credential reported revoked")
	}
	if _, err := NewCredential(CredentialID{}, tenantID, createdAt, nil); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid credential id")
	}
	if _, err := NewCredential(credentialID, ID{}, createdAt, nil); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid credential tenant")
	}
	if _, err := NewCredential(credentialID, tenantID, time.Time{}, nil); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid credential time")
	}

	ctx := WithAuthenticatedID(context.Background(), tenantID)
	if got, ok := AuthenticatedID(ctx); !ok || got != tenantID {
		test.Fatalf("authenticated tenant context")
	}
	if _, ok := AuthenticatedID(context.Background()); ok {
		test.Fatalf("unexpected authenticated tenant")
	}
}

func TestTenantService(test *testing.T) {
	store := &storeStub{}
	fixedTime := time.Date(2026, 9, 1, 2, 3, 4, 0, time.UTC)
	service, err := NewService(store, func() time.Time { return fixedTime }, func() string { return tenantIDValue }, bytes.NewReader(bytes.Repeat([]byte{7}, credentialSecretSize)))
	if err != nil {
		test.Fatalf("service: %v", err)
	}
	ownerID, _ := useraccount.NewID(ownerIDValue)
	name, _ := NewName("Tenant")
	key, _ := NewIdempotencyKey("key")
	tenantID, _ := NewID(tenantIDValue)
	wantTenant, _ := NewTenant(tenantID, ownerID, name, fixedTime)
	store.createdTenant, store.createCreated = wantTenant, true

	gotTenant, created, err := service.Create(context.Background(), ownerID, name, key, "request")
	if err != nil || !created || gotTenant != wantTenant || store.createInput.Tenant != wantTenant || store.createInput.RequestID != "request" {
		test.Fatalf("create tenant failed: %v", err)
	}
	badUUIDService, _ := NewService(store, func() time.Time { return fixedTime }, func() string { return "bad" }, bytes.NewReader(nil))
	if _, _, err := badUUIDService.Create(context.Background(), ownerID, name, key, "request"); !errors.Is(err, ErrInvalidID) {
		test.Fatalf("expected invalid generated tenant id, got %v", err)
	}
	badTimeService, _ := NewService(store, func() time.Time { return time.Time{} }, func() string { return tenantIDValue }, bytes.NewReader(nil))
	if _, _, err := badTimeService.Create(context.Background(), ownerID, name, key, "request"); err == nil {
		test.Fatalf("expected invalid generated tenant time")
	}

	cursor := &Cursor{CreatedAt: fixedTime, TenantID: tenantID}
	store.listed = []Tenant{wantTenant}
	if listed, err := service.List(context.Background(), ownerID, cursor, 9); err != nil || len(listed) != 1 || store.listCursor != cursor || store.listLimit != 9 || store.listOwner != ownerID {
		test.Fatalf("list did not forward")
	}
	store.gotTenant = wantTenant
	if got, err := service.Get(context.Background(), ownerID, tenantID); err != nil || got != wantTenant || store.getTenantID != tenantID {
		test.Fatalf("get did not forward")
	}

	credentialID, _ := NewCredentialID(credentialIDValue)
	wantCredential, _ := NewCredential(credentialID, tenantID, fixedTime, nil)
	credentialService, _ := NewService(store, func() time.Time { return fixedTime }, func() string { return credentialIDValue }, bytes.NewReader(bytes.Repeat([]byte{7}, credentialSecretSize*3)))
	store.createdCredential, store.credentialCreated = wantCredential, true
	gotCredential, secret, created, err := credentialService.CreateCredential(context.Background(), ownerID, tenantID, key, "request")
	if err != nil || !created || gotCredential != wantCredential || !strings.HasPrefix(secret, "ledger_"+credentialIDValue+"_") {
		test.Fatalf("create credential: created=%v secret=%q err=%v", created, secret, err)
	}
	if store.credentialOwner != ownerID || store.credentialInput.Credential != wantCredential || len(store.credentialInput.SecretDigest) != sha256.Size {
		test.Fatalf("unexpected credential input")
	}
	store.credentialCreated = false
	if _, replaySecret, replayCreated, err := credentialService.CreateCredential(context.Background(), ownerID, tenantID, key, "request"); err != nil || replayCreated || replaySecret != "" {
		test.Fatalf("credential retry exposed secret")
	}
	if _, _, _, err := badUUIDService.CreateCredential(context.Background(), ownerID, tenantID, key, "request"); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid generated credential id")
	}
	readErrorService, _ := NewService(store, func() time.Time { return fixedTime }, func() string { return credentialIDValue }, errorReader{})
	if _, _, _, err := readErrorService.CreateCredential(context.Background(), ownerID, tenantID, key, "request"); err == nil {
		test.Fatalf("expected random source error")
	}
	zeroTimeCredentialService, _ := NewService(store, func() time.Time { return time.Time{} }, func() string { return credentialIDValue }, bytes.NewReader(bytes.Repeat([]byte{7}, credentialSecretSize)))
	if _, _, _, err := zeroTimeCredentialService.CreateCredential(context.Background(), ownerID, tenantID, key, "request"); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("expected invalid credential time")
	}
	store.credentialErr = errors.New("store failed")
	if _, _, _, err := credentialService.CreateCredential(context.Background(), ownerID, tenantID, key, "request"); err == nil {
		test.Fatalf("expected credential store error")
	}
	store.credentialErr = nil

	store.listedCredentials = []Credential{wantCredential}
	if listed, err := service.ListCredentials(context.Background(), ownerID, tenantID); err != nil || len(listed) != 1 {
		test.Fatalf("list credentials did not forward")
	}
	store.revokedCredential = wantCredential
	if _, err := service.RevokeCredential(context.Background(), ownerID, tenantID, credentialID, "request-2"); err != nil || !store.revokedAt.Equal(fixedTime) || store.revokeRequestID != "request-2" {
		test.Fatalf("revoke did not forward")
	}

	defaultService, err := NewDefaultService(store)
	if err != nil {
		test.Fatalf("default service: %v", err)
	}
	if _, _, err := defaultService.Create(context.Background(), ownerID, name, key, "default-request"); err != nil {
		test.Fatalf("default service create: %v", err)
	}
	for _, constructor := range []func() (*Service, error){
		func() (*Service, error) {
			return NewService(nil, func() time.Time { return fixedTime }, func() string { return tenantIDValue }, bytes.NewReader(nil))
		},
		func() (*Service, error) {
			return NewService(store, nil, func() string { return tenantIDValue }, bytes.NewReader(nil))
		},
		func() (*Service, error) {
			return NewService(store, func() time.Time { return fixedTime }, nil, bytes.NewReader(nil))
		},
		func() (*Service, error) {
			return NewService(store, func() time.Time { return fixedTime }, func() string { return tenantIDValue }, nil)
		},
	} {
		if _, err := constructor(); err == nil {
			test.Fatalf("expected invalid service dependency")
		}
	}
}

func TestTenantCredentialAuthentication(test *testing.T) {
	store := &storeStub{}
	service, _ := NewService(store, time.Now, func() string { return credentialIDValue }, bytes.NewReader(nil))
	tenantID, _ := NewID(tenantIDValue)
	credentialID, _ := NewCredentialID(credentialIDValue)
	createdAt := time.Now().UTC()
	credential, _ := NewCredential(credentialID, tenantID, createdAt, nil)
	secretPart := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, credentialSecretSize))
	secret := "ledger_" + credentialIDValue + "_" + secretPart
	digest := sha256.Sum256([]byte(secretPart))
	store.storedCredential = StoredCredential{Credential: credential, Digest: digest[:]}
	if authenticatedID, err := service.Authenticate(context.Background(), secret); err != nil || authenticatedID != tenantID {
		test.Fatalf("authenticate: %v", err)
	}

	for _, raw := range []string{"", "bad", "wrong_" + credentialIDValue + "_" + secretPart, "ledger_bad_" + secretPart, "ledger_" + credentialIDValue + "_bad!", "ledger_" + credentialIDValue + "_YQ"} {
		if _, err := service.Authenticate(context.Background(), raw); !errors.Is(err, ErrCredentialInvalid) {
			test.Fatalf("expected invalid credential for %q, got %v", raw, err)
		}
	}
	store.findErr = errors.New("database")
	if _, err := service.Authenticate(context.Background(), secret); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("store error must be opaque")
	}
	store.findErr = nil
	revokedAt := createdAt.Add(time.Minute)
	revoked, _ := NewCredential(credentialID, tenantID, createdAt, &revokedAt)
	store.storedCredential.Credential = revoked
	if _, err := service.Authenticate(context.Background(), secret); !errors.Is(err, ErrCredentialRevoked) {
		test.Fatalf("revoked credential must fail")
	}
	store.storedCredential.Credential = credential
	store.storedCredential.Digest = []byte{1}
	if _, err := service.Authenticate(context.Background(), secret); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("short digest must fail")
	}
	store.storedCredential.Digest = bytes.Repeat([]byte{1}, sha256.Size)
	if _, err := service.Authenticate(context.Background(), secret); !errors.Is(err, ErrCredentialInvalid) {
		test.Fatalf("wrong digest must fail")
	}
}

type errorReader struct{}

func (errorReader) Read(_ []byte) (int, error) { return 0, io.ErrUnexpectedEOF }
