package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

const (
	testOrigin       = "https://ledger.example.test"
	testSigningKey   = "test-signing-key"
	testIssuer       = "tauth"
	testAuthTenantID = "mprlab"
	testCookieName   = "app_session"
	accountOneID     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf810"
	accountTwoID     = "0196f0ec-3e80-7a54-bd2b-56cfe90bf820"
	tenantOneID      = "0196f0ec-3e80-7a54-bd2b-56cfe90bf801"
	tenantTwoID      = "0196f0ec-3e80-7a54-bd2b-56cfe90bf802"
	credentialOneID  = "0196f0ec-3e80-7a54-bd2b-56cfe90bf811"
)

func testBrowserConfiguration() BrowserConfig {
	return BrowserConfig{
		Description:    "Ledger",
		TAuthURL:       "https://auth.example.test",
		GoogleClientID: "google-client-id",
		LoginPath:      "/auth/google",
		LogoutPath:     "/auth/logout",
		NoncePath:      "/auth/nonce",
		SessionPath:    "/auth/session",
	}
}

type controlHarness struct {
	test        *testing.T
	database    *gorm.DB
	handler     *Handler
	server      *httptest.Server
	service     *tenant.Service
	cookieOne   *http.Cookie
	cookieTwo   *http.Cookie
	wrongCookie *http.Cookie
	logger      *requestLoggerStub
}

type requestLoggerStub struct {
	logs []RequestLog
}

func (logger *requestLoggerStub) LogControlRequest(entry RequestLog) {
	logger.logs = append(logger.logs, entry)
}

func newControlHarness(test *testing.T) *controlHarness {
	test.Helper()
	database, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(test.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		test.Fatalf("open database: %v", err)
	}
	sqlDatabase, err := database.DB()
	if err != nil {
		test.Fatalf("sql database: %v", err)
	}
	test.Cleanup(func() { _ = sqlDatabase.Close() })
	if err := database.Exec("PRAGMA foreign_keys=ON;").Error; err != nil {
		test.Fatalf("foreign keys: %v", err)
	}
	if err := database.AutoMigrate(
		&gormstore.UserAccount{},
		&gormstore.LedgerTenant{},
		&gormstore.TenantCredential{},
		&gormstore.IdempotencyRecord{},
		&gormstore.ControlEvent{},
		&gormstore.LedgerAccount{},
		&gormstore.LedgerEntry{},
		&gormstore.Reservation{},
	); err != nil {
		test.Fatalf("migrate: %v", err)
	}

	store := gormstore.New(database)
	baseTime := time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC)
	accountIDs := []string{accountOneID, "0196f0ec-3e80-7a54-bd2b-56cfe90bf812", accountTwoID}
	accountService, err := useraccount.NewService(store, func() time.Time { return baseTime }, sequenceUUID(accountIDs))
	if err != nil {
		test.Fatalf("account service: %v", err)
	}
	tenantIDs := []string{
		tenantOneID,
		"0196f0ec-3e80-7a54-bd2b-56cfe90bf803",
		"0196f0ec-3e80-7a54-bd2b-56cfe90bf804",
		tenantTwoID,
		credentialOneID,
		"0196f0ec-3e80-7a54-bd2b-56cfe90bf813",
		"0196f0ec-3e80-7a54-bd2b-56cfe90bf814",
		"0196f0ec-3e80-7a54-bd2b-56cfe90bf815",
	}
	clockValue := baseTime
	tenantService, err := tenant.NewService(store, func() time.Time {
		value := clockValue
		clockValue = clockValue.Add(time.Second)
		return value
	}, sequenceUUID(tenantIDs), bytes.NewReader(bytes.Repeat([]byte{7}, 256)))
	if err != nil {
		test.Fatalf("tenant service: %v", err)
	}
	validator, err := sessionvalidator.New(sessionvalidator.Config{SigningKey: []byte(testSigningKey), Issuer: testIssuer, CookieName: testCookieName})
	if err != nil {
		test.Fatalf("session validator: %v", err)
	}
	requestLogger := &requestLoggerStub{}
	handler, err := NewHandler(accountService, tenantService, validator, testAuthTenantID, testOrigin+"/", testBrowserConfiguration(), requestLogger)
	if err != nil {
		test.Fatalf("handler: %v", err)
	}
	server := httptest.NewServer(handler)
	test.Cleanup(server.Close)
	return &controlHarness{
		test:        test,
		database:    database,
		handler:     handler,
		server:      server,
		service:     tenantService,
		cookieOne:   signedCookie(test, "user-one", testAuthTenantID, true),
		cookieTwo:   signedCookie(test, "user-two", testAuthTenantID, true),
		wrongCookie: signedCookie(test, "user-one", "wrong-tenant", true),
		logger:      requestLogger,
	}
}

