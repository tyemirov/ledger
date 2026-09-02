// @ts-check

export const AUTH_STATES = Object.freeze({
  LOADING: "loading",
  AUTHENTICATED: "authenticated",
  UNAUTHENTICATED: "unauthenticated",
  ERROR: "error",
});

export const RESOURCE_STATES = Object.freeze({
  IDLE: "idle",
  LOADING: "loading",
  READY: "ready",
  EMPTY: "empty",
  ERROR: "error",
});

export const SECTIONS = Object.freeze({
  OVERVIEW: "overview",
  CREDENTIALS: "credentials",
  INTEGRATION: "integration",
});

export const EVENTS = Object.freeze({
  AUTHENTICATED: "mpr-ui:auth:authenticated",
  AUTH_STATUS_CHANGE: "mpr-ui:auth:status-change",
  UNAUTHENTICATED: "mpr-ui:auth:unauthenticated",
  WORKSPACE_READY: "ledger:workspace-ready",
});

export const MPR_UI = Object.freeze({
  AUTH_STATUS_ATTRIBUTE: "data-mpr-auth-status",
  HEADER_ID: "ledger-header",
});

export const API = Object.freeze({
  USER_ACCOUNT: "/api/user-account",
  TENANTS: "/api/tenants",
  TENANT_PAGE_SIZE: 50,
  CSRF_HEADER: "X-Ledger-CSRF",
  IDEMPOTENCY_HEADER: "Idempotency-Key",
});

export const COPY = Object.freeze({
  APP_ERROR: "Ledger could not load the workspace.",
  AUTH_REQUIRED: "Sign in to open your Ledger workspace.",
  TENANTS_EMPTY: "Create your first tenant to start a Ledger integration.",
  TENANTS_ERROR: "Ledger could not load your tenants.",
  CREDENTIALS_EMPTY: "This tenant has no credentials.",
  CREDENTIALS_ERROR: "Ledger could not load tenant credentials.",
  TENANT_CREATED: "Tenant created.",
  CREDENTIAL_CREATED: "Credential created. Save it before you close this panel.",
  CREDENTIAL_REVOKED: "Credential revoked.",
  COPY_COMPLETE: "Credential copied.",
  COPY_FAILED: "Copy is unavailable. Select and copy the credential manually.",
});
