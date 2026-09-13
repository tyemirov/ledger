package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/tenant"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/google/uuid"
	"github.com/tyemirov/tauth/pkg/sessionvalidator"
)

const (
	csrfHeader        = "X-Ledger-CSRF"
	idempotencyHeader = "Idempotency-Key"
	requestIDHeader   = "X-Request-ID"
	corsAllowHeaders  = "Content-Type, Idempotency-Key, X-Ledger-CSRF, X-Request-ID"
	corsAllowMethods  = "DELETE, GET, POST, PUT"
	defaultPageSize   = 50
	maximumPageSize   = 200
)

type SessionValidator interface {
	ValidateRequest(*http.Request) (*sessionvalidator.Claims, error)
}

type RequestLog struct {
	Operation     string
	StatusCode    int
	Duration      time.Duration
	UserAccountID string
	ResourceID    string
	Error         error
}

type RequestLogger interface {
	LogControlRequest(RequestLog)
}

type Handler struct {
	accounts     *useraccount.Service
	tenants      *tenant.Service
	sessions     SessionValidator
	authTenantID string
	publicOrigin string
	logger       RequestLogger
	browser      BrowserConfig
	mux          *http.ServeMux
}

type requestState struct {
	requestID     string
	userAccountID string
	resourceID    string
	internalError error
}

type requestStateKey struct{}

type statusWriter struct {
	http.ResponseWriter
	statusCode int
}