func sequenceUUID(values []string) func() string {
	index := 0
	return func() string {
		value := values[index]
		index++
		return value
	}
}

func signedCookie(test *testing.T, userID string, tenantID string, withExpiry bool) *http.Cookie {
	test.Helper()
	claims := &sessionvalidator.Claims{
		TenantID: tenantID,
		UserID:   userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   testIssuer,
			IssuedAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	if withExpiry {
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSigningKey))
	if err != nil {
		test.Fatalf("sign cookie: %v", err)
	}
	return &http.Cookie{Name: testCookieName, Value: token}
}

func (harness *controlHarness) request(method string, path string, body string, cookie *http.Cookie, mutation bool, headers map[string]string) (*http.Response, map[string]any) {
	harness.test.Helper()
	request, err := http.NewRequest(method, harness.server.URL+path, strings.NewReader(body))
	if err != nil {
		harness.test.Fatalf("new request: %v", err)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	if mutation {
		request.Header.Set("Origin", testOrigin)
		request.Header.Set(csrfHeader, "1")
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		harness.test.Fatalf("request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	payload := map[string]any{}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		harness.test.Fatalf("decode response: %v", err)
	}
	wantCacheControl := "private, no-store"
	if path == "/healthz" {
		wantCacheControl = "no-store"
	}
	if response.Header.Get("Cache-Control") != wantCacheControl || response.Header.Get("X-Content-Type-Options") != "nosniff" {
		harness.test.Fatalf("security response headers are missing")
	}
	return response, payload
}

func TestHealthReportsUnavailableDatabase(test *testing.T) {
	harness := newControlHarness(test)
	response, _ := harness.request(http.MethodGet, "/healthz", "", nil, false, nil)
	assertStatus(test, response, http.StatusOK)
	var count int64
	if err := harness.database.Model(&gormstore.ControlEvent{}).Count(&count).Error; err != nil || count != 0 {
		test.Fatalf("health created an audit event: count=%d error=%v", count, err)
	}
	database, err := harness.database.DB()
	if err != nil {
		test.Fatal(err)
	}
	if err := database.Close(); err != nil {
		test.Fatal(err)
	}
	response, payload := harness.request(http.MethodGet, "/healthz", "", nil, false, nil)
	assertStatus(test, response, http.StatusServiceUnavailable)
	if len(payload) != 1 || payload["status"] != "unavailable" {
		test.Fatalf("health exposed internal state: %v", payload)
	}
	if len(harness.logger.logs) != 1 || harness.logger.logs[0].StatusCode != http.StatusServiceUnavailable || harness.logger.logs[0].Error == nil {
		test.Fatalf("missing failure diagnostics: %+v", harness.logger.logs)
	}
}

func TestControlPlaneLifecycle(test *testing.T) {
	harness := newControlHarness(test)

	response, payload := harness.request(http.MethodGet, "/healthz", "", nil, false, nil)
	assertStatus(test, response, http.StatusOK)
	if payload["status"] != "ok" {
		test.Fatalf("health payload: %v", payload)
	}
	if len(harness.logger.logs) != 0 {
		test.Fatalf("health request log: %+v", harness.logger.logs)
	}
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", nil, false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", harness.wrongCookie, false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", signedCookie(test, "user", testAuthTenantID, false), false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", signedCookie(test, "", testAuthTenantID, true), false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/tenants", "", nil, false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID, "", nil, false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID+"/credentials", "", nil, false, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusNotFound)

	response, _ = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusForbidden)
	response, _ = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, false, map[string]string{"Origin": testOrigin, csrfHeader: "0"})
	assertStatus(test, response, http.StatusForbidden)
	response, payload = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, true, map[string]string{requestIDHeader: "request-provision"})
	assertStatus(test, response, http.StatusCreated)
	if response.Header.Get(requestIDHeader) != "request-provision" || nestedString(payload, "user_account", "id") != accountOneID {
		test.Fatalf("provision response: %v", payload)
	}
	response, _ = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusOK)
	response, payload = harness.request(http.MethodGet, "/api/user-account", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusOK)
	if nestedString(payload, "user_account", "id") != accountOneID {
		test.Fatalf("get user account: %v", payload)
	}

	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"One"}`, harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusBadRequest)
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"One"}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "tenant-one", "Content-Type": "text/plain"})
	assertStatus(test, response, http.StatusUnsupportedMediaType)
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":""}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "bad-name"})
	assertStatus(test, response, http.StatusBadRequest)
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"One","unknown":true}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "bad-json"})
	assertStatus(test, response, http.StatusBadRequest)
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"One"}{}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "trailing-json"})
	assertStatus(test, response, http.StatusBadRequest)

	response, payload = harness.request(http.MethodPost, "/api/tenants", `{"name":"One"}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "tenant-one"})
	assertStatus(test, response, http.StatusCreated)
	if nestedString(payload, "tenant", "id") != tenantOneID {
		test.Fatalf("create tenant: %v", payload)
	}
	response, payload = harness.request(http.MethodPost, "/api/tenants", `{"name":"One"}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "tenant-one"})
	assertStatus(test, response, http.StatusOK)
	if nestedString(payload, "tenant", "id") != tenantOneID {
		test.Fatalf("tenant replay: %v", payload)
	}
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"Different"}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "tenant-one"})
	assertStatus(test, response, http.StatusConflict)
	response, _ = harness.request(http.MethodPost, "/api/tenants", `{"name":"Two"}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "tenant-two"})
	assertStatus(test, response, http.StatusCreated)

	response, payload = harness.request(http.MethodGet, "/api/tenants?limit=1", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusOK)
	nextCursor, _ := payload["next_cursor"].(string)
	if nextCursor == "" || len(payload["tenants"].([]any)) != 1 {
		test.Fatalf("tenant page: %v", payload)
	}
	response, payload = harness.request(http.MethodGet, "/api/tenants?limit=1&cursor="+nextCursor, "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusOK)
	if len(payload["tenants"].([]any)) != 1 {
		test.Fatalf("second tenant page: %v", payload)
	}
	for _, path := range []string{"/api/tenants?cursor=bad", "/api/tenants?limit=0", "/api/tenants?limit=201", "/api/tenants?limit=bad"} {
		response, _ = harness.request(http.MethodGet, path, "", harness.cookieOne, false, nil)
		assertStatus(test, response, http.StatusBadRequest)
	}
	response, payload = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID, "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusOK)
	if nestedString(payload, "tenant", "name") != "One" {
		test.Fatalf("get tenant: %v", payload)
	}
	response, _ = harness.request(http.MethodGet, "/api/tenants/not-a-uuid", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusNotFound)

	response, payload = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "credential-one"})
	assertStatus(test, response, http.StatusCreated)
	secret := nestedString(payload, "credential", "secret")
	if nestedString(payload, "credential", "id") != credentialOneID || secret == "" {
		test.Fatalf("create credential: %v", payload)
	}
	addressedTenant, _ := tenant.NewID(tenantOneID)
	if authenticatedID, err := harness.service.Authenticate(context.Background(), secret); err != nil || authenticatedID != addressedTenant {
		test.Fatalf("created credential does not authenticate: %v", err)
	}
	response, _ = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, nil, true, map[string]string{idempotencyHeader: "credential-unauthenticated"})
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, harness.cookieOne, false, map[string]string{idempotencyHeader: "credential-origin"})
	assertStatus(test, response, http.StatusForbidden)
	response, _ = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusBadRequest)
	response, _ = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{"unknown":true}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "credential-json"})
	assertStatus(test, response, http.StatusBadRequest)
	response, payload = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, harness.cookieOne, true, map[string]string{idempotencyHeader: "credential-one"})
	assertStatus(test, response, http.StatusOK)
	if nestedString(payload, "credential", "secret") != "" {
		test.Fatalf("credential replay exposed secret")
	}
	response, payload = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID+"/credentials", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusOK)
	if len(payload["credentials"].([]any)) != 1 {
		test.Fatalf("list credentials: %v", payload)
	}
	response, _ = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/not-a-uuid", "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusNotFound)
	response, _ = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/"+credentialOneID, "", nil, true, nil)
	assertStatus(test, response, http.StatusUnauthorized)
	response, _ = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/"+credentialOneID, "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusForbidden)
	response, payload = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/"+credentialOneID, "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusOK)
	if nestedString(payload, "credential", "revoked_at") == "" {
		test.Fatalf("revoked timestamp missing: %v", payload)
	}
	response, _ = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/"+credentialOneID, "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusOK)
	if _, err := harness.service.Authenticate(context.Background(), secret); !errors.Is(err, tenant.ErrCredentialRevoked) {
		test.Fatalf("revoked credential authenticated: %v", err)
	}

	response, _ = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieTwo, true, nil)
	assertStatus(test, response, http.StatusCreated)
	response, _ = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID, "", harness.cookieTwo, false, nil)
	assertStatus(test, response, http.StatusNotFound)
	response, _ = harness.request(http.MethodGet, "/api/tenants/"+tenantOneID+"/credentials", "", harness.cookieTwo, false, nil)
	assertStatus(test, response, http.StatusNotFound)
	response, _ = harness.request(http.MethodPost, "/api/tenants/"+tenantOneID+"/credentials", `{}`, harness.cookieTwo, true, map[string]string{idempotencyHeader: "cross-owner-credential"})
	assertStatus(test, response, http.StatusNotFound)
	response, _ = harness.request(http.MethodDelete, "/api/tenants/"+tenantOneID+"/credentials/0196f0ec-3e80-7a54-bd2b-56cfe90bf899", "", harness.cookieTwo, true, nil)
	assertStatus(test, response, http.StatusNotFound)

	var eventCount int64
	if err := harness.database.Model(&gormstore.ControlEvent{}).Count(&eventCount).Error; err != nil || eventCount != 6 {
		test.Fatalf("control events: count=%d err=%v", eventCount, err)
	}
	foundOwnedResourceLog := false
	for _, entry := range harness.logger.logs {
		if entry.UserAccountID == accountOneID && entry.ResourceID == tenantOneID {
			foundOwnedResourceLog = true
		}
	}
	if !foundOwnedResourceLog {
		test.Fatalf("owned resource request was not logged")
	}
}

