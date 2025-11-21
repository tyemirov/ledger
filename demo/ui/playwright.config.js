// @ts-check
import { defineConfig } from '@playwright/test';
import path from 'path';

export default defineConfig({
  testDir: path.join(__dirname, 'tests'),
  timeout: 30000,
  use: {
    headless: true,
  },
});
