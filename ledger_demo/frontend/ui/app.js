// @ts-check
import { createWalletClient } from './wallet-api.js';
import {
  API_BASE_URL,
  AUTH_BASE_URL,
  TRANSACTION_COINS,
  PURCHASE_OPTIONS,
  STATUS_MESSAGES,
} from './constants.js';
import { createAuthFlow } from './auth-flow.js';

const walletClient = createWalletClient({ baseUrl: API_BASE_URL });

/**
 * @typedef {{ baseUrl: string, onAuthenticated: (profile: any) => void, onUnauthenticated: () => void }} AuthClientOptions
 */

/** @typedef {{ balance: { total_coins: number, available_coins: number, total_cents: number, available_cents: number }, entries: Array<EntryPayload> }} WalletResponse */
/** @typedef {{ entry_id: string, type: string, amount_coins: number, amount_cents: number, created_unix_utc: number, metadata: any, reservation_id: string, idempotency_key: string }} EntryPayload */

const elements = {
  header: document.querySelector("#demo-header"),
  walletPanel: document.querySelector("[data-wallet]"),
  transactionsPanel: document.querySelector("[data-transactions]"),
  purchasePanel: document.querySelector("[data-purchase]"),
  historyPanel: document.querySelector("[data-history]"),
  authMessage: document.querySelector("[data-auth-message]"),
  availableCoins: document.querySelector("[data-available-coins]"),
  totalCoins: document.querySelector("[data-total-coins]"),
  availableCents: document.querySelector("[data-available-cents]"),
  totalCents: document.querySelector("[data-total-cents]"),
  transactionButton: document.querySelector("[data-transact]"),
  transactionStatus: document.querySelector("[data-transaction-status]"),
  entryList: document.querySelector("[data-entry-list]"),
  purchaseForm: document.querySelector("[data-purchase-form]"),
  statusBanner: document.querySelector("[data-status-banner]"),
};

const state = {
  wallet: /** @type {WalletResponse | null} */ (null),
  busy: false,
};

function init() {
  if (!elements.header) {
    showBanner("Header element missing; cannot initialize demo.", "error");
    return;
  }
  const clientId = elements.header.getAttribute("site-id");
  if (!clientId) {
    showBanner("Google client ID missing; check ledger_demo/.env.tauth and reload.", "error");
    return;
  }
  if (elements.transactionButton) {
    elements.transactionButton.addEventListener("click", handleTransactionClick);
  }
  if (elements.purchaseForm) {
    elements.purchaseForm.addEventListener("submit", handlePurchaseSubmit);
  }
  document.addEventListener("mpr-ui:auth:unauthenticated", handleUnauthenticatedEvent);

  const authFlow = createAuthFlow({
    walletClient,
    onAuthenticated: handleAuthenticated,
    onSignOut: handleSignOut,
    onMissingClient: () => {
      console.warn('auth-client not loaded');
      showBanner('TAuth auth-client missing from http://localhost:8080/static/auth-client.js', 'error');
    },
  });
  void authFlow.restoreSession();
  authFlow.attachAuthClient();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init, { once: true });
} else {
  init();
}

/**
 * @param {any} profile
 */
async function handleAuthenticated(profile, options = { bootstrap: true }) {
  setAuthState('ready');
  showBanner(`Signed in as ${profile?.display || 'user'}`, 'success');
  try {
    if (options.bootstrap !== false) {
      await walletClient.bootstrap({ source: 'ui' });
    }
    await refreshWallet();
  } catch (error) {
    showBanner(STATUS_MESSAGES.bootstrapError, 'error');
    console.error(error);
  }
}

function handleSignOut() {
  state.wallet = null;
  setAuthState('signed-out');
  showBanner('Signed out', 'success');
}

/** @param {Event} event */
function handleUnauthenticatedEvent(event) {
  event.preventDefault();
  handleSignOut();
}

async function refreshWallet() {
  try {
    const response = await walletClient.getWallet();
    renderWallet(response.wallet);
  } catch (error) {
    console.error(error);
    showBanner(STATUS_MESSAGES.loadWalletError, 'error');
  }
}

async function handleTransactionClick() {
  if (state.busy) {
    return;
  }
  state.busy = true;
  updateTransactionStatus(STATUS_MESSAGES.processing, 'info');
  if (elements.transactionButton) {
    elements.transactionButton.disabled = true;
  }
  try {
    const response = await walletClient.spend({ source: 'ui', coins: TRANSACTION_COINS });
    renderWallet(response.wallet);
    if (response.status === 'insufficient_funds') {
      updateTransactionStatus(STATUS_MESSAGES.spendInsufficient, 'error');
    } else {
      updateTransactionStatus(STATUS_MESSAGES.spendSuccess, 'success');
    }
    checkZeroBalance();
  } catch (error) {
    console.error(error);
    updateTransactionStatus(STATUS_MESSAGES.spendError, 'error');
  } finally {
    state.busy = false;
    if (elements.transactionButton) {
      elements.transactionButton.disabled = false;
    }
  }
}

