const { test, expect } = require('@playwright/test');
const { setupDemoStubs } = require('./stub-server');
const { demoUrl, login, expectLedgerEntries } = require('./helpers');

test.beforeEach(async ({ page }) => {
  page.on('console', (message) => {
    console.log(`[browser] ${message.text()}`);
  });
  await setupDemoStubs(page);
  await page.goto(demoUrl);
});

test('shows sign-in prompt before authentication', async ({ page }) => {
  await expect(page.locator('[data-auth-message]')).toBeVisible();
  await expect(page.locator('[data-wallet]')).toBeHidden();
});

test('login reveals wallet panels and balance', async ({ page }) => {
  await login(page);
  await expect(page.locator('[data-wallet]')).toBeVisible();
  await expect(page.locator('[data-available-coins]')).toHaveText('20');
});

test('transaction flow updates balance and surfaces insufficient funds', async ({ page }) => {
  await login(page);
  const status = page.locator('[data-transaction-status]');
  await page.locator('[data-transact]').click();
  await expect(status).toHaveText('Transaction succeeded.');
  await expect(page.locator('[data-available-coins]')).toHaveText('15');
  await expectLedgerEntries(page, 2);
  await page.locator('[data-transact]').click();
  await expect(status).toHaveText('Insufficient funds. Purchase more coins to continue.');
  await expectLedgerEntries(page, 2);
});

test('purchase replenishes coins', async ({ page }) => {
  await login(page);
  await page.locator('[data-purchase-form] input[value="10"]').check();
  await page.locator('[data-purchase-form] button[type="submit"]').click();
  await expect(page.locator('[data-available-coins]')).toHaveText('30');
  await expectLedgerEntries(page, 2);
});
