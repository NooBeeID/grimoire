import { expect, request as pwRequest, test, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createKeyApi } from './helpers/apikeys';
import { ADMIN_EMAIL, ADMIN_PASSWORD, apiLogin, createReadyUserApi } from './helpers/auth';
import { callTool } from './helpers/knowledge-tools';
import { createProjectApi } from './helpers/projects';

// E2E T15 Dashboard (GET /api/audit-log/summary) terhadap binary sungguhan.
// DB e2e dipakai bersama: assert hanya pada data yang dibuat tes ini (filter project) atau dengan ">=".

const baseURL = () => test.info().project.use.baseURL as string;
const SUMMARY = '/api/audit-log/summary';

async function loginAdmin(page: Page): Promise<void> {
  await apiLogin(page.request, ADMIN_EMAIL, ADMIN_PASSWORD);
}

async function loginDeveloper(page: Page): Promise<void> {
  const admin = await pwRequest.newContext({ baseURL: baseURL() });
  try {
    await apiLogin(admin, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(admin, { role: 'developer', name: 'E2E Dev' });
    await apiLogin(page.request, dev.email, dev.password);
  } finally {
    await admin.dispose();
  }
}

/** Dua project: A (5x list_files) dan B (2x search_knowledge). */
async function seedProjects(page: Page) {
  const a = await createProjectApi(page.request);
  const b = await createProjectApi(page.request);
  const keyA = await createKeyApi(page.request, a.id);
  const keyB = await createKeyApi(page.request, b.id);
  const mcp = await pwRequest.newContext({ baseURL: baseURL() });
  try {
    for (let i = 0; i < 5; i++) await callTool(mcp, keyA.raw_key, 'list_files', {});
    for (let i = 0; i < 2; i++) await callTool(mcp, keyB.raw_key, 'search_knowledge', { query: 'login' });
  } finally {
    await mcp.dispose();
  }
  return { a, b };
}

const summaryRequests = (page: Page) => {
  const urls: string[] = [];
  page.on('request', (r) => {
    const u = new URL(r.url());
    if (u.pathname === SUMMARY) urls.push(u.search);
  });
  return urls;
};

const rowOf = (page: Page, caption: string, cell: string) =>
  page.locator('figure', { has: page.locator('caption', { hasText: caption }) }).getByRole('row').filter({ hasText: cell });

test.describe('dashboard sebagai super admin', () => {
  test('1. default lewat navigasi (AC1)', async ({ page }) => {
    await loginAdmin(page);
    const reqs = summaryRequests(page);
    await page.goto('/');
    const nav = page.getByRole('navigation', { name: 'Navigasi utama' });
    await nav.getByRole('link', { name: 'Dashboard' }).click();
    await expect(page).toHaveURL(/\/dashboard$/);
    await expect(nav.getByRole('link', { name: 'Dashboard' })).toHaveAttribute('aria-current', 'page');
    await expect(page.getByRole('radio', { name: '7 hari' })).toBeChecked();
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toHaveCount(1);
    await expect(page.locator('figure canvas[role=img][aria-label]')).toHaveCount(5);
    await expect(page.getByText('Pilih project untuk melihat kualitas pencarian')).toBeVisible();
    expect(reqs).toEqual(['?range=7d']);
  });

  test('2. ganti rentang dan reload (AC11)', async ({ page }) => {
    await loginAdmin(page);
    await page.goto('/dashboard');
    const req = page.waitForRequest((r) => r.url().includes(`${SUMMARY}?range=24h`));
    await page.getByRole('radio', { name: '24 jam' }).check({ force: true });
    await req;
    await expect(page).toHaveURL(/\/dashboard\?range=24h$/);
    const trafficRows = rowOf(page, 'Trafik', '').filter({ has: page.locator('td') });
    await expect.poll(() => trafficRows.count()).toBeGreaterThanOrEqual(24);
    expect(await trafficRows.count()).toBeLessThanOrEqual(25);
    await page.reload();
    await expect(page.getByRole('radio', { name: '24 jam' })).toBeChecked();
  });

  test('3. pilih project (AC8, AC11)', async ({ page }) => {
    await loginAdmin(page);
    const { a } = await seedProjects(page);
    await page.goto('/dashboard?range=24h');
    await page.getByLabel('Project', { exact: true }).selectOption({ label: a.name });
    await expect(page).toHaveURL(new RegExp(`project=${a.id}`));
    await expect(page.locator('figcaption', { hasText: 'Project teratas' })).toHaveCount(0);
    await expect(rowOf(page, 'Tool paling sering dipakai', 'list_files')).toContainText('5');
    await expect(page.getByText('Zero-result')).toBeVisible();
    await expect(page.getByText('Follow-up')).toBeVisible();
    await expect(page.getByText('Reformulasi')).toBeVisible();
    await page.reload();
    await expect(page).toHaveURL(new RegExp(`project=${a.id}`));
    await expect(page.getByLabel('Project', { exact: true })).toHaveValue(a.id);
  });

  test('4. project teratas tanpa filter', async ({ page }) => {
    await loginAdmin(page);
    const { a, b } = await seedProjects(page);
    await page.goto('/dashboard?range=24h');
    const rowA = rowOf(page, 'Project teratas', a.name);
    const rowB = rowOf(page, 'Project teratas', b.name);
    await expect(rowA).toContainText('5');
    await expect(rowB).toContainText('2');
    const names = await page
      .locator('figure', { has: page.locator('caption', { hasText: 'Project teratas' }) })
      .locator('tbody tr')
      .allInnerTexts();
    expect(names.findIndex((t) => t.includes(a.name))).toBeLessThan(names.findIndex((t) => t.includes(b.name)));
  });

  test('5. chart.js dimuat lazy (AC13)', async ({ page }) => {
    const manifest = JSON.parse(readFileSync(join(process.cwd(), 'dist', '.vite', 'manifest.json'), 'utf8')) as Record<
      string,
      { file: string; src?: string }
    >;
    const lazy = new Set(
      Object.entries(manifest)
        .filter(([k, v]) => (v.src ?? k).includes('node_modules/chart.js') || k.includes('audit-log/summary/index.ts'))
        .map(([, v]) => v.file),
    );
    expect(lazy.size).toBeGreaterThan(0);
    await loginAdmin(page);
    const loaded: string[] = [];
    page.on('request', (r) => {
      const p = new URL(r.url()).pathname;
      if ([...lazy].some((f) => p.endsWith(f))) loaded.push(p);
    });
    await page.goto('/');
    await page.getByRole('link', { name: 'Audit Log' }).click();
    await expect(page.getByRole('table')).toBeVisible();
    expect(loaded).toEqual([]);
    await page.getByRole('link', { name: 'Dashboard' }).click();
    await expect(page.locator('figure canvas')).toHaveCount(5);
    expect(loaded.length).toBeGreaterThan(0);
  });

  test('7. tema gelap (AC12)', async ({ page }) => {
    await page.emulateMedia({ colorScheme: 'dark' });
    await loginAdmin(page);
    await page.goto('/dashboard');
    await expect(page.locator('figure canvas')).toHaveCount(5);
    const token = await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--color-chart-1').trim());
    expect(token).not.toBe('');
    await test.info().attach('dashboard-dark', { body: await page.screenshot(), contentType: 'image/png' });
  });

  test('8. ponsel 390x844', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await loginAdmin(page);
    await page.goto('/dashboard');
    await expect(page.locator('figure canvas')).toHaveCount(5);
    expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390);
    const widths = await page.locator('figure').evaluateAll((els) => els.slice(0, 3).map((e) => Math.round(e.getBoundingClientRect().width)));
    expect(new Set(widths).size).toBe(1);
  });

  test('10. kualitas pencarian gagal (AC9)', async ({ page }) => {
    await loginAdmin(page);
    const { a } = await seedProjects(page);
    await page.route('**/api/audit-log/summary?*project_id=*', async (route) => {
      const res = await route.fetch();
      const body = await res.json();
      body.search_quality = { available: false, error_code: 'UNAVAILABLE' };
      await route.fulfill({ response: res, json: body });
    });
    await page.goto(`/dashboard?range=24h&project=${a.id}`);
    await expect(page.locator('figure canvas')).toHaveCount(4);
    await expect(page.getByText('Kualitas pencarian tidak dapat dimuat.')).toBeVisible();
    await page.unroute('**/api/audit-log/summary?*project_id=*');
    const extra = page.waitForRequest((r) => r.url().includes(SUMMARY));
    await page.getByRole('button', { name: 'Coba lagi' }).click();
    await extra;
    await expect(page.getByText('Zero-result')).toBeVisible();
  });

  test('11. server 500', async ({ page }) => {
    await loginAdmin(page);
    await page.route('**/api/audit-log/summary*', (route) =>
      route.fulfill({ status: 500, json: { error: { code: 'INTERNAL', message: 'x' }, trace_id: 'trace-e2e-1' } }),
    );
    await page.goto('/dashboard');
    await expect(page.getByRole('alert')).toHaveCount(1);
    await expect(page.getByText('Data tidak tersedia').first()).toBeVisible();
    await page.unroute('**/api/audit-log/summary*');
    await page.getByRole('link', { name: 'Audit Log' }).click();
    await expect(page.getByRole('table')).toBeVisible();
  });

  test('12. parameter URL rusak dinormalisasi', async ({ page }) => {
    await loginAdmin(page);
    const reqs = summaryRequests(page);
    await page.goto('/dashboard?range=90d&project=bukan-uuid');
    await expect(page.locator('figure canvas')).toHaveCount(5);
    expect(reqs).toEqual(['?range=7d']);
    await expect(page.getByLabel('Project', { exact: true })).toHaveValue('');
  });

  test('13. daftar project gagal', async ({ page }) => {
    await loginAdmin(page);
    await page.route('**/api/projects*', (route) =>
      route.fulfill({ status: 500, json: { error: { code: 'INTERNAL', message: 'x' }, trace_id: 'trace-e2e-2' } }),
    );
    await page.goto('/dashboard');
    await expect(page.getByText('Daftar project gagal dimuat')).toBeVisible();
    await expect(page.getByLabel('Project', { exact: true })).toBeDisabled();
    await expect(page.locator('figure canvas')).toHaveCount(5);
  });
});

test.describe('dashboard sebagai developer', () => {
  test('9. link dan halaman tidak tersedia (AC1)', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && !m.location().url.includes('/api/') && errors.push(m.text()));
    await loginDeveloper(page);
    const reqs = summaryRequests(page);
    await page.goto('/');
    await expect(page.getByRole('navigation', { name: 'Navigasi utama' })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Dashboard' })).toHaveCount(0);
    const res = await page.goto('/dashboard');
    expect(res?.status()).toBe(200);
    await expect(page.getByTestId('notfound-title')).toBeVisible();
    await expect(page.getByRole('heading', { level: 1, name: 'Dashboard' })).toHaveCount(0);
    await expect(page.locator('figure, canvas')).toHaveCount(0);
    await page.waitForTimeout(300);
    expect(reqs).toEqual([]);
    expect(errors).toEqual([]);
  });
});
