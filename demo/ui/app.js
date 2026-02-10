// @ts-check
'use strict';

const SPEND_COINS = 5;
const USER_MENU_ACCOUNT_ACTION = 'account-details';
const USER_MENU_ITEMS = JSON.stringify([
  {
    label: 'Account details',
    action: USER_MENU_ACCOUNT_ACTION,
  },
]);

const FALLBACK_ORIGIN =
  typeof window === 'object' &&
  window.location &&
  typeof window.location.origin === 'string' &&
  window.location.origin.trim() &&
  window.location.origin !== 'null'
    ? window.location.origin.trim().replace(/\/+$/, '')
    : 'https://localhost:4443';

const DEFAULT_CONFIG = {
  tauthBaseUrl: FALLBACK_ORIGIN,
  apiBaseUrl: FALLBACK_ORIGIN,
  googleClientId: '991677581607-r0dj8q6irjagipali0jpca7nfp8sfj9r.apps.googleusercontent.com',
};

const state = {
  wallet: null,
  busy: false,
  profile: null,
  reservation: null,
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
  reservationForm: /** @type {HTMLFormElement|null} */ (document.getElementById('reservation-form')),
  reserveButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('reserve-button')),
  captureButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('capture-button')),
  releaseButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('release-button')),
  reservationRefreshButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('reservation-refresh-button')),
  reservationStatusPill: document.getElementById('reservation-status-pill'),
  reservationID: document.getElementById('reservation-id'),
  reservationMeta: document.getElementById('reservation-meta'),
  batchSpendButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('batch-spend-button')),
  batchSpendAtomicButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('batch-spend-atomic-button')),
  batchRefundButton: /** @type {HTMLButtonElement|null} */ (document.getElementById('batch-refund-button')),
  accountModal: document.getElementById('account-modal'),
  accountModalDialog: /** @type {HTMLElement|null} */ (document.querySelector('#account-modal [data-mpr-modal="dialog"]')),
  accountModalBackdrop: /** @type {HTMLElement|null} */ (document.querySelector('#account-modal [data-account-modal="backdrop"]')),
  accountModalClose: /** @type {HTMLButtonElement|null} */ (document.querySelector('#account-modal [data-account-modal="close"]')),
  accountAvatar: /** @type {HTMLImageElement|null} */ (document.getElementById('account-avatar')),
  accountName: document.getElementById('account-name'),
  accountEmail: document.getElementById('account-email'),
};

const config = normalizeConfig(window.DEMO_LEDGER_CONFIG || {});
applyHeaderConfig(config);
attachAuthEventHandlers();
attachAccountMenuHandlers();
wireUI();
renderWallet(null);
renderReservation(null);
renderEntries([]);
setBusy(false);
setStatus('Sign in to continue.', 'info');
syncUserMenuItems();
bootstrapExistingSession();

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

function attachAuthEventHandlers() {
  document.addEventListener('mpr-ui:auth:authenticated', (event) => {
    const profile = event?.detail?.profile || null;
    handleAuthenticated(profile);
  });
  document.addEventListener('mpr-ui:auth:unauthenticated', () => resetUI());
  document.addEventListener('mpr-ui:auth:error', () => {
    setStatus('Authentication failed. Try again.', 'error');
  });
}

function attachAccountMenuHandlers() {
  document.addEventListener('mpr-user:menu-item', (event) => {
    if (event?.detail?.action !== USER_MENU_ACCOUNT_ACTION) {
      return;
    }
    openAccountModal();
  });
  if (selectors.accountModalBackdrop) {
    selectors.accountModalBackdrop.addEventListener('click', () => closeAccountModal());
  }
  if (selectors.accountModalClose) {
    selectors.accountModalClose.addEventListener('click', () => closeAccountModal());
  }
  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') {
      return;
    }
    if (!isAccountModalOpen()) {
      return;
    }
    closeAccountModal();
  });
}