func (writer *statusWriter) WriteHeader(statusCode int) {
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

func (writer *statusWriter) Write(data []byte) (int, error) {
	if writer.statusCode == 0 {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(data)
}

type principal struct {
	identity useraccount.ExternalIdentity
	account  *useraccount.Account
}

type errorDocument struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type userAccountDocument struct {
	UserAccount userAccountBody `json:"user_account"`
}

type userAccountBody struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

type tenantDocument struct {
	Tenant tenantBody `json:"tenant"`
}

type tenantsDocument struct {
	Tenants    []tenantBody `json:"tenants"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

type tenantBody struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type credentialDocument struct {
	Credential credentialBody `json:"credential"`
}

type credentialsDocument struct {
	Credentials []credentialBody `json:"credentials"`
}

type credentialBody struct {
	ID        string     `json:"id"`
	TenantID  string     `json:"tenant_id"`
	Secret    string     `json:"secret,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func NewHandler(accounts *useraccount.Service, tenants *tenant.Service, sessions SessionValidator, authTenantID string, publicOrigin string, browser BrowserConfig, logger RequestLogger) (*Handler, error) {
	if accounts == nil || tenants == nil || sessions == nil || logger == nil || strings.TrimSpace(authTenantID) == "" {
		return nil, errors.New("control_plane_invalid_dependency")
	}
	origin := strings.TrimRight(strings.TrimSpace(publicOrigin), "/")
	if origin == "" {
		return nil, errors.New("control_plane_invalid_public_origin")
	}
	if err := browser.validate(origin, authTenantID); err != nil {
		return nil, err
	}
	handler := &Handler{
		accounts:     accounts,
		tenants:      tenants,
		sessions:     sessions,
		authTenantID: strings.TrimSpace(authTenantID),
		publicOrigin: origin,
		logger:       logger,
		browser:      browser,
		mux:          http.NewServeMux(),
	}
	handler.routes()
	return handler, nil
}

func (handler *Handler) routes() {
	handler.mux.HandleFunc("GET /{$}", handler.workspace)
	handler.mux.HandleFunc("GET /index.html", handler.workspace)
	handler.mux.HandleFunc("GET /config-ui.yaml", handler.browserConfiguration)
	handler.mux.HandleFunc("GET /assets/ledger/", handler.workspaceAsset)
	handler.mux.HandleFunc("GET /healthz", handler.health)
	handler.mux.HandleFunc("GET /api/user-account", handler.getUserAccount)
	handler.mux.HandleFunc("PUT /api/user-account", handler.provisionUserAccount)
	handler.mux.HandleFunc("GET /api/tenants", handler.listTenants)
	handler.mux.HandleFunc("POST /api/tenants", handler.createTenant)
	handler.mux.HandleFunc("GET /api/tenants/{tenantID}", handler.getTenant)
	handler.mux.HandleFunc("GET /api/tenants/{tenantID}/credentials", handler.listCredentials)
	handler.mux.HandleFunc("POST /api/tenants/{tenantID}/credentials", handler.createCredential)
	handler.mux.HandleFunc("DELETE /api/tenants/{tenantID}/credentials/{credentialID}", handler.revokeCredential)
}

func (handler *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	startedAt := time.Now()
	state := &requestState{requestID: requestID(request)}
	request = request.WithContext(context.WithValue(request.Context(), requestStateKey{}, state))
	observed := &statusWriter{ResponseWriter: response}
	defer func() {
		statusCode := observed.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if request.URL.Path == "/healthz" && statusCode == http.StatusOK {
			return
		}
		handler.logger.LogControlRequest(RequestLog{
			Operation:     request.Method + " " + request.URL.Path,
			StatusCode:    statusCode,
			Duration:      time.Since(startedAt),
			UserAccountID: state.userAccountID,
			ResourceID:    state.resourceID,
			Error:         state.internalError,
		})
	}()
	observed.Header().Set("Cache-Control", "private, no-store")
	observed.Header().Set("X-Content-Type-Options", "nosniff")
	if !handler.authorizeCrossOrigin(observed, request) {
		return
	}
	handler.mux.ServeHTTP(observed, request)
}

func (handler *Handler) authorizeCrossOrigin(response http.ResponseWriter, request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin != "" && origin != handler.publicOrigin {
		writeError(response, http.StatusForbidden, "origin_rejected", "The request origin is not allowed.", requestID(request))
		return false
	}
	if origin == handler.publicOrigin {
		response.Header().Set("Access-Control-Allow-Origin", handler.publicOrigin)
		response.Header().Set("Access-Control-Allow-Credentials", "true")
		response.Header().Add("Vary", "Origin")
	}
	if request.Method != http.MethodOptions {
		return true
	}
	if origin == "" || !strings.HasPrefix(request.URL.Path, "/api/") {
		http.NotFound(response, request)
		return false
	}
	requestedMethod := request.Header.Get("Access-Control-Request-Method")
	switch requestedMethod {
	case http.MethodDelete, http.MethodGet, http.MethodPost, http.MethodPut:
	default:
		response.Header().Set("Allow", corsAllowMethods)
		writeError(response, http.StatusMethodNotAllowed, "cors_method_rejected", "The requested method is not allowed.", requestID(request))
		return false
	}
	allowedHeaders := map[string]struct{}{
		"content-type":    {},
		"idempotency-key": {},
		"x-ledger-csrf":   {},
		"x-request-id":    {},
	}
	for _, name := range strings.Split(request.Header.Get("Access-Control-Request-Headers"), ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			continue
		}
		if _, ok := allowedHeaders[name]; !ok {
			writeError(response, http.StatusForbidden, "cors_header_rejected", "A requested header is not allowed.", requestID(request))
			return false
		}
	}
	response.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
	response.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)
	response.WriteHeader(http.StatusNoContent)
	return false
}

