const { test, expect } = require('@playwright/test');
const { setupDemoStubs } = require('./stub-server');
const { demoUrl, login, expectWalletPanelsVisible } = require('./helpers');

test.beforeEach(async ({ page }) => {
  await setupDemoStubs(page);
  await page.goto(demoUrl);
});

test('user stays signed in after page reload', async ({ page }) => {
  await login(page);
  await expectWalletPanelsVisible(page);
  await page.reload();
  await expect(page.locator('[data-auth-message]')).toBeHidden();
  await expectWalletPanelsVisible(page);
});
