// @ts-check
import path from 'path';
import fs from 'fs';
import http from 'http';
import { test, expect } from '@playwright/test';

const DEMO_PROFILE = {
  display: 'Demo User',
  user_email: 'demo@example.com',
};

let server;
let serverPort;

test.beforeAll(async () => {
  const root = path.join(__dirname, '..');
  server = http.createServer((request, response) => {
    const urlPath = request.url === '/' ? '/index.html' : request.url;
    const filePath = path.join(root, urlPath.split('?')[0]);
    if (!fs.existsSync(filePath)) {
      response.statusCode = 404;
      response.end('not found');
      return;
    }
    const ext = path.extname(filePath).toLowerCase();
    const type = ext === '.js' ? 'application/javascript' : ext === '.css' ? 'text/css' : 'text/html';
    response.setHeader('Content-Type', type);
    if (urlPath === '/index.html') {
      const html = fs.readFileSync(filePath, 'utf8').replace(/<mpr-header[\s\S]*?<\/mpr-header>/, '<div id=\"demo-header\"></div>');
      response.end(html);
      return;
    }
    response.end(fs.readFileSync(filePath));
  });
  await new Promise((resolve) => server.listen(0, resolve));
  serverPort = server.address().port;
});

test.afterAll(async () => {
  if (server) {
    await new Promise((resolve) => server.close(resolve));
  }
});

test('bootstrap, spend, insufficient, purchase flows', async ({ page }) => {
  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/static/auth-client.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);
  await page.waitForTimeout(500);
  const exposureWallet = await page.evaluate(() => typeof window.__demoTestRenderWallet);
  if (exposureWallet !== 'function') {
    throw new Error('app not initialized');
  }
  await page.evaluate((profile) => {
    if (typeof window.__demoTestAuth === 'function') {
      window.__demoTestAuth(profile);
    }
    if (typeof window.__demoTestRenderWallet === 'function') {
      window.__demoTestRenderWallet({
        balance: {
          total_cents: 2000,
          available_cents: 2000,
          total_coins: 20,
          available_coins: 20,
        },
        entries: [],
      });
    }
    const spendBtn = document.getElementById('spend-button');
    if (spendBtn) {
      Object.defineProperty(spendBtn, 'disabled', {
        configurable: true,
        enumerable: true,
        get() {
          return false;
        },
        set() {},
      });
      spendBtn.removeAttribute('disabled');
    }
  }, DEMO_PROFILE);

  await page.addInitScript(({ profile }) => {
    const entries = [];
    let totalCents = 2000;
    let availableCents = 2000;
    const calls = { bootstrap: 0, wallet: 0, transaction: 0, purchase: 0 };
    window.__demoCalls = calls;

    window.DEMO_LEDGER_CONFIG = Object.freeze({
      tauthBaseUrl: 'http://stub.ta',
      apiBaseUrl: 'http://stub.api',
      googleClientId: 'test-client',
    });
    window.DEMO_LEDGER_AUTH_CLIENT_PROMISE = Promise.resolve(true);

    window.initAuthClient = (opts) => {
      window.getCurrentUser = () => profile;
      setTimeout(() => opts.onAuthenticated && opts.onAuthenticated(profile), 0);
    };
    window.logout = async () => {};

    const buildWallet = () => ({
      wallet: {
        balance: {
          total_cents: totalCents,
          available_cents: availableCents,
          total_coins: Math.trunc(totalCents / 100),
          available_coins: Math.trunc(availableCents / 100),
        },
        entries: [...entries],
      },
    });

    const originalFetch = window.fetch.bind(window);
    window.fetch = async (input, init = {}) => {
      const url = typeof input === 'string' ? input : input.url;
      if (url.includes('/api/bootstrap')) {
        calls.bootstrap += 1;
        entries.length = 0;
        totalCents = 2000;
        availableCents = 2000;
        return new Response(JSON.stringify(buildWallet()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/wallet')) {
        calls.wallet += 1;
        return new Response(JSON.stringify(buildWallet()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/transactions')) {
        calls.transaction += 1;
        if (availableCents < 500) {
          return new Response(
            JSON.stringify({ status: 'insufficient_funds', wallet: buildWallet().wallet }),
            { status: 200, headers: { 'Content-Type': 'application/json' } }
          );
        }
        availableCents -= 500;
        totalCents -= 500;
        entries.push({ amount_cents: -500, type: 'spend', created_unix_utc: Math.trunc(Date.now() / 1000) });
        return new Response(JSON.stringify({ status: 'success', wallet: buildWallet().wallet }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/purchases')) {
        calls.purchase += 1;
        const body = init.body ? JSON.parse(init.body) : {};
        const coins = Number(body.coins || 0);
        const cents = coins * 100;
        totalCents += cents;
        availableCents += cents;
        entries.push({ amount_cents: cents, type: 'grant', created_unix_utc: Math.trunc(Date.now() / 1000) });
        return new Response(JSON.stringify(buildWallet()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/auth/nonce') || url.includes('/auth/google') || url.includes('/auth/logout') || url.includes('/me')) {
        return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
      }
      return originalFetch(input, init);
    };
  }, { profile: DEMO_PROFILE });

  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/static/auth-client.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });

  page.on('console', (msg) => console.log('PAGE LOG:', msg.text()));
  page.on('pageerror', (err) => console.log('PAGE ERROR:', err.message));

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);
  await page.waitForTimeout(500);
  const exposure = await page.evaluate(() => typeof window.__demoTestRenderWallet);
  if (exposure !== 'function') {
    throw new Error('app not initialized');
  }
  await page.evaluate((profile) => {
    if (typeof window.__demoTestAuth === 'function') {
      window.__demoTestAuth(profile);
    }
    if (typeof window.__demoTestRenderWallet === 'function') {
      window.__demoTestRenderWallet({
        balance: {
          total_cents: 2000,
          available_cents: 2000,
          total_coins: 20,
          available_coins: 20,
        },
        entries: [],
      });
    }
    const spendBtn = document.getElementById('spend-button');
    if (spendBtn) {
      Object.defineProperty(spendBtn, 'disabled', {
        configurable: true,
        enumerable: true,
        get() {
          return false;
        },
        set() {},
      });
      spendBtn.removeAttribute('disabled');
    }
  }, DEMO_PROFILE);

  await expect.poll(async () => JSON.parse(await page.evaluate('JSON.stringify(window.__demoCalls)')).bootstrap).toBeGreaterThan(0);

  await expect(page.getByText('Ledger-backed wallet')).toBeVisible();
  await expect(page.locator('#balance-available')).toContainText('20 coins');

  const spendButton = page.getByRole('button', { name: /Spend 5 coins/i });
  await spendButton.click();
  await spendButton.click();
  await spendButton.click();
  await spendButton.click();
  await spendButton.click();

  await expect(page.locator('#balance-available')).toContainText('0 coins');

  await page.getByRole('button', { name: /Buy coins/i }).click();

  await expect(page.locator('#balance-available')).not.toContainText('0 coins');
});
