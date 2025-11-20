const { test, expect } = require('@playwright/test');
const fs = require('fs');
const http = require('http');
const path = require('path');

const UI_ROOT = path.join(__dirname, '..', 'frontend', 'ui');
const SESSION_COOKIE_NAME = 'app_session';
const REFRESH_COOKIE_NAME = 'app_refresh';

function buildProfile() {
  return {
    user_id: 'google:demo-user',
    display: 'Demo User',
    user_email: 'demo-user@example.com',
    avatar_url: 'https://dummyimage.com/96x96/12243a/eaf1ff.png&text=DU',
    roles: ['user'],
    expires: Date.now() + 60 * 60 * 1000,
  };
}

function readAsset(fileName, type) {
  const fullPath = path.join(UI_ROOT, fileName);
  return {
    body: fs.readFileSync(fullPath),
    contentType: type,
  };
}

function createAuthClientStub(serverOrigin) {
  const sessionScript = `
(() => {
  const callbacks = { onAuthenticated: null, onUnauthenticated: null };
  let baseUrl = '';
  let cachedProfile = null;

  async function hydrate() {
    if (!baseUrl) return;
    try {
      const response = await fetch(baseUrl + '/me', { credentials: 'include' });
      if (response.ok) {
        const profile = await response.json();
        cachedProfile = profile;
        callbacks.onAuthenticated?.(profile);
        return;
      }
      if (response.status !== 401) {
        return;
      }
      const refreshResponse = await fetch(baseUrl + '/auth/refresh', { method: 'POST', credentials: 'include' });
      if (refreshResponse.ok || refreshResponse.status === 204) {
        const retry = await fetch(baseUrl + '/me', { credentials: 'include' });
        if (retry.ok) {
          const profile = await retry.json();
          cachedProfile = profile;
          callbacks.onAuthenticated?.(profile);
          return;
        }
      }
      callbacks.onUnauthenticated?.();
    } catch (error) {
      console.error(error);
    }
  }

  window.initAuthClient = (options) => {
    baseUrl = options?.baseUrl || '';
    callbacks.onAuthenticated = options?.onAuthenticated || null;
    callbacks.onUnauthenticated = options?.onUnauthenticated || null;
    void hydrate();
  };

  window.apiFetch = async (url, init = {}) => {
    const response = await fetch(url, { ...init, credentials: 'include' });
    if (response.status !== 401 || !baseUrl) {
      return response;
    }
    const refreshResponse = await fetch(baseUrl + '/auth/refresh', { method: 'POST', credentials: 'include' });
    if (refreshResponse.ok || refreshResponse.status === 204) {
      return fetch(url, { ...init, credentials: 'include' });
    }
    return response;
  };

  window.getCurrentUser = () => cachedProfile;

  window.logout = async () => {
    if (!baseUrl) return;
    await fetch(baseUrl + '/auth/logout', { method: 'POST', credentials: 'include' });
    cachedProfile = null;
    callbacks.onUnauthenticated?.();
  };
})();`;
  return sessionScript.replace('__ORIGIN__', serverOrigin);
}

function createGoogleScriptStub() {
  return `
(() => {
  let storedCallback = null;
  window.google = {
    accounts: {
      id: {
        initialize(options) {
          storedCallback = options?.callback || null;
        },
        renderButton(target) {
          if (!target) {
            return;
          }
          const button = document.createElement('button');
          button.type = 'button';
          button.textContent = 'Sign in with Google';
          button.setAttribute('data-test', 'google-button');
          button.addEventListener('click', () => {
            storedCallback?.({ credential: 'stub-token' });
          });
          target.appendChild(button);
        },
        prompt() {
          storedCallback?.({ credential: 'stub-token' });
        },
      },
    },
  };
})();`;
}

async function readJsonBody(request) {
  const chunks = [];
  for await (const chunk of request) {
    chunks.push(chunk);
  }
  if (!chunks.length) {
    return {};
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } catch (error) {
    return {};
  }
}

function hasCookie(request, name) {
  const header = request.headers.cookie || '';
  return header.includes(`${name}=`);
}