func (handler *Handler) health(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(request.Context(), time.Second)
	defer cancel()
	if err := handler.accounts.CheckHealth(ctx); err != nil {
		setRequestError(request, err)
		writeJSON(response, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

func (handler *Handler) authenticate(response http.ResponseWriter, request *http.Request, provisioned bool) (principal, string, bool) {
	requestID := requestID(request)
	claims, err := handler.sessions.ValidateRequest(request)
	if err != nil || claims == nil || claims.ExpiresAt == nil || strings.TrimSpace(claims.TenantID) != handler.authTenantID {
		writeError(response, http.StatusUnauthorized, "unauthenticated", "A valid Ledger session is required.", requestID)
		return principal{}, requestID, false
	}
	identity, err := useraccount.NewExternalIdentity(claims.Issuer, claims.TenantID, claims.UserID)
	if err != nil {
		writeError(response, http.StatusUnauthorized, "unauthenticated", "A valid Ledger session is required.", requestID)
		return principal{}, requestID, false
	}
	result := principal{identity: identity}
	if !provisioned {
		return result, requestID, true
	}
	account, err := handler.accounts.Get(request.Context(), identity)
	if errors.Is(err, useraccount.ErrNotFound) {
		writeError(response, http.StatusNotFound, "user_account_not_found", "Provision the UserAccount before managing tenants.", requestID)
		return principal{}, requestID, false
	}
	if err != nil {
		setRequestError(request, err)
		writeError(response, http.StatusInternalServerError, "internal", "The request could not be completed.", requestID)
		return principal{}, requestID, false
	}
	result.account = &account
	if state, ok := request.Context().Value(requestStateKey{}).(*requestState); ok {
		state.userAccountID = account.ID().String()
	}
	return result, requestID, true
}

func (handler *Handler) authorizeMutation(response http.ResponseWriter, request *http.Request, requestID string, bodyRequired bool) bool {
	if request.Header.Get("Origin") != handler.publicOrigin {
		writeError(response, http.StatusForbidden, "origin_rejected", "The request origin is not allowed.", requestID)
		return false
	}
	if request.Header.Get(csrfHeader) != "1" {
		writeError(response, http.StatusForbidden, "csrf_rejected", "The CSRF header is required.", requestID)
		return false
	}
	if bodyRequired && request.Header.Get("Content-Type") != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "content_type_invalid", "Content-Type must be application/json.", requestID)
		return false
	}
	return true
}

func (handler *Handler) getUserAccount(response http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := handler.authenticate(response, request, true)
	if !ok {
		return
	}
	writeJSONWithRequestID(response, http.StatusOK, userAccountDocument{UserAccount: mapUserAccount(*actor.account)}, requestID)
}

func (handler *Handler) provisionUserAccount(response http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := handler.authenticate(response, request, false)
	if !ok || !handler.authorizeMutation(response, request, requestID, false) {
		return
	}
	account, created, err := handler.accounts.Provision(request.Context(), actor.identity, requestID)
	if err != nil {
		setRequestError(request, err)
		writeError(response, http.StatusInternalServerError, "internal", "The request could not be completed.", requestID)
		return
	}
	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}
	setRequestResource(request, account.ID().String(), account.ID().String())
	response.Header().Set("Location", "/api/user-account")
	writeJSONWithRequestID(response, statusCode, userAccountDocument{UserAccount: mapUserAccount(account)}, requestID)
}

func (handler *Handler) listTenants(response http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := handler.authenticate(response, request, true)
	if !ok {
		return
	}
	cursor, err := decodeCursor(request.URL.Query().Get("cursor"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "cursor_invalid", "The tenant cursor is invalid.", requestID)
		return
	}
	limit, err := pageSize(request.URL.Query().Get("limit"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "limit_invalid", "The tenant limit is invalid.", requestID)
		return
	}
	items, err := handler.tenants.List(request.Context(), actor.account.ID(), cursor, limit+1)
	if err != nil {
		setRequestError(request, err)
		writeError(response, http.StatusInternalServerError, "internal", "The request could not be completed.", requestID)
		return
	}
	document := tenantsDocument{Tenants: make([]tenantBody, 0, min(len(items), limit))}
	for _, item := range items[:min(len(items), limit)] {
		document.Tenants = append(document.Tenants, mapTenant(item))
	}
	if len(items) > limit {
		last := items[limit-1]
		document.NextCursor = encodeCursor(last)
	}
	writeJSONWithRequestID(response, http.StatusOK, document, requestID)
}

func (handler *Handler) createTenant(response http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := handler.authenticate(response, request, true)
	if !ok || !handler.authorizeMutation(response, request, requestID, true) {
		return
	}
	key, ok := parseIdempotencyKey(response, request, requestID)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(response, request, requestID, &input) {
		return
	}
	name, err := tenant.NewName(input.Name)
	if err != nil {
		writeError(response, http.StatusBadRequest, "tenant_name_invalid", "The tenant name is invalid.", requestID)
		return
	}
	item, created, err := handler.tenants.Create(request.Context(), actor.account.ID(), name, key, requestID)
	if err != nil {
		handler.writeTenantError(response, request, err, requestID)
		return
	}
	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}
	setRequestResource(request, actor.account.ID().String(), item.ID().String())
	response.Header().Set("Location", "/api/tenants/"+item.ID().String())
	writeJSONWithRequestID(response, statusCode, tenantDocument{Tenant: mapTenant(item)}, requestID)
}