function handleAuthenticated(profile) {
  state.profile = profile;
  renderAccountDetails(profile);
  syncUserMenuItems();
  setStatus('Signed in. Bootstrapping wallet…', 'info');
  setBusy(true);
  bootstrapWallet()
    .then(loadWallet)
    .then(() => setBusy(false))
    .catch((error) => {
      console.error(error);
      setStatus('Failed to bootstrap wallet.', 'error');
      setBusy(false);
    });
}

function resetUI() {
  state.wallet = null;
  state.profile = null;
  state.reservation = null;
  renderAccountDetails(null);
  closeAccountModal();
  renderWallet(null);
  renderReservation(null);
  renderEntries([]);
  setBusy(false);
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
  if (selectors.reservationForm) {
    selectors.reservationForm.addEventListener('submit', async (event) => {
      event.preventDefault();
      const formData = new FormData(selectors.reservationForm);
      const holdRaw = formData.get('hold');
      const ttlRaw = formData.get('ttl');
      const holdCoins = typeof holdRaw === 'string' ? parseInt(holdRaw, 10) : NaN;
      const ttlSeconds = typeof ttlRaw === 'string' ? parseInt(ttlRaw, 10) : NaN;
      if (Number.isNaN(holdCoins) || holdCoins <= 0) {
        setStatus('Choose a hold amount.', 'warning');
        return;
      }
      if (Number.isNaN(ttlSeconds) || ttlSeconds < 0) {
        setStatus('Choose a valid TTL.', 'warning');
        return;
      }
      await reserveHold(holdCoins, ttlSeconds);
    });
  }
  if (selectors.captureButton) {
    selectors.captureButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await captureHold();
    });
  }
  if (selectors.releaseButton) {
    selectors.releaseButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await releaseHold();
    });
  }
  if (selectors.reservationRefreshButton) {
    selectors.reservationRefreshButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await refreshReservation();
    });
  }
  if (selectors.batchSpendButton) {
    selectors.batchSpendButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await batchSpend(10, SPEND_COINS, false);
    });
  }
  if (selectors.batchSpendAtomicButton) {
    selectors.batchSpendAtomicButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await batchSpend(10, SPEND_COINS, true);
    });
  }
  if (selectors.batchRefundButton) {
    selectors.batchRefundButton.addEventListener('click', async (event) => {
      event.preventDefault();
      await batchRefundLastSpends(3, false);
    });
  }
}

function applyHeaderConfig(currentConfig) {
  const header = document.getElementById('demo-header');
  if (!header) {
    return;
  }
  header.setAttribute('google-site-id', currentConfig.googleClientId);
  header.setAttribute('tauth-url', currentConfig.tauthBaseUrl);
  header.setAttribute('tauth-login-path', '/auth/google');
  header.setAttribute('tauth-logout-path', '/auth/logout');
  header.setAttribute('tauth-nonce-path', '/auth/nonce');
}

function setStatus(message, level) {
  if (!selectors.status || !message) {
    return;
  }
  selectors.status.textContent = message;
  selectors.status.dataset.level = level || 'info';
}

function renderAccountDetails(profile) {
  if (selectors.accountAvatar) {
    const avatarUrl = profile?.avatar_url || '';
    if (avatarUrl) {
      selectors.accountAvatar.src = avatarUrl;
    } else {
      selectors.accountAvatar.removeAttribute('src');
    }
    selectors.accountAvatar.alt = profile?.display ? `${profile.display} avatar` : '';
  }
  if (selectors.accountName) {
    selectors.accountName.textContent = profile?.display || 'Account';
  }
  if (selectors.accountEmail) {
    selectors.accountEmail.textContent = profile?.user_email || '';
  }
}

function resolveAccountProfile() {
  if (state.profile) {
    return state.profile;
  }
  const userMenu = document.querySelector('#demo-header mpr-user');
  if (!userMenu) {
    return null;
  }
  const resolved = {
    user_id: userMenu.getAttribute('data-user-id') || '',
    user_email: userMenu.getAttribute('data-user-email') || '',
    display: userMenu.getAttribute('data-user-display') || '',
    avatar_url: userMenu.getAttribute('data-user-avatar-url') || '',
  };
  if (!resolved.user_email && !resolved.display && !resolved.avatar_url) {
    return null;
  }
  return resolved;
}

