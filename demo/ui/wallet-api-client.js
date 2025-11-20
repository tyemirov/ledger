// @ts-check

import { DEFAULT_API_BASE_URL } from "./constants.js";

/**
 * @typedef {object} BalanceSnapshot
 * @property {number} totalCents
 * @property {number} availableCents
 * @property {number} totalCoins
 * @property {number} availableCoins
 *
 * @typedef {object} LedgerEntrySnapshot
 * @property {string} entryId
 * @property {string} type
 * @property {number} amountCents
 * @property {number} amountCoins
 * @property {string} reservationId
 * @property {string} idempotencyKey
 * @property {Record<string, unknown> | null} metadata
 * @property {string} metadataText
 * @property {number} createdAtUnix
 *
 * @typedef {object} WalletSnapshot
 * @property {BalanceSnapshot} balance
 * @property {LedgerEntrySnapshot[]} entries
 *
 * @typedef {object} TransactionResult
 * @property {string} status
 * @property {WalletSnapshot} wallet
 *
 * @typedef {object} WalletApiClient
 * @property {() => Promise<WalletSnapshot>} fetchWallet
 * @property {() => Promise<WalletSnapshot>} bootstrapWallet
 * @property {(coins: number) => Promise<WalletSnapshot>} purchaseCoins
 * @property {() => Promise<TransactionResult>} spendCoins
 */

/**
 * HTTP error wrapper so the UI can respond to 4xx/5xx states.
 */
export class HttpError extends Error {
  /**
   * @param {number} status
   * @param {string} message
   */
  constructor(status, message) {
    super(message);
    this.name = "HttpError";
    this.status = status;
  }
}

/**
 * @param {string | undefined} rawBaseUrl
 * @returns {WalletApiClient}
 */
export function createWalletApiClient(rawBaseUrl) {
  const baseUrl = normalizeApiBaseUrl(rawBaseUrl || "");
  const requestJSON = createRequest(baseUrl);

  return {
    async fetchWallet() {
      const payload = await requestJSON("/api/wallet");
      return parseWalletEnvelope(payload);
    },
    async bootstrapWallet() {
      const payload = await requestJSON("/api/bootstrap", {
        method: "POST",
      });
      return parseWalletEnvelope(payload);
    },
    async purchaseCoins(coins) {
      const payload = await requestJSON("/api/purchases", {
        method: "POST",
        body: JSON.stringify({ coins }),
      });
      return parseWalletEnvelope(payload);
    },
    async spendCoins() {
      const payload = await requestJSON("/api/transactions", {
        method: "POST",
      });
      return parseTransactionEnvelope(payload);
    },
  };
}

/**
 * @param {string} baseUrl
 * @returns {(path: string, init?: RequestInit) => Promise<any>}
 */
function createRequest(baseUrl) {
  return async function request(path, init) {
    const url = `${baseUrl}${path}`;
    const fetchImpl =
      typeof window.apiFetch === "function"
        ? window.apiFetch.bind(window)
        : window.fetch.bind(window);
    const options = { ...(init || {}) };
    const headers = new Headers(options.headers || undefined);
    if (options.body && !headers.has("Content-Type")) {
      headers.set("Content-Type", "application/json");
    }
    options.headers = headers;
    options.credentials = "include";
    if (!options.method) {
      options.method = "GET";
    }
    const response = await fetchImpl(url, options);
    if (!response.ok) {
      throw new HttpError(response.status, await extractErrorMessage(response));
    }
    if (response.status === 204) {
      return {};
    }
    return response.json();
  };
}

/**
 * @param {Response} response
 * @returns {Promise<string>}
 */
async function extractErrorMessage(response) {
  try {
    const payload = await response.json();
    if (payload && typeof payload === "object") {
      const errorValue = /** @type {Record<string, unknown>} */ (payload).error;
      if (
        errorValue &&
        typeof errorValue === "object" &&
        typeof errorValue.message === "string"
      ) {
        return errorValue.message;
      }
    }
  } catch {
    // fall through to text extraction
  }
  const text = await response.text();
  if (text.trim().length > 0) {
    return text.trim();
  }
  return `request failed with status ${response.status}`;
}

/**
 * @param {any} payload
 * @returns {WalletSnapshot}
 */
function parseWalletEnvelope(payload) {
  if (!payload || typeof payload !== "object") {
    throw new Error("invalid wallet payload");
  }
  const source = /** @type {Record<string, unknown>} */ (payload);
  const walletValue = source.wallet;
  if (!walletValue || typeof walletValue !== "object") {
    throw new Error("wallet field missing in response");
  }
  return normalizeWalletSnapshot(walletValue);
}

