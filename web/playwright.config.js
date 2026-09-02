// @ts-check

import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  timeout: 45_000,
  fullyParallel: false,
  use: {
    browserName: "chromium",
    headless: true,
  },
});