function isAccountModalOpen() {
  return selectors.accountModal?.getAttribute('data-mpr-modal-open') === 'true';
}

function openAccountModal() {
  if (!selectors.accountModal) {
    return;
  }
  renderAccountDetails(resolveAccountProfile());
  selectors.accountModal.setAttribute('data-mpr-modal-open', 'true');
  selectors.accountModal.setAttribute('aria-hidden', 'false');
  if (selectors.accountModalDialog && typeof selectors.accountModalDialog.focus === 'function') {
    selectors.accountModalDialog.focus();
  }
}

function closeAccountModal() {
  if (!selectors.accountModal) {
    return;
  }
  selectors.accountModal.setAttribute('data-mpr-modal-open', 'false');
  selectors.accountModal.setAttribute('aria-hidden', 'true');
}

function syncUserMenuItems() {
  const header = document.getElementById('demo-header');
  if (!header || typeof header.querySelector !== 'function') {
    return;
  }

  const apply = () => {
    const userMenu = header.querySelector('mpr-user');
    if (!userMenu || typeof userMenu.setAttribute !== 'function') {
      return false;
    }
    userMenu.setAttribute('menu-items', USER_MENU_ITEMS);
    return true;
  };

  if (apply()) {
    return;
  }

  const observer = new MutationObserver(() => {
    if (apply()) {
      observer.disconnect();
    }
  });
  observer.observe(header, { childList: true, subtree: true });
}