/** @param {SubmitEvent} event */
async function handlePurchaseSubmit(event) {
  event.preventDefault();
  if (state.busy) {
    return;
  }
  const formData = new FormData(event.currentTarget);
  const selected = Number(formData.get('purchase'));
  if (!PURCHASE_OPTIONS.includes(selected)) {
    updateTransactionStatus(STATUS_MESSAGES.selectValidAmount, 'error');
    return;
  }
  state.busy = true;
  if (elements.transactionButton) {
    elements.transactionButton.disabled = true;
  }
  try {
    const response = await walletClient.purchase(selected, { source: 'ui', coins: selected });
    renderWallet(response.wallet);
    updateTransactionStatus(`${STATUS_MESSAGES.purchaseSuccessPrefix} ${selected} coins.`, 'success');
  } catch (error) {
    console.error(error);
    updateTransactionStatus(STATUS_MESSAGES.purchaseError, 'error');
  } finally {
    state.busy = false;
    if (elements.transactionButton) {
      elements.transactionButton.disabled = false;
    }
  }
}

/**
 * @param {WalletResponse} wallet
 */
function renderWallet(wallet) {
  state.wallet = wallet;
  if (!wallet) {
    return;
  }
  togglePanels(true);
  if (elements.availableCoins) {
    elements.availableCoins.textContent = wallet.balance.available_coins.toString();
  }
  if (elements.totalCoins) {
    elements.totalCoins.textContent = wallet.balance.total_coins.toString();
  }
  if (elements.availableCents) {
    elements.availableCents.textContent = wallet.balance.available_cents.toString();
  }
  if (elements.totalCents) {
    elements.totalCents.textContent = wallet.balance.total_cents.toString();
  }
  renderEntries(wallet.entries);
}

/**
 * @param {EntryPayload[]} entries
 */
function renderEntries(entries) {
  if (!elements.entryList) {
    return;
  }
  elements.entryList.innerHTML = '';
  entries.forEach((entry) => {
    const item = document.createElement('li');
    const description = document.createElement('div');
    description.innerHTML = `<span class="entry-type">${entry.type}</span><br/><small>${new Date(
      entry.created_unix_utc * 1000,
    ).toLocaleString()}</small>`;
    const amount = document.createElement('div');
    amount.classList.add('entry-amount');
    if (entry.amount_coins < 0) {
      amount.classList.add('negative');
    } else {
      amount.classList.add('positive');
    }
    amount.textContent = `${entry.amount_coins > 0 ? '+' : ''}${entry.amount_coins} coins`;
    item.append(description, amount);
    elements.entryList.append(item);
  });
}

function checkZeroBalance() {
  if (state.wallet && state.wallet.balance.available_coins === 0) {
    showBanner(STATUS_MESSAGES.zeroBalance, 'error');
  }
}

/**
 * @param {string} text
 * @param {'success' | 'error' | 'info'} kind
 */
function updateTransactionStatus(text, kind) {
  if (!elements.transactionStatus) {
    return;
  }
  elements.transactionStatus.textContent = text;
  elements.transactionStatus.dataset.statusKind = kind;
}

/**
 * @param {string} message
 * @param {'success' | 'error'} kind
 */
function showBanner(message, kind) {
  if (!elements.statusBanner) {
    return;
  }
  elements.statusBanner.textContent = message;
  elements.statusBanner.dataset.statusKind = kind;
  elements.statusBanner.hidden = false;
  window.clearTimeout(Number(elements.statusBanner.dataset.timeoutId));
  const timeoutId = window.setTimeout(() => {
    if (elements.statusBanner) {
      elements.statusBanner.hidden = true;
    }
  }, 4000);
  elements.statusBanner.dataset.timeoutId = timeoutId.toString();
}

/**
 * @param {'ready' | 'signed-out'} stateValue
 */
function setAuthState(stateValue) {
  document.body.dataset.authState = stateValue;
  togglePanels(stateValue === 'ready');
}

function togglePanels(visible) {
  const targets = [elements.walletPanel, elements.transactionsPanel, elements.purchasePanel, elements.historyPanel];
  targets.forEach((panel) => {
    if (!panel) {
      return;
    }
    panel.hidden = !visible;
  });
  if (elements.authMessage) {
    elements.authMessage.hidden = visible;
  }
}
