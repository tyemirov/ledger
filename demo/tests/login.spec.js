const { test, expect } = require('@playwright/test');

const demoBaseUrl = process.env.DEMO_BASE_URL || 'http://localhost:8000';

async function navigateToDemo(page) {
  await page.goto(demoBaseUrl);
}

test.describe('LG-105 login regression', () => {
  test('user can sign in and stay signed in after reload', async ({ page }) => {
    test.fail(true, 'Login via TAuth currently fails with nonce mismatch (LG-105).');
    await navigateToDemo(page);
    await page.getByRole('button', { name: /sign in/i }).click();
    await expect(page.locator('[data-wallet]')).toBeVisible();
    await page.reload();
    await expect(page.locator('[data-wallet]')).toBeVisible();
  });

  test('authenticated user can spend coins and see updated history', async ({ page }) => {
    test.fail(true, 'Spend flow blocked until login bug is resolved (LG-105).');
    await navigateToDemo(page);
    await page.getByRole('button', { name: /sign in/i }).click();
    const spendButton = page.locator('[data-transact]');
    await spendButton.click();
    await expect(page.locator('[data-available-coins]')).toHaveText(/\d+/);
    const entries = page.locator('[data-entry-list] li');
    expect(await entries.count()).toBeGreaterThan(0);
  });
});
