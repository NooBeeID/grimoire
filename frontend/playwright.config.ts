import { defineConfig, devices } from '@playwright/test';

const external = process.env.E2E_BASE_URL;
const baseURL = external ?? 'http://localhost:8080';
const limitsURL = 'http://localhost:8081';
// MCP_AUTH_FAIL_PER_MINUTE: semua tes memakai IP 127.0.0.1; limiter gagal-auth /mcp per IP (default 20/menit)
// akan mengunci tes paralel lain. Bila E2E_BASE_URL dipakai, operator wajib menyetel env yang sama.
const serverEnv = { COOKIE_SECURE: 'false', LOGIN_RATE_PER_MINUTE: '1000', MCP_AUTH_FAIL_PER_MINUTE: '1000' };

// Sync Git (T14): server GitHub palsu di e2e/fixtures/fake-git-server.ts. Backend e2e diarahkan ke sana lewat
// GIT_SYNC_INSECURE_TEST_HOSTS + GIT_SYNC_GITHUB_API_BASE, yang hanya diterima saat APP_ENV=development.
// Bila E2E_BASE_URL dipakai, operator wajib menyetel env yang sama pada server eksternal.
const fakeGitPort = process.env.E2E_FAKE_GIT_PORT ?? '8089';
const fakeGitURL = `http://127.0.0.1:${fakeGitPort}`;
const gitEnv = {
  APP_ENV: 'development',
  GIT_CREDENTIAL_KEY: Buffer.alloc(32, 7).toString('base64'),
  GIT_SYNC_INSECURE_TEST_HOSTS: `127.0.0.1:${fakeGitPort}`,
  GIT_SYNC_GITHUB_API_BASE: fakeGitURL,
};

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
  webServer: [
    {
      command: 'node e2e/fixtures/fake-git-server.ts',
      url: `${fakeGitURL}/healthz`,
      reuseExistingServer: true,
      timeout: 30_000,
      env: { E2E_FAKE_GIT_PORT: fakeGitPort },
    },
    ...(external
      ? []
      : [
        {
          command: '../bin/grimoire',
          url: `${baseURL}/healthz`,
          reuseExistingServer: true,
          timeout: 60_000,
          env: { ...serverEnv, ...gitEnv, HTTP_ADDR: '127.0.0.1:8080' },
        },
        {
          command: '../bin/grimoire',
          url: `${limitsURL}/healthz`,
          reuseExistingServer: true,
          timeout: 60_000,
          env: { ...serverEnv, HTTP_ADDR: '127.0.0.1:8081', MAX_UPLOAD_BYTES: '2097152', MAX_PROJECT_FILES: '5', APISPEC_MAX_FILE_BYTES: '1048576' },
        },
      ]),
  ],
});
