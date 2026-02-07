// @ts-check
import path from 'path';
import fs from 'fs';
import http from 'http';
import { test, expect } from '@playwright/test';

const PROFILE = {
  user_id: 'google:demo-user',
  user_email: 'demo@example.com',
  display: 'Demo User',
  avatar_url:
    'data:image/svg+xml;utf8,' +
    encodeURIComponent(
      '<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128"><rect width="128" height="128" fill="#0ea5e9"/><text x="50%" y="54%" dominant-baseline="middle" text-anchor="middle" font-family="Arial" font-size="48" fill="white">D</text></svg>',
    ),
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
    const type =
      ext === '.js' ? 'application/javascript' : ext === '.css' ? 'text/css' : 'text/html';
    response.setHeader('Content-Type', type);
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

test('user menu account details opens modal', async ({ page }) => {
  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/tauth.js', async (route) => {
    await route.fulfill({
      contentType: 'application/javascript',
      body: `
        window.setAuthTenantId = () => {};
        window.getCurrentUser = () => Promise.resolve(${JSON.stringify(PROFILE)});
        window.logout = () => Promise.resolve(true);
        window.initAuthClient = ({ onAuthenticated }) => {
          if (typeof onAuthenticated === 'function') {
            onAuthenticated(${JSON.stringify(PROFILE)});
          }
          return Promise.resolve();
        };
      `,
    });
  });
  await page.route('**/gsi/client', async (route) => {
    await route.fulfill({
      contentType: 'application/javascript',
      body:
        "window.google = window.google || {}; window.google.accounts = window.google.accounts || {}; window.google.accounts.id = window.google.accounts.id || { initialize() {}, renderButton(target) { const button = document.createElement('button'); button.textContent = 'Sign in'; target.replaceChildren(button); }, prompt() {} };",
    });
  });
  await page.route('**/api/bootstrap', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ wallet: { balance: { total_coins: 20, available_coins: 20 }, entries: [] } }),
    });
  });
  await page.route('**/api/wallet', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ wallet: { balance: { total_coins: 20, available_coins: 20 }, entries: [] } }),
    });
  });

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);

  const userMenuTrigger = page.locator('mpr-user [data-mpr-user="trigger"]');
  await expect(userMenuTrigger).toBeVisible();
  await userMenuTrigger.click();

  const accountItem = page.getByRole('menuitem', { name: 'Account details' });
  await expect(accountItem).toBeVisible();
  await accountItem.click();

  const modal = page.locator('#account-modal[data-mpr-modal-open="true"]');
  await expect(modal).toBeVisible();
  await expect(page.locator('#account-name')).toHaveText('Demo User');
  await expect(page.locator('#account-email')).toHaveText('demo@example.com');

  await page.getByRole('button', { name: 'Close account details' }).click();
  await expect(page.locator('#account-modal')).toHaveAttribute('data-mpr-modal-open', 'false');
});
