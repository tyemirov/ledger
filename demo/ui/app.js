// @ts-check
'use strict';

const SPEND_COINS = 5;
const DEFAULT_CONFIG = {
  tauthBaseUrl: 'http://localhost:8080',
  apiBaseUrl: 'http://localhost:9090',
  googleClientId: '991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com',
};

const state = {
  wallet: null,
  busy: false,
  profile: null,
};

const selectors = {
  status: document.getElementById('status-banner'),
  spendButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('spend-button')),
  balanceTotal: document.getElementById('balance-total'),
  balanceAvailable: document.getElementById('balance-available'),
  balancePill: document.getElementById('balance-available-pill'),
  entryList: document.getElementById('entry-list'),
  entryCount: document.getElementById('entry-count'),
  purchaseForm: /** @type {HTMLFormElement|null} */ (document.getElementById('purchase-form')),
  sessionState: document.querySelector('[data-session-state]'),
  sessionEmail: document.querySelector('[data-session-email]'),
};

const config = normalizeConfig(window.DEMO_LEDGER_CONFIG || {});
applyHeaderConfig(config);
wireUI();

run().catch((error) => {
  console.error(error);
  setStatus('Failed to start the demo. See console for details.', 'error');
});

function normalizeConfig(raw) {
  return {
    tauthBaseUrl: sanitizeUrl(raw.tauthBaseUrl) || DEFAULT_CONFIG.tauthBaseUrl,
    apiBaseUrl: sanitizeUrl(raw.apiBaseUrl) || DEFAULT_CONFIG.apiBaseUrl,
    googleClientId: typeof raw.googleClientId === 'string' && raw.googleClientId.trim()
      ? raw.googleClientId.trim()
      : DEFAULT_CONFIG.googleClientId,
  };
}

function sanitizeUrl(value) {
  if (typeof value !== 'string') {
    return '';
  }
  const trimmed = value.trim();
  if (!trimmed) {
    return '';
  }
  return trimmed.replace(/\/+$/, '');
}

async function run() {
  await ensureAuthClientLoaded();
  initializeAuthFlow();
}

async function ensureAuthClientLoaded() {
  if (window.DEMO_LEDGER_AUTH_CLIENT_PROMISE) {
    try {
      await window.DEMO_LEDGER_AUTH_CLIENT_PROMISE;
    } catch (error) {
      setStatus('Could not load auth-client.js from TAuth.', 'error');
      throw error;
    }
  }
}

function initializeAuthFlow() {
  attachAuthEventHandlers();
  if (typeof initAuthClient !== 'function') {
    setStatus('Auth client missing; check TAuth URL.', 'error');
    return;
  }
  initAuthClient({
    baseUrl: config.tauthBaseUrl,
    onAuthenticated(profile) {
      handleAuthenticated(profile || null);
    },
    onUnauthenticated() {
      resetUI();
    },
  });
}

function attachAuthEventHandlers() {
  document.addEventListener('mpr-ui:auth:authenticated', (event) => {
    const profile = event?.detail?.profile || null;
    handleAuthenticated(profile);
  });
  document.addEventListener('mpr-ui:auth:unauthenticated', () => resetUI());
}

function handleAuthenticated(profile) {
  state.profile = profile;
  updateSession(profile);
  setStatus('Signed in. Bootstrapping wallet…', 'info');
  bootstrapWallet()
    .then(loadWallet)
    .catch((error) => {
      console.error(error);
      setStatus('Failed to bootstrap wallet.', 'error');
    });
}

function resetUI() {
  state.wallet = null;
  state.profile = null;
  updateSession(null);
  renderWallet(null);
  renderEntries([]);
  setStatus('Signed out. Sign in to continue.', 'info');
}

function wireUI() {
  if (selectors.spendButton) {
    selectors.spendButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await spendCoins();
    });
  }
  if (selectors.purchaseForm) {
    selectors.purchaseForm.addEventListener('submit', async (event) => {
      event.preventDefault();
      const formData = new FormData(selectors.purchaseForm);
      const coinsRaw = formData.get('purchase');
      const coins = typeof coinsRaw === 'string' ? parseInt(coinsRaw, 10) : NaN;
      if (Number.isNaN(coins)) {
        setStatus('Choose a purchase option.', 'warning');
        return;
      }
      await purchaseCoins(coins);
    });
  }
}

function applyHeaderConfig(currentConfig) {
  const header = document.getElementById('demo-header');
  if (!header) {
    return;
  }
  header.setAttribute('site-id', currentConfig.googleClientId);
  header.setAttribute('base-url', currentConfig.tauthBaseUrl);
  header.setAttribute('login-path', '/auth/google');
  header.setAttribute('logout-path', '/auth/logout');
  header.setAttribute('nonce-path', '/auth/nonce');
}

