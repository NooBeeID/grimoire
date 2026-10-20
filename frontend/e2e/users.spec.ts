import { expect, test, type Page } from '@playwright/test';
import {
  ADMIN_EMAIL,
  ADMIN_PASSWORD,
  adminContext,
  apiLogin,
  createReadyUserApi,
  createUserApi,
  loginUi,
  newContext,
  submitLogin,
  uniqueEmail,
} from './helpers/auth';

const dialog = (page: Page) => page.getByRole('dialog');
const row = (page: Page, email: string) => page.getByRole('row', { name: new RegExp(email.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')) });

async function searchUser(page: Page, q: string) {
  await page.getByLabel('Cari nama atau email').fill(q);
  await expect(page.getByRole('row', { name: new RegExp(q) }).first()).toBeVisible();
}

test.describe('positif', () => {
  test('4. buat developer tanpa password: rahasia sekali tampil', async ({ browser }) => {
    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    const email = uniqueEmail('new');
    await page.getByRole('button', { name: 'Buat user' }).first().click();
    await dialog(page).getByLabel('Email').fill(email);
    await dialog(page).getByLabel('Nama').fill('Baru Dibuat');
    await dialog(page).getByRole('button', { name: 'Simpan' }).click();

    const secretInput = dialog(page).getByLabel('Password sementara');
    await expect(secretInput).toBeVisible();
    const secret = await secretInput.inputValue();
    expect(secret.length).toBeGreaterThan(0);
    await dialog(page).getByRole('button', { name: 'Salin' }).click();
    await expect(dialog(page).getByRole('button', { name: 'Tersalin' })).toBeVisible();
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(secret);

    await dialog(page).getByRole('button', { name: 'Saya sudah menyimpan password' }).click();
    await expect(dialog(page)).toBeHidden();
    expect(await page.content()).not.toContain(secret);
    expect(page.url()).not.toContain(secret);
    const stored = await page.evaluate(() => JSON.stringify({ ...localStorage }) + JSON.stringify({ ...sessionStorage }));
    expect(stored).not.toContain(secret);

    await searchUser(page, email);
    await expect(row(page, email)).toContainText('Wajib ganti password');
    await context.close();
  });

  test('7. cari, filter, paginasi', async ({ browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const prefix = `pg${Date.now().toString(36)}${Math.random().toString(36).slice(2, 5)}`;
    const emails = Array.from({ length: 25 }, (_, i) => `${prefix}-${String(i).padStart(2, '0')}@tim.id`);
    for (let i = 0; i < emails.length; i += 5) {
      await Promise.all(emails.slice(i, i + 5).map((email) => createUserApi(request, { email })));
    }
    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await page.getByLabel('Cari nama atau email').fill(prefix);
    await expect(page.getByText('Halaman 1 dari 2')).toBeVisible();
    await expect(page.locator('tbody tr')).toHaveCount(20);
    await page.getByRole('button', { name: 'Berikutnya' }).click();
    await expect(page.getByText('Halaman 2 dari 2')).toBeVisible();
    await expect(page.locator('tbody tr')).toHaveCount(5);
    await page.getByLabel('Status').selectOption('inactive');
    await expect(page.getByText('Tidak ada user yang cocok')).toBeVisible();
    await page.getByRole('button', { name: 'Reset filter' }).click();
    await expect(page.locator('tbody tr').first()).toBeVisible();
    await context.close();
  });

  test('8. edit nama dan promosi role; sesi developer aktif melihat menu Users', async ({ browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request, { name: 'Calon Admin' });
    const devCtx = await newContext(browser);
    const devPage = await devCtx.newPage();
    await loginUi(devPage, dev.email, dev.password);
    await expect(devPage.getByRole('link', { name: 'Users' })).toHaveCount(0);

    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await searchUser(page, dev.email);
    await row(page, dev.email).getByRole('button', { name: 'Ubah' }).click();
    await dialog(page).getByLabel('Nama').fill('Admin Baru');
    await dialog(page).getByLabel('Role').selectOption('super_admin');
    await dialog(page).getByRole('button', { name: 'Simpan' }).click();
    await expect(dialog(page)).toBeHidden();
    await expect(row(page, dev.email)).toContainText('Admin Baru');
    await expect(row(page, dev.email)).toContainText('Super Admin');

    await devPage.reload();
    await expect(devPage.getByRole('link', { name: 'Users' })).toBeVisible();
    await devCtx.close();
    await context.close();
  });

  test('9. reset password: rahasia baru, sesi developer putus, wajib ganti lagi', async ({ browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request);
    const devCtx = await newContext(browser);
    const devPage = await devCtx.newPage();
    await loginUi(devPage, dev.email, dev.password);
    await expect(devPage.getByTestId('home-title')).toBeVisible();

    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await searchUser(page, dev.email);
    await row(page, dev.email).getByRole('button', { name: 'Reset password' }).click();
    await dialog(page).getByRole('button', { name: 'Reset password' }).click();
    const secret = await dialog(page).getByLabel('Password sementara').inputValue();
    expect(secret).not.toBe('');
    await dialog(page).getByRole('button', { name: 'Saya sudah menyimpan password' }).click();
    expect(await page.content()).not.toContain(secret);

    await devPage.reload();
    await expect(devPage).toHaveURL(/\/login/);
    await submitLogin(devPage, dev.email, secret);
    await expect(devPage).toHaveURL(/\/change-password$/);
    await devCtx.close();
    await context.close();
  });

  test('10. nonaktif lalu aktif', async ({ browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request);
    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await searchUser(page, dev.email);
    await row(page, dev.email).getByRole('button', { name: 'Nonaktifkan' }).click();
    await dialog(page).getByRole('button', { name: 'Nonaktifkan' }).click();
    await expect(row(page, dev.email)).toContainText('Nonaktif');

    const devCtx = await newContext(browser);
    const devPage = await devCtx.newPage();
    await loginUi(devPage, dev.email, dev.password);
    await expect(devPage.getByRole('alert')).toHaveText('Email atau password salah');

    await row(page, dev.email).getByRole('button', { name: 'Aktifkan' }).click();
    await dialog(page).getByRole('button', { name: 'Aktifkan' }).click();
    await expect(row(page, dev.email)).toContainText('Aktif');
    await submitLogin(devPage, dev.email, dev.password);
    await expect(devPage.getByTestId('home-title')).toBeVisible();
    await devCtx.close();
    await context.close();
  });
});

test.describe('negatif', () => {
  test('4. developer tidak bisa membuka /admin/users', async ({ page, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const dev = await createReadyUserApi(request);
    await loginUi(page, dev.email, dev.password);
    const calls: string[] = [];
    page.on('request', (r) => new URL(r.url()).pathname === '/api/users' && calls.push(r.url()));
    await page.goto('/admin/users');
    await expect(page.getByTestId('notfound-title')).toBeVisible();
    expect(calls).toHaveLength(0);
    const res = await page.evaluate(async () => {
      const r = await fetch('/api/users');
      return { status: r.status, body: await r.json() };
    });
    expect(res.status).toBe(403);
    expect(res.body.error.code).toBe('FORBIDDEN');
  });

  test('6. aksi pada akun sendiri dilarang di UI dan API', async ({ browser }) => {
    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await searchUser(page, ADMIN_EMAIL);
    const self = row(page, ADMIN_EMAIL);
    await expect(self.getByRole('button', { name: 'Nonaktifkan' })).toBeDisabled();
    await expect(self.getByRole('button', { name: 'Reset password' })).toBeDisabled();
    await self.getByRole('button', { name: 'Ubah' }).click();
    await expect(dialog(page).getByLabel('Role')).toBeDisabled();
    await dialog(page).getByRole('button', { name: 'Batal' }).click();

    const me = await (await context.request.get('/api/auth/me')).json();
    const res = await page.evaluate(async (id) => {
      const r = await fetch(`/api/users/${id}/deactivate`, { method: 'POST', headers: { 'X-Requested-With': 'grimoire' } });
      return { status: r.status, body: await r.json() };
    }, me.id);
    expect(res.status).toBe(403);
    expect(res.body.error.code).toBe('SELF_ACTION_FORBIDDEN');
    await context.close();
  });

  test('7. email duplikat (beda huruf) menampilkan error di field', async ({ browser, request }) => {
    await apiLogin(request, ADMIN_EMAIL, ADMIN_PASSWORD);
    const existing = await createUserApi(request, { email: uniqueEmail('dup') });
    const context = await adminContext(browser);
    const page = await context.newPage();
    await page.goto('/admin/users');
    await page.getByRole('button', { name: 'Buat user' }).first().click();
    await dialog(page).getByLabel('Email').fill(existing.email.toUpperCase());
    await dialog(page).getByLabel('Nama').fill('Duplikat');
    await dialog(page).getByRole('button', { name: 'Simpan' }).click();
    await expect(dialog(page).getByText('Email sudah terdaftar')).toBeVisible();
    await expect(dialog(page)).toBeVisible();
    await context.close();
  });

  test('13. respons rusak menampilkan ErrorState tanpa crash', async ({ page }) => {
    await apiLogin(page.request, ADMIN_EMAIL, ADMIN_PASSWORD);
    await page.route('**/api/users?*', (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: 'x' }) }),
    );
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto('/admin/users');
    await expect(page.getByText('Respons server tidak sesuai format')).toBeVisible();
    expect(errors).toEqual([]);
  });
});

