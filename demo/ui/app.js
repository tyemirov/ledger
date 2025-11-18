// @ts-check

const API_BASE_URL = "http://localhost:9090/api";
const AUTH_BASE_URL = "http://localhost:8080";
const TRANSACTION_COINS = 5;
const PURCHASE_OPTIONS = [5, 10, 20];

/**
 * @typedef {{ baseUrl: string, onAuthenticated: (profile: any) => void, onUnauthenticated: () => void }} AuthClientOptions
 */

/** @typedef {{ balance: { total_coins: number, available_coins: number, total_cents: number, available_cents: number }, entries: Array<EntryPayload> }} WalletResponse */
/** @typedef {{ entry_id: string, type: string, amount_coins: number, amount_cents: number, created_unix_utc: number, metadata: any, reservation_id: string, idempotency_key: string }} EntryPayload */

/** @type {Window & typeof globalThis & { initAuthClient?: (options: AuthClientOptions) => void }} */
const runtimeWindow = window;

const elements = {
  walletPanel: document.querySelector('[data-wallet]'),
  transactionsPanel: document.querySelector('[data-transactions]'),
  purchasePanel: document.querySelector('[data-purchase]'),
  historyPanel: document.querySelector('[data-history]'),
  authMessage: document.querySelector('[data-auth-message]'),
  availableCoins: document.querySelector('[data-available-coins]'),
  totalCoins: document.querySelector('[data-total-coins]'),
  availableCents: document.querySelector('[data-available-cents]'),
  totalCents: document.querySelector('[data-total-cents]'),
  transactionButton: document.querySelector('[data-transact]'),
  transactionStatus: document.querySelector('[data-transaction-status]'),
  entryList: document.querySelector('[data-entry-list]'),
  purchaseForm: document.querySelector('[data-purchase-form]'),
  statusBanner: document.querySelector('[data-status-banner]'),
};

const state = {
  wallet: /** @type {WalletResponse | null} */ (null),
  busy: false,
};

function init() {
  if (elements.transactionButton) {
    elements.transactionButton.addEventListener('click', handleTransactionClick);
  }
  if (elements.purchaseForm) {
    elements.purchaseForm.addEventListener('submit', handlePurchaseSubmit);
  }
  document.addEventListener('mpr-ui:auth:unauthenticated', handleUnauthenticatedEvent);

  if (typeof runtimeWindow.initAuthClient === "function") {
    runtimeWindow.initAuthClient({
      baseUrl: AUTH_BASE_URL,
      onAuthenticated: handleAuthenticated,
      onUnauthenticated: handleSignOut,
    });
  } else {
    console.warn('auth-client not loaded');
  }
}

document.addEventListener('DOMContentLoaded', init);

/**
 * @param {any} profile
 */
async function handleAuthenticated(profile) {
  setAuthState('ready');
  showBanner(`Signed in as ${profile?.display || 'user'}`, 'success');
  try {
    await apiFetch('/bootstrap', { method: 'POST' });
    await refreshWallet();
  } catch (error) {
    showBanner('Bootstrap failed. Check the API logs.', 'error');
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
    const response = await apiFetch('/wallet');
    renderWallet(response.wallet);
  } catch (error) {
    console.error(error);
    showBanner('Unable to load wallet', 'error');
  }
}

async function handleTransactionClick() {
  if (state.busy) {
    return;
  }
  state.busy = true;
  updateTransactionStatus('Processing…', 'info');
  if (elements.transactionButton) {
    elements.transactionButton.disabled = true;
  }
  try {
    const response = await apiFetch('/transactions', {
      method: 'POST',
      body: JSON.stringify({ metadata: { source: 'ui', coins: TRANSACTION_COINS } }),
    });
    renderWallet(response.wallet);
    if (response.status === 'insufficient_funds') {
      updateTransactionStatus('Insufficient funds. Purchase more coins to continue.', 'error');
    } else {
      updateTransactionStatus('Transaction succeeded.', 'success');
    }
    checkZeroBalance();
  } catch (error) {
    console.error(error);
    updateTransactionStatus('Unexpected error while spending coins.', 'error');
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
    updateTransactionStatus('Select a valid purchase amount.', 'error');
    return;
  }
  state.busy = true;
  if (elements.transactionButton) {
    elements.transactionButton.disabled = true;
  }
  try {
    const response = await apiFetch('/purchases', {
      method: 'POST',
      body: JSON.stringify({ coins: selected, metadata: { source: 'ui', coins: selected } }),
    });
    renderWallet(response.wallet);
    updateTransactionStatus(`Added ${selected} coins.`, 'success');
  } catch (error) {
    console.error(error);
    updateTransactionStatus('Unable to purchase coins.', 'error');
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
    showBanner('Balance is zero. Purchase coins to continue.', 'error');
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

/**
 * @param {string} path
 * @param {RequestInit} [options]
 */
async function apiFetch(path, options = {}) {
  const response = await fetch(`${API_BASE_URL}${path}`, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
    ...options,
  });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`);
  }
  const contentType = response.headers.get('content-type') || '';
  if (!contentType.includes('application/json')) {
    return {};
  }
  return response.json();
}