func (handler *Handler) getTenant(response http.ResponseWriter, request *http.Request) {
	actor, requestID, ok := handler.authenticate(response, request, true)
	if !ok {
		return
	}
	tenantID, ok := parseTenantID(response, request.PathValue("tenantID"), requestID)
	if !ok {
		return
	}
	item, err := handler.tenants.Get(request.Context(), actor.account.ID(), tenantID)
	if err != nil {
		handler.writeTenantError(response, request, err, requestID)
		return
	}
	setRequestResource(request, actor.account.ID().String(), item.ID().String())
	writeJSONWithRequestID(response, http.StatusOK, tenantDocument{Tenant: mapTenant(item)}, requestID)
}

func (handler *Handler) listCredentials(response http.ResponseWriter, request *http.Request) {
	actor, requestID, tenantID, ok := handler.tenantActor(response, request)
	if !ok {
		return
	}
	items, err := handler.tenants.ListCredentials(request.Context(), actor.account.ID(), tenantID)
	if err != nil {
		handler.writeTenantError(response, request, err, requestID)
		return
	}
	document := credentialsDocument{Credentials: make([]credentialBody, 0, len(items))}
	for _, item := range items {
		document.Credentials = append(document.Credentials, mapCredential(item, ""))
	}
	writeJSONWithRequestID(response, http.StatusOK, document, requestID)
}

func (handler *Handler) createCredential(response http.ResponseWriter, request *http.Request) {
	actor, requestID, tenantID, ok := handler.tenantActor(response, request)
	if !ok || !handler.authorizeMutation(response, request, requestID, true) {
		return
	}
	key, ok := parseIdempotencyKey(response, request, requestID)
	if !ok {
		return
	}
	var input struct{}
	if !decodeJSON(response, request, requestID, &input) {
		return
	}
	credential, secret, created, err := handler.tenants.CreateCredential(request.Context(), actor.account.ID(), tenantID, key, requestID)
	if err != nil {
		handler.writeTenantError(response, request, err, requestID)
		return
	}
	statusCode := http.StatusOK
	if created {
		statusCode = http.StatusCreated
	}
	setRequestResource(request, actor.account.ID().String(), credential.ID().String())
	response.Header().Set("Location", "/api/tenants/"+tenantID.String()+"/credentials/"+credential.ID().String())
	writeJSONWithRequestID(response, statusCode, credentialDocument{Credential: mapCredential(credential, secret)}, requestID)
}

func (handler *Handler) revokeCredential(response http.ResponseWriter, request *http.Request) {
	actor, requestID, tenantID, ok := handler.tenantActor(response, request)
	if !ok || !handler.authorizeMutation(response, request, requestID, false) {
		return
	}
	credentialID, err := tenant.NewCredentialID(request.PathValue("credentialID"))
	if err != nil {
		writeError(response, http.StatusNotFound, "tenant_not_found", "The tenant resource was not found.", requestID)
		return
	}
	credential, err := handler.tenants.RevokeCredential(request.Context(), actor.account.ID(), tenantID, credentialID, requestID)
	if err != nil {
		handler.writeTenantError(response, request, err, requestID)
		return
	}
	setRequestResource(request, actor.account.ID().String(), credential.ID().String())
	writeJSONWithRequestID(response, http.StatusOK, credentialDocument{Credential: mapCredential(credential, "")}, requestID)
}

func (handler *Handler) tenantActor(response http.ResponseWriter, request *http.Request) (principal, string, tenant.ID, bool) {
	actor, requestID, ok := handler.authenticate(response, request, true)
	if !ok {
		return principal{}, requestID, tenant.ID{}, false
	}
	tenantID, ok := parseTenantID(response, request.PathValue("tenantID"), requestID)
	if ok {
		setRequestResource(request, actor.account.ID().String(), tenantID.String())
	}
	return actor, requestID, tenantID, ok
}

func setRequestResource(request *http.Request, userAccountID string, resourceID string) {
	state, ok := request.Context().Value(requestStateKey{}).(*requestState)
	if !ok {
		return
	}
	state.userAccountID = userAccountID
	state.resourceID = resourceID
}

func setRequestError(request *http.Request, err error) {
	state, ok := request.Context().Value(requestStateKey{}).(*requestState)
	if ok {
		state.internalError = err
	}
}

