const fs = require('fs').promises;
const path = require('path');

const demoOrigin = 'http://ledger-demo.test';

const corsHeaders = {
  'access-control-allow-origin': demoOrigin,
  'access-control-allow-credentials': 'true',
};

const authClientStub = `(() => {
  const state = {
    onAuthenticated: null,
    onUnauthenticated: null,
    authenticated: false,
    profile: null,
  };
  window.initAuthClient = function initAuthClient(options) {
    state.onAuthenticated = options.onAuthenticated;
    state.onUnauthenticated = options.onUnauthenticated;
    if (state.authenticated && state.profile) {
      options.onAuthenticated(state.profile);
    } else if (typeof options.onUnauthenticated === 'function') {
      options.onUnauthenticated();
    }
    return Promise.resolve();
  };
  window.logout = function logout() {
    if (state.authenticated) {
      state.authenticated = false;
      state.profile = null;
      if (typeof state.onUnauthenticated === 'function') {
        state.onUnauthenticated();
      }
    }
  };
  window.__playwrightAuth = {
    login(profile) {
      state.authenticated = true;
      state.profile = profile;
      if (typeof state.onAuthenticated === 'function') {
        state.onAuthenticated(profile);
      }
    },
  };
})();`; // end stub

function createGoogleStubScript(profile) {
  return `(() => {
    const google = (window.google = window.google || {});
    google.accounts = google.accounts || {};
    const id = (google.accounts.id = google.accounts.id || {});
    let latestConfig = null;
    id.initialize = (config) => {
      latestConfig = config || null;
    };
    id.renderButton = (container, options) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.dataset.testid = 'google-signin';
      button.textContent =
        (options && options.text) || 'Sign in with Google';
      button.addEventListener('click', () => {
        if (window.__playwrightAuth && window.__playwrightAuth.login) {
          window.__playwrightAuth.login(${JSON.stringify(profile)});
        }
        if (latestConfig && typeof latestConfig.callback === 'function') {
          latestConfig.callback({ credential: 'playwright-credential' });
        }
      });
      container.innerHTML = '';
      container.appendChild(button);
    };
    id.prompt = () => {};
  })();`;
}

function createTAuthConfigScript(clientId) {
  return `window.__TAUTH_DEMO_CONFIG = Object.freeze({ googleClientId: ${JSON.stringify(
    clientId,
  )} });`;
}

function ledgerWalletPayload(state) {
  return {
    wallet: {
      balance: {
        total_cents: state.balance.total,
        available_cents: state.balance.available,
        total_coins: state.balance.total / 100,
        available_coins: state.balance.available / 100,
      },
      entries: state.entries,
    },
  };
}

function contentTypeFor(filePath) {
  if (filePath.endsWith('.html')) {
    return 'text/html; charset=utf-8';
  }
  if (filePath.endsWith('.js')) {
    return 'application/javascript; charset=utf-8';
  }
  if (filePath.endsWith('.css')) {
    return 'text/css; charset=utf-8';
  }
  return 'text/plain; charset=utf-8';
}

async function serveDemoUi(page) {
  const uiRoot = path.resolve(__dirname, '../../ledger_demo/frontend/ui');
  const htmlPath = path.join(uiRoot, 'index.html');
  await page.route(`${demoOrigin}/**`, async (route) => {
    const url = new URL(route.request().url());
    let relativePath = url.pathname;
    if (relativePath === '/' || relativePath === '') {
      relativePath = '/index.html';
    }
    const normalized = path.normalize(relativePath).replace(/^\//, '');
    const assetPath = path.resolve(uiRoot, normalized);
    if (!assetPath.startsWith(uiRoot)) {
      await route.fulfill({ status: 403, body: 'forbidden' });
      return;
    }
    let body;
    try {
      body = await fs.readFile(assetPath);
    } catch (error) {
      if (assetPath === htmlPath) {
        throw error;
      }
      await route.fulfill({ status: 404, body: 'not found' });
      return;
    }
    const headers = { 'content-type': contentTypeFor(assetPath) };
    await route.fulfill({ status: 200, headers, body });
  });
}

function isPreflight(route) {
  return route.request().method().toUpperCase() === 'OPTIONS';
}

async function handlePreflight(route) {
  await route.fulfill({
    status: 204,
    headers: {
      ...corsHeaders,
      'access-control-allow-methods': 'GET,POST,OPTIONS',
      'access-control-allow-headers': 'Content-Type',
    },
    body: '',
  });
}

async function setupDemoStubs(page) {
  const clientId = 'playwright-test-client-id';
  const profile = {
    user_id: 'google:test-user',
    user_email: 'demo@example.com',
    display: 'Demo User',
    avatar_url:
      'https://cdn.jsdelivr.net/gh/MarcoPoloResearchLab/mpr-ui@latest/mpr-ui.png',
    roles: ['user'],
  };
  const state = {
    balance: {
      total: 2000,
      available: 2000,
    },
    entries: [],
    transactionResponses: ['success', 'insufficient_funds'],
  };

  await serveDemoUi(page);

  await page.route('http://localhost:8080/demo/config.js', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/javascript',
      headers: corsHeaders,
      body: createTAuthConfigScript(clientId),
    });
  });

  await page.route('http://localhost:8080/static/auth-client.js', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/javascript',
      headers: corsHeaders,
      body: authClientStub,
    });
  });

  await page.route('https://accounts.google.com/gsi/client', (route) => {
    route.fulfill({
      status: 200,
      contentType: 'application/javascript',
      body: createGoogleStubScript(profile),
    });
  });

  await page.route('http://localhost:9090/api/bootstrap', async (route) => {
    if (isPreflight(route)) {
      await handlePreflight(route);
      return;
    }
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: corsHeaders,
      body: JSON.stringify(ledgerWalletPayload(state)),
    });
  });

  await page.route('http://localhost:9090/api/wallet', async (route) => {
    if (isPreflight(route)) {
      await handlePreflight(route);
      return;
    }
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: corsHeaders,
      body: JSON.stringify(ledgerWalletPayload(state)),
    });
  });

  await page.route('http://localhost:9090/api/transactions', async (route) => {
    if (isPreflight(route)) {
      await handlePreflight(route);
      return;
    }
    if (!state.transactionResponses.length) {
      state.transactionResponses.push('success');
    }
    const status = state.transactionResponses.shift();
    if (status === 'success') {
      state.balance.available -= 500;
    }
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: corsHeaders,
      body: JSON.stringify({
        status,
        wallet: ledgerWalletPayload(state).wallet,
      }),
    });
  });

  await page.route('http://localhost:9090/api/purchases', async (route) => {
    if (isPreflight(route)) {
      await handlePreflight(route);
      return;
    }
    const body = JSON.parse((await route.request().postData()) || '{}');
    const coins = Number(body.coins || 0);
    state.balance.total += coins * 100;
    state.balance.available += coins * 100;
    route.fulfill({
      status: 200,
      contentType: 'application/json',
      headers: corsHeaders,
      body: JSON.stringify({
        wallet: ledgerWalletPayload(state).wallet,
      }),
    });
  });
}

module.exports = {
  setupDemoStubs,
  demoOrigin,
};