func TestControlPlaneDependencyAndDatabaseErrors(test *testing.T) {
	harness := newControlHarness(test)
	for _, build := range []func() (*Handler, error){
		func() (*Handler, error) {
			return NewHandler(nil, harness.handler.tenants, harness.handler.sessions, testAuthTenantID, testOrigin, testBrowserConfiguration(), harness.logger)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, nil, harness.handler.sessions, testAuthTenantID, testOrigin, testBrowserConfiguration(), harness.logger)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, harness.handler.tenants, nil, testAuthTenantID, testOrigin, testBrowserConfiguration(), harness.logger)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, harness.handler.tenants, harness.handler.sessions, "", testOrigin, testBrowserConfiguration(), harness.logger)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, harness.handler.tenants, harness.handler.sessions, testAuthTenantID, " ", testBrowserConfiguration(), harness.logger)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, harness.handler.tenants, harness.handler.sessions, testAuthTenantID, testOrigin, testBrowserConfiguration(), nil)
		},
		func() (*Handler, error) {
			return NewHandler(harness.handler.accounts, harness.handler.tenants, harness.handler.sessions, testAuthTenantID, testOrigin, BrowserConfig{}, harness.logger)
		},
	} {
		if _, err := build(); err == nil {
			test.Fatalf("expected handler dependency error")
		}
	}

	response, _ := harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusCreated)
	sqlDatabase, _ := harness.database.DB()
	if err := sqlDatabase.Close(); err != nil {
		test.Fatalf("close database: %v", err)
	}
	response, _ = harness.request(http.MethodGet, "/api/user-account", "", harness.cookieOne, false, nil)
	assertStatus(test, response, http.StatusInternalServerError)
	if harness.logger.logs[len(harness.logger.logs)-1].Error == nil {
		test.Fatalf("user account lookup failure was not retained in the request log")
	}
	response, _ = harness.request(http.MethodPut, "/api/user-account", "", harness.cookieTwo, true, nil)
	assertStatus(test, response, http.StatusInternalServerError)
	if harness.logger.logs[len(harness.logger.logs)-1].Error == nil {
		test.Fatalf("user account provision failure was not retained in the request log")
	}
}

