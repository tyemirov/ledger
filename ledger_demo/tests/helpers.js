const { expect } = require('@playwright/test');
const { demoOrigin } = require('./stub-server');

const demoUrl = `${demoOrigin}/index.html`;

async function login(page) {
  await page.getByTestId('google-signin').click();
  await page.evaluate(() => {
    if (window.__playwrightAuth && typeof window.__playwrightAuth.login === 'function') {
      window.__playwrightAuth.login({
        user_id: 'google:test-user',
        user_email: 'demo@example.com',
        display: 'Demo User',
        avatar_url: '',
        roles: ['user'],
      });
    }
  });
  await expect(page.locator('[data-auth-message]')).toBeHidden();
}

async function expectWalletPanelsVisible(page) {
  await expect(page.locator('[data-wallet]')).toBeVisible();
  await expect(page.locator('[data-transactions]')).toBeVisible();
  await expect(page.locator('[data-purchase]')).toBeVisible();
}

async function expectLedgerEntries(page, minimumCount) {
  const items = page.locator('[data-entry-list] li');
  const count = await items.count();
  if (count < minimumCount) {
    throw new Error(`expected at least ${minimumCount} ledger entries, received ${count}`);
  }
}

async function expectSignedOut(page) {
  await expect(page.locator('[data-auth-message]')).toBeVisible();
  await expect(page.locator('[data-wallet]')).toBeHidden();
}

module.exports = {
  demoUrl,
  login,
  expectWalletPanelsVisible,
  expectLedgerEntries,
  expectSignedOut,
};
