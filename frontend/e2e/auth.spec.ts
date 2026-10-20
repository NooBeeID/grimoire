import { expect, test, type Page } from '@playwright/test';
import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  CSRF,
  STRONG_PASSWORD,
  apiLogin,
  createReadyUserApi,
  createUserApi,
  loginUi,
  newContext,
  submitLogin,
  uniqueEmail,
} from './helpers/auth';

const loginHeading = (page: Page) => page.getByRole('heading', { name: /^(Masuk|Sign in)$/ });
const alertText = async (page: Page) => (await page.getByRole('alert').first().innerText()).trim();

test.describe('positif', () => {
  test('1. login admin: redirect, dashboard, menu Users, cookie HttpOnly', async ({ page, context }) => {
    await page.goto('/');
    await expect(page).toHaveURL(/\/login\?next=%2F$/);
    await submitLogin(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByTestId('home-title')).toBeVisible();
    await expect(page.getByText('Halo, E2E Admin')).toBeVisible();
    await expect(page.getByRole('navigation', { name: 'Navigasi utama' }).getByRole('link', { name: 'Users' })).toBeVisible();
    const cookie = (await context.cookies()).find((c) => c.name === 'grimoire_session');
    expect(cookie?.httpOnly).toBe(true);
    expect(await page.evaluate(() => document.cookie)).not.toContain('grimoire_session');
  });

  test('2. deep link setelah login', async ({ page }) => {
    await page.goto('/admin/users?x=1');
    await expect(page).toHaveURL(/\/login\?next=/);
    await submitLogin(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();
  });

  test('3. reload memulihkan sesi', async ({ page }) => {
    await loginUi(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.goto('/admin/users');
    await page.reload();
    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.getByRole('heading', { name: 'Users' })).toBeVisible();
  });

  test('5. login pertama developer dipaksa ganti password', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createUserApi(request);
    await loginUi(page, dev.email, dev.password);
    await expect(page).toHaveURL(/\/change-password$/);
    await page.goto('/');
    await expect(page).toHaveURL(/\/change-password$/);
    await page.getByLabel('Password saat ini').fill(dev.password);
    await page.getByLabel('Password baru', { exact: true }).fill(STRONG_PASSWORD);
    await page.getByLabel('Ulangi password baru').fill(STRONG_PASSWORD);
    await page.getByRole('button', { name: 'Simpan password' }).click();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole('link', { name: 'Users' })).toHaveCount(0);
    await page.reload();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByTestId('home-title')).toBeVisible();
  });

  test('6. profil: ganti password mengeluarkan sesi lain', async ({ page, browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request);
    const other = await newContext(browser);
    await apiLogin(other.request, dev.email, dev.password);
    await loginUi(page, dev.email, dev.password);
    await page.goto('/profile');
    await page.getByLabel('Password saat ini').fill(dev.password);
    await page.getByLabel('Password baru', { exact: true }).fill('Password-Baru-456!');
    await page.getByLabel('Ulangi password baru').fill('Password-Baru-456!');
    await page.getByRole('button', { name: 'Simpan password' }).click();
    await expect(page.getByText('Password diganti. Sesi di perangkat lain telah dikeluarkan.')).toBeVisible();
    expect((await other.request.get('/api/auth/me')).status()).toBe(401);
    const p2 = await other.newPage();
    await p2.goto('/');
    await expect(p2).toHaveURL(/\/login/);
    await other.close();
  });

  test('7. logout menghapus sesi', async ({ page }) => {
    await loginUi(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page.getByTestId('home-title')).toBeVisible();
    await page.getByRole('button', { name: 'Keluar' }).click();
    await expect(page).toHaveURL(/\/login/);
    const status = await page.evaluate(async () => (await fetch('/api/auth/me')).status);
    expect(status).toBe(401);
    await page.goBack();
    await expect(page).toHaveURL(/\/login/);
    await expect(loginHeading(page)).toBeVisible();
  });

  test('12. locale English pada halaman login', async ({ page }) => {
    await page.goto('/login');
    await page.getByLabel('Bahasa').selectOption('en');
    await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
    await submitLogin(page, uniqueEmail('nobody'), 'wrong-password-123');
    await expect(page.getByRole('alert')).toHaveText('Incorrect email or password');
  });
});

