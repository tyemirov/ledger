// @ts-check

import { ensureAuthClient } from "./auth-client-loader.js";
import {
  COPY,
  DEFAULT_API_BASE_URL,
  DEFAULT_TAUTH_BASE_URL,
  LOGIN_PATH,
  LOGOUT_PATH,
  NONCE_PATH,
  PURCHASE_PRESETS,
  SELECTORS,
  TRANSACTION_COINS,
} from "./constants.js";
import { loadDemoConfig } from "./config.js";
import {
  createWalletApiClient,
  HttpError,
} from "./wallet-api-client.js";

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
 * @typedef {import("./config.js").DemoConfig} DemoConfig
 * @typedef {object} NormalizedProfile
 * @property {string} userId
 * @property {string} display
 * @property {string} email
 * @property {string | null} avatarUrl
 * @property {string[]} roles
 * @property {string | null} expiresDisplay
 */

const numberFormatter = new Intl.NumberFormat("en-US");
const HEADER_MOUNT_ID = "header-mount";
const HEADER_NAV_LINKS = JSON.stringify([
  { label: "Ledger README", href: "https://github.com/tyemirov/ledger#readme" },
  { label: "Demo guide", href: "https://github.com/tyemirov/ledger/blob/master/demo/docs/demo.md" },
  { label: "TAuth usage", href: "https://github.com/tyemirov/ledger/blob/master/docs/TAuth/usage.md" },
]);

/**
 * Alpine factory powering the ledger demo UI.
 * @returns {ReturnType<typeof createLedgerDemo>}
 */
function LedgerDemo() {
  return createLedgerDemo();
}

/**
 * @returns {{
 *  copy: typeof COPY,
 *  config: DemoConfig | null,
 *  apiClient: ReturnType<typeof createWalletApiClient> | null,
 *  authState: "loading" | "authenticated" | "unauthenticated" | "error",
 *  statusMessage: string,
 *  errorMessage: string,
 *  profile: NormalizedProfile | null,
 *  wallet: WalletSnapshot | null,
 *  banner: { tone: "success" | "info" | "error", title: string, detail: string } | null,
 *  zeroBalanceNotice: boolean,
 *  isInitializing: boolean,
 *  isSpendPending: boolean,
 *  isPurchasePending: boolean,
 *  purchaseCoins: number,
 *  purchaseOptions: number[],
 *  init: () => Promise<void>,
 *  bootstrapAuthClient: () => void,
 *  ensureWalletReady: () => Promise<void>,
 *  refreshWallet: () => Promise<void>,
 *  spendWalletCoins: () => Promise<void>,
 *  purchaseWalletCoins: () => Promise<void>,
 *  selectPurchaseAmount: (amount: number) => void,
 *  canSpend: () => boolean,
 *  canPurchase: () => boolean,
 *  formattedCoins: (value: number) => string,
 *  formattedTimestamp: (entry: LedgerEntrySnapshot) => string,
 *  formattedMetadata: (entry: LedgerEntrySnapshot) => string,
 *  statusTitle: () => string,
 *  statusDetail: () => string,
 *  signOut: () => Promise<void>,
 *  handleUnauthenticated: () => void,
 * }}
 */