function bootstrapExistingSession() {
  if (typeof window !== 'object' || typeof window.getCurrentUser !== 'function') {
    return;
  }

  Promise.resolve()
    .then(() => window.getCurrentUser())
    .then((profile) => {
      if (!profile) {
        return;
      }
      handleAuthenticated(profile);
    })
    .catch(() => {});
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

async function reserveHold(coins, ttlSeconds) {
  setBusy(true);
  try {
    const response = await apiRequest('/api/reservations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ coins, ttl_seconds: ttlSeconds, metadata: { source: 'demo' } }),
    });
    const wallet = response.wallet || null;
    if (response.status === 'insufficient_funds') {
      setStatus(`Not enough coins to reserve ${coins}.`, 'warning');
      renderWallet(wallet);
      renderEntries(wallet?.entries || []);
      return;
    }
    renderWallet(wallet);
    renderEntries(wallet?.entries || []);
    renderReservation(response.reservation || null);
    setStatus('Hold reserved.', 'success');
  } catch (error) {
    console.error(error);
    setStatus('Reserve failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function captureHold() {
  if (!state.reservation || !state.reservation.reservation_id) {
    setStatus('No active reservation to capture.', 'warning');
    return;
  }
  setBusy(true);
  try {
    const reservationID = state.reservation.reservation_id;
    const response = await apiRequest(`/api/reservations/${encodeURIComponent(reservationID)}/capture`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    renderReservation(response.reservation || null);
    setStatus('Hold captured.', 'success');
  } catch (error) {
    console.error(error);
    setStatus('Capture failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function releaseHold() {
  if (!state.reservation || !state.reservation.reservation_id) {
    setStatus('No active reservation to release.', 'warning');
    return;
  }
  setBusy(true);
  try {
    const reservationID = state.reservation.reservation_id;
    const response = await apiRequest(`/api/reservations/${encodeURIComponent(reservationID)}/release`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    renderReservation(response.reservation || null);
    setStatus('Hold released.', 'success');
  } catch (error) {
    console.error(error);
    setStatus('Release failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function refreshReservation() {
  if (!state.reservation || !state.reservation.reservation_id) {
    setStatus('No reservation to refresh.', 'warning');
    return;
  }
  setBusy(true);
  try {
    const reservationID = state.reservation.reservation_id;
    const response = await apiRequest(`/api/reservations/${encodeURIComponent(reservationID)}`);
    renderReservation(response.reservation || null);
    setStatus('Reservation refreshed.', 'info');
  } catch (error) {
    console.error(error);
    setStatus('Refresh failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function refundEntry(entry) {
  if (!entry || !entry.entry_id || !entry.amount_cents) {
    setStatus('Refund unavailable for this entry.', 'warning');
    return;
  }
  const amountCents = Math.abs(entry.amount_cents);
  if (!amountCents) {
    setStatus('Refund amount is zero.', 'warning');
    return;
  }
  setBusy(true);
  try {
    const response = await apiRequest('/api/refunds', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        original_entry_id: entry.entry_id,
        amount_cents: amountCents,
        metadata: { source: 'demo_refund' },
      }),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    if (response.status === 'duplicate') {
      setStatus('Refund already applied (idempotent).', 'info');
      return;
    }
    setStatus('Refund applied.', 'success');
  } catch (error) {
    console.error(error);
    setStatus('Refund failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function batchSpend(count, coins, atomic) {
  setBusy(true);
  try {
    const response = await apiRequest('/api/batch/spend', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ count, coins, atomic: Boolean(atomic) }),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    const ok = response.batch?.ok ?? 0;
    const duplicate = response.batch?.duplicate ?? 0;
    const failed = response.batch?.failed ?? 0;
    setStatus(`Batch spend: ok=${ok} dup=${duplicate} failed=${failed}`, failed ? 'warning' : 'success');
  } catch (error) {
    console.error(error);
    setStatus('Batch spend failed.', 'error');
  } finally {
    setBusy(false);
  }
}

async function batchRefundLastSpends(count, atomic) {
  const entries = Array.isArray(state.wallet?.entries) ? state.wallet.entries : [];
  const spends = entries.filter((entry) => entry?.type === 'spend' && entry.amount_cents < 0 && entry.entry_id);
  const targets = spends.slice(0, count).map((entry) => ({
    original_entry_id: entry.entry_id,
    amount_cents: Math.abs(entry.amount_cents),
  }));
  if (!targets.length) {
    setStatus('No spend entries available to refund.', 'warning');
    return;
  }
  setBusy(true);
  try {
    const response = await apiRequest('/api/batch/refund', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ items: targets, atomic: Boolean(atomic) }),
    });
    renderWallet(response.wallet || null);
    renderEntries(response.wallet?.entries || []);
    const ok = response.batch?.ok ?? 0;
    const duplicate = response.batch?.duplicate ?? 0;
    const failed = response.batch?.failed ?? 0;
    setStatus(`Batch refund: ok=${ok} dup=${duplicate} failed=${failed}`, failed ? 'warning' : 'success');
  } catch (error) {
    console.error(error);
    setStatus('Batch refund failed.', 'error');
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
      element.disabled = isBusy || !state.profile;
    });
  }
  if (selectors.reservationForm) {
    selectors.reservationForm.querySelectorAll('button, input').forEach((element) => {
      element.disabled = isBusy || !state.profile;
    });
  }
  if (selectors.captureButton) {
    selectors.captureButton.disabled = isBusy || !canMutateReservation();
  }
  if (selectors.releaseButton) {
    selectors.releaseButton.disabled = isBusy || !canMutateReservation();
  }
  if (selectors.reservationRefreshButton) {
    selectors.reservationRefreshButton.disabled = isBusy || !canRefreshReservation();
  }
  if (selectors.batchSpendButton) {
    selectors.batchSpendButton.disabled = isBusy || !state.profile;
  }
  if (selectors.batchSpendAtomicButton) {
    selectors.batchSpendAtomicButton.disabled = isBusy || !state.profile;
  }
  if (selectors.batchRefundButton) {
    selectors.batchRefundButton.disabled = isBusy || !state.profile;
  }
  renderEntries(state.wallet?.entries || []);
}

function canSpend() {
  if (!state.profile || !state.wallet) {
    return false;
  }
  return state.wallet.balance?.available_coins >= SPEND_COINS;
}

function canMutateReservation() {
  if (!state.profile || !state.reservation) {
    return false;
  }
  return state.reservation.status === 'active' && !state.reservation.expired;
}

function canRefreshReservation() {
  if (!state.profile || !state.reservation) {
    return false;
  }
  return Boolean(state.reservation.reservation_id);
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

function renderReservation(reservation) {
  state.reservation = reservation;
  const reservationID = reservation?.reservation_id || '';
  if (selectors.reservationID) {
    selectors.reservationID.textContent = reservationID || '--';
  }
  if (selectors.reservationMeta) {
    if (!reservationID) {
      selectors.reservationMeta.textContent = '--';
    } else {
      const status = reservation.status || 'unknown';
      const held = reservation.held_coins ?? 0;
      const captured = reservation.captured_coins ?? 0;
      const ttl = reservation.expires_at_unix_utc ? new Date(reservation.expires_at_unix_utc * 1000).toLocaleString() : 'never';
      selectors.reservationMeta.textContent = `status=${status} held=${held} captured=${captured} expires=${ttl}`;
    }
  }
  if (selectors.reservationStatusPill) {
    if (!reservationID) {
      selectors.reservationStatusPill.textContent = 'No hold';
      selectors.reservationStatusPill.dataset.state = 'empty';
    } else if (reservation.expired) {
      selectors.reservationStatusPill.textContent = 'Expired';
      selectors.reservationStatusPill.dataset.state = 'empty';
    } else if (reservation.status === 'active') {
      selectors.reservationStatusPill.textContent = 'Active';
      selectors.reservationStatusPill.dataset.state = 'ok';
    } else {
      selectors.reservationStatusPill.textContent = reservation.status || 'Closed';
      selectors.reservationStatusPill.dataset.state = 'empty';
    }
  }
  if (selectors.captureButton) {
    selectors.captureButton.disabled = state.busy || !canMutateReservation();
  }
  if (selectors.releaseButton) {
    selectors.releaseButton.disabled = state.busy || !canMutateReservation();
  }
  if (selectors.reservationRefreshButton) {
    selectors.reservationRefreshButton.disabled = state.busy || !canRefreshReservation();
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
    const left = document.createElement('div');
    left.className = 'entry__left';
    const typeNode = document.createElement('p');
    typeNode.className = 'entry__type';
    typeNode.textContent = type;
    const metaNode = document.createElement('p');
    metaNode.className = 'entry__meta';
    metaNode.textContent = created;
    left.append(typeNode, metaNode);

    const right = document.createElement('div');
    right.className = 'entry__right';
    const amountNode = document.createElement('div');
    amountNode.className = 'entry__amount';
    amountNode.dataset.direction = isCredit ? 'in' : 'out';
    amountNode.textContent = `${sign}${Math.abs(amountCoins)} coins`;
    right.appendChild(amountNode);

    const actions = document.createElement('div');
    actions.className = 'entry__actions';
    if (entry.type === 'spend' && entry.amount_cents < 0 && entry.entry_id) {
      const refundButton = document.createElement('button');
      refundButton.type = 'button';
      refundButton.className = 'entry__action';
      refundButton.textContent = 'Refund';
      refundButton.disabled = state.busy || !state.profile;
      refundButton.addEventListener('click', async () => {
        if (state.busy) {
          return;
        }
        await refundEntry(entry);
      });
      actions.appendChild(refundButton);
    }
    if (actions.children.length) {
      right.appendChild(actions);
    }

    item.append(left, right);
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

if (typeof window === 'object') {
  window.__demoTestAuth = (profile) => handleAuthenticated(profile);
  window.__demoTestRenderWallet = (wallet) => renderWallet(wallet);
  window.__demoTestSetStatus = (message, level) => setStatus(message, level);
}
