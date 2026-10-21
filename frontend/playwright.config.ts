import { defineConfig, devices } from '@playwright/test';

const external = process.env.E2E_BASE_URL;
const baseURL = external ?? 'http://localhost:8080';
const limitsURL = 'http://localhost:8081';
const serverEnv = { COOKIE_SECURE: 'false', LOGIN_RATE_PER_MINUTE: '1000' };

export default defineConfig({
  testDir: 'e2e',
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  use: { baseURL, trace: 'retain-on-failure' },
  projects: [
    { name: 'chromium', use: devices['Desktop Chrome'], testIgnore: /knowledge-limits\.spec\.ts/ },
    // Server kedua dengan batas kecil; dilewati bila memakai server eksternal (E2E_BASE_URL).
    {
      name: 'knowledge-limits',
      testMatch: /knowledge-limits\.spec\.ts/,
      use: { ...devices['Desktop Chrome'], baseURL: limitsURL },
      ...(external ? { grep: /^$/ } : {}),
    },
  ],
  webServer: external
    ? undefined
    : [
        {
          command: '../bin/grimoire',
          url: `${baseURL}/healthz`,
          reuseExistingServer: true,
          timeout: 60_000,
          env: { ...serverEnv, HTTP_ADDR: '127.0.0.1:8080' },
        },
        {
          command: '../bin/grimoire',
          url: `${limitsURL}/healthz`,
          reuseExistingServer: true,
          timeout: 60_000,
          env: { ...serverEnv, HTTP_ADDR: '127.0.0.1:8081', MAX_UPLOAD_BYTES: '2097152', MAX_PROJECT_FILES: '5' },
        },
      ],
});