test.describe('negatif', () => {
  test('1. pesan login seragam', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request);
    await loginUi(page, uniqueEmail('ghost'), 'wrong-password-123');
    await expect(page.getByRole('alert')).toBeVisible();
    const unknown = await alertText(page);
    await page.getByLabel('Email').fill(dev.email);
    await page.getByLabel('Password', { exact: true }).fill('wrong-password-123');
    await page.getByRole('button', { name: 'Masuk' }).click();
    await expect(page.getByRole('alert')).toHaveText('Email atau password salah');
    expect(await alertText(page)).toBe(unknown);
    expect(unknown).toBe('Email atau password salah');
  });

  test('2. lockout setelah 5 kali salah; email lain tetap bisa login', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const locked = await createReadyUserApi(request);
    const other = await createReadyUserApi(request);
    for (let i = 0; i < 5; i++) {
      const r = await request.post('/api/auth/login', { headers: CSRF, data: { email: locked.email, password: `salah-salah-${i}` } });
      expect(r.status()).toBe(401);
    }
    await loginUi(page, locked.email, locked.password);
    await expect(page.getByRole('alert')).toContainText(/Terlalu banyak percobaan login\. Coba lagi dalam \d+ menit\./);
    await expect(page).toHaveURL(/\/login/);
    await submitLogin(page, other.email, other.password);
    await expect(page).toHaveURL(/\/$/);
  });

  test('3. rate limit IP (UI) dengan hitung mundur', async ({ page }) => {
    await page.route('**/api/auth/login', (route) =>
      route.fulfill({
        status: 429,
        contentType: 'application/json',
        headers: { 'Retry-After': '3' },
        body: JSON.stringify({
          error: { code: 'RATE_LIMITED', message: 'terlalu banyak permintaan', details: { retry_after_seconds: 3 } },
          trace_id: 't-1',
        }),
      }),
    );
    await loginUi(page, 'x@tim.id', 'whatever-password');
    await expect(page.getByRole('alert')).toContainText('Terlalu banyak permintaan dari perangkat ini');
    await expect(page.getByRole('button', { name: /Tunggu \d+ dtk/ })).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Masuk' })).toBeEnabled({ timeout: 10_000 });
  });

  test('5. gate password: /api/users 403 PASSWORD_CHANGE_REQUIRED, /api/auth/me 200', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createUserApi(request);
    await loginUi(page, dev.email, dev.password);
    await expect(page).toHaveURL(/\/change-password$/);
    const users = await page.evaluate(async () => {
      const r = await fetch('/api/users');
      return { status: r.status, body: await r.json() };
    });
    expect(users.status).toBe(403);
    expect(users.body.error.code).toBe('PASSWORD_CHANGE_REQUIRED');
    const me = await page.evaluate(async () => (await fetch('/api/auth/me')).status);
    expect(me).toBe(200);
  });

  test('8. validasi password: server per field, konfirmasi beda ditolak di klien', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createUserApi(request);
    await loginUi(page, dev.email, dev.password);
    await expect(page).toHaveURL(/\/change-password$/);
    const posts: string[] = [];
    page.on('request', (r) => r.method() === 'POST' && r.url().includes('/api/auth/change-password') && posts.push(r.url()));

    await page.getByLabel('Password saat ini').fill(dev.password);
    await page.getByLabel('Password baru', { exact: true }).fill('Password-Baru-456!');
    await page.getByLabel('Ulangi password baru').fill('Password-Beda-789!');
    await page.getByRole('button', { name: 'Simpan password' }).click();
    await expect(page.getByText('Password baru tidak sama')).toBeVisible();
    expect(posts).toHaveLength(0);

    for (const bad of ['abcdefghi', dev.email]) {
      await page.getByLabel('Password baru', { exact: true }).fill(bad);
      await page.getByLabel('Ulangi password baru').fill(bad);
      await page.getByRole('button', { name: 'Simpan password' }).click();
      await expect(page.getByLabel('Password baru', { exact: true })).toHaveAttribute('aria-invalid', 'true');
      await expect(page).toHaveURL(/\/change-password$/);
    }
    expect(posts).toHaveLength(2);
  });

  test('9. password lama salah tampil di field password saat ini', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createUserApi(request);
    await loginUi(page, dev.email, dev.password);
    await page.getByLabel('Password saat ini').fill('password-lama-yang-salah');
    await page.getByLabel('Password baru', { exact: true }).fill(STRONG_PASSWORD);
    await page.getByLabel('Ulangi password baru').fill(STRONG_PASSWORD);
    await page.getByRole('button', { name: 'Simpan password' }).click();
    await expect(page.getByText(/password lama salah/i)).toBeVisible();
    await expect(page.getByLabel('Password saat ini')).toHaveAttribute('aria-invalid', 'true');
  });

  test('10. dinonaktifkan saat tab terbuka: aksi berikutnya ke /login?next', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request, { name: 'Dina Tab' });
    await loginUi(page, dev.email, dev.password);
    await page.goto('/profile');
    await expect(page.getByText('Dina Tab').first()).toBeVisible();
    const res = await request.post(`/api/users/${dev.id}/deactivate`, { headers: CSRF });
    expect(res.status()).toBe(200);
    await page.getByLabel('Password saat ini').fill(dev.password);
    await page.getByLabel('Password baru', { exact: true }).fill('Password-Baru-456!');
    await page.getByLabel('Ulangi password baru').fill('Password-Baru-456!');
    await page.getByRole('button', { name: 'Simpan password' }).click();
    await expect(page).toHaveURL(/\/login\?next=%2Fprofile/);
    await expect(page.getByText('Dina Tab')).toHaveCount(0);
  });

  test('11. bootstrap gagal: Gagal memuat sesi lalu pulih dengan Coba lagi', async ({ page }) => {
    const pattern = '**/api/auth/me';
    await page.route(pattern, (route) =>
      route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: { code: 'INTERNAL', message: 'boom', details: null }, trace_id: 't-2' }),
      }),
    );
    await page.goto('/');
    await expect(page.getByText('Gagal memuat sesi')).toBeVisible();
    await expect(loginHeading(page)).toHaveCount(0);
    await page.unroute(pattern);
    await page.getByRole('button', { name: 'Coba lagi' }).click();
    await expect(loginHeading(page)).toBeVisible();
  });

  test('12. open redirect ditolak', async ({ page, baseURL }) => {
    await page.goto('/login?next=//evil.com');
    await submitLogin(page, ADMIN_EMAIL, ADMIN_PASSWORD);
    await expect(page.getByTestId('home-title')).toBeVisible();
    const url = new URL(page.url());
    expect(url.origin).toBe(new URL(baseURL!).origin);
    expect(url.pathname).toBe('/');
  });

  test('14. cookie sesi palsu berakhir di /login tanpa console error', async ({ page, context, baseURL }) => {
    const errors: string[] = [];
    page.on('console', (m) => m.type() === 'error' && !m.text().includes('401') && errors.push(m.text()));
    await context.addCookies([{ name: 'grimoire_session', value: 'abc', url: baseURL! }]);
    await page.goto('/');
    await expect(page).toHaveURL(/\/login/);
    await expect(loginHeading(page)).toBeVisible();
    expect(errors).toEqual([]);
  });
});

