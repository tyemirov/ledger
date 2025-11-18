// @ts-check

const JSON_TYPE = 'application/json';

/**
 * @param {{ baseUrl: string }} options
 */
export function createWalletClient(options) {
  if (!options || typeof options.baseUrl !== 'string' || !options.baseUrl.trim()) {
    throw new Error('wallet_api.invalid_base_url');
  }
  const baseUrl = options.baseUrl.replace(/\/$/, '');

  /**
   * @param {string} path
   * @param {RequestInit} [init]
   */
  async function request(path, init = {}) {
    const response = await fetch(`${baseUrl}${path}`, {
      credentials: 'include',
      headers: {
        'Content-Type': JSON_TYPE,
        ...(init.headers || {}),
      },
      ...init,
    });
    if (!response.ok) {
      const message = await safeReadMessage(response);
      throw new Error(`wallet_api.${response.status}:${message}`);
    }
    if (response.headers.get('content-type')?.includes(JSON_TYPE)) {
      return response.json();
    }
    return {};
  }

  return {
    async fetchSession() {
      const payload = await request('/session', { method: 'GET' });
      return normalizeSession(payload);
    },
    async bootstrap(metadata) {
      const payload = await request('/bootstrap', {
        method: 'POST',
        body: JSON.stringify({ metadata }),
      });
      return normalizeWallet(payload);
    },
    async getWallet() {
      const payload = await request('/wallet', { method: 'GET' });
      return normalizeWallet(payload);
    },
    async spend(metadata) {
      const payload = await request('/transactions', {
        method: 'POST',
        body: JSON.stringify({ metadata }),
      });
      return normalizeTransaction(payload);
    },
    async purchase(coins, metadata) {
      const payload = await request('/purchases', {
        method: 'POST',
        body: JSON.stringify({ coins, metadata }),
      });
      return normalizeWallet(payload);
    },
  };
}

async function safeReadMessage(response) {
  try {
    const body = await response.json();
    if (body && typeof body.error === 'string') {
      return body.error;
    }
  } catch (_) {}
  return 'unknown_error';
}

function normalizeWallet(payload) {
  if (!payload || typeof payload !== 'object') {
    throw new Error('wallet_api.invalid_wallet_payload');
  }
  const wallet = payload.wallet || {};
  return {
    wallet: {
      balance: {
        total_coins: Number(wallet.balance?.total_coins ?? 0),
        available_coins: Number(wallet.balance?.available_coins ?? 0),
        total_cents: Number(wallet.balance?.total_cents ?? 0),
        available_cents: Number(wallet.balance?.available_cents ?? 0),
      },
      entries: Array.isArray(wallet.entries)
        ? wallet.entries.map((entry) => ({
            entry_id: String(entry.entry_id || ''),
            type: String(entry.type || ''),
            amount_coins: Number(entry.amount_coins ?? 0),
            amount_cents: Number(entry.amount_cents ?? 0),
            created_unix_utc: Number(entry.created_unix_utc ?? 0),
            metadata: entry.metadata || {},
          }))
        : [],
    },
  };
}

function normalizeTransaction(payload) {
  const wallet = normalizeWallet(payload);
  return {
    status: typeof payload.status === 'string' ? payload.status : 'unknown',
    wallet: wallet.wallet,
  };
}

function normalizeSession(payload) {
  if (!payload || typeof payload !== 'object') {
    throw new Error('wallet_api.invalid_session');
  }
  return {
    user_id: String(payload.user_id || ''),
    display: String(payload.display || ''),
    email: String(payload.email || ''),
    avatar_url: String(payload.avatar_url || ''),
    roles: Array.isArray(payload.roles) ? payload.roles : [],
    expires: Number(payload.expires ?? 0),
  };
}
