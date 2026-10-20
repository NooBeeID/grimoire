import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.E2E_BASE_URL ?? 'http://localhost:8080';

export default defineConfig({
  testDir: 'e2e',
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  use: { baseURL, trace: 'retain-on-failure' },
  projects: [{ name: 'chromium', use: devices['Desktop Chrome'] }],
  webServer: process.env.E2E_BASE_URL
    ? undefined
    : {
        command: '../bin/grimoire',
        url: `${baseURL}/healthz`,
        reuseExistingServer: true,
        timeout: 60_000,
        env: { HTTP_ADDR: '127.0.0.1:8080', COOKIE_SECURE: 'false', LOGIN_RATE_PER_MINUTE: '1000' },
      },
});