func (handler *Handler) writeTenantError(response http.ResponseWriter, request *http.Request, err error, requestID string) {
	switch {
	case errors.Is(err, tenant.ErrNotFound):
		writeError(response, http.StatusNotFound, "tenant_not_found", "The tenant resource was not found.", requestID)
	case errors.Is(err, tenant.ErrIdempotencyConflict):
		writeError(response, http.StatusConflict, "idempotency_conflict", "The idempotency key was used for another request.", requestID)
	default:
		setRequestError(request, err)
		writeError(response, http.StatusInternalServerError, "internal", "The request could not be completed.", requestID)
	}
}

func parseTenantID(response http.ResponseWriter, raw string, requestID string) (tenant.ID, bool) {
	id, err := tenant.NewID(raw)
	if err != nil {
		writeError(response, http.StatusNotFound, "tenant_not_found", "The tenant resource was not found.", requestID)
		return tenant.ID{}, false
	}
	return id, true
}

func parseIdempotencyKey(response http.ResponseWriter, request *http.Request, requestID string) (tenant.IdempotencyKey, bool) {
	key, err := tenant.NewIdempotencyKey(request.Header.Get(idempotencyHeader))
	if err != nil {
		writeError(response, http.StatusBadRequest, "idempotency_key_invalid", "Idempotency-Key is required.", requestID)
		return tenant.IdempotencyKey{}, false
	}
	return key, true
}

func decodeJSON(response http.ResponseWriter, request *http.Request, requestID string, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(response, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(response, http.StatusBadRequest, "json_invalid", "The JSON request is invalid.", requestID)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, "json_invalid", "The JSON request is invalid.", requestID)
		return false
	}
	return true
}

func pageSize(raw string) (int, error) {
	if raw == "" {
		return defaultPageSize, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximumPageSize {
		return 0, errors.New("page_size_invalid")
	}
	return value, nil
}

func encodeCursor(item tenant.Tenant) string {
	value := item.CreatedAt().Format(time.RFC3339Nano) + "\n" + item.ID().String()
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(raw string) (*tenant.Cursor, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, tenant.ErrInvalidCursor
	}
	parts := strings.Split(string(decoded), "\n")
	if len(parts) != 2 {
		return nil, tenant.ErrInvalidCursor
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, tenant.ErrInvalidCursor
	}
	tenantID, err := tenant.NewID(parts[1])
	if err != nil {
		return nil, tenant.ErrInvalidCursor
	}
	return &tenant.Cursor{CreatedAt: createdAt.UTC(), TenantID: tenantID}, nil
}

func mapUserAccount(account useraccount.Account) userAccountBody {
	return userAccountBody{ID: account.ID().String(), CreatedAt: account.CreatedAt()}
}

func mapTenant(item tenant.Tenant) tenantBody {
	return tenantBody{ID: item.ID().String(), Name: item.Name().String(), CreatedAt: item.CreatedAt()}
}

func mapCredential(item tenant.Credential, secret string) credentialBody {
	body := credentialBody{ID: item.ID().String(), TenantID: item.TenantID().String(), Secret: secret, CreatedAt: item.CreatedAt()}
	if revokedAt, revoked := item.RevokedAt(); revoked {
		body.RevokedAt = &revokedAt
	}
	return body
}

func requestID(request *http.Request) string {
	if state, ok := request.Context().Value(requestStateKey{}).(*requestState); ok && state.requestID != "" {
		return state.requestID
	}
	value := strings.TrimSpace(request.Header.Get(requestIDHeader))
	if value != "" && len(value) <= 128 {
		return value
	}
	return uuid.NewString()
}

func writeError(response http.ResponseWriter, statusCode int, code string, message string, requestID string) {
	writeJSONWithRequestID(response, statusCode, errorDocument{Error: errorBody{Code: code, Message: message, RequestID: requestID}}, requestID)
}

func writeJSONWithRequestID(response http.ResponseWriter, statusCode int, body any, requestID string) {
	response.Header().Set(requestIDHeader, requestID)
	writeJSON(response, statusCode, body)
}

func writeJSON(response http.ResponseWriter, statusCode int, body any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(statusCode)
	_ = json.NewEncoder(response).Encode(body)
}
