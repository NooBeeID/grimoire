import { expect, test } from '@playwright/test';
import { ErrorEnvelopeForE2E } from './envelope';

test('home renders in id without console errors', async ({ page }) => {
  const errors: string[] = [];
  // T01: 401 dari /api/auth/me tanpa sesi adalah hasil normal; browser mencatatnya sebagai console error.
  page.on('console', (m) => m.type() === 'error' && !m.text().includes('401') && errors.push(m.text()));
  page.on('pageerror', (e) => errors.push(e.message));
  const bad: string[] = [];
  page.on('response', (r) => r.status() >= 400 && !r.url().endsWith('/api/auth/me') && bad.push(`${r.status()} ${r.url()}`));
  const res = await page.goto('/');
  expect(res?.status()).toBe(200);
  await expect(page.getByRole('heading', { name: /Masuk|Sign in/ })).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'id');
  expect(errors).toEqual([]);
  expect(bad).toEqual([]);
});

test('cache headers', async ({ page }) => {
  const assets: string[] = [];
  page.on('response', (r) => {
    if (new URL(r.url()).pathname.startsWith('/assets/')) assets.push(r.headers()['cache-control'] ?? '');
  });
  const res = await page.goto('/');
  expect(res?.headers()['cache-control']).toContain('no-cache');
  for (const c of assets) expect(c).toContain('immutable');
});

test('deep link falls back to SPA and client navigation does not reload', async ({ page }) => {
  const res = await page.goto('/projects/abc');
  expect(res?.status()).toBe(200);
  await expect(page.getByTestId('notfound-title')).toBeVisible();
  await page.evaluate(() => ((window as unknown as { __navMarker: number }).__navMarker = 1));
  await page.getByRole('link').first().click();
  await expect(page.getByRole('heading', { name: /Masuk|Sign in/ })).toBeVisible();
  expect(await page.evaluate(() => (window as unknown as { __navMarker?: number }).__navMarker)).toBe(1);
});

test('locale switch persists across reload', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Bahasa').selectOption('en');
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await page.reload();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
});

test('back button returns to home', async ({ page }) => {
  await page.goto('/');
  await page.evaluate(() => {
    history.pushState({}, '', '/zzz');
    window.dispatchEvent(new PopStateEvent('popstate'));
  });
  await expect(page.getByTestId('notfound-title')).toBeVisible();
  await page.goBack();
  await expect(page.getByRole('heading', { name: /Masuk|Sign in/ })).toBeVisible();
});

test('missing asset is 404 non-HTML', async ({ request }) => {
  const r = await request.get('/assets/missing.js');
  expect(r.status()).toBe(404);
  expect(await r.text()).not.toContain('<div id="app">');
});

test('unknown api uses uniform error envelope', async ({ request }) => {
  const r = await request.get('/api/does-not-exist');
  expect(r.status()).toBe(404);
  expect(r.headers()['content-type']).toContain('json');
  const body = ErrorEnvelopeForE2E.parse(await r.json());
  expect(body.error.code).toBe('NOT_FOUND');
  expect(body.trace_id).toBeTruthy();
});

test('csrf header required on mutations', async ({ request }) => {
  const a = await request.post('/api/anything');
  expect(a.status()).toBe(403);
  expect((await a.json()).error.code).toBe('CSRF_REQUIRED');
  const b = await request.post('/api/anything', { headers: { 'X-Requested-With': 'grimoire' } });
  expect(b.status()).not.toBe(403);
});

test('blocked localStorage still renders in id', async ({ page }) => {
  const errs: string[] = [];
  page.on('pageerror', (e) => errs.push(e.message));
  await page.addInitScript(() => {
    Object.defineProperty(window, 'localStorage', {
      get() {
        throw new Error('blocked');
      },
    });
  });
  await page.goto('/');
  await expect(page.getByRole('heading', { name: /Masuk|Sign in/ })).toBeVisible();
  expect(errs).toEqual([]);
});

test('session cookie before T01 does not break SPA', async ({ page, context, baseURL }) => {
  await context.addCookies([{ name: 'grimoire_session', value: 'abc', url: baseURL! }]);
  const errs: string[] = [];
  page.on('console', (m) => m.type() === 'error' && !m.text().includes('401') && errs.push(m.text()));
  await page.goto('/');
  await expect(page.getByRole('heading', { name: /Masuk|Sign in/ })).toBeVisible();
  expect(errs).toEqual([]);
});
