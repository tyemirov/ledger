// @ts-check

import {
  AUTH_CLIENT_PATH,
  CONFIG_SCRIPT_PATH,
  DEFAULT_TAUTH_BASE_URL,
  DEFAULT_SITE_ID,
  LOGIN_PATH,
  LOGOUT_PATH,
  NONCE_PATH,
} from "./constants.js";

/**
 * @typedef {object} DemoConfig
 * @property {string} baseUrl
 * @property {string} siteId
 * @property {string} loginPath
 * @property {string} logoutPath
 * @property {string} noncePath
 * @property {string} authClientUrl
 */

const GLOBAL_CONFIG_KEYS = Object.freeze([
  "demoConfig",
  "tauthDemoConfig",
  "tauthConfig",
  "__TAUTH_DEMO_CONFIG",
  "__TAUTH_CONFIG__",
  "__TAUTH_DEMO_CONFIG__",
  "mprDemoConfig",
]);

/**
 * Loads configuration from the TAuth-served config script and merges it with defaults.
 * @param {string | undefined} baseUrlHint
 * @returns {Promise<DemoConfig>}
 */
export async function loadDemoConfig(baseUrlHint) {
  const fallbackBase = normalizeBaseUrl(baseUrlHint || DEFAULT_TAUTH_BASE_URL);
  const defaults = {
    baseUrl: fallbackBase,
    siteId: DEFAULT_SITE_ID,
    loginPath: LOGIN_PATH,
  logoutPath: LOGOUT_PATH,
  noncePath: NONCE_PATH,
  authClientUrl: `${fallbackBase}${AUTH_CLIENT_PATH}`,
  };
  const scriptConfig = await loadConfigScript(defaults.baseUrl);
  const safeScriptConfig = scriptConfig || {};
  const merged = {
    ...defaults,
    ...safeScriptConfig,
  };
  merged.siteId = normalizeSiteId(merged.siteId);
  merged.baseUrl = normalizeBaseUrl(merged.baseUrl || defaults.baseUrl);
  merged.authClientUrl =
    typeof merged.authClientUrl === "string" && merged.authClientUrl.trim().length > 0
      ? merged.authClientUrl
      : `${merged.baseUrl}${AUTH_CLIENT_PATH}`;
  return merged;
}

/**
 * @param {string} baseUrl
 * @returns {Promise<Partial<DemoConfig>>}
 */
async function loadConfigScript(baseUrl) {
  const scriptUrl = `${baseUrl}${CONFIG_SCRIPT_PATH}`;
  try {
    await injectScript(scriptUrl);
  } catch {
    return null;
  }
  const config = readConfigFromWindow();
  return config || {};
}

/**
 * @returns {Partial<DemoConfig> | null}
 */
function readConfigFromWindow() {
  for (const key of GLOBAL_CONFIG_KEYS) {
    const candidate = window[key];
    if (candidate && typeof candidate === "object") {
      return normalizeConfigShape(candidate);
    }
  }
  return null;
}

/**
 * @param {unknown} raw
 * @returns {Partial<DemoConfig>}
 */
function normalizeConfigShape(raw) {
  if (!raw || typeof raw !== "object") {
    return {};
  }
  const candidate = /** @type {Record<string, unknown>} */ (raw);
  const normalized = /** @type {Partial<DemoConfig>} */ ({});
  const baseUrlValue =
    typeof candidate.baseUrl === "string"
      ? candidate.baseUrl
      : typeof candidate.base_url === "string"
        ? candidate.base_url
        : typeof candidate.tauthBaseUrl === "string"
          ? candidate.tauthBaseUrl
          : undefined;
  if (typeof baseUrlValue === "string") {
    normalized.baseUrl = normalizeBaseUrl(baseUrlValue);
  }
  const siteIdValue =
    typeof candidate.siteId === "string"
      ? candidate.siteId
      : typeof candidate.site_id === "string"
        ? candidate.site_id
        : undefined;
  if (typeof siteIdValue === "string") {
    normalized.siteId = normalizeSiteId(siteIdValue);
  }
  const loginPathValue =
    typeof candidate.loginPath === "string"
      ? candidate.loginPath
      : typeof candidate.login_path === "string"
        ? candidate.login_path
        : undefined;
  if (typeof loginPathValue === "string") {
    normalized.loginPath = loginPathValue;
  }
  const logoutPathValue =
    typeof candidate.logoutPath === "string"
      ? candidate.logoutPath
      : typeof candidate.logout_path === "string"
        ? candidate.logout_path
        : undefined;
  if (typeof logoutPathValue === "string") {
    normalized.logoutPath = logoutPathValue;
  }
  const noncePathValue =
    typeof candidate.noncePath === "string"
      ? candidate.noncePath
      : typeof candidate.nonce_path === "string"
        ? candidate.nonce_path
        : undefined;
  if (typeof noncePathValue === "string") {
    normalized.noncePath = noncePathValue;
  }
  const authClientUrlValue =
    typeof candidate.authClientUrl === "string"
      ? candidate.authClientUrl
      : typeof candidate.auth_client_url === "string"
        ? candidate.auth_client_url
        : typeof candidate.authClient === "string"
          ? candidate.authClient
          : undefined;
  if (typeof authClientUrlValue === "string") {
    normalized.authClientUrl = authClientUrlValue;
  }
  const googleClientIdValue =
    typeof candidate.googleClientId === "string"
      ? candidate.googleClientId
      : typeof candidate.google_client_id === "string"
        ? candidate.google_client_id
        : undefined;
  if (typeof googleClientIdValue === "string") {
    normalized.siteId = normalizeSiteId(googleClientIdValue);
  }
  return normalized;
}

/**
 * @param {string | undefined} value
 * @returns {string}
 */
function normalizeSiteId(value) {
  const trimmed = typeof value === "string" ? value.trim() : "";
  if (trimmed.length > 0) {
    return trimmed;
  }
  return DEFAULT_SITE_ID;
}

/**
 * @param {string} url
 * @returns {Promise<void>}
 */
function injectScript(url) {
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = url;
    script.defer = true;
    script.crossOrigin = "anonymous";
    script.onload = () => resolve();
    script.onerror = () =>
      reject(new Error(`failed to load config script from ${url}`));
    document.head.append(script);
  });
}

/**
 * @param {string} raw
 * @returns {string}
 */
export function normalizeBaseUrl(raw) {
  const trimmed = raw.trim();
  if (!trimmed.length) {
    return DEFAULT_TAUTH_BASE_URL;
  }
  return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}