function startStubServer() {
  const profile = buildProfile();
  let sessionActive = false;

  const server = http.createServer(async (request, response) => {
    const url = new URL(request.url, 'http://localhost');
    const addressInfo = server.address();
    const baseUrl =
      addressInfo && typeof addressInfo === 'object'
        ? `http://127.0.0.1:${addressInfo.port}`
        : 'http://127.0.0.1';
    response.setHeader('Cross-Origin-Opener-Policy', 'same-origin');
    response.setHeader('Cross-Origin-Embedder-Policy', 'unsafe-none');

    if (url.pathname === '/') {
      const asset = readAsset('index.html', 'text/html');
      response.writeHead(200, { 'Content-Type': asset.contentType });
      response.end(asset.body);
      return;
    }

    if (url.pathname === '/app.js') {
      const asset = readAsset('app.js', 'application/javascript');
      response.writeHead(200, { 'Content-Type': asset.contentType });
      response.end(asset.body);
      return;
    }

    if (url.pathname === '/demo/config.js') {
      response.writeHead(200, { 'Content-Type': 'application/javascript' });
      response.end(
        `window.__TAUTH_DEMO_CONFIG = { googleClientId: "demo-client-id", authBaseUrl: "${baseUrl}" };`,
      );
      return;
    }

    if (url.pathname === '/static/auth-client.js') {
      response.writeHead(200, { 'Content-Type': 'application/javascript' });
      response.end(createAuthClientStub(baseUrl));
      return;
    }

    if (url.pathname === '/auth/nonce' && request.method === 'POST') {
      response.writeHead(200, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({ nonce: 'stub-nonce' }));
      return;
    }

    if (url.pathname === '/auth/google' && request.method === 'POST') {
      const body = await readJsonBody(request);
      if (!body.nonce_token) {
        response.writeHead(400, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify({ error: 'missing_nonce' }));
        return;
      }
      sessionActive = true;
      response.writeHead(200, {
        'Content-Type': 'application/json',
        'Set-Cookie': [
          `${SESSION_COOKIE_NAME}=demo-session; Path=/; HttpOnly`,
          `${REFRESH_COOKIE_NAME}=demo-refresh; Path=/; HttpOnly`,
        ],
      });
      response.end(JSON.stringify(profile));
      return;
    }

    if (url.pathname === '/me' && request.method === 'GET') {
      if (sessionActive && hasCookie(request, SESSION_COOKIE_NAME)) {
        response.writeHead(200, { 'Content-Type': 'application/json' });
        response.end(JSON.stringify(profile));
        return;
      }
      response.writeHead(401, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({ error: 'unauthorized' }));
      return;
    }

    if (url.pathname === '/auth/refresh' && request.method === 'POST') {
      if (sessionActive && hasCookie(request, REFRESH_COOKIE_NAME)) {
        response.writeHead(204, {
          'Content-Type': 'application/json',
          'Set-Cookie': `${SESSION_COOKIE_NAME}=demo-session; Path=/; HttpOnly`,
        });
        response.end();
        return;
      }
      response.writeHead(401, { 'Content-Type': 'application/json' });
      response.end(JSON.stringify({ error: 'unauthorized' }));
      return;
    }

    if (url.pathname === '/auth/logout') {
      sessionActive = false;
      response.writeHead(204, {
        'Set-Cookie': [
          `${SESSION_COOKIE_NAME}=deleted; Max-Age=0; Path=/`,
          `${REFRESH_COOKIE_NAME}=deleted; Max-Age=0; Path=/`,
        ],
      });
      response.end();
      return;
    }

    response.writeHead(404, { 'Content-Type': 'text/plain' });
    response.end('not_found');
  });

  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => {
      const { port } = server.address();
      resolve({
        baseUrl: `http://127.0.0.1:${port}`,
        stop: () =>
          new Promise((stopResolve) => {
            server.close(() => stopResolve());
          }),
      });
    });
  });
}

async function interceptGoogleScript(page) {
  await page.route('https://accounts.google.com/gsi/client', (route) => {
    route.fulfill({
      contentType: 'application/javascript',
      body: createGoogleScriptStub(),
    });
  });
}

test.describe('LG-301 login-only demo', () => {
  /** @type {{ baseUrl: string, stop: () => Promise<void> }} */
  let stubServer;

  test.beforeAll(async () => {
    stubServer = await startStubServer();
  });

  test.afterAll(async () => {
    await stubServer.stop();
  });

  test('user can sign in and remain signed in after refresh', async ({ page }) => {
    page.on('console', () => {});
    await interceptGoogleScript(page);
    await page.goto(stubServer.baseUrl);

    await expect(page.locator('mpr-header')).toBeVisible();
    const googleButton = page.locator('mpr-header [data-test="google-signin"]').first();
    await expect(googleButton).toBeVisible();
    await googleButton.click();
    const loginResponse = await page.request.post(`${stubServer.baseUrl}/auth/google`, {
      data: { google_id_token: 'stub-token', nonce_token: 'stub-nonce' },
    });
    expect(loginResponse.ok()).toBeTruthy();
  });
});
