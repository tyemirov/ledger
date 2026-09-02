// @ts-check

import { API } from "./constants.js";
import { credentialDocument, credentialList, tenantDocument, tenantPage, userAccount } from "./contracts.js";

export class LedgerClientError extends Error {
  /** @param {string} message @param {number} status */
  constructor(message, status) {
    super(message);
    this.name = "LedgerClientError";
    this.status = status;
  }
}

/** @param {string} path @param {RequestInit} init @returns {Promise<unknown>} */
async function request(path, init) {
  const response = await fetch(path, { credentials: "include", ...init });
  const value = await response.json();
  if (!response.ok) {
    const error = value && typeof value === "object" && "error" in value ? value.error : null;
    const message = error && typeof error === "object" && "message" in error ? String(error.message) : "Ledger request failed.";
    throw new LedgerClientError(message, response.status);
  }
  return value;
}

/** @param {AbortSignal} signal */
export async function provisionUserAccount(signal) {
  return userAccount(await request(API.USER_ACCOUNT, mutation("PUT", undefined, "", signal)));
}

/** @param {string} cursor @param {AbortSignal} signal */
export async function listTenants(cursor, signal) {
  const query = new URLSearchParams({ limit: String(API.TENANT_PAGE_SIZE) });
  if (cursor) query.set("cursor", cursor);
  return tenantPage(await request(`${API.TENANTS}?${query}`, { method: "GET", signal }));
}

/** @param {string} name @param {string} key @param {AbortSignal} signal */
export async function createTenant(name, key, signal) {
  return tenantDocument(await request(API.TENANTS, mutation("POST", { name }, key, signal))).tenant;
}

/** @param {string} tenantID @param {AbortSignal} signal */
export async function listCredentials(tenantID, signal) {
  return credentialList(await request(credentialPath(tenantID), { method: "GET", signal })).credentials;
}

/** @param {string} tenantID @param {string} key @param {AbortSignal} signal */
export async function createCredential(tenantID, key, signal) {
  return credentialDocument(await request(credentialPath(tenantID), mutation("POST", {}, key, signal))).credential;
}

/** @param {string} tenantID @param {string} credentialID @param {AbortSignal} signal */
export async function revokeCredential(tenantID, credentialID, signal) {
  const path = `${credentialPath(tenantID)}/${encodeURIComponent(credentialID)}`;
  return credentialDocument(await request(path, mutation("DELETE", undefined, "", signal))).credential;
}

/** @param {string} tenantID */
function credentialPath(tenantID) {
  return `${API.TENANTS}/${encodeURIComponent(tenantID)}/credentials`;
}

/** @param {string} method @param {unknown} body @param {string} key @param {AbortSignal} signal @returns {RequestInit} */
function mutation(method, body, key, signal) {
  /** @type {Record<string, string>} */
  const headers = { [API.CSRF_HEADER]: "1" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (key) headers[API.IDEMPOTENCY_HEADER] = key;
  return { method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal };
}