function createLedgerDemo() {
  return {
    copy: COPY,
    config: null,
    apiClient: null,
    authState: "loading",
    statusMessage: COPY.loadingConfig,
    errorMessage: "",
    profile: null,
    wallet: null,
    banner: null,
    zeroBalanceNotice: false,
    isInitializing: false,
    isSpendPending: false,
    isPurchasePending: false,
    purchaseCoins: PURCHASE_PRESETS[0],
    purchaseOptions: PURCHASE_PRESETS,
    async init() {
      const tauthHintElement =
        document.querySelector(SELECTORS.header) ||
        document.getElementById(HEADER_MOUNT_ID);
      const baseHint =
        (tauthHintElement &&
          tauthHintElement.dataset &&
          tauthHintElement.dataset.tauthBaseUrl) ||
        DEFAULT_TAUTH_BASE_URL;
      try {
        this.config = await loadDemoConfig(baseHint);
        applyHeaderConfig(this.config);
      } catch (error) {
        this.authState = "error";
        this.errorMessage =
          error instanceof Error ? error.message : COPY.configError;
        return;
      }
      const apiBase = readApiBaseUrl();
      this.apiClient = createWalletApiClient(apiBase);
      try {
        await ensureAuthClient(this.config.authClientUrl);
      } catch (error) {
        this.authState = "error";
        this.errorMessage =
          error instanceof Error ? error.message : COPY.authClientMissing;
        return;
      }
      this.bootstrapAuthClient();
    },
    bootstrapAuthClient() {
      if (!this.config || typeof window.initAuthClient !== "function") {
        this.authState = "error";
        this.errorMessage = COPY.authClientMissing;
        return;
      }
      window.initAuthClient({
        baseUrl: this.config.baseUrl,
        onAuthenticated: (profile) => {
          this.profile = normalizeProfile(profile);
          this.authState = "authenticated";
          this.statusMessage = COPY.walletReadyTitle;
          this.clearBanner();
          this.ensureWalletReady();
        },
        onUnauthenticated: () => {
          this.handleUnauthenticated();
        },
      });
      const cachedProfile =
        typeof window.getCurrentUser === "function"
          ? window.getCurrentUser()
          : null;
      if (cachedProfile) {
        this.profile = normalizeProfile(cachedProfile);
        this.authState = "authenticated";
        this.ensureWalletReady();
      } else if (this.authState !== "error") {
        this.authState = "unauthenticated";
        this.statusMessage = COPY.signedOutDetail;
      }
    },
    async ensureWalletReady() {
      if (!this.apiClient || this.isInitializing) {
        return;
      }
      this.isInitializing = true;
      this.statusMessage = COPY.walletLoading;
      try {
        const snapshot = await this.apiClient.bootstrapWallet();
        this.updateWallet(snapshot);
        this.setBanner(
          "success",
          COPY.walletReadyTitle,
          COPY.walletReadyDetail
        );
      } catch (error) {
        if (error instanceof HttpError && error.status === 401) {
          this.handleUnauthenticated();
          return;
        }
        this.errorMessage = formatError(error);
        this.setBanner(
          "error",
          COPY.walletErrorTitle,
          COPY.walletErrorDetail
        );
      } finally {
        this.isInitializing = false;
      }
    },
    async refreshWallet() {
      if (!this.apiClient) {
        return;
      }
      try {
        const snapshot = await this.apiClient.fetchWallet();
        this.updateWallet(snapshot);
      } catch (error) {
        if (error instanceof HttpError && error.status === 401) {
          this.handleUnauthenticated();
          return;
        }
        this.setBanner("error", COPY.apiErrorTitle, formatError(error));
      }
    },
    async spendWalletCoins() {
      if (!this.apiClient || this.isSpendPending) {
        return;
      }
      this.isSpendPending = true;
      try {
        const result = await this.apiClient.spendCoins();
        this.applyTransactionResult(result);
      } catch (error) {
        if (error instanceof HttpError && error.status === 401) {
          this.handleUnauthenticated();
        } else {
          this.setBanner("error", COPY.apiErrorTitle, formatError(error));
        }
      } finally {
        this.isSpendPending = false;
      }
    },
    async purchaseWalletCoins() {
      if (
        !this.apiClient ||
        this.isPurchasePending ||
        this.purchaseCoins < TRANSACTION_COINS
      ) {
        return;
      }
      this.isPurchasePending = true;
      const coinsToBuy = this.purchaseCoins;
      try {
        const snapshot = await this.apiClient.purchaseCoins(coinsToBuy);
        this.updateWallet(snapshot);
        this.setBanner(
          "success",
          COPY.purchaseSuccessTitle,
          COPY.purchaseSuccessDetail(coinsToBuy)
        );
      } catch (error) {
        if (error instanceof HttpError && error.status === 401) {
          this.handleUnauthenticated();
        } else {
          this.setBanner("error", COPY.apiErrorTitle, formatError(error));
        }
      } finally {
        this.isPurchasePending = false;
      }
    },
    applyTransactionResult(result) {
      this.updateWallet(result.wallet);
      if (result.status === "insufficient_funds") {
        this.setBanner(
          "error",
          COPY.insufficientTitle,
          COPY.insufficientDetail
        );
      } else {
        this.setBanner(
          "success",
          COPY.spendSuccessTitle,
          COPY.spendSuccessDetail
        );
      }
    },
    selectPurchaseAmount(amount) {
      this.purchaseCoins = amount;
    },
    canSpend() {
      return (
        this.authState === "authenticated" &&
        !this.isInitializing &&
        !this.isSpendPending &&
        !!this.wallet &&
        this.wallet.balance.availableCoins >= TRANSACTION_COINS
      );
    },
    canPurchase() {
      return (
        this.authState === "authenticated" &&
        !this.isInitializing &&
        !this.isPurchasePending &&
        this.purchaseCoins >= TRANSACTION_COINS &&
        this.purchaseCoins % TRANSACTION_COINS === 0
      );
    },
    updateWallet(snapshot) {
      this.wallet = snapshot;
      this.zeroBalanceNotice =
        snapshot.balance.availableCoins <= 0 &&
        snapshot.balance.totalCoins >= 0;
    },
    setBanner(tone, title, detail) {
      this.banner = { tone, title, detail };
    },
    clearBanner() {
      this.banner = null;
    },
    formattedCoins(value) {
      return `${numberFormatter.format(value)} coins`;
    },
    formattedTimestamp(entry) {
      return formatTimestamp(entry.createdAtUnix);
    },
    formattedMetadata(entry) {
      return entry.metadataText || "—";
    },
    statusTitle() {
      if (this.errorMessage) {
        return COPY.authUnavailableTitle;
      }
      if (this.authState === "authenticated") {
        return COPY.walletReadyTitle;
      }
      if (this.authState === "loading") {
        return COPY.connectingTitle;
      }
      return "Signed out";
    },
    statusDetail() {
      if (this.errorMessage) {
        return this.errorMessage;
      }
      if (this.authState === "authenticated") {
        return COPY.signedInDetail;
      }
      if (this.authState === "loading") {
        return this.statusMessage;
      }
      return COPY.signedOutDetail;
    },
    async signOut() {
      if (typeof window.logout !== "function") {
        this.setBanner(
          "error",
          COPY.authUnavailableTitle,
          COPY.authClientMissing
        );
        return;
      }
      try {
        await window.logout();
        this.handleUnauthenticated();
      } catch (error) {
        this.setBanner("error", COPY.apiErrorTitle, formatError(error));
      }
    },
    handleUnauthenticated() {
      this.profile = null;
      this.wallet = null;
      this.zeroBalanceNotice = false;
      this.authState = "unauthenticated";
      this.statusMessage = COPY.signedOutDetail;
      this.clearBanner();
    },
  };
}

