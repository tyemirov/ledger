// @ts-check

export const DEFAULT_TAUTH_BASE_URL = "http://localhost:8080";
export const DEFAULT_API_BASE_URL = "http://localhost:9090";
export const CONFIG_SCRIPT_PATH = "/demo/config.js";
export const AUTH_CLIENT_PATH = "/static/auth-client.js";
export const LOGIN_PATH = "/auth/google";
export const LOGOUT_PATH = "/auth/logout";
export const NONCE_PATH = "/auth/nonce";

export const TRANSACTION_COINS = 5;
export const BOOTSTRAP_COINS = 20;

export const PURCHASE_PRESETS = Object.freeze([5, 10, 20]);

export const SELECTORS = Object.freeze({
  header: "mpr-header",
});

export const COPY = Object.freeze({
  loadingConfig: "Preparing authentication…",
  configError:
    "Unable to load TAuth configuration. Ensure TAuth is running, exposes /demo/config.js, and allows http://localhost:8000 in CORS.",
  authClientMissing:
    "auth-client.js is unavailable; verify TAuth is running and reachable.",
  connectingTitle: "Connecting",
  authUnavailableTitle: "Authentication unavailable",
  signedOutDetail: "Use the Google button in the header to sign in.",
  signedInDetail: "Bootstrap starts automatically once you sign in.",
  walletLoading: "Bootstrapping wallet…",
  walletReadyTitle: "Wallet ready",
  walletReadyDetail: "You can now spend 5 coins per click, or buy more coins.",
  walletErrorTitle: "Wallet unavailable",
  walletErrorDetail:
    "Ledger calls failed. Check demoapi logs and ensure ledgerd is reachable.",
  spendButtonLabel: "Spend 5 coins",
  purchaseButtonLabel: "Buy coins",
  purchaseLabel: "Coins to buy",
  purchaseHint: "Coins are available in 5-coin increments.",
  ledgerEmpty: "Ledger history will appear after your first transaction.",
  zeroBalanceTitle: "Balance is zero",
  zeroBalanceDetail: "Spend attempts will fail until you buy more coins.",
  insufficientTitle: "Insufficient funds",
  insufficientDetail:
    "Add coins before attempting another 5-coin transaction.",
  spendSuccessTitle: "Transaction successful",
  spendSuccessDetail: "Deducted 5 coins from your wallet.",
  purchaseSuccessTitle: "Coins purchased",
  purchaseSuccessDetail: (coins) => `Added ${coins} coins to your wallet.`,
  apiErrorTitle: "Request failed",
  apiErrorDetail:
    "The demo API did not respond successfully. Check your network tab and docker logs.",
  profileFallback: "Unknown user",
  emailHidden: "Hidden",
  sessionExpiryUnknown: "session expiry unknown",
});