func TestControlPlaneWorkspaceAssetsAndConfiguration(test *testing.T) {
	harness := newControlHarness(test)
	get := func(path string) (*http.Response, []byte) {
		test.Helper()
		response, err := http.Get(harness.server.URL + path)
		if err != nil {
			test.Fatalf("get %s: %v", path, err)
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			test.Fatalf("read %s: %v", path, err)
		}
		return response, body
	}
	for path, contentType := range map[string]string{
		"/":                                   "text/html; charset=utf-8",
		"/index.html":                         "text/html; charset=utf-8",
		"/assets/ledger/styles.css":           "text/css; charset=utf-8",
		"/assets/ledger/js/alpine-runtime.js": "text/javascript; charset=utf-8",
		"/assets/ledger/js/app.js":            "text/javascript; charset=utf-8",
		"/assets/ledger/js/client.js":         "text/javascript; charset=utf-8",
		"/assets/ledger/js/constants.js":      "text/javascript; charset=utf-8",
		"/assets/ledger/js/contracts.js":      "text/javascript; charset=utf-8",
	} {
		response, _ := get(path)
		assertStatus(test, response, http.StatusOK)
		if response.Header.Get("Content-Type") != contentType || response.Header.Get("Cache-Control") != "no-store" {
			test.Fatalf("asset %s headers=%v", path, response.Header)
		}
	}
	response, _ := get("/assets/ledger/unknown.js")
	assertStatus(test, response, http.StatusNotFound)
	response, _ = get("/unknown")
	assertStatus(test, response, http.StatusNotFound)
	response, body := get("/config-ui.yaml")
	assertStatus(test, response, http.StatusOK)
	if response.Header.Get("Content-Type") != "application/yaml" || response.Header.Get("Cache-Control") != "no-store" {
		test.Fatalf("browser configuration headers=%v", response.Header)
	}
	for _, fragment := range []string{testOrigin, testAuthTenantID, "google-client-id", "/auth/session"} {
		if !strings.Contains(string(body), fragment) {
			test.Fatalf("browser configuration missing %q: %s", fragment, body)
		}
	}

	recorder := httptest.NewRecorder()
	func() {
		defer func() {
			if recover() == nil {
				test.Fatalf("missing embedded file did not panic")
			}
		}()
		harness.handler.serveBrowserFile(recorder, "web/missing", "text/plain")
	}()
}