/**
 * @param {HTMLElement | null} header
 * @param {DemoConfig} config
 * @returns {void}
 */
function applyHeaderConfig(config) {
  const header = ensureHeaderElement();
  header.setAttribute("site-id", config.siteId);
  header.setAttribute("base-url", config.baseUrl);
  header.setAttribute("login-path", config.loginPath);
  header.setAttribute("logout-path", config.logoutPath);
  header.setAttribute("nonce-path", config.noncePath);
  header.dataset.tauthBaseUrl = config.baseUrl;
}

/**
 * @returns {string}
 */
function readApiBaseUrl() {
  const datasetBase =
    (document.body && document.body.dataset
      ? document.body.dataset.apiBaseUrl
      : "") || "";
  const trimmed = datasetBase.trim();
  if (!trimmed) {
    return DEFAULT_API_BASE_URL;
  }
  return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}

/**
 * @returns {HTMLElement}
 */
function ensureHeaderElement() {
  let header = document.querySelector(SELECTORS.header);
  if (header) {
    return header;
  }
  header = document.createElement("mpr-header");
  header.id = "auth-header";
  header.setAttribute("brand-label", "Marco Polo Research Lab");
  header.setAttribute("brand-href", "https://mprlab.com/");
  header.setAttribute("nav-links", HEADER_NAV_LINKS);
  header.setAttribute("settings", "true");
  header.setAttribute("settings-label", "Settings");
  header.setAttribute("login-path", LOGIN_PATH);
  header.setAttribute("logout-path", LOGOUT_PATH);
  header.setAttribute("nonce-path", NONCE_PATH);
  const mount = document.getElementById(HEADER_MOUNT_ID);
  if (mount && mount.dataset && mount.dataset.tauthBaseUrl) {
    header.dataset.tauthBaseUrl = mount.dataset.tauthBaseUrl;
  }
  if (mount) {
    mount.replaceWith(header);
  } else {
    document.body.prepend(header);
  }
  return header;
}

/**
 * @param {Record<string, unknown> | null | undefined} raw
 * @returns {NormalizedProfile | null}
 */
function normalizeProfile(raw) {
  if (!raw || typeof raw !== "object") {
    return null;
  }
  const source = /** @type {Record<string, unknown>} */ (raw);
  const rolesValue = source.roles;
  const roles =
    Array.isArray(rolesValue) && rolesValue.length > 0
      ? rolesValue.map((role) => String(role))
      : [];
  const expiresValue =
    typeof source.expires === "string"
      ? source.expires
      : typeof source.expires === "number"
        ? source.expires
        : typeof source.expires_at === "string"
          ? source.expires_at
          : typeof source.expires_at === "number"
            ? source.expires_at
            : null;
  const expiresDisplay =
    expiresValue === null ? null : formatExpires(expiresValue);
  return {
    userId: typeof source.user_id === "string" ? source.user_id : "unknown",
    display:
      typeof source.display === "string"
        ? source.display
        : COPY.profileFallback,
    email:
      typeof source.user_email === "string"
        ? source.user_email
        : typeof source.email === "string"
          ? source.email
          : COPY.emailHidden,
    avatarUrl:
      typeof source.avatar_url === "string" ? source.avatar_url : null,
    roles,
    expiresDisplay,
  };
}

/**
 * @param {string | number} value
 * @returns {string}
 */
function formatExpires(value) {
  if (typeof value === "number") {
    return new Date(value * 1000).toLocaleString();
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return COPY.sessionExpiryUnknown;
  }
  return parsed.toLocaleString();
}

/**
 * @param {number} unix
 * @returns {string}
 */
function formatTimestamp(unix) {
  if (Number.isNaN(unix)) {
    return "unknown";
  }
  return new Date(unix * 1000).toLocaleString();
}

/**
 * @param {unknown} error
 * @returns {string}
 */
function formatError(error) {
  if (error instanceof HttpError) {
    return `${error.message} (HTTP ${error.status})`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  if (typeof error === "string") {
    return error;
  }
  return COPY.apiErrorDetail;
}

window.LedgerDemo = LedgerDemo;

document.addEventListener("alpine:init", () => {
  if (window.Alpine && typeof window.Alpine.data === "function") {
    window.Alpine.data("LedgerDemo", LedgerDemo);
  }
});