/**
 * @param {any} payload
 * @returns {TransactionResult}
 */
function parseTransactionEnvelope(payload) {
  if (!payload || typeof payload !== "object") {
    throw new Error("invalid transaction payload");
  }
  const source = /** @type {Record<string, unknown>} */ (payload);
  const statusValue = source.status;
  if (typeof statusValue !== "string" || statusValue.trim().length === 0) {
    throw new Error("transaction status missing");
  }
  const walletValue = source.wallet;
  if (!walletValue || typeof walletValue !== "object") {
    throw new Error("transaction wallet missing");
  }
  return {
    status: statusValue,
    wallet: normalizeWalletSnapshot(walletValue),
  };
}

/**
 * @param {unknown} raw
 * @returns {WalletSnapshot}
 */
function normalizeWalletSnapshot(raw) {
  if (!raw || typeof raw !== "object") {
    throw new Error("wallet payload invalid");
  }
  const source = /** @type {Record<string, unknown>} */ (raw);
  const balanceValue = source.balance;
  if (!balanceValue || typeof balanceValue !== "object") {
    throw new Error("wallet balance missing");
  }
  const entriesValue = source.entries;
  const entries = Array.isArray(entriesValue)
    ? entriesValue
        .map((entry) =>
          typeof entry === "object" && entry !== null
            ? normalizeEntry(entry)
            : null,
        )
        .filter((entry) => entry !== null)
    : [];
  return {
    balance: normalizeBalance(balanceValue),
    entries,
  };
}

/**
 * @param {unknown} raw
 * @returns {BalanceSnapshot}
 */
function normalizeBalance(raw) {
  if (!raw || typeof raw !== "object") {
    throw new Error("balance payload invalid");
  }
  const source = /** @type {Record<string, unknown>} */ (raw);
  return {
    totalCents: readNumber(source.total_cents),
    availableCents: readNumber(source.available_cents),
    totalCoins: readNumber(source.total_coins),
    availableCoins: readNumber(source.available_coins),
  };
}

/**
 * @param {Record<string, unknown>} raw
 * @returns {LedgerEntrySnapshot}
 */
function normalizeEntry(raw) {
  const entryId = readString(raw.entry_id, "entry_id");
  const type = readString(raw.type, "type");
  const amountCents = readNumber(raw.amount_cents);
  const amountCoins = readNumber(raw.amount_coins);
  const reservationId =
    typeof raw.reservation_id === "string" ? raw.reservation_id : "";
  const idempotencyKey =
    typeof raw.idempotency_key === "string" ? raw.idempotency_key : "";
  const createdAtUnix = readNumber(raw.created_unix_utc);
  const metadataValue = parseMetadata(raw.metadata);
  return {
    entryId,
    type,
    amountCents,
    amountCoins,
    reservationId,
    idempotencyKey,
    metadata: metadataValue,
    metadataText: stringifyMetadata(metadataValue),
    createdAtUnix,
  };
}

/**
 * @param {unknown} raw
 * @returns {Record<string, unknown> | null}
 */
function parseMetadata(raw) {
  if (!raw) {
    return null;
  }
  if (typeof raw === "object") {
    return /** @type {Record<string, unknown>} */ (raw);
  }
  if (typeof raw === "string") {
    try {
      const parsed = JSON.parse(raw);
      return typeof parsed === "object" && parsed !== null
        ? /** @type {Record<string, unknown>} */ (parsed)
        : null;
    } catch {
      return null;
    }
  }
  return null;
}

/**
 * @param {Record<string, unknown> | null} metadata
 * @returns {string}
 */
function stringifyMetadata(metadata) {
  if (!metadata) {
    return "";
  }
  try {
    return JSON.stringify(metadata);
  } catch {
    return "";
  }
}

/**
 * @param {unknown} raw
 * @param {string} [field]
 * @returns {string}
 */
function readString(raw, field) {
  if (typeof raw === "string" && raw.trim().length > 0) {
    return raw;
  }
  if (field) {
    throw new Error(`${field} missing in response`);
  }
  return "";
}

/**
 * @param {unknown} raw
 * @returns {number}
 */
function readNumber(raw) {
  if (typeof raw === "number" && !Number.isNaN(raw)) {
    return raw;
  }
  if (typeof raw === "string") {
    const parsed = Number(raw);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
  }
  return 0;
}

/**
 * @param {string} raw
 * @returns {string}
 */
function normalizeApiBaseUrl(raw) {
  const trimmed = raw.trim();
  if (!trimmed) {
    return DEFAULT_API_BASE_URL;
  }
  return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}
