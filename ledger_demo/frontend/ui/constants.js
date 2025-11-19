// @ts-check

export const AUTH_BASE_URL = 'http://localhost:8080';
export const API_BASE_URL = 'http://localhost:9090/api';
export const TRANSACTION_COINS = 5;
export const PURCHASE_OPTIONS = Object.freeze([5, 10, 20]);
export const STATUS_MESSAGES = Object.freeze({
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
});