func TestControlPlaneTenantStorageErrors(test *testing.T) {
	harness := newControlHarness(test)
	response, _ := harness.request(http.MethodPut, "/api/user-account", "", harness.cookieOne, true, nil)
	assertStatus(test, response, http.StatusCreated)
	if err := harness.database.Migrator().DropTable(&gormstore.LedgerTenant{}); err != nil {
		test.Fatalf("drop tenants: %v", err)
	}
	requests := []struct {
		method  string
		path    string
		body    string
		headers map[string]string
	}{
		{method: http.MethodGet, path: "/api/tenants"},
		{method: http.MethodPost, path: "/api/tenants", body: `{"name":"One"}`, headers: map[string]string{idempotencyHeader: "tenant"}},
		{method: http.MethodGet, path: "/api/tenants/" + tenantOneID},
		{method: http.MethodGet, path: "/api/tenants/" + tenantOneID + "/credentials"},
		{method: http.MethodPost, path: "/api/tenants/" + tenantOneID + "/credentials", body: `{}`, headers: map[string]string{idempotencyHeader: "credential"}},
		{method: http.MethodDelete, path: "/api/tenants/" + tenantOneID + "/credentials/" + credentialOneID},
	}
	for _, request := range requests {
		mutation := request.method == http.MethodPost || request.method == http.MethodDelete
		response, _ = harness.request(request.method, request.path, request.body, harness.cookieOne, mutation, request.headers)
		assertStatus(test, response, http.StatusInternalServerError)
		if harness.logger.logs[len(harness.logger.logs)-1].Error == nil {
			test.Fatalf("%s %s storage failure was not retained in the request log", request.method, request.path)
		}
	}
}

