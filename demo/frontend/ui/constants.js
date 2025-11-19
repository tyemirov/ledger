// @ts-check

const demoConfig =
  typeof window !== 'undefined' && window.__TAUTH_DEMO_CONFIG
    ? window.__TAUTH_DEMO_CONFIG
    : {};

function determineAuthBaseUrl() {
  const configured =
    demoConfig && typeof demoConfig.authBaseUrl === 'string'
      ? demoConfig.authBaseUrl.trim()
      : '';
  if (configured) {
    return configured;
  }
  if (typeof window === 'undefined') {
    return 'http://localhost:8080';
  }
  return '/auth';
}

const DEFAULT_AUTH_BASE_URL = determineAuthBaseUrl();

export const AUTH_BASE_URL = DEFAULT_AUTH_BASE_URL;
export const API_BASE_URL = '/api';
export const TRANSACTION_COINS = 5;
export const PURCHASE_OPTIONS = Object.freeze([5, 10, 20]);
export const STATUS_MESSAGES = Object.freeze({
  idle: 'Waiting for action…',
  processing: 'Processing…',
  spendSuccess: 'Transaction succeeded.',
  spendInsufficient: 'Insufficient funds. Purchase more coins to continue.',
  spendError: 'Unexpected error while spending coins.',
  purchaseSuccessPrefix: 'Added',
  purchaseError: 'Unable to purchase coins.',
  selectValidAmount: 'Select a valid purchase amount.',
  bootstrapError: 'Bootstrap failed. Check the API logs.',
  loadWalletError: 'Unable to load wallet',
  zeroBalance: 'Balance is zero. Purchase coins to continue.',
  signedOut: 'Signed out',
  authClientMissing: `TAuth auth-client missing from ${AUTH_BASE_URL}/static/auth-client.js`,
});
