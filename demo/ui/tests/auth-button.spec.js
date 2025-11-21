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
    const type = ext === '.js' ? 'application/javascript' : ext === '.css' ? 'text/css' : 'text/html';
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

test('shows Google sign-in button via mpr-ui header', async ({ page }) => {
  await page.route('**/config.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });
  await page.route('**/static/auth-client.js', async (route) => {
    await route.fulfill({ contentType: 'application/javascript', body: '' });
  });

  await page.goto(`http://127.0.0.1:${serverPort}/index.html`);

  await expect(page.locator('[data-test="google-signin"]')).toBeVisible();
});
