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

test('footer square theme toggle updates theme + palette attributes', async ({ page }) => {
  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/tauth.js', async (route) => {
    await route.fulfill({
      contentType: 'application/javascript',
      body:
        "window.initAuthClient = ({ onUnauthenticated }) => { if (typeof onUnauthenticated === 'function') { onUnauthenticated(); } return Promise.resolve(); }; window.getCurrentUser = () => Promise.resolve(null);",
    });
  });
  await page.route('**/gsi/client', async (route) => {
    await route.fulfill({
      contentType: 'application/javascript',
      body:
        "window.google = window.google || {}; window.google.accounts = window.google.accounts || {}; window.google.accounts.id = window.google.accounts.id || { initialize() {}, renderButton() {}, prompt() {} };",
    });
  });
  await page.route('**/api/bootstrap', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({}) });
  });
  await page.route('**/api/wallet', async (route) => {
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ wallet: { balance: { total_coins: 0, available_coins: 0 }, entries: [] } }) });
  });

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);

  await expect
    .poll(async () =>
      page.evaluate(() => document.body.getAttribute('data-demo-palette') || ''),
    )
    .toBe('default');
  await expect
    .poll(async () => page.evaluate(() => document.body.getAttribute('data-mpr-theme') || ''))
    .toBe('light');

  const squareGrid = page.locator('mpr-footer [data-mpr-theme-toggle="grid"]');
  await expect(squareGrid).toBeVisible();

  const gridBox = await squareGrid.boundingBox();
  if (!gridBox) {
    throw new Error('theme grid missing bounding box');
  }

  // Bottom-right quadrant -> forest-dark.
  await squareGrid.click({ position: { x: gridBox.width * 0.75, y: gridBox.height * 0.75 } });
  await expect
    .poll(async () => page.evaluate(() => document.body.getAttribute('data-mpr-theme') || ''))
    .toBe('dark');
  await expect
    .poll(async () =>
      page.evaluate(() => document.body.getAttribute('data-demo-palette') || ''),
    )
    .toBe('forest');

  // Top-right quadrant -> sunrise-light.
  await squareGrid.click({ position: { x: gridBox.width * 0.75, y: gridBox.height * 0.25 } });
  await expect
    .poll(async () => page.evaluate(() => document.body.getAttribute('data-mpr-theme') || ''))
    .toBe('light');
  await expect
    .poll(async () =>
      page.evaluate(() => document.body.getAttribute('data-demo-palette') || ''),
    )
    .toBe('sunrise');
});
