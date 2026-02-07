// @ts-check
import path from 'path';
import fs from 'fs';
import http from 'http';
import { test, expect } from '@playwright/test';

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

test('header Docs link points at the rendered integration doc on GitHub', async ({ page }) => {
  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/tauth.js', async (route) => {
    await route.fulfill({
      contentType: 'application/javascript',
      body: "window.initAuthClient = ({ onUnauthenticated }) => { if (typeof onUnauthenticated === 'function') { onUnauthenticated(); } return Promise.resolve(); }; window.getCurrentUser = () => Promise.resolve(null);",
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
      body: JSON.stringify({ wallet: { balance: { total_coins: 0, available_coins: 0 }, entries: [] } }),
    });
  });
  await page.route('**/api/wallet', async (route) => {
    await route.fulfill({
      contentType: 'application/json',
      body: JSON.stringify({ wallet: { balance: { total_coins: 0, available_coins: 0 }, entries: [] } }),
    });
  });

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);

  const docsLink = page.getByRole('link', { name: 'Docs' });
  await expect(docsLink).toBeVisible();
  await expect(docsLink).toHaveAttribute(
    'href',
    'https://github.com/tyemirov/ledger/blob/master/docs/integration.md',
  );
});