function setStatus(message, level) {
  if (!selectors.status || !message) {
    return;
  }
  selectors.status.textContent = message;
  selectors.status.dataset.level = level || 'info';
}

function updateSession(profile) {
  if (!selectors.sessionState || !selectors.sessionEmail) {
    return;
  }
  if (!profile) {
    selectors.sessionState.textContent = 'Signed out';
    selectors.sessionEmail.textContent = '';
    return;
  }
  selectors.sessionState.textContent = profile.display || 'Authenticated';
  selectors.sessionEmail.textContent = profile.user_email || '';
}

async function bootstrapWallet() {
  await apiRequest('/api/bootstrap', { method: 'POST' });
}

async function loadWallet() {
  const response = await apiRequest('/api/wallet');
  renderWallet(response.wallet || null);
  renderEntries(response.wallet?.entries || []);
}

async function spendCoins() {
  setBusy(true);
  try {
    const response = await apiRequest('/api/transactions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ metadata: { source: 'demo' } }),
    });
    const wallet = response.wallet || null;
    if (response.status === 'insufficient_funds') {
      setStatus('Not enough coins to spend 5.', 'warning');
    } else {
      setStatus('Spend succeeded.', 'success');
    }
    renderWallet(wallet);
    renderEntries(wallet?.entries || []);
  } catch (error) {
    console.error(error);
    setStatus('Transaction failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function purchaseCoins(coins) {
  setBusy(true);
  try {
    const response = await apiRequest('/api/purchases', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ coins, metadata: { source: 'demo_purchase' } }),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    setStatus(`Purchased ${coins} coins.`, 'success');
  } catch (error) {
    console.error(error);
    setStatus('Purchase failed.', 'error');
  } finally {
    setBusy(false);
  }
}

function setBusy(isBusy) {
  state.busy = isBusy;
  if (selectors.spendButton) {
    selectors.spendButton.disabled = isBusy || !canSpend();
  }
  if (selectors.purchaseForm) {
    selectors.purchaseForm.querySelectorAll('button, input').forEach((element) => {
      element.disabled = isBusy;
    });
  }
}

function canSpend() {
  if (!state.wallet) {
    return false;
  }
  return state.wallet.balance?.available_coins >= SPEND_COINS;
}

function renderWallet(wallet) {
  state.wallet = wallet;
  const totalCoins = wallet?.balance?.total_coins ?? 0;
  const availableCoins = wallet?.balance?.available_coins ?? 0;
  if (selectors.balanceTotal) {
    selectors.balanceTotal.textContent = `${totalCoins} coins`;
  }
  if (selectors.balanceAvailable) {
    selectors.balanceAvailable.textContent = `${availableCoins} coins`;
  }
  if (selectors.balancePill) {
    const canSpendNow = availableCoins >= SPEND_COINS;
    selectors.balancePill.textContent = canSpendNow ? 'Ready to spend' : 'Add funds';
    selectors.balancePill.dataset.state = canSpendNow ? 'ok' : 'empty';
  }
  if (selectors.spendButton) {
    selectors.spendButton.disabled = state.busy || !canSpend();
  }
}

function renderEntries(entries) {
  if (!selectors.entryList || !selectors.entryCount) {
    return;
  }
  selectors.entryList.replaceChildren();
  const safeEntries = Array.isArray(entries) ? entries : [];
  selectors.entryCount.textContent = `${safeEntries.length} items`;
  safeEntries.forEach((entry) => {
    const item = document.createElement('li');
    item.className = 'entry';
    const amountCoins = entry.amount_coins || Math.trunc((entry.amount_cents || 0) / 100);
    const isCredit = entry.amount_cents >= 0;
    const sign = isCredit ? '+' : '–';
    const type = entry.type || 'entry';
    const created = entry.created_unix_utc
      ? new Date(entry.created_unix_utc * 1000).toLocaleString()
      : 'recently';
    item.innerHTML = `
      <div class="entry__left">
        <p class="entry__type">${type}</p>
        <p class="entry__meta">${created}</p>
      </div>
      <div class="entry__amount" data-direction="${isCredit ? 'in' : 'out'}">${sign}${Math.abs(amountCoins)} coins</div>
    `;
    selectors.entryList.appendChild(item);
  });
}

async function apiRequest(path, options) {
  const merged = options || {};
  const response = await fetch(`${config.apiBaseUrl}${path}`, {
    credentials: 'include',
    ...merged,
  });
  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }
  return response.json();
}
