// @ts-check

/** @param {unknown} value @param {string} label @returns {Record<string, unknown>} */
function objectValue(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label}_invalid`);
  }
  return /** @type {Record<string, unknown>} */ (value);
}

/** @param {unknown} value @param {string} label @returns {string} */
function stringValue(value, label) {
  if (typeof value !== "string" || !value.trim()) {
    throw new Error(`${label}_invalid`);
  }
  return value;
}

/** @param {unknown} value @returns {{id: string, created_at: string}} */
export function userAccount(value) {
  const document = objectValue(value, "user_account_document");
  const account = objectValue(document.user_account, "user_account");
  return {
    id: stringValue(account.id, "user_account_id"),
    created_at: stringValue(account.created_at, "user_account_created_at"),
  };
}

/** @param {unknown} value @returns {{id: string, name: string, created_at: string}} */
export function tenant(value) {
  const source = objectValue(value, "tenant");
  return {
    id: stringValue(source.id, "tenant_id"),
    name: stringValue(source.name, "tenant_name"),
    created_at: stringValue(source.created_at, "tenant_created_at"),
  };
}

/** @param {unknown} value @returns {{tenant: ReturnType<typeof tenant>}} */
export function tenantDocument(value) {
  const document = objectValue(value, "tenant_document");
  return { tenant: tenant(document.tenant) };
}

/** @param {unknown} value @returns {{tenants: ReturnType<typeof tenant>[], next_cursor: string}} */
export function tenantPage(value) {
  const document = objectValue(value, "tenants_document");
  if (!Array.isArray(document.tenants)) {
    throw new Error("tenants_invalid");
  }
  if (document.next_cursor !== undefined && typeof document.next_cursor !== "string") {
    throw new Error("tenant_cursor_invalid");
  }
  return {
    tenants: document.tenants.map(tenant),
    next_cursor: typeof document.next_cursor === "string" ? document.next_cursor : "",
  };
}

/** @param {unknown} value @returns {{id: string, tenant_id: string, created_at: string, revoked_at: string, secret: string}} */
export function credential(value) {
  const source = objectValue(value, "credential");
  if (source.secret !== undefined && typeof source.secret !== "string") {
    throw new Error("credential_secret_invalid");
  }
  if (source.revoked_at !== undefined && typeof source.revoked_at !== "string") {
    throw new Error("credential_revoked_at_invalid");
  }
  return {
    id: stringValue(source.id, "credential_id"),
    tenant_id: stringValue(source.tenant_id, "credential_tenant_id"),
    created_at: stringValue(source.created_at, "credential_created_at"),
    revoked_at: typeof source.revoked_at === "string" ? source.revoked_at : "",
    secret: typeof source.secret === "string" ? source.secret : "",
  };
}

/** @param {unknown} value @returns {{credential: ReturnType<typeof credential>}} */
export function credentialDocument(value) {
  const document = objectValue(value, "credential_document");
  return { credential: credential(document.credential) };
}

/** @param {unknown} value @returns {{credentials: ReturnType<typeof credential>[]}} */
export function credentialList(value) {
  const document = objectValue(value, "credentials_document");
  if (!Array.isArray(document.credentials)) {
    throw new Error("credentials_invalid");
  }
  return { credentials: document.credentials.map(credential) };
}