func TestCursorAndRequestHelpers(test *testing.T) {
	if cursor, err := decodeCursor(""); err != nil || cursor != nil {
		test.Fatalf("empty cursor")
	}
	for _, raw := range []string{
		"!!!",
		"bm8tbmV3bGluZQ",
		"YmFkLXRpbWUKMDE5NmYwZWMtM2U4MC03YTU0LWJkMmItNTZjZmU5MGJmODAx",
		"MjAyNi0wOS0wMVQwMTowMjowM1oKYmFkLWlk",
	} {
		if _, err := decodeCursor(raw); !errors.Is(err, tenant.ErrInvalidCursor) {
			test.Fatalf("expected invalid cursor for %q", raw)
		}
	}
	if size, err := pageSize(""); err != nil || size != defaultPageSize {
		test.Fatalf("default page size")
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(requestIDHeader, strings.Repeat("x", 129))
	if value := requestID(request); value == "" || len(value) == 129 {
		test.Fatalf("oversized request id was accepted")
	}
	request.Header.Set(requestIDHeader, " fixed ")
	if value := requestID(request); value != "fixed" {
		test.Fatalf("request id was not normalized: %q", value)
	}

	recorder := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/", io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1))))
	request.Header.Set("Content-Type", "application/json")
	var target map[string]any
	if decodeJSON(recorder, request, "request", &target) {
		test.Fatalf("oversized JSON was accepted")
	}
}

func TestResponseObservationHelpers(test *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: recorder}
	if _, err := writer.Write([]byte("ok")); err != nil || writer.statusCode != http.StatusOK {
		test.Fatalf("implicit response status: status=%d err=%v", writer.statusCode, err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	setRequestResource(request, accountOneID, tenantOneID)

	harness := newControlHarness(test)
	harness.handler.mux = http.NewServeMux()
	harness.handler.mux.HandleFunc("GET /empty", func(http.ResponseWriter, *http.Request) {})
	recorder = httptest.NewRecorder()
	harness.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/empty", nil))
	if recorder.Code != http.StatusOK || harness.logger.logs[len(harness.logger.logs)-1].StatusCode != http.StatusOK {
		test.Fatalf("empty handler status was not observed")
	}
}

func assertStatus(test *testing.T, response *http.Response, expected int) {
	test.Helper()
	if response.StatusCode != expected {
		test.Fatalf("expected status %d, got %d", expected, response.StatusCode)
	}
}

func nestedString(payload map[string]any, objectKey string, fieldKey string) string {
	object, _ := payload[objectKey].(map[string]any)
	value, _ := object[fieldKey].(string)
	return value
}
